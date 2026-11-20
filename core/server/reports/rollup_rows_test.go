package reports

import (
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// A balance list by account: a line's debit or credit on detail rows; on the
// account's row the stored months' net added to the lines' net, on its side
// (the rollup); on every row above it the sum of the rows shown under it
// (total "rows"). Its lines are July's; the stored months are June's.
// groups is the report's groups, with NET for an account's net and BEFORE_D,
// BEFORE_C for its stored months on each side.
func rowsRollupSpec(groups string) string {
	months := strings.ReplaceAll(monthsOf, "ACCOUNT", `{"record":"g","path":[{"field":"id"}]}`)
	net := `{"op":"add","args":[` + months + `,{"op":"coalesce","args":[{"op":"sum","args":[` + glNet + `]},{"value":0}]}]}`
	side := func(x string, credit bool) string {
		if credit {
			x = `{"op":"sub","args":[{"value":0},` + x + `]}`
		}
		return `{"op":"if","args":[{"op":"gt","args":[` + x + `,{"value":0}]},` + x + `,{"value":null}]}`
	}
	groups = strings.NewReplacer("NET", net, "BEFORE_D", side(months, false), "BEFORE_C", side(months, true)).Replace(groups)
	return `{"version":4,"source":"zoo.ledger_lines","columns":[
		{"id":"account","path":[{"link":"account"}],"labelField":"number"},
		{"id":"kind","path":[{"link":"account"},{"field":"type"}]},
		{"id":"day","path":[{"field":"date"}]},
		{"id":"debit","formula":` + side(glNet, false) + `,"total":"rows","rollup":` + side(net, false) + `},
		{"id":"credit","formula":` + side(glNet, true) + `,"total":"rows","rollup":` + side(net, true) + `},
		{"id":"gap","formula":{"op":"sub","args":[{"op":"coalesce","args":[{"col":"debit"},{"value":0}]},{"op":"coalesce","args":[{"col":"credit"},{"value":0}]}]}}],
		"filter":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"field":"posted"}]},{"value":true}]},
			{"op":"gte","args":[{"path":[{"field":"date"}]},{"value":"2026-07-01"}]}]},
		"groups":` + groups + `,
		"sort":[{"col":"kind"},{"formula":{"record":"g","path":[{"field":"number"}]}},{"col":"day"}]}`
}

const (
	// One row per account, also for an account with no line of July (fill).
	rowsRollupFilled = `[{"id":"g","col":"account","fill":{},"label":{"record":"g","path":[{"field":"number"}]}HAVING,
		"opening":{"label":"Balance before July","cells":{"debit":BEFORE_D,"credit":BEFORE_C}}}]`
	rowsRollupHaving = `,"having":{"op":"ne","args":[NET,{"value":0}]}`
	// The accounts under their type.
	rowsRollupByKind = `[{"id":"k","col":"kind"},{"id":"g","col":"account","label":{"record":"g","path":[{"field":"number"}]}}]`
)

// rowsRollupLine is one row of the balance list: its kind and label, and its
// debit, credit and gap cells with the debit's currency code.
type rowsRollupLine struct {
	Kind, Label              string
	Debit, Credit, Gap, Code any
}

func rowsRollupLines(res *Result) []rowsRollupLine {
	var out []rowsRollupLine
	for _, r := range res.Rows {
		out = append(out, rowsRollupLine{r.Kind, r.Label, r.Cells["debit"], r.Cells["credit"], r.Cells["gap"], r.Cells["debit@currency"]})
	}
	return out
}

// rowsRollupZoo is the ledger zoo with its stored months (cash 500 USD, bank
// 900 EUR, petty 250 GBP) and one more: idle, 300 USD. idle has no line.
func rowsRollupZoo(t *testing.T) *zoo {
	t.Helper()
	z := newZoo(t)
	withBalanceZoo(t, z)
	withLedgerMonths(t, z, "zoo.ledger_lines.currency")
	z.rec(t, "idleJune", "zoo_ledger_months", map[string]any{
		"account": z.id("idle"), "first_day": "2026-06-01", "debit_total": 300, "credit_total": 0, "currency": "USD",
	})
	return z
}

func requireRowsRollupLines(t *testing.T, got, want []rowsRollupLine) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("rows =\n%#v\nwant\n%#v", got, want)
	}
}

