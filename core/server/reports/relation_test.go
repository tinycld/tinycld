package reports

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// A path that ends at a to-one link reads the linked row's id.
func TestAPathEndingAtALinkReadsTheLinkedRowsID(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"animal","path":[{"link":"animal"}]}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "animal", z.id("leo"), z.id("leo"), z.id("zed"), z.id("zed"), nil)
	if c := res.Columns[1]; c.Type != Relation || c.Label != "Animal" {
		t.Fatalf("animal column = %+v", c)
	}
	requireOrderedRows(t, spec, res, "animal@label", "Leo", "Leo", "Zed", "Zed", nil)
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
	const spec = `{"version":2,"source":"zoo.animals","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"vault","path":[{"link":"vault"}]}],"sort":[{"col":"name"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "name", "Kit", "Leo", "Zed", nil)
	requireRows(t, res, "vault", nil, nil, nil, nil)
	requireRows(t, res, "vault@label", nil, nil, nil, nil)
}

func TestAFilterComparesARelation(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":2,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}],
		"filter":{"op":"eq","args":[{"path":[{"link":"animal"}]},{"value":"` + z.id("zed") + `"}]},
		"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "note", "f3", "f4", nil)
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
	const species = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"animal","path":[{"link":"animal"}],"labelField":"species"}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", species, Options{}, nil)
	requireOrderedRows(t, species, res, "animal@label", "lion", "lion", "zebra", "zebra", nil)
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

// A label field is a field of the record, on a column that links to a
// record: never a relation (the record's id), which would show an id.
func TestALabelFieldIsRefusedWhereItCannotLabel(t *testing.T) {
	app := withZooRegistry(t)
	cases := map[string]struct{ col, want string }{
		"the id":         {`{"id":"a","path":[{"link":"animal"}],"labelField":"id"}`, `"id" is a record id or a link, which would show an id`},
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
	const spec = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"site","path":[{"link":"animal"},{"link":"site"}],"labelField":"region"}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "site@label", "North", "North", "South", "South", nil)
}

// A link column first in the report labels its detail rows by the record.
func TestADetailRowIsLabelledByItsLinkedRecord(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},{"id":"note","path":[{"field":"note"}]}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "label", "Leo", "Leo", "Zed", "Zed", "Total")
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
	const spec = `{"version":2,"source":"zoo.animals","columns":[
		{"id":"site","path":[{"link":"site"}]},{"id":"name","path":[{"field":"name"}]}],"sort":[{"col":"name"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "site@label", "(no name)", "North", "South", nil)
	requireOrderedRows(t, spec, res, "label", "(no name)", "North", "South", "Total")
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
	const spec = `{"version":2,"source":"zoo.animals","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"keeper","path":[{"link":"keepers","mode":"nest"},{"link":"user"}]}],"sort":[{"col":"name"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "label", "Kit", "Leo", "ann", "Zed", "ann", "Total")
	requireOrderedRows(t, spec, res, "keeper@label", nil, nil, "ann", nil, "ann", nil)
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
	const aliased = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"animal","path":[{"link":"animal"}],"labelField":"mass"}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", aliased, Options{}, nil)
	requireOrderedRows(t, aliased, res, "animal@label", int64(190), int64(190), int64(300), int64(300), nil)
}

// A record shown by a field that is not text is labelled by that field's
// stored value, typed as the field is, so the client formats it as a cell of
// that type; an amount label carries its currency.
func TestALabelFieldOfAnyTypeIsTyped(t *testing.T) {
	z := newZoo(t)
	zed := z.recs["zed"]
	zed.Set("active", false)
	if err := z.app.Save(zed); err != nil {
		t.Fatal(err)
	}
	for field, c := range map[string]struct {
		typ  FieldType
		want []any
	}{
		"price":  {Money, []any{int64(500000), int64(500000), int64(120000), int64(120000), nil}},
		"active": {Bool, []any{true, true, false, false, nil}},
		"weight": {Number, []any{int64(190), int64(190), int64(300), int64(300), nil}},
		"born":   {Date, []any{"2020-05-01", "2020-05-01", "2019-02-10", "2019-02-10", nil}},
	} {
		t.Run(field, func(t *testing.T) {
			spec := `{"version":2,"source":"zoo.feedings","columns":[
				{"id":"animal","path":[{"link":"animal"}],"labelField":"` + field + `"},
				{"id":"note","path":[{"field":"note"}]}],"sort":[{"col":"note"}]}`
			res := z.run(t, "ann", spec, Options{}, nil)
			requireOrderedRows(t, spec, res, "animal@label", c.want...)
			if col := res.Columns[0]; col.LabelType != c.typ {
				t.Fatalf("animal column = %+v", col)
			}
			// The detail row is labelled by the typed record label.
			if row := res.Rows[0]; row.LabelType != c.typ || row.LabelValue != c.want[0] {
				t.Fatalf("first row label = %+v", row)
			}
			if c.typ != Money {
				return
			}
			requireRows(t, res, "animal@labelCurrency", "USD", "USD", "USD", "USD", nil)
			if col := res.Columns[0]; col.LabelCurrencyCell != "animal@labelCurrency" {
				t.Fatalf("animal column = %+v", col)
			}
			if row := res.Rows[0]; row.LabelCurrency != "USD" {
				t.Fatalf("first row label = %+v", row)
			}
		})
	}
	text := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},{"id":"site","path":[{"link":"animal"},{"link":"site"}],"labelField":"region"}]}`, Options{}, nil)
	if text.Columns[0].LabelType != Text || text.Columns[1].LabelType != Text || text.Rows[0].LabelType != "" {
		t.Fatalf("text labels = %+v / %+v", text.Columns, text.Rows[0])
	}
}

