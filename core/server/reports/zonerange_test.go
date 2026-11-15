package reports

import (
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// PocketBase names some parts of a rule's SQL at random on every build: a
// bound placeholder ({:f…} for @request.auth.*, {:t…} for a literal;
// core/record_field_resolver.go:377, tools/search/filter.go:279) and a join
// alias suffix (__collection_…, __ml…, __sm…; record_field_resolver.go:170,
// filter.go:649). Two runs of one rule differ only in them.
var (
	placeholderName = regexp.MustCompile(`\{:([^{}]+)\}`)
	joinAlias       = regexp.MustCompile(`__\w+`)
)

// sameShape is each statement with its placeholders and join aliases renamed
// in order of first appearance, and its params renamed to match, so two runs
// compare by the SQL they send and the values they bind, never by
// PocketBase's random names. A param the SQL does not reference (PocketBase
// binds a rule literal such as "" under a random key, then inlines it) is
// dropped: its key is random and it sends nothing.
func sameShape(statements []readStatement) []readStatement {
	out := make([]readStatement, 0, len(statements))
	for _, s := range statements {
		names := map[string]string{}
		sql := placeholderName.ReplaceAllStringFunc(s.sql, func(m string) string {
			name := m[2 : len(m)-1]
			if _, ok := names[name]; !ok {
				names[name] = fmt.Sprintf("v%d", len(names))
			}
			return "{:" + names[name] + "}"
		})
		aliases := map[string]string{}
		sql = joinAlias.ReplaceAllStringFunc(sql, func(m string) string {
			if _, ok := aliases[m]; !ok {
				aliases[m] = fmt.Sprintf("__a%d", len(aliases))
			}
			return aliases[m]
		})
		params := dbx.Params{}
		for k, v := range s.params {
			renamed, ok := names[k]
			if !ok {
				continue // bound by PocketBase but not in the SQL (a literal it inlined)
			}
			params[renamed] = v
		}
		out = append(out, readStatement{sql: sql, params: params})
	}
	return out
}

// The zoo's sightings: one row on each side of the edges of three local days
// in America/Chicago. Each note names its row.
//
//	day          local midnight      the day ends     before / start / end / after
//	2026-03-08   06:00Z (DST start)  2026-03-09 05:00Z  before8 start8 end8 after8
//	2026-11-01   05:00Z (DST end)    2026-11-02 06:00Z  before1 start1 end1 after1
//	2026-11-14   06:00Z              2026-11-15 06:00Z  before14 start14 end14 after14
var sightingRows = []struct{ note, seen string }{
	{"before8", "2026-03-08 05:59:59.000Z"}, {"start8", "2026-03-08 06:00:00.000Z"},
	{"end8", "2026-03-09 04:59:59.000Z"}, {"after8", "2026-03-09 05:00:00.000Z"},
	{"before1", "2026-11-01 04:59:59.000Z"}, {"start1", "2026-11-01 05:00:00.000Z"},
	{"end1", "2026-11-02 05:59:59.000Z"}, {"after1", "2026-11-02 06:00:00.000Z"},
	{"before14", "2026-11-14 05:59:59.000Z"}, {"start14", "2026-11-14 06:00:00.000Z"},
	{"end14", "2026-11-15 05:59:59.000Z"}, {"after14", "2026-11-15 06:00:00.000Z"},
}

// addZooSightings adds zoo_sightings (a date column holding full instants,
// which an autodate cannot be set to) and its source zoo.sightings.
func addZooSightings(t testing.TB, z *zoo) {
	t.Helper()
	sightings := core.NewBaseCollection("zoo_sightings")
	sightings.ListRule = rule(`@request.auth.id != ""`)
	sightings.ViewRule = sightings.ListRule
	sightings.Fields.Add(&core.DateField{Name: "seen"}, &core.TextField{Name: "note"})
	z.save(t, sightings)
	for _, s := range sightingRows {
		z.rec(t, s.note, "zoo_sightings", map[string]any{"seen": s.seen, "note": s.note})
	}
	RegisterSource(Source{
		Key: "zoo.sightings", Collection: "zoo_sightings", Label: "Sightings",
		Fields: []Field{
			{Key: "seen", Label: "Seen", Type: DateTime, Aggregates: MinMax},
			{Key: "note", Label: "Note", Type: Text},
		},
	})
}

const chicagoZone = "America/Chicago"

// nov14 is a date range of one day, 2026-11-14.
const nov14 = `{"from":"2026-11-14","to":"2026-11-14"}`

const sightingsInPeriod = `{"version":4,"source":"zoo.sightings",
	"params":[{"id":"period","type":"dateRange"}],
	"columns":[{"id":"note","path":[{"field":"note"}]},{"id":"seen","path":[{"field":"seen"}]}],
	"filter":{"op":"between","args":[{"path":[{"field":"seen"}]},{"param":"period"}]},
	"sort":[{"col":"seen"}]}`

func rangeOf(from, to string) string { return `{"from":"` + from + `","to":"` + to + `"}` }

func TestBetweenDateTimeInTheReadersZone(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	period := map[string]string{"period": nov14}
	requireDetails(t, z.run(t, "ann", sightingsInPeriod, Options{TimeZone: chicagoZone}, period), "note", "start14", "end14")
	// With no zone the day is the UTC day, as before.
	requireDetails(t, z.run(t, "ann", sightingsInPeriod, Options{}, period), "note", "before14", "start14")
	// A range of many days keeps from the first local midnight to the last.
	many := map[string]string{"period": rangeOf("2026-11-01", "2026-11-14")}
	requireDetails(t, z.run(t, "ann", sightingsInPeriod, Options{TimeZone: chicagoZone}, many), "note",
		"start1", "end1", "after1", "before14", "start14", "end14")
}

// A 25-hour day: local midnight is 05:00Z on Nov 1 and 06:00Z on Nov 2.
func TestBetweenAcrossDSTEnd(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	period := map[string]string{"period": rangeOf("2026-11-01", "2026-11-01")}
	requireDetails(t, z.run(t, "ann", sightingsInPeriod, Options{TimeZone: chicagoZone}, period), "note", "start1", "end1")
}

// A 23-hour day: local midnight is 06:00Z on Mar 8 and 05:00Z on Mar 9.
func TestBetweenAcrossDSTStart(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	period := map[string]string{"period": rangeOf("2026-03-08", "2026-03-08")}
	requireDetails(t, z.run(t, "ann", sightingsInPeriod, Options{TimeZone: chicagoZone}, period), "note", "start8", "end8")
}

// sightingsWhere is every sighting, sorted by when it was seen, filtered by
// where (a formula that reads the params day and period).
func sightingsWhere(where string) string {
	return `{"version":4,"source":"zoo.sightings",
	"params":[{"id":"day","type":"date"},{"id":"period","type":"dateRange"},{"id":"asof","type":"asOf"}],
	"columns":[{"id":"note","path":[{"field":"note"}]},{"id":"seen","path":[{"field":"seen"}]}],
	"filter":` + where + `,"sort":[{"col":"seen"}]}`
}

func opOf(op, left, right string) string {
	return `{"op":"` + op + `","args":[` + left + `,` + right + `]}`
}

const (
	seenLeaf  = `{"path":[{"field":"seen"}]}`
	dayLeaf   = `{"param":"day"}`
	fromBound = `{"param":"period","bound":"from"}`
	toBound   = `{"param":"period","bound":"to"}`
)

// A comparison of a date-time with a day of the reader's zone compares the
// instants of that local day: gte from its start, gt from its end, lt before
// its start, lte before its end (R1); the day on the left mirrors; a range's
// bound is its first or last day.
func TestDayComparisonsOfADateTimeInTheZone(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	all := []any{"before8", "start8", "end8", "after8", "before1", "start1", "end1", "after1", "before14", "start14", "end14", "after14"}
	fromStart := []any{"start14", "end14", "after14"}
	fromEnd := []any{"after14"}
	beforeStart := all[:9]
	beforeEnd := all[:11]
	params := map[string]string{"day": `"2026-11-14"`, "period": nov14, "asof": `"2026-11-14"`}
	for name, c := range map[string]struct {
		where string
		want  []any
	}{
		"gte a day":                    {opOf("gte", seenLeaf, dayLeaf), fromStart},
		"gt a day":                     {opOf("gt", seenLeaf, dayLeaf), fromEnd},
		"lt a day":                     {opOf("lt", seenLeaf, dayLeaf), beforeStart},
		"lte a day":                    {opOf("lte", seenLeaf, dayLeaf), beforeEnd},
		"a day lte":                    {opOf("lte", dayLeaf, seenLeaf), fromStart},
		"a day gt":                     {opOf("gt", dayLeaf, seenLeaf), beforeStart},
		"a day gte":                    {opOf("gte", dayLeaf, seenLeaf), beforeEnd},
		"a day lt":                     {opOf("lt", dayLeaf, seenLeaf), fromEnd},
		"gte an asOf param":            {opOf("gte", seenLeaf, `{"param":"asof"}`), fromStart},
		"between a day and an instant": {`{"op":"between","args":[` + seenLeaf + `,` + dayLeaf + `,{"value":"2026-11-15 05:59:59.000Z"}]}`, []any{"start14", "end14"}},
		"gte the range's from":         {opOf("gte", seenLeaf, fromBound), fromStart},
		"gt the range's from":          {opOf("gt", seenLeaf, fromBound), fromEnd},
		"lt the range's to":            {opOf("lt", seenLeaf, toBound), beforeStart},
		"lte the range's to":           {opOf("lte", seenLeaf, toBound), beforeEnd},
		"between a day and a day":      {`{"op":"between","args":[` + seenLeaf + `,` + dayLeaf + `,` + dayLeaf + `]}`, []any{"start14", "end14"}},
	} {
		t.Run(name, func(t *testing.T) {
			res := z.run(t, "ann", sightingsWhere(c.where), Options{TimeZone: chicagoZone}, params)
			requireDetails(t, res, "note", c.want...)
		})
	}
}

// R2: no zone, the names of UTC itself, the server's own zone and a name the
// server does not know all run in UTC, with the statements of a run with no
// zone (compared after sameShape: PocketBase's rule SQL has random names).
func TestZoneUnknownOrLocalIsUTC(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	period := map[string]string{"period": nov14}
	reads := func(zone string, want ...any) []readStatement {
		seen := observeReads(t)
		requireDetails(t, z.run(t, "ann", sightingsInPeriod, Options{TimeZone: zone}, period), "note", want...)
		return sameShape(*seen)
	}
	none := reads("", "before14", "start14")
	// The compare sees a zone: a Chicago run's statements differ.
	if chicago := reads(chicagoZone, "start14", "end14"); reflect.DeepEqual(chicago, none) {
		t.Fatal("a Chicago run sent the statements of a run with no zone: the compare cannot see a zone")
	}
	for _, zone := range []string{"UTC", "Etc/UTC", "Etc/GMT", "GMT", "Local", "Not/AZone"} {
		if got := reads(zone, "before14", "start14"); !reflect.DeepEqual(got, none) {
			t.Fatalf("zone %q: the statements differ from a run with no zone:\n got  %v\n want %v", zone, got, none)
		}
	}
}

// A Date field is a day: a zone does not move it.
func TestDateColumnIgnoresTheZone(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":4,"source":"zoo.feedings","params":[{"id":"period","type":"dateRange"}],
		"columns":[{"id":"note","path":[{"field":"note"}]}],
		"filter":{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"period"}]},
		"sort":[{"col":"note"}]}`
	period := map[string]string{"period": rangeOf("2026-01-05", "2026-01-20")}
	seen := observeReads(t)
	requireDetails(t, z.run(t, "ann", spec, Options{}, period), "note", "f1", "f3")
	utc := sameShape(*seen)
	*seen = nil
	requireDetails(t, z.run(t, "ann", spec, Options{TimeZone: chicagoZone}, period), "note", "f1", "f3")
	if got := sameShape(*seen); !reflect.DeepEqual(got, utc) {
		t.Fatalf("a zone changed a date column's statements:\n got  %v\n want %v", got, utc)
	}
}

// Q7: a date part and a bucket by day of a date-time keep the UTC day.
// 2026-11-01 04:59:59Z is Oct 31 in Chicago, and stays November 1.
func TestDatePartsAndDayBucketsKeepTheUTCDay(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	chicago := Options{TimeZone: chicagoZone}
	const onlyBefore1 = `"filter":{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"before1"}]}`
	parts := `{"version":4,"source":"zoo.sightings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"month","formula":{"op":"month","args":[{"path":[{"field":"seen"}]}]}}],` + onlyBefore1 + `}`
	requireDetails(t, z.run(t, "ann", parts, chicago, nil), "month", int64(11))
	buckets := `{"version":4,"source":"zoo.sightings","columns":[
		{"id":"seen","path":[{"field":"seen"}]},
		{"id":"note","path":[{"field":"note"}]}],` + onlyBefore1 + `,
		"groups":[{"id":"d","col":"seen","bucket":"day"}]}`
	if got, want := groupLabelTypes(z.run(t, "ann", buckets, chicago, nil)), []string{"2026-11-01 day=2026-11-01"}; !slices.Equal(got, want) {
		t.Fatalf("group labels = %v, want %v", got, want)
	}
}