// The account row shows the rollup (the months before July and July's lines),
// its opening row the months, its detail rows each line, and the Total adds
// the account rows shown: idle (a month, no line) counts, and the accounts
// whose net is 0 are hidden and add nothing. bank and petty have a month in
// another currency than the lines': the having cannot read their net, so they
// are hidden and the run says so.
func TestARowsColumnShowsItsRollupOnTheInnermostGroup(t *testing.T) {
	z := rowsRollupZoo(t)
	res := z.run(t, "ann", rowsRollupSpec(strings.ReplaceAll(rowsRollupFilled, "HAVING", rowsRollupHaving)), Options{ExpandAll: true}, nil)
	const before = "Balance before July"
	requireRowsRollupLines(t, rowsRollupLines(res), []rowsRollupLine{
		{KindGroup, "1000", nil, int64(6500), int64(-6500), "USD"},
		{KindOpening, before, int64(500), nil, nil, "USD"},
		{KindDetail, "1000", nil, int64(10000), int64(-10000), "USD"},
		{KindDetail, "1000", int64(3000), nil, int64(3000), "USD"},
		{KindGroup, "1300", int64(300), nil, int64(300), "USD"},
		{KindOpening, before, int64(300), nil, nil, "USD"},
		{KindGroup, "2000", nil, int64(4500), int64(-4500), "USD"},
		{KindOpening, before, nil, nil, nil, "USD"},
		{KindDetail, "2000", nil, int64(4500), int64(-4500), "USD"},
		{KindGroup, "4000", int64(1500), nil, int64(1500), "USD"},
		{KindOpening, before, nil, nil, nil, "USD"},
		{KindDetail, "4000", int64(1500), nil, int64(1500), "USD"},
		{KindGroup, "5000", int64(10000), nil, int64(10000), "USD"},
		{KindOpening, before, nil, nil, nil, "USD"},
		{KindDetail, "5000", int64(10000), nil, int64(10000), "USD"},
		{KindTotal, "Total", int64(11800), int64(11000), int64(800), "USD"},
	})
	if !slices.ContainsFunc(res.Notices, func(n Notice) bool {
		return n.Code == "mixedCurrency" && slices.Contains(n.Conditions, "groups.0.having")
	}) {
		t.Fatalf("notices = %+v, want mixedCurrency naming groups.0.having", res.Notices)
	}
}

// withLedgerCurrency gives zoo.ledger_lines a home currency, as a package
// whose lines name a per-row currency field also does. newZoo's cleanup
// resets the registry.
func withLedgerCurrency(t *testing.T, code string) {
	t.Helper()
	src := *lookupRegistered("zoo.ledger_lines")
	src.Currency = func(core.App, *core.RequestInfo) string { return code }
	RegisterSource(src)
}

