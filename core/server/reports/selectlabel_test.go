package reports

import (
	"reflect"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// A select value that a label formula or a text operator reads shows its
// option's label ("North"), never its stored value ("north").

const regionPath = `{"path":[{"link":"animal"},{"link":"site"},{"field":"region"}]}`

func TestADetailLabelThatReadsASelectShowsItsOptionLabel(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", withDetailLabel(regionPath), Options{}, nil)
	requireRows(t, res, "label", "North", "North", "South", "South", "Total")
	// The option label is text: the row's label, untyped.
	text := [3]any{FieldType(""), nil, ""}
	requireLabels(t, res, text, text, text, text)
}

func TestAGroupLabelThatReadsASelectShowsItsOptionLabel(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[{"id":"animal","path":[{"link":"animal"}]},{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal","label":{"record":"g","path":[{"link":"site"},{"field":"region"}]}}]}`, Options{}, nil)
	requireRows(t, res, "label", "North", "South", "Total")
}

func TestTextOperatorsReadASelectsOptionLabel(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]},
		{"id":"where","formula":{"op":"concat","args":[`+regionPath+`,{"value":" / "},{"col":"note"}]}},
		{"id":"loud","formula":{"op":"upper","args":[`+regionPath+`]}},
		{"id":"north","formula":{"op":"startsWith","args":[`+regionPath+`,{"value":"Nor"}]}}],"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "where", "North / f1", "North / f2", "South / f3", "South / f4", nil)
	requireRows(t, res, "loud", "NORTH", "NORTH", "SOUTH", "SOUTH", nil)
	requireRows(t, res, "north", true, true, false, false, nil)
}

// A result that mixes a select with other text shows the select by its
// option label; one select's values keep its options, so a formula column of
// them describes its options and a group by it is labelled by them.
func TestIfAndCoalesceOverASelectShowItsOptionLabels(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", withDetailLabel(`{"op":"if","args":[{"op":"gt","args":[{"path":[{"field":"cost"}]},{"value":800}]},`+regionPath+`,{"value":"cheap"}]}`), Options{}, nil)
	requireRows(t, res, "label", "North", "North", "cheap", "cheap", "Total")

	grouped := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"region","formula":{"op":"coalesce","args":[`+regionPath+`]}},{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"region"}]}`, Options{}, nil)
	requireRows(t, grouped, "label", "North", "South", "Total")
	want := []Option{{Value: "north", Label: "North"}, {Value: "south", Label: "South"}}
	if got := grouped.Columns[0]; got.Type != Select || !reflect.DeepEqual(got.Options, want) {
		t.Fatalf("region column = %+v", got)
	}
}

func TestACaseOverASelectShowsItsOptionLabels(t *testing.T) {
	z := newZoo(t)
	cased := `{"op":"case","args":[{"op":"gt","args":[{"path":[{"field":"cost"}]},{"value":800}]},` + regionPath + `,{"value":"cheap"}]}`
	res := z.run(t, "ann", withDetailLabel(cased), Options{}, nil)
	requireRows(t, res, "label", "North", "North", "cheap", "cheap", "Total")
	// The case's value is still the stored one, which a filter compares.
	filtered := z.run(t, "ann", strings.TrimSuffix(withDetailLabel(cased), "}")+`,"filter":{"op":"eq","args":[`+cased+`,{"value":"north"}]}}`, Options{}, nil)
	requireRows(t, filtered, "note", "f1", "f2", nil)
}

// A mix of a select's values and other text is shown by option labels, but
// conditions compare its stored values: a filter on it matches what is stored.
func TestAConditionComparesAMixsStoredValues(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":2,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}],"sort":[{"col":"note"}],
		"params":[{"id":"p","type":"choice","options":[{"value":"north","label":"North"},{"value":"south","label":"South"}]}],"filter":FILTER}`
	blankOr := `{"op":"eq","args":[{"op":"coalesce","args":[` + regionPath + `,{"value":""}]},{"value":"north"}]}`
	requireRows(t, z.run(t, "ann", strings.Replace(spec, "FILTER", blankOr, 1), Options{}, nil), "note", "f1", "f2", nil)
	mixed := `{"op":"eq","args":[{"op":"if","args":[{"op":"gt","args":[{"path":[{"field":"cost"}]},{"value":800}]},` + regionPath + `,{"value":"x"}]},{"param":"p"}]}`
	requireRows(t, z.run(t, "ann", strings.Replace(spec, "FILTER", mixed, 1), Options{}, map[string]string{"p": `"north"`}), "note", "f1", "f2", nil)
}