// C4: in America/Santiago the clock jumps from 00:00 to 01:00 on 2026-09-06,
// so that day starts at 01:00 local (04:00Z), where the day before ends.
func TestZoneDayWhereMidnightIsSkipped(t *testing.T) {
	loc := readerZone("America/Santiago")
	if loc == nil {
		t.Fatal("America/Santiago is not known")
	}
	if got, want := zoneDay("2026-09-06", loc), (zonedDay{start: "2026-09-06 04:00:00.000Z", end: "2026-09-07 03:00:00.000Z"}); got != want {
		t.Fatalf("2026-09-06 = %+v, want %+v", got, want)
	}
	if before := zoneDay("2026-09-05", loc); before.end != "2026-09-06 04:00:00.000Z" {
		t.Fatalf("2026-09-05 ends at %s, want where 2026-09-06 starts", before.end)
	}
}

// R2: a run with no today resolves its presets on the reader's day. Kiritimati
// (UTC+14) and Pago Pago (UTC−11) are never on one day.
func TestTodayIsTheReadersDay(t *testing.T) {
	days := map[string]string{}
	for _, name := range []string{"Pacific/Kiritimati", "Pacific/Pago_Pago"} {
		loc := readerZone(name)
		before := time.Now().In(loc).Format(time.DateOnly)
		got, err := todayOf("", "options.today", loc)
		after := time.Now().In(loc).Format(time.DateOnly)
		if err != nil {
			t.Fatal(err)
		}
		day := got.Format(time.DateOnly)
		if day != before && day != after {
			t.Fatalf("%s: today = %s, want %s", name, day, before)
		}
		days[name] = day
	}
	if days["Pacific/Kiritimati"] == days["Pacific/Pago_Pago"] {
		t.Fatalf("both zones read one day: %v", days)
	}
}