// With no line in the period every account row is a fill row, and the Total
// still adds the rows shown: it is a row of the statement, in the source's
// currency, as the account rows are. A source with no currency of its own
// names none for the Total, while each account row shows its months' code:
// the Total would add three currencies as one, so it keeps no cells (R3).
func TestARowsTotalAddsTheRollupsWhenNoBaseRowPasses(t *testing.T) {
	noLine := func(having string) string {
		spec := rowsRollupSpec(strings.ReplaceAll(rowsRollupFilled, "HAVING", having))
		return strings.Replace(spec, `"2026-07-01"`, `"2026-08-01"`, 1)
	}
	t.Run("the source's currency", func(t *testing.T) {
		z := rowsRollupZoo(t)
		withLedgerCurrency(t, "USD")
		res := z.run(t, "ann", noLine(rowsRollupHaving), Options{}, nil)
		requireRowsRollupLines(t, rowsRollupLines(res), []rowsRollupLine{
			{KindGroup, "1000", int64(500), nil, int64(500), "USD"},
			{KindGroup, "1300", int64(300), nil, int64(300), "USD"},
			{KindTotal, "Total", int64(800), int64(0), int64(800), "USD"},
		})
	})
	t.Run("two currencies", func(t *testing.T) {
		z := rowsRollupZoo(t)
		withLedgerCurrency(t, "USD")
		res := z.run(t, "ann", noLine(""), Options{}, nil)
		total := res.Rows[len(res.Rows)-1]
		if _, has := total.Cells["debit"]; total.Kind != KindTotal || !has || total.Cells["debit"] != nil || total.Cells["debit@currency"] != nil {
			t.Fatalf("Total = %s %#v, want a blank debit cell with no code", total.Kind, total.Cells)
		}
		if !namesMixed(res, "debit") {
			t.Fatalf("notices = %+v, want mixedCurrency naming debit", res.Notices)
		}
	})
	t.Run("no source currency", func(t *testing.T) {
		res := rowsRollupZoo(t).run(t, "ann", noLine(""), Options{}, nil)
		total := res.Rows[len(res.Rows)-1]
		if total.Kind != KindTotal || len(total.Cells) != 0 {
			t.Fatalf("Total = %s %#v, want no cells", total.Kind, total.Cells)
		}
	})
	// A fill that matches no record shows no row; the Total adds none.
	t.Run("no fill record", func(t *testing.T) {
		z := rowsRollupZoo(t)
		withLedgerCurrency(t, "USD")
		spec := strings.Replace(noLine(""), `"fill":{}`, `"fill":{"where":{"op":"eq","args":[{"path":[{"field":"number"}]},{"value":"none"}]}}`, 1)
		res := z.run(t, "ann", spec, Options{}, nil)
		requireRowsRollupLines(t, rowsRollupLines(res), []rowsRollupLine{{KindTotal, "Total", int64(0), int64(0), int64(0), "USD"}})
	})
	// A report with no currency key has its one total already.
	t.Run("no currency key", func(t *testing.T) {
		const spec = `{"version":4,"source":"zoo.feedings","columns":[
			{"id":"animal","path":[{"link":"animal"}]},
			{"id":"grams","path":[{"field":"grams"}],"total":"rows"}],
			"filter":{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"no such note"}]},
			"groups":[{"id":"g","col":"animal","fill":{}}]}`
		res := newZoo(t).run(t, "ann", spec, Options{}, nil)
		var totals []Row
		for _, r := range res.Rows {
			if r.Kind == KindTotal {
				totals = append(totals, r)
			}
		}
		if len(totals) != 1 || totals[0].ID != "total" || totals[0].Cells["grams"] != int64(0) {
			t.Fatalf("totals = %#v, want one, id total, grams 0", totals)
		}
	})
	// A sum column adds no rows of the level below, so its Total over no rows
	// stays as it was: no cells.
	t.Run("a sum column keeps its total", func(t *testing.T) {
		z := newZoo(t)
		withEmptyAnimals(t, z)
		withFeedingsCurrency(t)
		none := `{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"none"}]}`
		res := z.run(t, "ann", animalFill(`{}`, none), Options{}, nil)
		total := res.Rows[len(res.Rows)-1]
		if total.ID != `total:["GBP"]` || len(total.Cells) != 0 {
			t.Fatalf("Total = %s %#v, want total:[\"GBP\"] with no cells", total.ID, total.Cells)
		}
	})
}

// Above the innermost group every row adds the rows shown under it: a type's
// row adds its accounts' rollups, the Total the types' rows. A column that
// reads a "rows" column with a rollup (gap) reads those cells on each level.
func TestARowsTotalAddsTheRollupsOfTheLevelBelow(t *testing.T) {
	z := rowsRollupZoo(t)
	res := z.run(t, "ann", rowsRollupSpec(rowsRollupByKind), Options{}, nil)
	requireRowsRollupLines(t, rowsRollupLines(res), []rowsRollupLine{
		{KindGroup, "Asset", int64(0), int64(6500), int64(-6500), "USD"},
		{KindGroup, "Expense", int64(10000), int64(0), int64(10000), "USD"},
		{KindGroup, "Income", int64(1500), int64(0), int64(1500), "USD"},
		{KindGroup, "Liability", int64(0), int64(4500), int64(-4500), "USD"},
		{KindTotal, "Total", int64(11500), int64(11000), int64(500), "USD"},
	})
}

