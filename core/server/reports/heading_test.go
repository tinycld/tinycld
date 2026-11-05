package reports

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func withHeading(t *testing.T, heading string, watch ...string) {
	t.Helper()
	src := *lookupRegistered("zoo.feedings")
	src.Heading = func(core.App, *core.RequestInfo) string { return heading }
	src.Watch = watch
	RegisterSource(src)
}

func TestARunPrintsItsSourcesHeadingAndWatchesWhatItReads(t *testing.T) {
	z := newZoo(t)
	withHeading(t, "  Zoo Ltd  ", "zoo_sites")
	res := z.run(t, "ann", feedingsByDay, Options{}, nil)
	if res.Heading != "Zoo Ltd" {
		t.Fatalf("heading = %q", res.Heading)
	}
	if !slices.Contains(res.Topics, "collection/zoo_sites") {
		t.Fatalf("topics = %v", res.Topics)
	}
	other := z.run(t, "ann", `{"version":2,"source":"zoo.animals","columns":[{"id":"n","path":[{"field":"name"}]}]}`, Options{}, nil)
	if other.Heading != "" || slices.Contains(other.Topics, "collection/zoo_sites") {
		t.Fatalf("animals: heading %q, topics %v", other.Heading, other.Topics)
	}
}

func TestCheckSourcesRefusesAWatchedCollectionItCannotSignal(t *testing.T) {
	z := newZoo(t)
	withHeading(t, "", "zoo_nothing")
	if err := CheckSources(z.app); err == nil || !strings.Contains(err.Error(), `watched collection "zoo_nothing" does not exist`) {
		t.Fatalf("err = %v", err)
	}
	withHeading(t, "", "_superusers")
	if err := CheckSources(z.app); err == nil || !strings.Contains(err.Error(), "is a system collection") {
		t.Fatalf("err = %v", err)
	}
}

// officeName is the heading the zoo office row gives a reader who may view
// it; officeFiscalMonth its fiscal start month; officeCurrency its home
// currency (currency_reader_test.go).
const (
	officeName        = "Zoo Office"
	officeFiscalMonth = 7
	officeCurrency    = "NZD"
)

// officeReads holds the reader of each call of the office source's Heading
// and FiscalStartMonth, in order.
type officeReads struct {
	heading []*core.RequestInfo
	fiscal  []*core.RequestInfo
}

// addOffice gives the zoo a settings row only its owner, ann, may view
// (zoo_office: name, fiscal_month, currency; view and list rule: the owner).
func addOffice(t *testing.T, z *zoo) *core.Record {
	t.Helper()
	users, err := z.app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	office := core.NewBaseCollection("zoo_office")
	office.ListRule = rule(`owner = @request.auth.id`)
	office.ViewRule = office.ListRule
	office.Fields.Add(
		&core.TextField{Name: "name"},
		&core.NumberField{Name: "fiscal_month"},
		&core.TextField{Name: "currency"},
		&core.RelationField{Name: "owner", CollectionId: users.Id, MaxSelect: 1},
	)
	z.save(t, office)
	return z.rec(t, "office", "zoo_office", map[string]any{"name": officeName, "fiscal_month": officeFiscalMonth, "currency": officeCurrency, "owner": z.id("ann")})
}

// withOffice adds the office row and re-registers zoo.feedings with a
// Heading and FiscalStartMonth that read it for their reader, as a package
// whose business name and fiscal year live in a rule-guarded collection
// does. Both ann and bob may run zoo.feedings (both keep zed); bob may not
// view the row, so he gets the organization's name and a January start.
// newZoo's cleanup resets the registry.
func withOffice(t *testing.T, z *zoo) *officeReads {
	t.Helper()
	row := addOffice(t, z)
	reads := &officeReads{}
	settings := func(app core.App, reader *core.RequestInfo) *core.Record {
		if reader == nil {
			return nil
		}
		rec, err := FindViewableRecord(app, reader, "zoo_office", row.Id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			t.Errorf("FindViewableRecord: %v", err)
			return nil
		}
		return rec
	}
	src := *lookupRegistered("zoo.feedings")
	src.Heading = func(app core.App, reader *core.RequestInfo) string {
		reads.heading = append(reads.heading, reader)
		if rec := settings(app, reader); rec != nil {
			return rec.GetString("name")
		}
		return ""
	}
	src.FiscalStartMonth = func(app core.App, reader *core.RequestInfo) int {
		reads.fiscal = append(reads.fiscal, reader)
		if rec := settings(app, reader); rec != nil {
			return rec.GetInt("fiscal_month")
		}
		return 1
	}
	RegisterSource(src)
	return reads
}

// readerIDs names the reader of each call: its auth record's id, or what is
// missing.
func readerIDs(readers []*core.RequestInfo) []string {
	out := make([]string, 0, len(readers))
	for _, r := range readers {
		switch {
		case r == nil:
			out = append(out, "<nil reader>")
		case r.Auth == nil:
			out = append(out, "<no auth>")
		default:
			out = append(out, r.Auth.Id)
		}
	}
	return out
}

