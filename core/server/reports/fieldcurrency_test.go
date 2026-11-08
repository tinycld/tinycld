package reports

import (
	"reflect"
	"slices"
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
var feedingsWithLookedUpCost = `{"version":4,"source":"zoo.feedings","columns":[
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
	spec := `{"version":4,"source":"zoo.feedings","columns":[
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

// f3 is in EUR: the zebra group splits by currency, and on the EUR group row
// and the EUR total row the looked-up USD cost of f1 cannot be added to the
// EUR total of cost, so those cells are blank. The USD rows keep their sums.
func TestALookedUpAmountOnAGroupOrTotalInAnotherCurrencyIsBlank(t *testing.T) {
	z := newZoo(t)
	f3 := z.recs["f3"]
	f3.Set("currency", "EUR")
	if err := z.app.Save(f3); err != nil {
		t.Fatal(err)
	}
	spec := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"rolled","formula":{"op":"add","args":[` + orZero(lookedUpCost("f1")) + `,{"op":"sum","args":[{"path":[{"field":"cost"}]}]}]}}],
		"groups":[{"id":"bySpecies","col":"species"}],
		"sort":[{"col":"species"},{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	byID := map[string]Row{}
	for _, r := range res.Rows {
		byID[r.ID] = r
	}
	// The EUR group row and the EUR total row are blank; every USD group and
	// total row adds f1's 1000 to its own total of cost.
	for id, want := range map[string]struct {
		cost, rolled any
		currency     string
	}{
		`g:["zebra","EUR"]`: {int64(700), nil, "EUR"},
		`total:["EUR"]`:     {int64(700), nil, "EUR"},
		`g:["zebra","USD"]`: {int64(300), int64(1300), "USD"},
		`g:["lion","USD"]`:  {int64(3500), int64(4500), "USD"},
		`total:["USD"]`:     {int64(3800), int64(4800), "USD"},
	} {
		r, ok := byID[id]
		if !ok {
			t.Fatalf("no row %s in %v", id, rowsOf(res, "id"))
		}
		if r.Cells["cost"] != want.cost || r.Cells["cost@currency"] != want.currency || r.Cells["rolled"] != want.rolled {
			t.Fatalf("%s: cost %v %v, rolled %v; want cost %v %s, rolled %v", id, r.Cells["cost"], r.Cells["cost@currency"], r.Cells["rolled"], want.cost, want.currency, want.rolled)
		}
	}
	if !slices.ContainsFunc(res.Notices, func(n Notice) bool { return n.Code == "mixedCurrency" && slices.Contains(n.Columns, "rolled") }) {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

// nestedSum is a sum lookup over every animal ann may read (Leo, Zed, Kit)
// whose value is itself a sum lookup: the costs of the report row's animal.
// The inner value is money whose currency each feeding carries, so the outer
// sum reads the inner one's currency beside it.
const nestedSum = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"nested","formula":{"op":"lookup","source":"zoo.animals","pick":"sum",
		"value":{"op":"lookup","source":"zoo.feedings","pick":"sum",
			"where":{"op":"eq","args":[{"path":[{"link":"animal"}]},{"outer":[{"link":"animal"}]}]},
			"value":{"path":[{"field":"cost"}]}}}}],
	"sort":[{"col":"note"}]}`

// The outer sum adds the inner sum once per animal row (3 rows): Leo's costs
// are 3500 (f1 + f2), Zed's 1000 (f3 + f4). Each report row keeps its own
// value: the report is not folded into one row by the inner aggregate.
func TestANestedSumLookupOverRowCurrencyMoneyKeepsItsCurrency(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", nestedSum, Options{}, nil)
	requireRows(t, res, "kind", "detail", "detail", "detail", "detail", "total")
	requireOrderedRows(t, nestedSum, res, "note", "f1", "f2", "f3", "f4", nil)
	requireOrderedRows(t, nestedSum, res, "nested", int64(10500), int64(10500), int64(3000), int64(3000), nil)
	requireRows(t, res, "nested@currency", "USD", "USD", "USD", "USD", nil)
	if len(res.Notices) != 0 {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

// f2 is in EUR: the inner sum of Leo's costs meets two currencies, so it has
// no value, and neither has the outer sum over it on Leo's rows. The run
// names the column in its mixedCurrency notice (R3), as for any sum lookup
// that meets two currencies. Zed's rows (all USD) keep their sums.
func TestANestedSumLookupOverTwoCurrenciesIsBlankWithANotice(t *testing.T) {
	z := newZoo(t)
	f2 := z.recs["f2"]
	f2.Set("currency", "EUR")
	if err := z.app.Save(f2); err != nil {
		t.Fatal(err)
	}
	res := z.run(t, "ann", nestedSum, Options{}, nil)
	requireOrderedRows(t, nestedSum, res, "note", "f1", "f2", "f3", "f4", nil)
	requireOrderedRows(t, nestedSum, res, "nested", nil, nil, int64(3000), int64(3000), nil)
	want := []Notice{{Code: "mixedCurrency", Message: "A looked-up total mixes currencies, so it shows no value.", Columns: []string{"nested"}}}
	if !reflect.DeepEqual(res.Notices, want) {
		t.Fatalf("notices = %+v\nwant      %+v", res.Notices, want)
	}
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
	spec := `{"version":4,"source":"zoo.feedings","columns":[
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
	spec := `{"version":4,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"add","args":[
		` + lookedUpCost("f1") + `,{"path":[{"link":"animal"},{"field":"fee"}]}]}}]}`
	requireSpecError(t, Validate(app, loadSpec(t, spec)), 400, "columns.0.formula", "amounts in different currencies cannot be combined")
}