// A total over two currencies stays blank (R3). An account whose months are
// in another currency than the lines' has a blank rollup: with no line of its
// own (bank EUR, petty GBP), and with one (bank, a USD line of July). The
// Total adds the account rows, so one blank row makes it blank, with no
// currency code, and the run names the columns.
func TestARowsTotalOverARollupOfTwoCurrenciesIsBlank(t *testing.T) {
	for name, withLine := range map[string]bool{"a month alone": false, "a month and a line": true} {
		t.Run(name, func(t *testing.T) {
			z := rowsRollupZoo(t)
			if withLine {
				z.rec(t, "j4a", "zoo_ledger_lines", map[string]any{
					"entry": z.id("J3"), "account": z.id("bank"), "date": "2026-07-05",
					"debit": 100, "credit": 0, "currency": "USD", "position": 3, "posted": true,
				})
			}
			res := z.run(t, "ann", rowsRollupSpec(strings.ReplaceAll(rowsRollupFilled, "HAVING", "")), Options{}, nil)
			requireRowsRollupLines(t, rowsRollupLines(res), []rowsRollupLine{
				{KindGroup, "1000", nil, int64(6500), int64(-6500), "USD"},
				{KindGroup, "1100", nil, nil, nil, nil},
				{KindGroup, "1200", nil, nil, nil, nil},
				{KindGroup, "1300", int64(300), nil, int64(300), "USD"},
				{KindGroup, "2000", nil, int64(4500), int64(-4500), "USD"},
				{KindGroup, "3900", nil, nil, int64(0), "USD"},
				{KindGroup, "4000", int64(1500), nil, int64(1500), "USD"},
				{KindGroup, "5000", int64(10000), nil, int64(10000), "USD"},
				{KindTotal, "Total", nil, nil, nil, nil},
			})
			if !slices.ContainsFunc(res.Notices, func(n Notice) bool {
				return n.Code == "mixedCurrency" && slices.Contains(n.Columns, "debit") && slices.Contains(n.Columns, "credit")
			}) {
				t.Fatalf("notices = %+v, want mixedCurrency naming debit and credit", res.Notices)
			}
		})
	}
}

// A rollup that is a looked-up amount alone carries its own currency on each
// row: no row is blank, and each shows its figure with its own code. The Total
// adds the account rows in the lines' currency, so a row in another one makes
// it blank, with no code, and the run names the column (R3). With every month
// in the lines' currency the Total is the sum.
func TestARowsTotalOverLookedUpAmountsOfTwoCurrenciesIsBlank(t *testing.T) {
	months := strings.ReplaceAll(monthsOf, "ACCOUNT", `{"record":"g","path":[{"field":"id"}]}`)
	spec := `{"version":4,"source":"zoo.ledger_lines","columns":[
		{"id":"account","path":[{"link":"account"}],"labelField":"number"},
		{"id":"debit","formula":{"path":[{"field":"debit"}]},"total":"rows","rollup":` + months + `}],
		"groups":[{"id":"g","col":"account","fill":{},"label":{"record":"g","path":[{"field":"number"}]}}],
		"sort":[{"formula":{"record":"g","path":[{"field":"number"}]}}]}`
	type line struct {
		Label       string
		Debit, Code any
	}
	lines := func(res *Result) []line {
		var out []line
		for _, r := range res.Rows {
			out = append(out, line{r.Label, r.Cells["debit"], r.Cells["debit@currency"]})
		}
		return out
	}
	names := func(n Notice) bool { return n.Code == "mixedCurrency" && slices.Contains(n.Columns, "debit") }

	t.Run("two currencies", func(t *testing.T) {
		res := rowsRollupZoo(t).run(t, "ann", spec, Options{}, nil)
		want := []line{
			{"1000", int64(500), "USD"}, {"1100", int64(900), "EUR"}, {"1200", int64(250), "GBP"}, {"1300", int64(300), "USD"},
			{"2000", int64(0), "USD"}, {"3900", int64(0), "USD"}, {"4000", int64(0), "USD"}, {"5000", int64(0), "USD"},
			{"Total", nil, nil},
		}
		if got := lines(res); !slices.Equal(got, want) {
			t.Fatalf("rows =\n%#v\nwant\n%#v", got, want)
		}
		if !slices.ContainsFunc(res.Notices, names) {
			t.Fatalf("notices = %+v, want mixedCurrency naming debit", res.Notices)
		}
	})
	t.Run("one currency", func(t *testing.T) {
		z := rowsRollupZoo(t)
		for _, key := range []string{"bankJune", "pettyJune"} {
			z.recs[key].Set("currency", "USD")
			if err := z.app.Save(z.recs[key]); err != nil {
				t.Fatal(err)
			}
		}
		res := z.run(t, "ann", spec, Options{}, nil)
		want := []line{
			{"1000", int64(500), "USD"}, {"1100", int64(900), "USD"}, {"1200", int64(250), "USD"}, {"1300", int64(300), "USD"},
			{"2000", int64(0), "USD"}, {"3900", int64(0), "USD"}, {"4000", int64(0), "USD"}, {"5000", int64(0), "USD"},
			{"Total", int64(1950), "USD"},
		}
		if got := lines(res); !slices.Equal(got, want) {
			t.Fatalf("rows =\n%#v\nwant\n%#v", got, want)
		}
		if slices.ContainsFunc(res.Notices, names) {
			t.Fatalf("notices = %+v, want no mixedCurrency for debit", res.Notices)
		}
	})
}