// Review Focus 1: a reader who may not view the settings row gets the
// organization's name (an empty heading), with no error and no notice that
// names the row.
func TestAHeadingShowsOnlyWhatTheReaderMayView(t *testing.T) {
	z := newZoo(t)
	withOffice(t, z)
	if got := z.run(t, "ann", feedingsByDay, Options{}, nil).Heading; got != officeName {
		t.Fatalf("ann's heading = %q, want %q", got, officeName)
	}
	bob := z.run(t, "bob", feedingsByDay, Options{}, nil)
	if bob.Heading != "" {
		t.Fatalf("bob's heading = %q, want none", bob.Heading)
	}
	for _, n := range bob.Notices {
		if strings.Contains(n.Message, officeName) || strings.Contains(n.Message, "zoo_office") {
			t.Fatalf("bob's notice names the office row: %+v", n)
		}
	}
}

// A feedings report on this fiscal year with each feeding's fiscal year. f9
// (zed, 2026-08-20) is a feeding both ann and bob may list.
const fiscalFeedings = `{"version":2,"source":"zoo.feedings",
	"params":[{"id":"period","type":"dateRange","default":{"preset":"thisFiscalYear"}}],
	"columns":[{"id":"note","path":[{"field":"note"}]},
		{"id":"fed","path":[{"field":"fed"}]},
		{"id":"fy","formula":{"op":"fiscalYear","args":[{"col":"fed"}]}}],
	"filter":{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"f9"}]}}`

// The fiscal presets and the fiscalYear formula read the fiscal start month
// for the run's reader: ann's July start, bob's January default.
func TestFiscalPresetsAndFormulasFollowTheReader(t *testing.T) {
	z := newZoo(t)
	withOffice(t, z)
	z.feeding(t, "f9", z.recs["zed"], 10, "2026-08-20", 1)
	cases := []struct {
		user     string
		from, to string
		fy       int64
	}{
		{"ann", "2026-07-01", "2027-06-30", 2027},
		{"bob", "2026-01-01", "2026-12-31", 2026},
	}
	for _, c := range cases {
		res := z.run(t, c.user, fiscalFeedings, Options{Today: "2026-08-15"}, nil)
		if len(res.Params) != 1 || res.Params[0].ID != "period" || res.Params[0].From != c.from || res.Params[0].To != c.to {
			t.Fatalf("%s: params = %+v, want period %s..%s", c.user, res.Params, c.from, c.to)
		}
		requireRows(t, res, "kind", "detail", "total")
		if got := res.Rows[0].Cells["fy"]; got != c.fy {
			t.Fatalf("%s: fiscal year of f9 = %#v, want %d", c.user, got, c.fy)
		}
	}
}

// Review Focus 2 and 4: one run reads the heading and the fiscal start month
// once each, for the run's reader, whose info carries the request's headers
// as the run's list rules see them.
func TestARunReadsHeadingAndFiscalStartOnceForItsReader(t *testing.T) {
	z := newZoo(t)
	reads := withOffice(t, z)
	spec, info, err := Load([]byte(fiscalFeedings))
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"x_zoo": "open"}
	if _, err := Run(context.Background(), z.app, Request{Spec: spec, Loaded: info, Options: Options{Today: "2026-08-15"}, Auth: z.recs["ann"], Headers: headers}); err != nil {
		t.Fatal(err)
	}
	ann := []string{z.id("ann")}
	if got := readerIDs(reads.heading); !slices.Equal(got, ann) {
		t.Fatalf("Heading readers = %v, want %v", got, ann)
	}
	if got := readerIDs(reads.fiscal); !slices.Equal(got, ann) {
		t.Fatalf("FiscalStartMonth readers = %v, want %v", got, ann)
	}
	for _, r := range append(reads.heading, reads.fiscal...) {
		if r.Headers["x_zoo"] != "open" {
			t.Fatalf("reader headers = %v, want the run's %v", r.Headers, headers)
		}
	}

	*reads = officeReads{}
	z.run(t, "bob", fiscalFeedings, Options{Today: "2026-08-15"}, nil)
	bob := []string{z.id("bob")}
	if got := readerIDs(reads.heading); !slices.Equal(got, bob) {
		t.Fatalf("bob: Heading readers = %v, want %v", got, bob)
	}
	if got := readerIDs(reads.fiscal); !slices.Equal(got, bob) {
		t.Fatalf("bob: FiscalStartMonth readers = %v, want %v", got, bob)
	}
}

// A param's options read the fiscal start month once, for their reader, and
// never the heading: they print no title.
func TestParamOptionsReadTheFiscalStartOnceForTheirReader(t *testing.T) {
	z := newZoo(t)
	reads := withOffice(t, z)
	z.options(t, "bob", animalsParam, OptionsRequest{Param: "animals"})
	if got, want := readerIDs(reads.fiscal), []string{z.id("bob")}; !slices.Equal(got, want) {
		t.Fatalf("FiscalStartMonth readers = %v, want %v", got, want)
	}
	if len(reads.heading) != 0 {
		t.Fatalf("Heading ran %d times for options", len(reads.heading))
	}
}

// Review Focus 5: Validate's dry compile has no reader, so it calls neither
// function.
func TestValidateCallsNeitherHeadingNorFiscalStart(t *testing.T) {
	z := newZoo(t)
	reads := withOffice(t, z)
	if err := Validate(z.app, loadSpec(t, fiscalFeedings)); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(reads.heading) != 0 || len(reads.fiscal) != 0 {
		t.Fatalf("Validate called Heading %d and FiscalStartMonth %d times", len(reads.heading), len(reads.fiscal))
	}
}
