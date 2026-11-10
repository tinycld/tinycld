package reports

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// A fictional statement in the shape of a Profit and Loss by Month (spec §5,
// with B1's amount): sections by a hidden account type, a record group with a
// having, four computed rows over sections and earlier rows, a month pivot over
// the period's every month with the total after the months, and a percent of
// the first section. It pins that the merged engine runs the shape end to end.

var plTypes = []string{"asset", "income", "cost_of_goods_sold", "expense", "other_income", "other_expense"}

// withProfitZoo adds accounts of each type (and an asset the statement leaves
// out) and journal lines: posted in January and March 2026, none in February,
// a draft, and posted lines before and after the quarter.
func withProfitZoo(t testing.TB, z *zoo) {
	t.Helper()
	readers := rule(`@request.auth.id != ""`)
	accounts := core.NewBaseCollection("zoo_pl_accounts")
	accounts.ListRule, accounts.ViewRule = readers, readers
	accounts.Fields.Add(
		&core.TextField{Name: "number"},
		&core.TextField{Name: "full_name"},
		&core.SelectField{Name: "type", Values: plTypes, MaxSelect: 1},
	)
	z.save(t, accounts)
	lines := core.NewBaseCollection("zoo_pl_lines")
	lines.ListRule, lines.ViewRule = readers, readers
	lines.Fields.Add(
		&core.RelationField{Name: "account", CollectionId: accounts.Id, MaxSelect: 1},
		&core.TextField{Name: "date"},
		&core.NumberField{Name: "debit"},
		&core.NumberField{Name: "credit"},
		&core.TextField{Name: "currency"},
		&core.BoolField{Name: "posted"},
	)
	z.save(t, lines)
	for _, a := range []struct{ key, number, name, typ string }{
		{"plBank", "1000", "Bank", "asset"},
		{"plSales", "4000", "Sales", "income"},
		{"plRefunds", "4200", "Refunds given", "income"},
		{"plGoods", "5000", "Cost of goods", "cost_of_goods_sold"},
		{"plRent", "6600", "Rent", "expense"},
		{"plIdle", "6700", "Idle expense", "expense"},
		{"plShipping", "6800", "Shipping", "expense"},
		{"plInterest", "7000", "Interest earned", "other_income"},
		{"plFees", "8000", "Bank fees", "other_expense"},
	} {
		z.rec(t, a.key, "zoo_pl_accounts", map[string]any{"number": a.number, "full_name": a.name, "type": a.typ})
	}
	for i, l := range []struct {
		account, date string
		debit, credit int
		posted        bool
	}{
		{"plBank", "2026-01-10", 10000, 0, true},
		{"plSales", "2026-01-10", 0, 10000, true},
		{"plGoods", "2026-01-12", 4000, 0, true},
		{"plRent", "2026-01-15", 2000, 0, true},
		{"plRefunds", "2026-01-20", 500, 0, true},
		// Shipping nets to 0: shown only when Rows is Active.
		{"plShipping", "2026-01-22", 300, 0, true},
		{"plShipping", "2026-01-25", 0, 300, true},
		{"plSales", "2026-03-05", 0, 6000, true},
		{"plGoods", "2026-03-06", 1500, 0, true},
		{"plInterest", "2026-03-08", 0, 300, true},
		{"plFees", "2026-03-09", 100, 0, true},
		{"plRent", "2026-03-10", 2000, 0, true},
		{"plSales", "2026-03-11", 0, 99999, false},
		{"plSales", "2026-04-02", 0, 7777, true},
		{"plRent", "2025-12-20", 5000, 0, true},
	} {
		z.rec(t, fmt.Sprintf("plLine%d", i), "zoo_pl_lines", map[string]any{
			"account": z.id(l.account), "date": l.date, "debit": l.debit, "credit": l.credit,
			"currency": "USD", "posted": l.posted,
		})
	}
	typeOptions := make([]Option, len(plTypes))
	for i, v := range plTypes {
		typeOptions[i] = Option{Value: v, Label: v}
	}
	RegisterSource(Source{
		Key: "zoo.pl_accounts", Collection: "zoo_pl_accounts", Label: "P&L accounts",
		Fields: []Field{
			{Key: "number", Label: "Number", Type: Text, Aggregates: MinMax},
			{Key: "full_name", Label: "Full name", Type: Text, Aggregates: MinMax},
			{Key: "type", Label: "Type", Type: Select, Options: typeOptions},
		},
	})
	RegisterSource(Source{
		Key: "zoo.pl_lines", Collection: "zoo_pl_lines", Label: "P&L lines",
		Fields: []Field{
			{Key: "date", Label: "Date", Type: Date, TextDay: true, Aggregates: MinMax},
			{Key: "debit", Label: "Debit", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "credit", Label: "Credit", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "currency", Label: "Currency", Type: Text},
			{Key: "posted", Label: "Posted", Type: Bool},
		},
		Links: []Link{{Key: "account", Label: "Account", Target: "zoo.pl_accounts", Cardinality: ToOne, Field: "account"}},
	})
}

