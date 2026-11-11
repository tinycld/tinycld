package reports

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
)

const speciesCostByMonth = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
	{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"species"}],
	"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"%s}}`

func columnIDs(res *Result) []string {
	ids := make([]string, len(res.Columns))
	for i, c := range res.Columns {
		ids[i] = c.ID
	}
	return ids
}

func TestPivotOrderSpecPutsThePivotColumnsBeforeTheirCellColumn(t *testing.T) {
	z := newZoo(t)
	after := z.run(t, "ann", fmt.Sprintf(speciesCostByMonth, ""), Options{}, nil)
	if got, want := columnIDs(after), []string{"species", "cost", "grams", "cost.p0", "cost.p1", "cost.p2"}; !slices.Equal(got, want) {
		t.Fatalf("columns = %v, want %v", got, want)
	}
	before := z.run(t, "ann", fmt.Sprintf(speciesCostByMonth, `,"order":"spec"`), Options{}, nil)
	if got, want := columnIDs(before), []string{"species", "cost.p0", "cost.p1", "cost.p2", "cost", "grams"}; !slices.Equal(got, want) {
		t.Fatalf("columns = %v, want %v", got, want)
	}
	// Only the order changes.
	for _, id := range []string{"cost", "cost.p0", "cost.p1", "cost.p2", "grams"} {
		if a, b := rowsOf(after, id), rowsOf(before, id); !reflect.DeepEqual(a, b) {
			t.Fatalf("%s = %v with order spec, %v without", id, b, a)
		}
	}
	requireRows(t, before, "cost.p1", int64(2500), int64(0), int64(2500))
}

func TestPivotOrderRefusals(t *testing.T) {
	z := newZoo(t)
	_, err := z.try(t, "ann", fmt.Sprintf(speciesCostByMonth, `,"order":"after"`), Options{}, nil)
	requireSpecError(t, err, 400, "pivot.order", `order is "spec"`)

	first := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]}],
		"groups":[{"id":"g","col":"species"}],
		"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost","order":"spec"}}`
	_, err = z.try(t, "ann", first, Options{}, nil)
	requireSpecError(t, err, 400, "pivot.order", "first column")
}

// fittedByMonth fits the label column 20ch wide on a month pivot; first is
// the label column, id "label".
func fittedByMonth(first, order string) string {
	return `{"version":4,"source":"zoo.feedings","columns":[` + first + `,
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"label"}],
		"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"` + order + `},
		"display":{"columns":{"label":{"width":"20ch"}}}}`
}

// The label column is the spec's first column in both orders, so a fit on it
// is judged the same way.
func TestPivotOrderKeepsTheLabelColumnsFit(t *testing.T) {
	app := withZooRegistry(t)
	typed := `{"id":"label","path":[{"link":"animal"}],"labelField":"born"}`
	text := `{"id":"label","path":[{"link":"animal"},{"field":"species"}]}`
	for _, order := range []string{"", `,"order":"spec"`} {
		requireSpecError(t, Validate(app, loadSpec(t, fittedByMonth(typed, order))), 400, "display.columns.label.width", neverCutMessage)
		if err := Validate(app, loadSpec(t, fittedByMonth(text, order))); err != nil {
			t.Fatalf("order %q: a text label's width was refused: %v", order, err)
		}
	}
}