// An amount label reads each record's own currency field.
func TestAnAmountLabelReadsItsRecordsCurrency(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "f7", "zoo_feedings", map[string]any{"animal": z.id("zed"), "cost": 400, "currency": "EUR", "fed": "2026-03-10 00:00:00.000Z", "note": "f7"})
	const spec = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"f","path":[{"field":"id"}],"labelField":"cost"},{"id":"note","path":[{"field":"note"}]}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "f@label", int64(1000), int64(2500), int64(700), int64(300), int64(400), nil)
	requireOrderedRows(t, spec, res, "f@labelCurrency", "USD", "USD", "USD", "USD", "EUR", nil)
	if row := res.Rows[4]; row.LabelType != Money || row.LabelValue != int64(400) || row.LabelCurrency != "EUR" {
		t.Fatalf("f7 label = %+v", row)
	}
}

// A formula column that is a link column shows its records as that column
// does.
func TestAFormulaOfALinkColumnShowsItsLabelField(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"animal","path":[{"link":"animal"}],"labelField":"species"},
		{"id":"again","formula":{"col":"animal"}}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "again@label", "lion", "lion", "zebra", "zebra", nil)
}

// A pivot over a link may name its columns by a label field, typed and in
// that field's order; the field is checked as a column's is.
func TestAPivotOverALinkChoosesItsLabelField(t *testing.T) {
	z := sortZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"currency","path":[{"field":"currency"}]},{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"currency"}],
		"pivot":{"path":[{"link":"animal"}],"labelField":"born","cell":"cost"}}`, Options{}, nil)
	if got := pivotIDs(res); !reflect.DeepEqual(got, []string{"cost.p0=2018-01-01", "cost.p1=2019-02-10", "cost.p2=2020-05-01", "cost.p3=2023-01-01"}) {
		t.Fatalf("pivot columns = %v", got)
	}
	for _, c := range res.Columns {
		if c.Pivot != nil && c.Pivot.LabelType != Date {
			t.Fatalf("pivot column = %+v", c.Pivot)
		}
	}
	app := withZooRegistry(t)
	cases := map[string]struct {
		pivot, at, want string
		status          int
	}{
		"unknown":    {`{"path":[{"link":"animal"}],"labelField":"nope","cell":"cost"}`, "pivot.labelField", "has no field", 422},
		"the id":     {`{"path":[{"link":"animal"}],"labelField":"id","cell":"cost"}`, "pivot.labelField", "is a record id or a link", 400},
		"not a link": {`{"path":[{"field":"note"}],"labelField":"note","cell":"cost"}`, "pivot.labelField", "only a pivot over a link", 400},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			raw := `{"version":2,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]},{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
				"groups":[{"id":"g","col":"note"}],"pivot":` + c.pivot + `}`
			requireSpecError(t, Validate(app, loadSpec(t, raw)), c.status, c.at, c.want)
		})
	}
}

// With no text field a record is shown by its first other field, whatever
// its type, never its id.
func TestALabelFallsBackToTheFirstFieldOfAnyType(t *testing.T) {
	src := &Source{Key: "x", Label: "X", Fields: []Field{idField, {Key: "amount", Type: Money}, {Key: "done", Type: Bool}}}
	if f := labelFieldOf(src, ""); f == nil || f.Key != "amount" {
		t.Fatalf("label field = %+v", f)
	}
	if f := labelFieldOf(&Source{Key: "y", Fields: []Field{idField}}, ""); f != nil {
		t.Fatalf("label field of a source with only its id = %+v", f)
	}
}

// sortZoo adds two animals ann keeps whose ids sort opposite to their names,
// each with a feeding: Abe's id sorts last, Zara's first, both in the north.
// Their weights, birthdays and prices put them at the other ends of those
// orders than of the names' (Leo 190, 2020-05-01, 500000; Zed 300,
// 2019-02-10, 120000), and text order would differ from the typed order.
func sortZoo(t *testing.T) *zoo {
	t.Helper()
	z := newZoo(t)
	for _, a := range []struct {
		key, id, name, born string
		weight, price       int
	}{
		{"abe", "zzzzzzzzzzzzzz1", "Abe", "2018-01-01 00:00:00.000Z", 500, 1000000},
		{"zara", "aaaaaaaaaaaaaa1", "Zara", "2023-01-01 00:00:00.000Z", 5, 999999},
	} {
		rec := z.rec(t, a.key, "zoo_animals", map[string]any{"id": a.id, "name": a.name, "site": z.id("north"), "owner": z.id("ann"),
			"weight": a.weight, "born": a.born, "price": a.price})
		z.rec(t, "k-"+a.key, "zoo_keepers", map[string]any{"user": z.id("ann"), "animal": rec.Id})
		z.feeding(t, "f-"+a.key, rec, 10, "2026-04-01", 1)
	}
	return z
}

// A detail report sorted by a link column reads in its records' label order,
// then by id, in either direction and across pages.
func TestSortingByALinkColumnSortsByItsLabel(t *testing.T) {
	z := sortZoo(t)
	const spec = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},{"id":"note","path":[{"field":"note"}]}],"sort":[{"col":"animal","dir":"%s"},{"col":"note"}]}`
	res := z.run(t, "ann", fmt.Sprintf(spec, "asc"), Options{}, nil)
	requireOrderedRows(t, fmt.Sprintf(spec, "asc"), res, "label", "Abe", "Leo", "Leo", "Zara", "Zed", "Zed", "Total")
	desc := z.run(t, "ann", fmt.Sprintf(spec, "desc"), Options{}, nil)
	requireOrderedRows(t, fmt.Sprintf(spec, "desc"), desc, "label", "Zed", "Zed", "Zara", "Leo", "Leo", "Abe", "Total")
	var paged []string
	opts := Options{Limit: 1}
	for range 10 {
		page := z.run(t, "ann", fmt.Sprintf(spec, "asc"), opts, nil)
		for _, r := range page.Rows {
			if r.Kind == KindDetail {
				paged = append(paged, r.Label)
			}
		}
		if page.Page.Next == "" {
			break
		}
		opts.Cursor = page.Page.Next
	}
	if !reflect.DeepEqual(paged, []string{"Abe", "Leo", "Leo", "Zara", "Zed", "Zed"}) {
		t.Fatalf("pages = %v", paged)
	}
}