// An amount param carries its own code too: a rollup that is the param shows
// it on each group row, and the Total adds those rows in the rows' currency
// only when the param is in it (R3).
func TestARowsTotalOverAnAmountParamInAnotherCurrencyIsBlank(t *testing.T) {
	const spec = `{"version":4,"source":"zoo.feedings","params":[{"id":"budget","type":"amount","label":"Budget"}],"columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"total":"rows","rollup":{"param":"budget"}}],
		"groups":[{"id":"g","col":"species"}]}`
	type line struct {
		Kind       string
		Cost, Code any
	}
	names := func(n Notice) bool { return n.Code == "mixedCurrency" && slices.Contains(n.Columns, "cost") }
	for code, total := range map[string]line{"EUR": {KindTotal, nil, nil}, "USD": {KindTotal, int64(100), "USD"}} {
		t.Run(code, func(t *testing.T) {
			res := newZoo(t).run(t, "ann", spec, Options{}, map[string]string{"budget": `{"amount":50,"currency":"` + code + `"}`})
			var got []line
			for _, r := range res.Rows {
				got = append(got, line{r.Kind, r.Cells["cost"], r.Cells["cost@currency"]})
			}
			want := []line{{KindGroup, int64(50), code}, {KindGroup, int64(50), code}, total}
			if !slices.Equal(got, want) {
				t.Fatalf("rows =\n%#v\nwant\n%#v", got, want)
			}
			if blank := total.Cost == nil; slices.ContainsFunc(res.Notices, names) != blank {
				t.Fatalf("notices = %+v, want mixedCurrency naming cost: %v", res.Notices, blank)
			}
		})
	}
}

// paramRollupSpec is a balance list by account whose debit rollup is the
// formula ROLLUP over the account's stored months (MONTHS) and an amount
// param (budget).
func paramRollupSpec(rollup string) string {
	months := strings.ReplaceAll(monthsOf, "ACCOUNT", `{"record":"g","path":[{"field":"id"}]}`)
	return `{"version":4,"source":"zoo.ledger_lines","params":[{"id":"budget","type":"amount","label":"Budget"}],"columns":[
		{"id":"account","path":[{"link":"account"}],"labelField":"number"},
		{"id":"debit","formula":{"path":[{"field":"debit"}]},"total":"rows","rollup":` + strings.ReplaceAll(rollup, "MONTHS", months) + `}],
		"groups":[{"id":"g","col":"account","fill":{},"label":{"record":"g","path":[{"field":"number"}]}}],
		"sort":[{"formula":{"record":"g","path":[{"field":"number"}]}}]}`
}

func budgetOf(code string) map[string]string {
	return map[string]string{"budget": `{"amount":50,"currency":"` + code + `"}`}
}

func namesMixed(res *Result, col string) bool {
	return slices.ContainsFunc(res.Notices, func(n Notice) bool {
		return n.Code == "mixedCurrency" && slices.Contains(n.Columns, col)
	})
}

