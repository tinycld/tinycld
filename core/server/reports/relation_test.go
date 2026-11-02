package reports

import (
	"reflect"
	"testing"
)

// A path that ends at a to-one link reads the linked row's id.
func TestAPathEndingAtALinkReadsTheLinkedRowsID(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"animal","path":[{"link":"animal"}]}],"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "animal", z.id("leo"), z.id("leo"), z.id("zed"), z.id("zed"), nil)
	if c := res.Columns[1]; c.Type != Relation || c.Label != "Animal" {
		t.Fatalf("animal column = %+v", c)
	}
	requireRows(t, res, "animal@label", "Leo", "Leo", "Zed", "Zed", nil)
	if c := res.Columns[1]; c.LabelCell != "animal@label" {
		t.Fatalf("animal column = %+v", c)
	}
	if c := res.Columns[0]; c.LabelCell != "" {
		t.Fatalf("note column = %+v", c)
	}
}

// A linked row the reader cannot see reads blank, as every to-one value does.
func TestALinkToAHiddenRowReadsBlank(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.animals","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"vault","path":[{"link":"vault"}]}],"sort":[{"col":"name"}]}`, Options{}, nil)
	requireRows(t, res, "name", "Kit", "Leo", "Zed", nil)
	requireRows(t, res, "vault", nil, nil, nil, nil)
	requireRows(t, res, "vault@label", nil, nil, nil, nil)
}

func TestAFilterComparesARelation(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}],
		"filter":{"op":"eq","args":[{"path":[{"link":"animal"}]},{"value":"`+z.id("zed")+`"}]},
		"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "note", "f3", "f4", nil)
}

func TestIfOverRelationsNeedsOneTarget(t *testing.T) {
	app := withZooRegistry(t)
	spec, _, err := Load([]byte(`{"version":2,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"if","args":[
		{"value":true},{"path":[{"link":"animal"}]},{"path":[{"link":"animal"},{"link":"site"}]}]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	requireSpecError(t, Validate(app, spec), 400, "columns.0.formula", "link to different sources")
}

func TestATotalAcrossAToManyLinkStillEndsWithAField(t *testing.T) {
	app := withZooRegistry(t)
	spec, _, err := Load([]byte(`{"version":2,"source":"zoo.animals","columns":[{"id":"n","path":[{"link":"feedings","mode":"aggregate"}],"aggregate":"count"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	requireSpecError(t, Validate(app, spec), 400, "columns.0.path", "or with a link to one row")
}

// The author picks the field a link column shows; a target with no fields
// shows its source's name, never an id.
func TestALinkColumnShowsTheChosenLabelField(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"animal","path":[{"link":"animal"}],"labelField":"species"}],"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "animal@label", "lion", "lion", "zebra", "zebra", nil)
	keepers := z.run(t, "ann", `{"version":2,"source":"zoo.keepers","columns":[{"id":"k","path":[{"field":"id"}]}]}`, Options{}, nil)
	requireRows(t, keepers, "k@label", "Keepers", "Keepers", nil)
	requireRows(t, keepers, "label", "Keepers", "Keepers", "Total")
	app := withZooRegistry(t)
	spec, _, err := Load([]byte(`{"version":2,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}],"labelField":"nope"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	requireSpecError(t, Validate(app, spec), 422, "columns.0.labelField", "has no field")
}

// A label field is a field that reads as text, on a column that links to a
// record: never the record's id, an amount or a yes/no.
func TestALabelFieldIsRefusedWhereItCannotLabel(t *testing.T) {
	app := withZooRegistry(t)
	cases := map[string]struct{ col, want string }{
		"the id":         {`{"id":"a","path":[{"link":"animal"}],"labelField":"id"}`, "a text, choice, number or date field"},
		"an amount":      {`{"id":"a","path":[{"link":"animal"}],"labelField":"price"}`, "a text, choice, number or date field"},
		"a yes/no":       {`{"id":"a","path":[{"link":"animal"}],"labelField":"active"}`, "a text, choice, number or date field"},
		"not a link":     {`{"id":"a","path":[{"field":"note"}],"labelField":"note"}`, "only a column that links to a record"},
		"a formula text": {`{"id":"a","formula":{"value":"x"},"labelField":"name"}`, "only a column that links to a record"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			raw := `{"version":2,"source":"zoo.feedings","columns":[` + c.col + `]}`
			requireSpecError(t, Validate(app, loadSpec(t, raw)), 400, "columns.0.labelField", c.want)
		})
	}
}

