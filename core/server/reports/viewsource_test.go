package reports

import (
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// addZooNotes registers zoo.notes: the feedings' text column note read as a
// Select field. Its options name two of the stored values (f1, f2); f3 and f4
// are values no option names.
func addZooNotes() {
	RegisterSource(Source{
		Key: "zoo.notes", Collection: "zoo_feedings", Label: "Notes",
		Fields: []Field{{Key: "kind", Column: "note", Label: "Kind", Type: Select,
			Options: []Option{{Value: "f1", Label: "First"}, {Value: "f2", Label: "Second"}}}},
	})
}

// addZooFeedLog adds the view zoo_feed_log over zoo_feedings, as a package's
// computed list would be: a bare autodate column (typed as the source field,
// so DateTime reads it), a computed text column (CAST … AS TEXT, so Select
// reads it) and a text column. Its list rule lets members read every row but
// one: the feeding whose note is "secret". A guest reads none.
//
//	f1 500 g big   f2 800 g big   f3 300 g small
//	f4 100 g small f5  50 g small f6  20 g small   secret 900 g (never read)
func addZooFeedLog(t testing.TB, z *zoo) {
	t.Helper()
	feedings, err := z.app.FindCollectionByNameOrId("zoo_feedings")
	if err != nil {
		t.Fatal(err)
	}
	feedings.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	z.save(t, feedings)
	z.rec(t, "secret", "zoo_feedings", map[string]any{"animal": z.id("leo"), "cost": 5, "currency": "USD", "grams": 900, "note": "secret"})
	view := core.NewViewCollection("zoo_feed_log")
	view.ListRule = rule(`@request.auth.role = "member" && note != "secret"`)
	view.ViewRule = view.ListRule
	view.ViewQuery = `SELECT f.id, f.created, CAST(CASE WHEN f.grams >= 500 THEN 'big' ELSE 'small' END AS TEXT) AS size, f.note FROM zoo_feedings f`
	z.save(t, view)
	RegisterSource(Source{
		Key: "zoo.feed_log", Collection: "zoo_feed_log", Label: "Feed log",
		Fields: []Field{
			{Key: "created", Label: "Created", Type: DateTime, Aggregates: MinMax},
			{Key: "size", Label: "Size", Type: Select, Options: []Option{{Value: "big", Label: "Big"}, {Value: "small", Label: "Small"}}},
			{Key: "note", Label: "Note", Type: Text},
		},
	})
}

func requireNoRefusal(t *testing.T, app core.App, key string) {
	t.Helper()
	if err := CheckSources(app); err != nil {
		t.Fatalf("CheckSources: %v", err)
	}
	if reason, bad := Refused(app)[key]; bad {
		t.Fatalf("%s refused: %s", key, reason)
	}
	if lookupSource(app, key) == nil {
		t.Fatalf("%s is not usable", key)
	}
}

func TestSelectOverTextPassesCheckSources(t *testing.T) {
	z := newZoo(t)
	addZooNotes()
	requireNoRefusal(t, z.app, "zoo.notes")
}

const notesByKind = `{"version":4,"source":"zoo.notes",
	"params":[{"id":"kinds","type":"choice","multiple":true,"options":[{"value":"f1","label":"First"},{"value":"f3","label":"Third"}]}],
	"columns":[{"id":"kind","path":[{"field":"kind"}]}],
	"filter":{"op":"in","args":[{"path":[{"field":"kind"}]},{"param":"kinds"}]},
	"sort":[{"col":"kind"}]}`

func TestSelectOverTextFiltersWithIn(t *testing.T) {
	z := newZoo(t)
	addZooNotes()
	requireNoRefusal(t, z.app, "zoo.notes")
	// ann keeps leo and zed: feedings f1–f4.
	requireDetails(t, z.run(t, "ann", notesByKind, Options{}, map[string]string{"kinds": `["f1","f3"]`}), "kind", "f1", "f3")
	requireDetails(t, z.run(t, "ann", notesByKind, Options{}, nil), "kind", "f1", "f2", "f3", "f4")
}

// R16: a stored value no option names is shown as stored: the cell holds the
// value (the client labels a cell from the column's options and falls back to
// it), and a group by the field is labelled with it.
func TestSelectOverTextShowsAnUnnamedValueAsItself(t *testing.T) {
	z := newZoo(t)
	addZooNotes()
	requireNoRefusal(t, z.app, "zoo.notes")
	const detail = `{"version":4,"source":"zoo.notes","columns":[{"id":"kind","path":[{"field":"kind"}]}],"sort":[{"col":"kind"}]}`
	requireDetails(t, z.run(t, "ann", detail, Options{}, nil), "kind", "f1", "f2", "f3", "f4")
	const grouped = `{"version":4,"source":"zoo.notes","columns":[{"id":"kind","path":[{"field":"kind"}]}],"groups":[{"id":"g","col":"kind"}]}`
	res := z.run(t, "ann", grouped, Options{}, nil)
	var labels []string
	for _, r := range res.Rows {
		if r.Kind == KindGroup {
			labels = append(labels, r.Label)
		}
	}
	sort.Strings(labels)
	if want := []string{"First", "Second", "f3", "f4"}; !slices.Equal(labels, want) {
		t.Fatalf("group labels = %v, want %v", labels, want)
	}
}

func TestViewSourcePassesCheckSources(t *testing.T) {
	z := newZoo(t)
	addZooFeedLog(t, z)
	requireNoRefusal(t, z.app, "zoo.feed_log")
}

const feedLog = `{"version":4,"source":"zoo.feed_log","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"size","path":[{"field":"size"}]},
	{"id":"created","path":[{"field":"created"}]}],
	"sort":[{"col":"note"}]}`

func TestViewSourceRunsUnderTheViewsListRule(t *testing.T) {
	z := newZoo(t)
	addZooFeedLog(t, z)
	z.person(t, "gus", "guest")
	requireNoRefusal(t, z.app, "zoo.feed_log")
	res := z.run(t, "ann", feedLog, Options{}, nil)
	requireDetails(t, res, "note", "f1", "f2", "f3", "f4", "f5", "f6")
	requireDetails(t, res, "size", "big", "big", "small", "small", "small", "small")
	types := make([]FieldType, 0, len(res.Columns))
	for _, c := range res.Columns {
		types = append(types, c.Type)
	}
	if want := []FieldType{Text, Select, DateTime}; !slices.Equal(types, want) {
		t.Fatalf("column types = %v, want %v", types, want)
	}
	for _, r := range res.Rows {
		if strings.Contains(r.Label, "secret") {
			t.Fatalf("the run read a row the view's list rule hides: %+v", r)
		}
	}
	// The rule refuses a guest every row.
	requireDetails(t, z.run(t, "gus", feedLog, Options{}, nil), "note")
}

// A select still reads only a select or a text column.
func TestSelectOverNumberIsStillRefused(t *testing.T) {
	z := newZoo(t)
	RegisterSource(Source{Key: "zoo.x", Collection: "zoo_feedings", Label: "X",
		Fields: []Field{{Key: "grams", Label: "Grams", Type: Select, Options: []Option{{Value: "1", Label: "One"}}}}})
	err := CheckSources(z.app)
	want := `column "grams" is a number field, which a select field cannot read`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("CheckSources = %v, want it to contain %q", err, want)
	}
}

// A select over a real select column still checks each option against the
// column's values: zoo_sites.region holds north and south only.
func TestSelectOverSelectRefusesAnOptionTheColumnLacks(t *testing.T) {
	z := newZoo(t)
	RegisterSource(Source{Key: "zoo.regions", Collection: "zoo_sites", Label: "Regions",
		Fields: []Field{{Key: "region", Label: "Region", Type: Select, Options: []Option{{Value: "east", Label: "East"}}}}})
	err := CheckSources(z.app)
	want := `option "east" is not a value of column "region"`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("CheckSources = %v, want it to contain %q", err, want)
	}
}