// C3: the run endpoint takes options.timeZone (decodeStrict refuses an
// unknown key) and answers in the zone.
func TestRunEndpointTakesTimeZone(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	Register(z.app)
	sc := tests.ApiScenario{
		Name: "a run in the reader's zone", Method: http.MethodPost, URL: "/api/reports/run",
		Body:                  strings.NewReader(`{"spec":` + sightingsInPeriod + `,"params":{"period":` + nov14 + `},"options":{"timeZone":"` + chicagoZone + `"}}`),
		Headers:               map[string]string{"Authorization": z.token(t, "ann")},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"note":"start14"`, `"note":"end14"`},
		NotExpectedContent:    []string{`"note":"before14"`, `"note":"after14"`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return z.app },
		DisableTestAppCleanup: true,
	}
	sc.Test(t)
}

// typedDay is a day typed in a condition: a builder filter's {value}.
func typedDay(day string) string { return `{"value":"` + day + `"}` }

// Q3: a day typed in a condition is a day of the reader's zone, as a date
// param is: a comparison or a between of a date-time with it compares the
// instants of that local day, with the same edges (R1).
func TestTypedDayComparisonsOfADateTimeInTheZone(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	all := []any{"before8", "start8", "end8", "after8", "before1", "start1", "end1", "after1", "before14", "start14", "end14", "after14"}
	day := typedDay("2026-11-14")
	params := map[string]string{"day": `"2026-11-14"`, "period": nov14}
	betweenOf := func(lo, hi string) string {
		return `{"op":"between","args":[` + seenLeaf + `,` + lo + `,` + hi + `]}`
	}
	for name, c := range map[string]struct {
		where string
		want  []any
	}{
		"gte a typed day":                    {opOf("gte", seenLeaf, day), []any{"start14", "end14", "after14"}},
		"gt a typed day":                     {opOf("gt", seenLeaf, day), []any{"after14"}},
		"lt a typed day":                     {opOf("lt", seenLeaf, day), all[:9]},
		"lte a typed day":                    {opOf("lte", seenLeaf, day), all[:11]},
		"a typed day lte":                    {opOf("lte", day, seenLeaf), []any{"start14", "end14", "after14"}},
		"a typed day gt":                     {opOf("gt", day, seenLeaf), all[:9]},
		"between two typed days":             {betweenOf(day, day), []any{"start14", "end14"}},
		"between typed days, the DST end":    {betweenOf(typedDay("2026-11-01"), typedDay("2026-11-01")), []any{"start1", "end1"}},
		"between typed days, the DST start":  {betweenOf(typedDay("2026-03-08"), typedDay("2026-03-08")), []any{"start8", "end8"}},
		"between a typed day and a param":    {betweenOf(typedDay("2026-11-01"), dayLeaf), []any{"start1", "end1", "after1", "before14", "start14", "end14"}},
		"between an instant and a typed day": {betweenOf(typedDay("2026-11-14 06:00:00.000Z"), day), []any{"start14", "end14"}},
	} {
		t.Run(name, func(t *testing.T) {
			res := z.run(t, "ann", sightingsWhere(c.where), Options{TimeZone: chicagoZone}, params)
			requireDetails(t, res, "note", c.want...)
		})
	}
}

// R1: for a UTC reader (no zone, a name of UTC itself, a name the server does
// not know) a typed day compares as before, with the statements of a run
// with no zone; a Chicago run's statements differ, so the compare can see a
// zone.
func TestTypedDayWithNoZoneIsUTC(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	spec := sightingsWhere(opOf("gte", seenLeaf, typedDay("2026-11-14")))
	reads := func(zone string, want ...any) []readStatement {
		seen := observeReads(t)
		requireDetails(t, z.run(t, "ann", spec, Options{TimeZone: zone}, nil), "note", want...)
		return sameShape(*seen)
	}
	none := reads("", "before14", "start14", "end14", "after14")
	if chicago := reads(chicagoZone, "start14", "end14", "after14"); reflect.DeepEqual(chicago, none) {
		t.Fatal("a Chicago run sent the statements of a run with no zone: the compare cannot see a zone")
	}
	for _, zone := range []string{"UTC", "Not/AZone"} {
		if got := reads(zone, "before14", "start14", "end14", "after14"); !reflect.DeepEqual(got, none) {
			t.Fatalf("zone %q: the statements differ from a run with no zone:\n got  %v\n want %v", zone, got, none)
		}
	}
}

// Text that is not a day (an instant) is not zoned: it compares as before.
func TestTypedInstantIsNotZoned(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	spec := sightingsWhere(opOf("gte", seenLeaf, typedDay("2026-11-14 06:00:00.000Z")))
	seen := observeReads(t)
	requireDetails(t, z.run(t, "ann", spec, Options{}, nil), "note", "start14", "end14", "after14")
	utc := sameShape(*seen)
	*seen = nil
	requireDetails(t, z.run(t, "ann", spec, Options{TimeZone: chicagoZone}, nil), "note", "start14", "end14", "after14")
	if got := sameShape(*seen); !reflect.DeepEqual(got, utc) {
		t.Fatalf("a zone changed the statements of a typed instant:\n got  %v\n want %v", got, utc)
	}
}

// A Date field is a day: a typed day compares with it as before, zone or not.
func TestTypedDayAgainstADateColumnIgnoresTheZone(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":4,"source":"zoo.feedings",
		"columns":[{"id":"note","path":[{"field":"note"}]}],
		"filter":{"op":"gte","args":[{"path":[{"field":"fed"}]},{"value":"2026-01-20"}]},
		"sort":[{"col":"note"}]}`
	seen := observeReads(t)
	requireDetails(t, z.run(t, "ann", spec, Options{}, nil), "note", "f2", "f3", "f4")
	utc := sameShape(*seen)
	*seen = nil
	requireDetails(t, z.run(t, "ann", spec, Options{TimeZone: chicagoZone}, nil), "note", "f2", "f3", "f4")
	if got := sameShape(*seen); !reflect.DeepEqual(got, utc) {
		t.Fatalf("a zone changed a date column's statements:\n got  %v\n want %v", got, utc)
	}
}
