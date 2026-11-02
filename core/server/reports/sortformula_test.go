package reports

import (
	"slices"
	"strings"
	"testing"
)

// Groups sort by a formula over the group record; details by a row formula.
func TestSortByAFormula(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal"}],
		"sort":[{"formula":{"record":"g","path":[{"field":"weight"}]},"dir":"desc"}]}`, Options{}, nil)
	requireRows(t, res, "label", "Zed", "Leo", "Total")

	byGrams := `{"version":2,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}],
		"sort":[{"formula":{"op":"sub","args":[{"value":0},{"path":[{"field":"grams"}]}]}}]}`
	res = z.run(t, "ann", byGrams, Options{}, nil)
	requireRows(t, res, "note", "f2", "f1", "f3", "f4", nil)
	first := z.run(t, "ann", byGrams, Options{Limit: 2}, nil)
	requireRows(t, first, "note", "f2", "f1", nil)
	next := z.run(t, "ann", byGrams, Options{Limit: 2, Cursor: first.Page.Next}, nil)
	requireRows(t, next, "note", "f3", "f4", nil)
}

func TestASortKeyIsAColumnOrAFormula(t *testing.T) {
	app := withZooRegistry(t)
	for _, c := range []struct{ spec, at, want string }{
		{`{"version":2,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}],"sort":[{"col":"n","formula":{"value":1}}]}`,
			"sort.0", "a column or a formula"},
		{`{"version":2,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}],"sort":[{}]}`,
			"sort.0", "a column or a formula"},
		{`{"version":2,"source":"zoo.feedings","columns":[{"id":"c","path":[{"field":"cost"}]}],"sort":[{"formula":{"op":"runningSum","args":[{"col":"c"}]}}]}`,
			"sort.0.formula", "not a sort key"},
	} {
		spec, _, err := Load([]byte(c.spec))
		if err != nil {
			t.Fatal(err)
		}
		requireSpecError(t, Validate(app, spec), 400, c.at, c.want)
	}
}

// A formula key that is blank on some rows pages with those rows last, in
// either direction, and a cursor from another sort is refused.
func TestAFormulaSortKeyPagesWithBlanksLast(t *testing.T) {
	z := newZoo(t)
	// grams above 400, else blank: f1 500, f2 800, f3 and f4 blank.
	spec := func(dir string) string {
		return `{"version":2,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}],
		"sort":[{"formula":{"op":"if","args":[{"op":"gt","args":[{"path":[{"field":"grams"}]},{"value":400}]},
			{"path":[{"field":"grams"}]},{"value":null}]},"dir":"` + dir + `"},{"col":"note"}]}`
	}
	if got, want := pageNotes(t, z, spec("desc")), []any{"f2", "f1", "f3", "f4"}; !slices.Equal(got, want) {
		t.Fatalf("desc = %v, want %v", got, want)
	}
	if got, want := pageNotes(t, z, spec("asc")), []any{"f1", "f2", "f3", "f4"}; !slices.Equal(got, want) {
		t.Fatalf("asc = %v, want %v", got, want)
	}

	first := z.run(t, "ann", spec("asc"), Options{Limit: 1}, nil)
	other := strings.Replace(spec("asc"), `"value":400`, `"value":401`, 1)
	_, err := z.try(t, "ann", other, Options{Limit: 1, Cursor: first.Page.Next}, nil)
	requireSpecError(t, err, 400, "options.cursor", "not one this report gave")
}

// A formula that reads a link sorts by the record's label (by the label field
// of the column it reads), then by its id, on detail rows and on a level.
// Prices order Zed (120,000) before Leo (500,000), the reverse of their names.
func TestAFormulaReadingALinkSortsByTheRecordsLabel(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},{"id":"animal","path":[{"link":"animal"}],"labelField":"price"}],
		"sort":[{"formula":{"col":"animal"}},{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "note", "f3", "f4", "f1", "f2", nil)

	res = z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}],"labelField":"price"},{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal"}],"sort":[{"formula":{"col":"animal"},"dir":"desc"}]}`, Options{}, nil)
	requireRows(t, res, "cost", int64(3500), int64(1000), int64(4500))
}