// A text literal that is one of the select's stored values keeps the select:
// Kit, with no site, reads "north", shown as North like Leo's.
func TestACoalesceWithAStoredValueShowsItConsistently(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.animals","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"region","formula":{"op":"coalesce","args":[{"path":[{"link":"site"},{"field":"region"}]},{"value":"north"}]}}],
		"sort":[{"col":"name"}],"detail":{"label":{"col":"region"}}}`, Options{}, nil)
	requireRows(t, res, "label", "North", "North", "South", "Total")
	requireRows(t, res, "region", "north", "north", "south", nil)
	if res.Columns[1].Type != Select || len(res.Columns[1].Options) != 2 {
		t.Fatalf("region column = %+v", res.Columns[1])
	}
}

// meals is a source whose select's labels sort the other way from its stored
// values: apple reads Zest, zebra reads Apricot.
func withMeals(t *testing.T, z *zoo) {
	t.Helper()
	meals := core.NewBaseCollection("zoo_meals")
	meals.ListRule = rule(`@request.auth.id != ""`)
	meals.Fields.Add(&core.TextField{Name: "note"}, &core.SelectField{Name: "kind", Values: []string{"apple", "zebra"}, MaxSelect: 1})
	z.save(t, meals)
	z.rec(t, "m1", "zoo_meals", map[string]any{"note": "m1", "kind": "apple"})
	z.rec(t, "m2", "zoo_meals", map[string]any{"note": "m2", "kind": "zebra"})
	z.rec(t, "m3", "zoo_meals", map[string]any{"note": "m3", "kind": "apple"})
	RegisterSource(Source{
		Key: "zoo.meals", Collection: "zoo_meals", Label: "Meals",
		Fields: []Field{
			{Key: "note", Label: "Note", Type: Text, Aggregates: CountOnly},
			{Key: "kind", Label: "Kind", Type: Select, Options: []Option{{Value: "apple", Label: "Zest"}, {Value: "zebra", Label: "Apricot"}}},
		},
	})
}

// Every group whose key is a select's value reads in its option labels'
// order: by the column, by a formula column, under a label formula, and a
// key that mixes the select with other text.
func TestSelectGroupsReadInOptionLabelOrder(t *testing.T) {
	z := newZoo(t)
	withMeals(t, z)
	spec := `{"version":2,"source":"zoo.meals","columns":[{"id":"kind","path":[{"field":"kind"}]},
		{"id":"byFormula","formula":{"op":"coalesce","args":[{"path":[{"field":"kind"}]}]}},
		{"id":"mixed","formula":{"op":"if","args":[{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"m3"}]},{"value":"Other"},{"path":[{"field":"kind"}]}]}},
		{"id":"n","path":[{"field":"note"}],"aggregate":"count"}],"groups":[GROUP]}`
	group := func(g string) []any {
		return rowsOf(z.run(t, "ann", strings.Replace(spec, "GROUP", g, 1), Options{}, nil), "label")
	}
	for name, c := range map[string]struct {
		group string
		want  []any
	}{
		"column":        {`{"id":"g","col":"kind"}`, []any{"Apricot", "Zest", "Total"}},
		"formula":       {`{"id":"g","col":"byFormula"}`, []any{"Apricot", "Zest", "Total"}},
		"label formula": {`{"id":"g","col":"kind","label":{"op":"concat","args":[{"col":"kind"},{"value":"!"}]}}`, []any{"Apricot!", "Zest!", "Total"}},
		"mixed":         {`{"id":"g","col":"mixed"}`, []any{"Apricot", "Other", "Zest", "Total"}},
	} {
		if got := group(c.group); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: labels = %v, want %v", name, got, c.want)
		}
	}
	// A mixed column's cells show it as text, and a filter on it compares
	// stored values.
	res := z.run(t, "ann", `{"version":2,"source":"zoo.meals","columns":[{"id":"note","path":[{"field":"note"}]},
		{"id":"mixed","formula":{"op":"if","args":[{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"m3"}]},{"value":"Other"},{"path":[{"field":"kind"}]}]}}],
		"sort":[{"col":"note"}],"filter":{"op":"eq","args":[{"col":"mixed"},{"value":"apple"}]}}`, Options{}, nil)
	requireRows(t, res, "mixed", "Zest", nil)
	if res.Columns[1].Type != Text {
		t.Fatalf("mixed column = %+v", res.Columns[1])
	}
}

func TestADetailLabelIsRefusedOnAPivot(t *testing.T) {
	app := withZooRegistry(t)
	spec := loadSpec(t, `{"version":2,"source":"zoo.feedings","columns":[{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"},"detail":{"label":{"col":"species"}}}`)
	requireSpecError(t, Validate(app, spec), 400, "detail.label", "a pivot report has no detail rows")
}

// Child rows keep their own first column's label under a labelled detail row.
func TestADetailLabelLeavesChildRowsTheirLabels(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", strings.TrimSuffix(sitesWithAnimals, "}")+`,"detail":{"label":{"op":"upper","args":[{"col":"site"}]}}}`, Options{}, nil)
	requireRows(t, res, "label", "NORTH", "Leo", "2026-01-05", "2026-02-10", "SOUTH", "Zed", "2026-01-20", "2026-03-03", "Total")
	var columns []string
	for _, r := range res.Rows {
		columns = append(columns, r.LabelColumn)
	}
	if want := []string{"", "animal", "fed", "fed", "", "animal", "fed", "fed", ""}; !reflect.DeepEqual(columns, want) {
		t.Fatalf("labelColumns = %v, want %v", columns, want)
	}
}
