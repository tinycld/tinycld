package reports

import (
	"reflect"
	"testing"
)

// lookedUpCost is a sum lookup of zoo.feedings.cost (an amount whose currency
// each row carries) over the feedings whose note is note.
func lookedUpCost(note string) string {
	return `{"op":"lookup","source":"zoo.feedings","pick":"sum",
		"where":{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"` + note + `"}]},
		"value":{"path":[{"field":"cost"}]}}`
}

func orZero(n string) string { return `{"op":"coalesce","args":[` + n + `,{"value":0}]}` }

// twoLookedUpCosts adds two different sum lookups of cost, each blank as 0.
var twoLookedUpCosts = `{"op":"add","args":[` + orZero(lookedUpCost("f1")) + `,` + orZero(lookedUpCost("f3")) + `]}`

// feedingsWithLookedUpCost adds looked-up costs to the row's own cost, to its
// running sum, and to each other. Every one reads zoo.feedings.cost, whose
// currency is the row's own currency field.
var feedingsWithLookedUpCost = `{"version":3,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"plus","formula":{"op":"add","args":[` + lookedUpCost("f1") + `,{"path":[{"field":"cost"}]}]}},
	{"id":"balance","formula":{"op":"add","args":[` + orZero(lookedUpCost("f1")) + `,{"op":"runningSum","args":[{"path":[{"field":"cost"}]}]}]}},
	{"id":"two","formula":` + twoLookedUpCosts + `},
	{"id":"signed","formula":{"op":"if","args":[{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"f2"}]},
		{"op":"sub","args":[{"value":0},{"op":"add","args":[` + twoLookedUpCosts + `,{"op":"runningSum","args":[{"path":[{"field":"cost"}]}]}]}]},
		{"op":"add","args":[` + twoLookedUpCosts + `,{"op":"runningSum","args":[{"path":[{"field":"cost"}]}]}]}]}},
	{"id":"none","formula":{"op":"add","args":[` + orZero(lookedUpCost("nobody")) + `,{"path":[{"field":"cost"}]}]}},
	{"id":"largest","formula":{"op":"add","args":[{"op":"lookup","source":"zoo.feedings","pick":"max","value":{"path":[{"field":"cost"}]}},{"path":[{"field":"cost"}]}]}}],
	"sort":[{"col":"note"}]}`

// ann's feedings: f1 1000, f2 2500 (leo), f3 700, f4 300 (zed), all USD.
func TestALookedUpAmountCombinesWithAmountsOfItsField(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feedingsWithLookedUpCost, Options{}, nil)
	requireOrderedRows(t, feedingsWithLookedUpCost, res, "note", "f1", "f2", "f3", "f4", nil)
	requireOrderedRows(t, feedingsWithLookedUpCost, res, "plus", int64(2000), int64(3500), int64(1700), int64(1300), nil)
	requireOrderedRows(t, feedingsWithLookedUpCost, res, "balance", int64(2000), int64(4500), int64(5200), int64(5500), nil)
	requireRows(t, res, "two", int64(1700), int64(1700), int64(1700), int64(1700), int64(1700))
	// Combined amounts combine again: signed reads two lookups and a running
	// sum twice, as a balance that flips its sign on some rows does.
	requireOrderedRows(t, feedingsWithLookedUpCost, res, "signed", int64(2700), int64(-5200), int64(5900), int64(6200), nil)
	requireOrderedRows(t, feedingsWithLookedUpCost, res, "largest", int64(3500), int64(5000), int64(3200), int64(2800), nil)
	// A lookup that matches nothing has no currency: the sum takes the row's.
	requireOrderedRows(t, feedingsWithLookedUpCost, res, "none", int64(1000), int64(2500), int64(700), int64(300), nil)
	requireRows(t, res, "none@currency", "USD", "USD", "USD", "USD", "USD")
	requireRows(t, res, "plus@currency", "USD", "USD", "USD", "USD", "USD")
	if len(res.Notices) != 0 {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

// On a group row a looked-up amount adds to the group's total of its field.
func TestALookedUpAmountCombinesWithATotalOfItsField(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"rolled","formula":{"op":"add","args":[` + orZero(lookedUpCost("f1")) + `,{"op":"sum","args":[{"path":[{"field":"cost"}]}]}]}},
		{"id":"both","formula":{"op":"add","args":[` + twoLookedUpCosts + `,` + orZero(`{"op":"sum","args":[{"path":[{"field":"cost"}]}]}`) + `]}}],
		"groups":[{"id":"bySpecies","col":"species"}],
		"sort":[{"col":"species"},{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	requireOrderedRows(t, spec, res, "note", nil, "f1", "f2", nil, "f3", "f4", nil)
	requireOrderedRows(t, spec, res, "rolled", int64(4500), int64(2000), int64(3500), int64(2000), int64(1700), int64(1300), int64(5500))
	requireRows(t, res, "rolled@currency", "USD", "USD", "USD", "USD", "USD", "USD", "USD")
	requireOrderedRows(t, spec, res, "both", int64(5200), int64(2700), int64(4200), int64(2700), int64(2400), int64(2000), int64(6200))
}

// f3 is in EUR: its cost and the looked-up USD cost of f1 cannot be added, so
// that one cell is blank and a notice names the column.
func TestALookedUpAmountInAnotherCurrencyThanItsRowIsBlank(t *testing.T) {
	z := newZoo(t)
	f3 := z.recs["f3"]
	f3.Set("currency", "EUR")
	if err := z.app.Save(f3); err != nil {
		t.Fatal(err)
	}
	spec := `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"plus","formula":{"op":"add","args":[` + lookedUpCost("f1") + `,{"path":[{"field":"cost"}]}]}}],
		"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	// Two total rows, one per currency: a formula that reads a row's cost has
	// no value on either.
	requireOrderedRows(t, spec, res, "plus", int64(2000), int64(3500), nil, int64(1300), nil, nil)
	want := []Notice{{Code: "mixedCurrency", Message: "A looked-up total mixes currencies, so it shows no value.", Columns: []string{"plus"}}}
	if !reflect.DeepEqual(res.Notices, want) {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

// Amounts of two currency fields still do not combine: a cost and an
// animal's fee. This passes before and after the task; it pins that the rule
// stays one field wide.
func TestAmountsOfTwoCurrencyFieldsStillDoNotCombine(t *testing.T) {
	app := withZooRegistry(t)
	spec := `{"version":3,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"add","args":[
		` + lookedUpCost("f1") + `,{"path":[{"link":"animal"},{"field":"fee"}]}]}}]}`
	requireSpecError(t, Validate(app, loadSpec(t, spec)), 400, "columns.0.formula", "amounts in different currencies cannot be combined")
}
