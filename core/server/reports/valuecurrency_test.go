package reports

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

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

// lionsLayout is ann's feedings by species with a Lions section (leo's two
// feedings, 3500 USD) and the computed rows rows.
func lionsLayout(rows string) string {
	return `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],
		"layout":[{"section":"lions","title":"Lions","key":"lion"},` + rows + `]}`
}

const leoFee = `{"op":"lookup","source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Leo"}]},"value":{"path":[{"field":"fee"}]}}`

const leoPrice = `{"op":"lookup","source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Leo"}]},"value":{"path":[{"field":"price"}]}}`

// A computed row shows a looked-up amount in its own currency, not in the
// currency of the row it is in; and a section's subtotal, which is in that
// row's currency, combines with neither a carried nor a fixed currency.
func TestAComputedRowKeepsALookedUpAmountsCurrency(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", lionsLayout(`{"row":"fee","label":"Leo fee","formula":`+leoFee+`},
		{"row":"price","label":"Leo price","formula":`+leoPrice+`}`), Options{}, nil)
	requireRows(t, res, "label", "Lions", "Total Lions", "Leo fee", "Leo price")
	requireRows(t, res, "cost", int64(3500), int64(3500), int64(700), int64(500000))
	requireRows(t, res, "cost@currency", "USD", "USD", "EUR", "USD")
	if c := res.Columns[1]; c.CurrencyCell != "cost@currency" {
		t.Fatalf("cost column = %+v", c)
	}
	for name, formula := range map[string]string{"carried": leoFee, "fixed": leoPrice} {
		t.Run(name, func(t *testing.T) {
			_, err := z.try(t, "ann", lionsLayout(`{"row":"x","label":"Lions plus Leo","formula":{"op":"add","args":[{"subtotal":"lions"},`+formula+`]}}`), Options{}, nil)
			requireSpecError(t, err, 400, "layout.1.formula", "different currencies")
		})
	}
}

// A column with a fixed currency (price, USD) and a computed row that carries
// its own (leo's fee, EUR) names a currency cell: the section and its
// subtotal show the fixed code there, the computed row its own.
func TestAFixedCurrencyColumnKeepsItsCodeBesideACarriedRow(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.animals","columns":[
		{"id":"species","path":[{"field":"species"}]},
		{"id":"price","path":[{"field":"price"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],
		"layout":[{"section":"lions","title":"Lions","key":"lion"},
			{"row":"fee","label":"Leo fee","formula":`+leoFee+`}]}`, Options{}, nil)
	requireRows(t, res, "label", "Lions", "Total Lions", "Leo fee")
	requireRows(t, res, "price", int64(500000), int64(500000), int64(700))
	requireRows(t, res, "price@currency", "USD", "USD", "EUR")
	if c := res.Columns[1]; c.CurrencyCell != "price@currency" {
		t.Fatalf("price column = %+v", c)
	}
	// Expanded, the section's detail rows read the fixed code too.
	open := z.run(t, "ann", `{"version":2,"source":"zoo.animals","columns":[
		{"id":"species","path":[{"field":"species"}]},
		{"id":"price","path":[{"field":"price"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],
		"layout":[{"section":"lions","title":"Lions","key":"lion"},
			{"row":"fee","label":"Leo fee","formula":`+leoFee+`}]}`, Options{ExpandAll: true}, nil)
	requireRows(t, open, "kind", "group", "detail", "subtotal", "computed")
	requireRows(t, open, "price@currency", "USD", "USD", "USD", "EUR")
}

// ivy is a second animal ann owns with a 300 fee, in euros: zed's 300 is in
// dollars.
func addIvy(t *testing.T, z *zoo) {
	t.Helper()
	z.rec(t, "ivy", "zoo_animals", map[string]any{"name": "Ivy", "species": "ibex", "owner": z.id("ann"), "fee": 300, "fee_currency": "EUR"})
}