// Group rows sorted by their link column read in label order, and child rows
// of records read in label order too.
func TestGroupsAndChildRowsSortByTheirRecordsLabel(t *testing.T) {
	z := sortZoo(t)
	const spec = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"animal","dir":"%s"}]}`
	asc := z.run(t, "ann", fmt.Sprintf(spec, "asc"), Options{}, nil)
	requireRows(t, asc, "label", "Abe", "Leo", "Zara", "Zed", "Total")
	desc := z.run(t, "ann", fmt.Sprintf(spec, "desc"), Options{}, nil)
	requireRows(t, desc, "label", "Zed", "Zara", "Leo", "Abe", "Total")
	const children = `{"version":2,"source":"zoo.sites","columns":[
		{"id":"site","path":[{"field":"name"}]},
		{"id":"animal","path":[{"link":"animals","mode":"nest"},{"field":"id"}]}],"sort":[{"col":"site"}]}`
	nested := z.run(t, "ann", children, Options{}, nil)
	requireOrderedRows(t, children, nested, "label", "North", "Abe", "Leo", "Zara", "South", "Zed", "Total")
}

// A text operator reads a relation as its record's label: concat, upper and
// lower write it, contains and startsWith match it; a blank or hidden link
// reads as empty text.
func TestTextOperatorsReadARelationAsItsLabel(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"animal","path":[{"link":"animal"}],"labelField":"species"},
		{"id":"fed","formula":{"op":"concat","args":[{"path":[{"link":"animal"}]},{"value":" fed"}]}},
		{"id":"shout","formula":{"op":"upper","args":[{"col":"animal"}]}},
		{"id":"vault","formula":{"op":"concat","args":[{"value":"["},{"path":[{"link":"animal"},{"link":"vault"}]},{"value":"]"}]}},
		{"id":"site","formula":{"op":"lower","args":[{"path":[{"link":"animal"},{"link":"site"}]}]}}],
		"filter":{"op":"startsWith","args":[{"path":[{"link":"animal"}]},{"value":"l"}]},
		"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "note", "f1", "f2", nil)
	requireRows(t, res, "fed", "Leo fed", "Leo fed", nil)
	requireRows(t, res, "shout", "LION", "LION", nil)
	requireRows(t, res, "vault", "[]", "[]", nil)
	requireRows(t, res, "site", "north", "north", nil)
	const zeds = `{"version":2,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}],
		"filter":{"op":"contains","args":[{"path":[{"link":"animal"}]},{"value":"ED"}]},"sort":[{"col":"note"}]}`
	zed := z.run(t, "ann", zeds, Options{}, nil)
	requireOrderedRows(t, zeds, zed, "note", "f3", "f4", nil)
	label := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal","label":{"op":"concat","args":[{"col":"animal"},{"value":" at "},{"record":"g","path":[{"link":"site"}]}]}}],
		"sort":[{"col":"cost"}]}`, Options{}, nil)
	requireRows(t, label, "label", "Zed at South", "Leo at North", "Total")
}

