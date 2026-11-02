package reports

import (
	"reflect"
	"testing"
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
