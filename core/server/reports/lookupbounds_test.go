package reports

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// The pieces of a lookup's where over the ledger lines: the line's account is
// the report row's, its day, and the two days of the period param.
const (
	lbLink = `{"op":"eq","args":[{"path":[{"link":"account"}]},{"outer":[{"field":"id"}]}]}`
	lbDay  = `{"path":[{"field":"date"}]}`
	lbFrom = `{"param":"period","bound":"from"}`
	lbTo   = `{"param":"period","bound":"to"}`
)

// lookupBoundsSpec lists the ledger accounts, each with the count of the
// ledger lines its where holds for.
func lookupBoundsSpec(where string) string {
	return `{"version":4,"source":"zoo.ledger_accounts",
		"params":[{"id":"period","type":"dateRange","label":"Period","default":{"preset":"thisMonthToDate"}}],
		"columns":[{"id":"name","path":[{"field":"full_name"}]},
			{"id":"n","formula":{"op":"lookup","source":"zoo.ledger_lines","pick":"count","value":{"path":[{"field":"id"}]},"where":` + where + `}}],
		"sort":[{"col":"name"}]}`
}

// storedDayBound finds a bound on the stored day column of the ledger lines:
// the bare column compared (the where itself reads the day through substr).
var storedDayBound = regexp.MustCompile(`\[\[t\d+\.date\]\] (>=|<=) `)

// dayBoundsOf lists the sides (">=", "<=") of the bounds on the lines' stored
// day in the statements seen, each once, sorted.
func dayBoundsOf(seen []readStatement) []string {
	var sides []string
	for _, st := range seen {
		for _, m := range storedDayBound.FindAllStringSubmatch(st.sql, -1) {
			if !slices.Contains(sides, m[1]) {
				sides = append(sides, m[1])
			}
		}
	}
	slices.Sort(sides)
	return sides
}

// addZooLineDays registers zoo.line_days: a view over the ledger lines that
// passes their text day on, as a package's computed list does. A view's column
// may hold NULL or no day, so a lookup on it must not bound it.
func addZooLineDays(t *testing.T, z *zoo) {
	t.Helper()
	view := core.NewViewCollection("zoo_line_days")
	view.ListRule = rule(`@request.auth.id != ""`)
	view.ViewRule = view.ListRule
	view.ViewQuery = `SELECT l.id, l.date FROM zoo_ledger_lines l`
	z.save(t, view)
	RegisterSource(Source{
		Key: "zoo.line_days", Collection: "zoo_line_days", Label: "Line days",
		Fields: []Field{{Key: "date", Label: "Date", Type: Date, TextDay: true, Aggregates: MinMax}},
	})
	requireNoRefusal(t, z.app, "zoo.line_days")
}

// withEdgeDays adds lines on Cash whose stored day is blank, or a day with a
// time after it on the last day before the period, its first and its last.
func withEdgeDays(t *testing.T, z *zoo) {
	t.Helper()
	for key, day := range map[string]string{
		"edgeBlank": "", "edgeBefore": "2026-06-30 23:59:59.000Z",
		"edgeFirst": "2026-07-01 00:00:00.000Z", "edgeLast": "2026-07-31 12:00:00.000Z",
	} {
		z.rec(t, key, "zoo_ledger_lines", map[string]any{
			"entry": z.id("J0"), "account": z.id("cash"), "date": day, "debit": 100, "currency": "USD", "posted": true, "position": 9,
		})
	}
}