// pageLabels pages through a detail report one row at a time and returns
// each detail row's label.
func pageLabels(t *testing.T, z *zoo, spec string) []any {
	t.Helper()
	var out []any
	opts := Options{Limit: 1}
	for range 20 {
		page := z.run(t, "ann", spec, opts, nil)
		for _, r := range page.Rows {
			if r.Kind == KindDetail {
				out = append(out, r.Label)
			}
		}
		if page.Page.Next == "" {
			return out
		}
		opts.Cursor = page.Page.Next
	}
	t.Fatal("too many pages")
	return nil
}

// A link column whose label field is a number, a date or an amount sorts by
// that typed value (not its text), then by id, in detail rows, across pages,
// on group levels, in child rows and in a pivot's columns.
func TestALinkColumnSortsByItsTypedLabel(t *testing.T) {
	z := sortZoo(t)
	for field, want := range map[string][]any{
		"weight": {"Zara", "Leo", "Leo", "Zed", "Zed", "Abe"},
		"born":   {"Abe", "Zed", "Zed", "Leo", "Leo", "Zara"},
		"price":  {"Zed", "Zed", "Leo", "Leo", "Zara", "Abe"},
	} {
		t.Run(field, func(t *testing.T) {
			detail := fmt.Sprintf(`{"version":2,"source":"zoo.feedings","columns":[
				{"id":"name","path":[{"link":"animal"},{"field":"name"}]},{"id":"animal","path":[{"link":"animal"}],"labelField":"%s"}],
				"sort":[{"col":"animal","dir":"%%s"},{"col":"name"}]}`, field)
			res := z.run(t, "ann", fmt.Sprintf(detail, "asc"), Options{}, nil)
			requireOrderedRows(t, fmt.Sprintf(detail, "asc"), res, "label", append(slices.Clone(want), "Total")...)
			if got := pageLabels(t, z, fmt.Sprintf(detail, "asc")); !reflect.DeepEqual(got, want) {
				t.Fatalf("pages = %v", got)
			}
			desc := slices.Clone(want)
			slices.Reverse(desc)
			if got := pageLabels(t, z, fmt.Sprintf(detail, "desc")); !reflect.DeepEqual(got, desc) {
				t.Fatalf("pages, descending = %v", got)
			}
			names := slices.Compact(slices.Clone(want))
			grouped := fmt.Sprintf(`{"version":2,"source":"zoo.feedings","columns":[
				{"id":"animal","path":[{"link":"animal"}],"labelField":"%s"},{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
				{"id":"name","formula":{"record":"g","path":[{"field":"name"}]}}],
				"groups":[{"id":"g","col":"animal"}]%%s}`, field)
			for _, sort := range []string{"", `,"sort":[{"col":"animal"}]`} {
				res := z.run(t, "ann", fmt.Sprintf(grouped, sort), Options{}, nil)
				requireRows(t, res, "name", append(names, nil)...)
			}
		})
	}
	// Child rows: Zara 5, Leo 190, Abe 500 in the north.
	const children = `{"version":2,"source":"zoo.sites","columns":[
		{"id":"site","path":[{"field":"name"}]},
		{"id":"animal","path":[{"link":"animals","mode":"nest"},{"field":"id"}],"labelField":"weight"}],"sort":[{"col":"site"}]}`
	nested := z.run(t, "ann", children, Options{}, nil)
	requireOrderedRows(t, children, nested, "label", "North", "5", "190", "500", "South", "300", "Total")
	if row := nested.Rows[1]; row.LabelType != Number || row.LabelValue != int64(5) {
		t.Fatalf("child row label = %+v", row)
	}
	// A pivot over a link by a number field: 5, 190, 300, 500.
	pivot := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"currency","path":[{"field":"currency"}]},{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"currency"}],"pivot":{"path":[{"link":"animal"}],"labelField":"weight","cell":"cost"}}`, Options{}, nil)
	if got := pivotIDs(pivot); !reflect.DeepEqual(got, []string{"cost.p0=5", "cost.p1=190", "cost.p2=300", "cost.p3=500"}) {
		t.Fatalf("pivot columns = %v", got)
	}
}

// Pages cross blank links and no-name records, which sort last in either
// direction (by link id, a blank one last, then by the next key).
func TestPagingCrossesBlankLinks(t *testing.T) {
	z := sortZoo(t)
	nameless := z.rec(t, "nameless", "zoo_sites", map[string]any{"region": "south"})
	zara := z.recs["zara"]
	zara.Set("site", nameless.Id)
	if err := z.app.Save(zara); err != nil {
		t.Fatal(err)
	}
	const spec = `{"version":2,"source":"zoo.animals","columns":[
		{"id":"name","path":[{"field":"name"}]},{"id":"site","path":[{"link":"site"}]}],
		"sort":[{"col":"site","dir":"%s"},{"col":"name"}]}`
	if got := pageLabels(t, z, fmt.Sprintf(spec, "asc")); !reflect.DeepEqual(got, []any{"Abe", "Leo", "Zed", "Zara", "Kit"}) {
		t.Fatalf("pages = %v", got)
	}
	if got := pageLabels(t, z, fmt.Sprintf(spec, "desc")); !reflect.DeepEqual(got, []any{"Zed", "Abe", "Leo", "Zara", "Kit"}) {
		t.Fatalf("pages, descending = %v", got)
	}
}

// Each row computes a record's label once, however many cells, sort keys and
// window orders read it.
func TestARecordLabelIsComputedOncePerRow(t *testing.T) {
	app := withZooRegistry(t)
	spec := loadSpec(t, `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}],"labelField":"born"},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"run","formula":{"op":"runningSum","args":[{"col":"cost"}]}}],
		"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"animal"}]}`)
	p, err := bind(app, spec)
	if err != nil {
		t.Fatal(err)
	}
	c := newCompiler(p, nil, nil, nil)
	s := newStmt(nil, nil)
	r, rerr := c.rows(s)
	if rerr != nil {
		t.Fatal(rerr)
	}
	sel, _, derr := c.detailSelect(r, "w")
	if derr != nil {
		t.Fatal(derr)
	}
	ctes, body, gerr := c.groupStatement(s, r, &expansion{}, nil, 1)
	if gerr != nil {
		t.Fatal(gerr)
	}
	const label = "COALESCE(NULLIF("
	if n := strings.Count(sel, label); n != 1 {
		t.Fatalf("the detail select computes the label %d times:\n%s", n, sel)
	}
	if n := strings.Count(strings.Join(ctes, "\n")+body, label); n != 1 {
		t.Fatalf("the group statement computes the label %d times", n)
	}
}