const (
	// plSigned is a line's amount by its account's normal side: credit − debit
	// on income and other income, debit − credit on the rest.
	plSigned = `{"op":"if","args":[
		{"op":"in","args":[{"path":[{"link":"account"},{"field":"type"}]},{"value":["income","other_income"]}]},
		{"op":"sub","args":[{"path":[{"field":"credit"}]},{"path":[{"field":"debit"}]}]},
		{"op":"sub","args":[{"path":[{"field":"debit"}]},{"path":[{"field":"credit"}]}]}]}`
	plByMonth = `{"version":4,"source":"zoo.pl_lines",
		"params":[{"id":"period","type":"dateRange","label":"Period"},
			{"id":"rows","type":"choice","label":"Rows","default":"nonzero",
			 "options":[{"value":"nonzero","label":"Non-zero"},{"value":"active","label":"Active"}]}],
		"columns":[
			{"id":"account","label":"Account","path":[{"link":"account"}]},
			{"id":"amount","label":"Total","formula":SIGNED,"aggregate":"sum"},
			{"id":"pct","label":"% of income","formula":{"op":"percentOfSection","args":[{"col":"amount"}],"subtotal":"income","scale":1}},
			{"id":"sec","hidden":true,"path":[{"link":"account"},{"field":"type"}]}],
		"filter":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"field":"posted"}]},{"value":true}]},
			{"op":"between","args":[{"path":[{"field":"date"}]},{"param":"period"}]},
			{"op":"in","args":[{"path":[{"link":"account"},{"field":"type"}]},
				{"value":["income","cost_of_goods_sold","expense","other_income","other_expense"]}]}]},
		"groups":[
			{"id":"section","col":"sec","open":true,"subtotals":true},
			{"id":"byAccount","col":"account",
			 "label":{"op":"concat","args":[{"record":"byAccount","path":[{"field":"number"}]},{"value":" "},{"record":"byAccount","path":[{"field":"full_name"}]}]},
			 "having":{"op":"or","args":[
				{"op":"eq","args":[{"param":"rows"},{"value":"active"}]},
				{"op":"ne","args":[{"col":"amount"},{"value":0}]}]}}],
		"layout":[
			{"section":"income","title":"Income","key":"income"},
			{"section":"cogs","title":"Cost of goods sold","key":"cost_of_goods_sold"},
			{"row":"grossProfit","label":"Gross profit","formula":{"op":"sub","args":[{"subtotal":"income"},{"subtotal":"cogs"}]}},
			{"section":"expenses","title":"Expenses","key":"expense"},
			{"row":"netOperatingIncome","label":"Net operating income","formula":{"op":"sub","args":[{"subtotal":"grossProfit"},{"subtotal":"expenses"}]}},
			{"section":"otherIncome","title":"Other income","key":"other_income"},
			{"section":"otherExpenses","title":"Other expenses","key":"other_expense"},
			{"row":"netOtherIncome","label":"Net other income","formula":{"op":"sub","args":[{"subtotal":"otherIncome"},{"subtotal":"otherExpenses"}]}},
			{"row":"netIncome","label":"Net income","formula":{"op":"add","args":[{"subtotal":"netOperatingIncome"},{"subtotal":"netOtherIncome"}]}}],
		"sort":[{"formula":{"record":"byAccount","path":[{"field":"number"}]}}],
		"pivot":{"path":[{"field":"date"}],"bucket":"month","cell":"amount","range":"period","order":"spec","maxColumns":36}}`
)