// A select label field shows its option's label, not the stored value.
func TestALinkLabelFieldThatIsASelectShowsItsOptionLabel(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"site","path":[{"link":"animal"},{"link":"site"}],"labelField":"region"}],"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "site@label", "North", "North", "South", "South", nil)
}

// A link column first in the report labels its detail rows by the record.
func TestADetailRowIsLabelledByItsLinkedRecord(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},{"id":"note","path":[{"field":"note"}]}],"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "label", "Leo", "Leo", "Zed", "Zed", "Total")
}

// A visible record whose label field is blank shows (no name); a blank link
// has no label (the client shows (none)).
func TestALinkedRecordWithABlankLabelShowsNoName(t *testing.T) {
	z := newZoo(t)
	nameless := z.rec(t, "nameless", "zoo_sites", map[string]any{"region": "north"})
	kit := z.recs["kit"]
	kit.Set("site", nameless.Id)
	if err := z.app.Save(kit); err != nil {
		t.Fatal(err)
	}
	res := z.run(t, "ann", `{"version":2,"source":"zoo.animals","columns":[
		{"id":"site","path":[{"link":"site"}]},{"id":"name","path":[{"field":"name"}]}],"sort":[{"col":"name"}]}`, Options{}, nil)
	requireRows(t, res, "site@label", "(no name)", "North", "South", nil)
	requireRows(t, res, "label", "(no name)", "North", "South", "Total")
}

// A label is read through the target's rule: a record the reader cannot see
// has no label even when its id is known.
func TestALabelNeverReadsARecordTheReaderCannotSee(t *testing.T) {
	z := newZoo(t)
	read := func(target, key string) any {
		t.Helper()
		src := lookupRegistered(target)
		s := newStmt(z.app, z.as("ann"))
		label := labelValueSQL(s, src, labelFieldOf(src, ""), s.bind(z.id(key)))
		with, err := s.with()
		if err != nil {
			t.Fatal(err)
		}
		rows, err := z.app.DB().NewQuery(with + "SELECT " + label + " AS l").Bind(s.params).Rows()
		if err != nil {
			t.Fatal(err)
		}
		out, err := scanRows(rows)
		if err != nil {
			t.Fatal(err)
		}
		return out[0]["l"]
	}
	if got := read("zoo.vault", "v1"); got != nil {
		t.Fatalf("hidden vault label = %v", got)
	}
	if got := read("zoo.animals", "mo"); got != nil {
		t.Fatalf("hidden animal label = %v", got)
	}
	if got := read("zoo.animals", "leo"); got != "Leo" {
		t.Fatalf("visible animal label = %v", got)
	}
}

// A child row column that links to a record shows the record's label, and
// labels the child row when it is the nest level's first column.
func TestAChildRowLinkShowsItsRecordsLabel(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.animals","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"keeper","path":[{"link":"keepers","mode":"nest"},{"link":"user"}]}],"sort":[{"col":"name"}]}`, Options{}, nil)
	requireRows(t, res, "label", "Kit", "Leo", "ann", "Zed", "ann", "Total")
	requireRows(t, res, "keeper@label", nil, nil, "ann", nil, "ann", nil)
	if c := res.Columns[1]; c.LabelCell != "keeper@label" {
		t.Fatalf("keeper column = %+v", c)
	}
}

// A pivot over a link names its columns by the records' labels, in label
// order, never by their ids.
func TestAPivotOverALinkNamesItsColumnsByLabel(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.animals","columns":[
		{"id":"species","path":[{"field":"species"}]},
		{"id":"weight","path":[{"field":"weight"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],
		"pivot":{"path":[{"link":"site"}],"cell":"weight"}}`, Options{}, nil)
	if got := pivotIDs(res); !reflect.DeepEqual(got, []string{"weight.p0=North", "weight.p1=South", "weight.p2=(none)"}) {
		t.Fatalf("pivot columns = %v", got)
	}
	requireRows(t, res, "weight.p0", int64(0), int64(190), int64(0), int64(190))
	requireRows(t, res, "weight.p2", int64(4), int64(0), int64(0), int64(4))
}

// An aliased label field reads its new name.
func TestALabelFieldFollowsItsAlias(t *testing.T) {
	z := newZoo(t)
	spec, info, err := Load([]byte(`{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"animal","path":[{"link":"animal"}],"labelField":"mass"}],"sort":[{"col":"note"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if spec.Columns[1].LabelField != "weight" || len(info.Aliased) != 1 {
		t.Fatalf("labelField = %q, aliased = %+v", spec.Columns[1].LabelField, info.Aliased)
	}
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"animal","path":[{"link":"animal"}],"labelField":"mass"}],"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "animal@label", "190", "190", "300", "300", nil)
}