// A comparison of the looked-up row's stored day with a day that reads
// nothing of that row bounds the stored column, when the where cannot hold
// without it: as the where itself or under its ands. Under or and not, and
// between two days of the row, there is no bound. The result is the same with
// and without the bounds, for a period and for All dates.
func TestALookupsDayComparisonBoundsTheStoredDay(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	withEdgeDays(t, z)
	addZooLineDays(t, z)
	and := func(terms ...string) string { return `{"op":"and","args":[` + strings.Join(terms, ",") + `]}` }
	op := func(name string, args ...string) string {
		return `{"op":"` + name + `","args":[` + strings.Join(args, ",") + `]}`
	}
	// A comparison of a line's day with the day of the report's own row: the
	// accounts have no day, so this one lists the lines.
	ownDay := `{"version":4,"source":"zoo.ledger_lines",
		"params":[{"id":"period","type":"dateRange","label":"Period","default":{"preset":"thisMonthToDate"}}],
		"columns":[{"id":"day","path":[{"field":"date"}]},
			{"id":"n","formula":{"op":"lookup","source":"zoo.ledger_lines","pick":"count","value":{"path":[{"field":"id"}]},
				"where":` + op("lt", lbDay, `{"outer":[{"field":"date"}]}`) + `}}],
		"sort":[{"col":"day"}]}`
	// The latest day of the account's lines: a lookup that returns a day.
	latest := `{"op":"lookup","source":"zoo.ledger_lines","pick":"max","value":` + lbDay + `,"where":` + lbLink + `}`
	cases := []struct {
		name, where string
		want        []string
	}{
		{"before a day", and(lbLink, op("lt", lbDay, lbFrom)), []string{"<="}},
		{"the day on the right", and(lbLink, op("gt", lbFrom, lbDay)), []string{"<="}},
		{"from a day", and(lbLink, op("gte", lbDay, lbFrom)), []string{">="}},
		{"month to date", and(lbLink, op("gte", lbDay, op("startOfMonth", lbTo)), op("lt", lbDay, lbTo)), []string{"<=", ">="}},
		{"a date range", and(lbLink, op("between", lbDay, `{"param":"period"}`)), []string{"<=", ">="}},
		{"between two days", and(lbLink, op("between", lbDay, lbFrom, lbTo)), []string{"<=", ">="}},
		{"an and inside the and", and(lbLink, and(op("gte", lbDay, lbFrom), op("lte", lbDay, lbTo))), []string{"<=", ">="}},
		{"the where itself", op("lt", lbDay, lbFrom), []string{"<="}},
		{"under or", and(lbLink, op("or", op("lt", lbDay, lbFrom), op("eq", `{"path":[{"field":"posted"}]}`, `{"value":false}`))), nil},
		{"under not", and(lbLink, op("not", op("gte", lbDay, lbFrom))), nil},
		{"two days of the row", and(lbLink, op("lte", lbDay, lbDay)), nil},
		{"under if", and(lbLink, op("if", op("lt", lbDay, lbFrom), `{"value":true}`, `{"value":false}`)), nil},
		{"under coalesce", and(lbLink, op("coalesce", op("lt", lbDay, lbFrom), `{"value":false}`)), nil},
		{"a lookup on the other side", and(lbLink, op("lt", lbDay, latest)), nil},
		{"the report's own day", "", []string{"<="}},
		{"a view source", `{"op":"lt","args":[{"path":[{"field":"date"}]},` + lbFrom + `]}`, nil},
	}
	july := map[string]string{"period": glJuly}
	t.Cleanup(func() { legacySQL.noLookupDayBounds = false })
	for _, c := range cases {
		spec := lookupBoundsSpec(c.where)
		switch c.name {
		case "the report's own day":
			spec = ownDay
		case "a view source":
			spec = strings.Replace(spec, `"source":"zoo.ledger_lines"`, `"source":"zoo.line_days"`, 1)
		}
		seen := observeReads(t)
		z.run(t, "ann", spec, Options{Today: "2026-07-15"}, july)
		if got := dayBoundsOf(*seen); !slices.Equal(got, c.want) {
			t.Errorf("%s: bounds on the stored day %v, want %v", c.name, got, c.want)
		}
		// The switch writes the SQL of before: no bound on the stored day.
		legacySQL.noLookupDayBounds = true
		*seen = nil
		z.run(t, "ann", spec, Options{Today: "2026-07-15"}, july)
		legacySQL.noLookupDayBounds = false
		if got := dayBoundsOf(*seen); len(got) != 0 {
			t.Errorf("%s: with the switch on, bounds on the stored day %v, want none", c.name, got)
		}
		for _, params := range []map[string]string{july, {"period": `{"preset":"allDates"}`}} {
			requireSameAsLegacy(t, z, "ann", spec, Options{Today: "2026-07-15"}, params, &legacySQL.noLookupDayBounds)
		}
	}
}

// A union source has no one stored column for its day (each part maps its
// own, and a part may map none): a lookup on it gets no bound and counts the
// same documents.
func TestALookupOnAUnionGetsNoDayBound(t *testing.T) {
	z := newZoo(t)
	addZooSales(t, z)
	registerZooSalesSources()
	seen := observeReads(t)
	res := z.run(t, "ann", `{"version":4,"source":"zoo.sites",
		"params":[{"id":"period","type":"dateRange","label":"Period","default":{"preset":"thisMonthToDate"}}],
		"columns":[{"id":"name","path":[{"field":"name"}]},
			{"id":"n","formula":{"op":"lookup","source":"zoo.documents","pick":"count","value":{"path":[{"field":"id"}]},
				"where":{"op":"lt","args":[{"path":[{"field":"day"}]},{"param":"period","bound":"from"}]}}}],
		"sort":[{"col":"name"}]}`, Options{Today: "2026-10-15"}, map[string]string{"period": `{"from":"2026-10-04","to":"2026-10-31"}`})
	// o1 (10-02) and k1 (10-03) are before the period; o2 and o3 are in it.
	for _, r := range res.Rows {
		if r.Cells["n"] != int64(2) {
			t.Fatalf("%v: %v documents before the period, want 2", r.Cells["name"], r.Cells["n"])
		}
	}
	if len(res.Rows) == 0 {
		t.Fatal("no rows")
	}
	bound := regexp.MustCompile(`\.day\]\] (>=|<=) `)
	for _, st := range *seen {
		if bound.MatchString(st.sql) {
			t.Fatalf("a lookup on a union bounds a stored day:\n%s", st.sql)
		}
	}
}

// The counts themselves, on the days at the edges of the period: a day with a
// time after it counts as its day, and a blank day matches no comparison.
func TestALookupsDayBoundsKeepTheEdgeDays(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	withEdgeDays(t, z)
	count := func(where string) any {
		t.Helper()
		spec := lookupBoundsSpec(`{"op":"and","args":[` + lbLink + `,` + where + `]}`)
		res := z.run(t, "ann", spec, Options{Today: "2026-07-15"}, map[string]string{"period": glJuly})
		for _, r := range res.Rows {
			if r.Cells["name"] == "Cash" {
				return r.Cells["n"]
			}
		}
		t.Fatal("no Cash row")
		return nil
	}
	// Cash: j1a 07-01, j2a 07-02, j3a 07-03, and the four edge lines.
	for _, c := range []struct {
		name, where string
		want        int64
	}{
		{"before the first day", `{"op":"lt","args":[` + lbDay + `,` + lbFrom + `]}`, 1},
		{"up to the first day", `{"op":"lte","args":[` + lbDay + `,` + lbFrom + `]}`, 3},
		{"after the first day", `{"op":"gt","args":[` + lbDay + `,` + lbFrom + `]}`, 3},
		{"from the last day", `{"op":"gte","args":[` + lbDay + `,` + lbTo + `]}`, 1},
		{"in the period", `{"op":"between","args":[` + lbDay + `,{"param":"period"}]}`, 5},
	} {
		if got := count(c.where); got != c.want {
			t.Errorf("%s: %v lines, want %d", c.name, got, c.want)
		}
	}
}