func TestAFictionalProfitAndLossByMonthRunsOnTheMergedEngine(t *testing.T) {
	z := newZoo(t)
	withProfitZoo(t, z)
	raw := strings.ReplaceAll(plByMonth, "SIGNED", plSigned)
	if err := Validate(z.app, loadSpec(t, raw)); err != nil {
		t.Fatal(err)
	}
	quarter := map[string]string{"period": `{"from":"2026-01-01","to":"2026-03-31"}`}
	res := z.run(t, "ann", raw, Options{ExpandAll: true}, quarter)

	// Every month of the period is a column, February with no line too, and the
	// total follows the months; the hidden section key is not in the result.
	if got := columnIDs(res); !slices.Equal(got, []string{"account", "amount.p0", "amount.p1", "amount.p2", "amount", "pct"}) {
		t.Fatalf("columns = %v", got)
	}
	requirePivotIDs(t, res, "amount.p0=2026-01", "amount.p1=2026-02", "amount.p2=2026-03")
	for _, c := range res.Columns {
		if c.Pivot != nil && c.Pivot.LabelType != Month {
			t.Fatalf("%s label type = %q", c.ID, c.Pivot.LabelType)
		}
	}

	// The statement order: five sections, each with its accounts by number and
	// its total, and the four computed rows. The idle account has no line and
	// shipping nets to 0, so Non-zero hides both; the asset is not a P&L type.
	requireRows(t, res, "label",
		"Income", "4000 Sales", "4200 Refunds given", "Total Income",
		"Cost of goods sold", "5000 Cost of goods", "Total Cost of goods sold",
		"Gross profit",
		"Expenses", "6600 Rent", "Total Expenses",
		"Net operating income",
		"Other income", "7000 Interest earned", "Total Other income",
		"Other expenses", "8000 Bank fees", "Total Other expenses",
		"Net other income", "Net income")

	// Jan, Feb, Mar, Total by normal side. The draft, the April sale and the
	// December rent are outside the run.
	figures := map[string][4]int64{
		"4000 Sales":               {10000, 0, 6000, 16000},
		"4200 Refunds given":       {-500, 0, 0, -500},
		"Total Income":             {9500, 0, 6000, 15500},
		"5000 Cost of goods":       {4000, 0, 1500, 5500},
		"Total Cost of goods sold": {4000, 0, 1500, 5500},
		"Gross profit":             {5500, 0, 4500, 10000},
		"6600 Rent":                {2000, 0, 2000, 4000},
		"Total Expenses":           {2000, 0, 2000, 4000},
		"Net operating income":     {3500, 0, 2500, 6000},
		"7000 Interest earned":     {0, 0, 300, 300},
		"Total Other income":       {0, 0, 300, 300},
		"8000 Bank fees":           {0, 0, 100, 100},
		"Total Other expenses":     {0, 0, 100, 100},
		"Net other income":         {0, 0, 200, 200},
		"Net income":               {3500, 0, 2700, 6200},
	}
	// % of income: each row's own Total over Total Income (15500), one place.
	shares := map[string]float64{
		"4000 Sales": 103.2, "4200 Refunds given": -3.2, "Total Income": 100.0,
		"5000 Cost of goods": 35.5, "Total Cost of goods sold": 35.5, "Gross profit": 64.5,
		"6600 Rent": 25.8, "Total Expenses": 25.8, "Net operating income": 38.7,
		"7000 Interest earned": 1.9, "Total Other income": 1.9,
		"8000 Bank fees": 0.6, "Total Other expenses": 0.6,
		"Net other income": 1.3, "Net income": 40.0,
	}
	// The section rows carry the same figures as their totals; every other row
	// is one of figures.
	sectionTitles := []string{"Income", "Cost of goods sold", "Expenses", "Other income", "Other expenses"}
	seen := map[string]bool{}
	computed := 0
	for _, r := range res.Rows {
		if slices.Contains(sectionTitles, r.Label) {
			continue
		}
		want, ok := figures[r.Label]
		if !ok {
			t.Fatalf("unexpected row %q (%s)", r.Label, r.Kind)
		}
		seen[r.Label] = true
		cells := []string{"amount.p0", "amount.p1", "amount.p2", "amount"}
		var months int64
		for i, col := range cells {
			got := r.Cells[col]
			if got != want[i] {
				t.Fatalf("%s %s = %#v, want %d", r.Label, col, got, want[i])
			}
			if i < 3 {
				months += got.(int64)
			}
		}
		// The engine's months add to the engine's total, a computed row's too.
		if total := r.Cells["amount"]; months != total {
			t.Fatalf("%s months add to %d, its total is %v", r.Label, months, total)
		}
		if got := r.Cells["pct"]; got != shares[r.Label] {
			t.Fatalf("%s %% of income = %#v, want %v", r.Label, got, shares[r.Label])
		}
		if r.Kind == KindComputed {
			computed++
		}
	}
	if len(seen) != len(figures) {
		t.Fatalf("saw %d of the %d expected rows: %v", len(seen), len(figures), seen)
	}
	if computed != 4 {
		t.Fatalf("checked %d computed rows, want 4", computed)
	}

	// Active shows an account with a posted line in the period, at 0 with a 0%
	// share; an account with no line in the period stays out.
	active := map[string]string{"period": quarter["period"], "rows": `"active"`}
	all := z.run(t, "ann", raw, Options{ExpandAll: true}, active)
	labels := rowsOf(all, "label")
	if slices.Contains(labels, "6700 Idle expense") {
		t.Fatalf("Active: the idle account has no line and stays out: %v", labels)
	}
	var shipping *Row
	for i := range all.Rows {
		if all.Rows[i].Label == "6800 Shipping" {
			shipping = &all.Rows[i]
		}
	}
	if shipping == nil || shipping.Cells["amount"] != int64(0) || shipping.Cells["pct"] != 0.0 {
		t.Fatalf("Active: shipping row = %+v", shipping)
	}
}