// A formula that reads a row value means nothing on a group row: groups keep
// their label order and the formula sorts the detail rows inside them.
func TestAFormulaReadingARowValueSortsOnlyDetailRows(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal"}],
		"sort":[{"formula":{"op":"sub","args":[{"value":0},{"path":[{"field":"grams"}]}]}}]}`, Options{ExpandAll: true}, nil)
	requireRows(t, res, "cost", int64(3500), int64(2500), int64(1000), int64(1000), int64(700), int64(300), int64(4500))
	// Leo's costs total 3,500 and Zed's 1,000, but the key also reads grams,
	// so a group row has none and the groups stay in label order.
	res = z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal"}],
		"sort":[{"formula":{"op":"add","args":[{"op":"coalesce","args":[{"path":[{"field":"grams"}]},{"value":0}]},
			{"op":"sum","args":[{"path":[{"field":"cost"}]}]}]}}]}`, Options{}, nil)
	requireRows(t, res, "label", "Leo", "Zed", "Total")
}

// A lookup in a sort formula is decoded and compiled like one in a column.
func TestASortFormulaMayLookUp(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},{"id":"animal","path":[{"link":"animal"}]}],
		"sort":[{"formula":{"op":"lookup","source":"zoo.animals",
			"where":{"op":"eq","args":[{"path":[{"field":"id"}]},{"col":"animal"}]},
			"value":{"path":[{"field":"weight"}]}},"dir":"desc"},{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "note", "f3", "f4", "f1", "f2", nil)

	// bind refuses a lookup value that does not decode where it is, with the
	// spec's other problems, before anything compiles.
	app := withZooRegistry(t)
	err := Validate(app, loadSpec(t, `{"version":2,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}],
		"sort":[{"formula":{"op":"lookup","source":"zoo.animals","value":{"nope":1}}},{"col":"note","dir":"sideways"}]}`))
	requireSpecError(t, err, 400, "sort.0.formula.value", "a lookup's value is a formula")
	requireSpecError(t, err, 400, "sort.1.dir", "asc or desc")
}

// A sort formula is held to the formula limits with the columns it reads.
func TestValidateRefusesASortFormulaTooLargeWithTheColumnsItReads(t *testing.T) {
	app := withZooRegistry(t)
	raw := strings.Replace(strings.TrimSuffix(chainSpec(8, doubling), "}"), `"version":1`, `"version":2`, 1) +
		`,"sort":[{"formula":{"op":"add","args":[{"col":"c8"},{"col":"c8"}]}}]}`
	requireSpecError(t, Validate(app, loadSpec(t, raw)), 400, "sort.0.formula", "with the formula columns it reads")
}

// An amount read only by a sort formula still totals per currency: its
// currency becomes a key of the rows, so each currency has its own group and
// Total rows (F5).
func TestASortFormulasAmountsTotalPerCurrency(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "f7", "zoo_feedings", map[string]any{
		"animal": z.id("zed"), "cost": 400, "currency": "EUR", "fed": "2026-03-10 00:00:00.000Z", "grams": 10, "note": "f7",
	})
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal"}],
		"sort":[{"formula":{"op":"sum","args":[{"path":[{"field":"cost"}]}]},"dir":"desc"}]}`, Options{}, nil)
	requireRows(t, res, "label", "Zed", "Total", "Leo", "Zed", "Total")
	if res.Rows[1].ID != `total:["EUR"]` || res.Rows[4].ID != `total:["USD"]` {
		t.Fatalf("total ids = %s, %s", res.Rows[1].ID, res.Rows[4].ID)
	}
	requireRows(t, res, "grams", int64(10), int64(10), int64(1300), int64(400), int64(1700))
}