// A group by a looked-up amount groups by its currency too: 300 EUR and 300
// USD are two groups, each under its currency's total.
func TestAGroupByALookedUpAmountGroupsByItsCurrency(t *testing.T) {
	z := newZoo(t)
	addIvy(t, z)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.animals","columns":[
		{"id":"aid","path":[{"field":"id"}]},
		{"id":"fe","formula":{"op":"lookup","source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"id"}]},{"col":"aid"}]},"value":{"path":[{"field":"fee"}]}}},
		{"id":"n","path":[{"field":"id"}],"aggregate":"count"}],
		"groups":[{"id":"g","col":"fe"}]}`, Options{}, nil)
	var groups []string
	for _, row := range res.Rows {
		if row.Kind == KindGroup {
			groups = append(groups, fmt.Sprintf("%v %v n=%v", row.Cells["fe"], row.Cells["fe@currency"], row.Cells["n"]))
		}
	}
	want := []string{"300 EUR n=1", "700 EUR n=1", "50 USD n=1", "300 USD n=1"}
	if !slices.Equal(groups, want) {
		t.Fatalf("groups = %v, want %v", groups, want)
	}
}

// Equal amounts in two currencies tie: the pick takes the lower currency code
// for min and for max, amount and currency from one row.
func TestALookupTieIsBrokenByCurrencyCode(t *testing.T) {
	z := newZoo(t)
	addIvy(t, z)
	tied := `{"op":"lookup","source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"fee"}]},{"value":300}]},"value":{"path":[{"field":"fee"}]},"pick":"PICK"}`
	res := z.run(t, "ann", `{"version":2,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"lo","formula":`+strings.Replace(tied, "PICK", "min", 1)+`},
		{"id":"hi","formula":`+strings.Replace(tied, "PICK", "max", 1)+`}],"sort":[{"col":"name"}]}`, Options{}, nil)
	c := res.Rows[0].Cells
	if c["lo"] != int64(300) || c["lo@currency"] != "EUR" || c["hi"] != int64(300) || c["hi@currency"] != "EUR" {
		t.Fatalf("row = %+v", c)
	}
}

// A looked-up row or a record the reader cannot see has no amount and no
// currency.
func TestAHiddenRowCarriesNoCurrency(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"mo","formula":{"op":"lookup","source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Mo"}]},"value":{"path":[{"field":"fee"}]}}}],"sort":[{"col":"name"}]}`, Options{}, nil)
	if c := res.Rows[0].Cells; c["mo"] != nil || c["mo@currency"] != nil {
		t.Fatalf("Mo's fee = %+v", c)
	}
	// leo's vault is hidden, so the animal read past it is too; zed has none.
	rec := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"fee","formula":{"record":"g","path":[{"field":"fee"}]}},
		{"id":"vfee","formula":{"record":"g","path":[{"link":"vault"},{"link":"animal"},{"field":"fee"}]}}],
		"groups":[{"id":"g","col":"animal"}]}`, Options{}, nil)
	requireRows(t, rec, "label", "Leo", "Zed", "Total")
	requireRows(t, rec, "fee@currency", "EUR", "USD", nil)
	requireRows(t, rec, "vfee", nil, nil, nil)
	requireRows(t, rec, "vfee@currency", nil, nil, nil)
}

// pivotLabels lists a result's pivot columns as "id=amount currency (type)".
func pivotLabels(res *Result) []string {
	var out []string
	for _, c := range res.Columns {
		if c.Pivot != nil {
			out = append(out, fmt.Sprintf("%s=%v %s (%s)", c.ID, c.Pivot.LabelValue, c.Pivot.LabelCurrency, c.Pivot.LabelType))
		}
	}
	return out
}

// A pivot over an amount with its own currency field pivots on the amount and
// its currency: a 300 EUR feeding of leo's and zed's 300 USD one are two
// columns, in currency order, then by amount, each labelled by its typed
// amount and currency.
func TestAPivotOverAnAmountSplitsItsCurrencies(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "f7", "zoo_feedings", map[string]any{"animal": z.id("leo"), "cost": 300, "currency": "EUR", "fed": "2026-04-01 00:00:00.000Z", "grams": 10, "note": "f7"})
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],
		"pivot":{"path":[{"field":"cost"}],"cell":"cost"}}`, Options{}, nil)
	want := []string{"cost.p0=300 EUR (money)", "cost.p1=300 USD (money)", "cost.p2=700 USD (money)", "cost.p3=1000 USD (money)", "cost.p4=2500 USD (money)"}
	if got := pivotLabels(res); !slices.Equal(got, want) {
		t.Fatalf("pivot columns = %v, want %v", got, want)
	}
	requireRows(t, res, "label", "lion", "Total", "lion", "zebra", "Total")
	requireRows(t, res, "cost@currency", "EUR", "EUR", "USD", "USD", "USD")
	requireRows(t, res, "cost.p0", int64(300), int64(300), int64(0), int64(0), int64(0))
	requireRows(t, res, "cost.p1", int64(0), int64(0), int64(0), int64(300), int64(300))
}

// A pivot over an amount in its source's fixed currency keeps one column per
// amount, each labelled by its typed amount and that currency.
func TestAPivotOverAFixedCurrencyAmountKeepsOneColumnPerAmount(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.animals","columns":[
		{"id":"active","path":[{"field":"active"}]},
		{"id":"weight","path":[{"field":"weight"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"active"}],
		"pivot":{"path":[{"field":"price"}],"cell":"weight"}}`, Options{}, nil)
	want := []string{"weight.p0=5000 USD (money)", "weight.p1=120000 USD (money)", "weight.p2=500000 USD (money)"}
	if got := pivotLabels(res); !slices.Equal(got, want) {
		t.Fatalf("pivot columns = %v, want %v", got, want)
	}
}
