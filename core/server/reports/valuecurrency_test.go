package reports

import "testing"

const feeByAnimal = `{"version":2,"source":"zoo.feedings","columns":[
	{"id":"animal","path":[{"link":"animal"}]},
	{"id":"fee","formula":{"record":"g","path":[{"field":"fee"}]}},
	{"id":"fees","formula":{"record":"g","path":[{"field":"fee"}]},"aggregate":"sum"}],
	"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"fees"}]}`

// A record's amount carries its record's currency into the cell; totalled, it
// groups by that currency.
func TestARecordAmountCarriesItsCurrency(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feeByAnimal, Options{}, nil)
	// One total per fee currency, each after its groups: EUR (leo, 2 × 700) sorts
	// before USD (zed, 2 × 300).
	requireRows(t, res, "label", "Leo", "Total", "Zed", "Total")
	cells := map[string]any{}
	for _, row := range res.Rows {
		if row.Kind == KindGroup {
			cells[row.Label] = row.Cells["fee@currency"]
		}
	}
	if cells["Leo"] != "EUR" || cells["Zed"] != "USD" {
		t.Fatalf("fee currencies = %v", cells)
	}
	totals := map[any]any{}
	for _, row := range res.Rows {
		if row.Kind == KindTotal {
			totals[row.Cells["fees@currency"]] = row.Cells["fees"]
		}
	}
	if totals["EUR"] != int64(1400) || totals["USD"] != int64(600) {
		t.Fatalf("totals = %v", totals)
	}
	if c := res.Columns[1]; c.CurrencyCell != "fee@currency" {
		t.Fatalf("fee column = %+v", c)
	}
}

// A looked-up amount carries the currency of the row it picked.
func TestALookedUpAmountCarriesThePickedRowsCurrency(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"top","formula":{"op":"lookup","source":"zoo.animals","value":{"path":[{"field":"fee"}]},"pick":"max"}},
		{"id":"low","formula":{"op":"lookup","source":"zoo.animals","value":{"path":[{"field":"fee"}]}}}],"sort":[{"col":"name"}]}`, Options{}, nil)
	// ann may list leo (700 EUR), zed (300 USD), kit (50 USD).
	first := res.Rows[0]
	if first.Cells["top"] != int64(700) || first.Cells["top@currency"] != "EUR" || first.Cells["low"] != int64(50) || first.Cells["low@currency"] != "USD" {
		t.Fatalf("row = %+v", first.Cells)
	}
}

func TestCarriedCurrenciesCombineOnlyWithTheirOwn(t *testing.T) {
	app := withZooRegistry(t)
	twice, _, err := Load([]byte(`{"version":2,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}]},
		{"id":"x","formula":{"op":"add","args":[{"record":"g","path":[{"field":"fee"}]},{"record":"g","path":[{"field":"fee"}]}]}}],"groups":[{"id":"g","col":"a"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(app, twice); err != nil {
		t.Fatalf("one record field twice combines: %v", err)
	}
	mixed, _, err := Load([]byte(`{"version":2,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}]},
		{"id":"x","formula":{"op":"add","args":[{"record":"g","path":[{"field":"fee"}]},{"op":"sum","args":[{"path":[{"field":"cost"}]}]}]}}],"groups":[{"id":"g","col":"a"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	requireSpecError(t, Validate(app, mixed), 400, "columns.1.formula", "different currencies")
	// A record amount read on a group row and the total of the same amount
	// read per base row come from one place.
	withTotal, _, err := Load([]byte(`{"version":2,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}]},
		{"id":"x","formula":{"op":"add","args":[{"record":"g","path":[{"field":"fee"}]},{"op":"sum","args":[{"record":"g","path":[{"field":"fee"}]}]}]}}],"groups":[{"id":"g","col":"a"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(app, withTotal); err != nil {
		t.Fatalf("a record amount and its own total combine: %v", err)
	}
}

// totalsByCurrency maps each total row's currency cell to its amount.
func totalsByCurrency(res *Result, col string) map[any]any {
	totals := map[any]any{}
	for _, row := range res.Rows {
		if row.Kind == KindTotal {
			totals[row.Cells[col+"@currency"]] = row.Cells[col]
		}
	}
	return totals
}

// Inside a total, a carried amount reached through a column it reads, a
// looked-up amount, and a lookup whose where reads the group's record each
// total by their own currency.
func TestCarriedAmountsInsideATotalGroupByTheirCurrency(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"fee","formula":{"record":"g","path":[{"field":"fee"}]}},
		{"id":"viaCol","formula":{"op":"sum","args":[{"col":"fee"}]}},
		{"id":"looked","formula":{"op":"lookup","source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"id"}]},{"col":"animal"}]},"value":{"path":[{"field":"fee"}]}}},
		{"id":"viaLookup","formula":{"col":"looked"},"aggregate":"sum"},
		{"id":"viaRecord","formula":{"op":"lookup","source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"id"}]},{"record":"g","path":[{"field":"id"}]}]},"value":{"path":[{"field":"fee"}]}},"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"viaCol"}]}`, Options{}, nil)
	requireRows(t, res, "label", "Leo", "Total", "Zed", "Total")
	for _, col := range []string{"viaCol", "viaLookup", "viaRecord"} {
		if got := totalsByCurrency(res, col); got["EUR"] != int64(1400) || got["USD"] != int64(600) {
			t.Fatalf("%s totals = %v", col, got)
		}
	}
	// On a group row (its lookup reads the group key) the looked-up amount
	// carries the picked row's currency.
	if c := res.Rows[0].Cells; c["looked"] != int64(700) || c["looked@currency"] != "EUR" {
		t.Fatalf("Leo's row = %+v", c)
	}
}

// A percent of the whole report sums each currency apart: leo's two EUR
// feedings are half of the EUR total each, zed's two USD feedings half of the
// USD total.
func TestAPercentOfCarriedAmountsSumsEachCurrencyApart(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"share","formula":{"op":"percentOfTotal","args":[{"record":"g","path":[{"field":"fee"}]}],"scale":0}}],
		"groups":[{"id":"g","col":"animal"}]}`, Options{ExpandAll: true}, nil)
	for _, row := range res.Rows {
		if row.Kind == KindDetail && row.Cells["share"] != int64(50) {
			t.Fatalf("share = %v in %+v", row.Cells["share"], row)
		}
	}
}

// A group labelled by a record amount shows the record's own currency.
func TestARecordAmountLabelCarriesItsCurrency(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal","label":{"record":"g","path":[{"field":"fee"}]}}]}`, Options{}, nil)
	got := map[any]string{}
	for _, row := range res.Rows {
		if row.Kind == KindGroup {
			got[row.LabelValue] = row.LabelCurrency
		}
	}
	if got[int64(700)] != "EUR" || got[int64(300)] != "USD" {
		t.Fatalf("labels = %v", got)
	}
}