// A rollup that adds a USD param to a month in EUR or GBP is blank for two
// currencies on that row: it shows no code, and the Total over it is blank
// with the notice (R3). A row with no month has no second currency, so it
// keeps its figure (the param's 50).
func TestARowsTotalOverAParamAddedToAnotherCurrencyIsBlank(t *testing.T) {
	res := rowsRollupZoo(t).run(t, "ann", paramRollupSpec(`{"op":"add","args":[MONTHS,{"param":"budget"}]}`), Options{}, budgetOf("USD"))
	var got []rowsRollupLine
	for _, r := range res.Rows {
		got = append(got, rowsRollupLine{r.Kind, r.Label, r.Cells["debit"], nil, nil, r.Cells["debit@currency"]})
	}
	requireRowsRollupLines(t, got, []rowsRollupLine{
		{KindGroup, "1000", int64(550), nil, nil, "USD"},
		{KindGroup, "1100", nil, nil, nil, nil},
		{KindGroup, "1200", nil, nil, nil, nil},
		{KindGroup, "1300", int64(350), nil, nil, "USD"},
		{KindGroup, "2000", int64(50), nil, nil, "USD"},
		{KindGroup, "3900", int64(50), nil, nil, "USD"},
		{KindGroup, "4000", int64(50), nil, nil, "USD"},
		{KindGroup, "5000", int64(50), nil, nil, "USD"},
		{KindTotal, "Total", nil, nil, nil, nil},
	})
	if !namesMixed(res, "debit") {
		t.Fatalf("notices = %+v, want mixedCurrency naming debit", res.Notices)
	}
}

// Other rollups that read a month and a USD param: where the month is in
// another currency the row is blank, so the Total is blank with the notice.
func TestARowsTotalOverAParamRollupOfTwoCurrenciesIsBlank(t *testing.T) {
	for name, rollup := range map[string]string{
		"coalesce": `{"op":"coalesce","args":[{"op":"lookup","source":"zoo.ledger_months",
			"where":{"op":"eq","args":[{"path":[{"link":"account"}]},{"record":"g","path":[{"field":"id"}]}]},
			"value":{"path":[{"field":"debit_total"}]},"pick":"sum"},{"param":"budget"}]}`,
		"if": `{"op":"if","args":[{"op":"gt","args":[MONTHS,{"value":0}]},MONTHS,{"param":"budget"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			res := rowsRollupZoo(t).run(t, "ann", paramRollupSpec(rollup), Options{}, budgetOf("USD"))
			total := res.Rows[len(res.Rows)-1]
			if total.Kind != KindTotal || total.Cells["debit"] != nil || total.Cells["debit@currency"] != nil {
				t.Fatalf("Total = %s %v %v, want blank with no code", total.Kind, total.Cells["debit"], total.Cells["debit@currency"])
			}
			if !namesMixed(res, "debit") {
				t.Fatalf("notices = %+v, want mixedCurrency naming debit", res.Notices)
			}
		})
	}
}

// A detail cell that adds a param in another currency than the row's is
// blank for two currencies (no code), and so is a "rows" Total over it.
func TestARowsTotalOverADetailParamInAnotherCurrencyIsBlank(t *testing.T) {
	const spec = `{"version":4,"source":"zoo.feedings","params":[{"id":"budget","type":"amount","label":"Budget"}],"columns":[
		{"id":"cost","formula":{"op":"add","args":[{"path":[{"field":"cost"}]},{"param":"budget"}]},"total":"rows"}]}`
	res := newZoo(t).run(t, "ann", spec, Options{}, budgetOf("EUR"))
	if len(res.Rows) < 2 {
		t.Fatalf("rows = %d, want detail rows and a Total", len(res.Rows))
	}
	for _, r := range res.Rows {
		if r.Cells["cost"] != nil || r.Cells["cost@currency"] != nil {
			t.Fatalf("%s row %q = %v %v, want blank with no code", r.Kind, r.Label, r.Cells["cost"], r.Cells["cost@currency"])
		}
	}
	if !namesMixed(res, "cost") {
		t.Fatalf("notices = %+v, want mixedCurrency naming cost", res.Notices)
	}
}
