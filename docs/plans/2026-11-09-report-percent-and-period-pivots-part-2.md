# Report percent of a section and period pivots — Part 2 (Tasks 9–14)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The rule-parity fuzz test and a fictional Profit and Loss by Month statement pin the engine of Part 1; the report builder writes `percentOfSection`, `pivot.range` and `pivot.order: "spec"`; an e2e drives both on the `salesstub` package at desktop and phone width; the help and the package docs name every new part.

**Architecture:** See the outline, `docs/plans/2026-11-09-report-percent-and-period-pivots.md` (Architecture, Rulings R1–R11). Part 1 (Tasks 1–8) built the engine and the client library. This part adds no engine code: a red Go test here is an engine defect of Tasks 1–8, fixed at its source (`pivot.go`, `section_percent.go`, `layout.go`, `assemble.go`), never by a change to the expected figures. The builder keeps its R17 model: a formula or pivot field the editor cannot write stays read-only and is saved unchanged.

**Tech Stack:** Go 1.26 (PocketBase, SQLite), TypeScript (React Native + web), zod v4, vitest, Playwright.

**Spec:** `/Users/nas/tmp/tinycld/books-purchases/books/docs/specs/profit-and-loss-design.md` §4 (and §5 for the statement shape). Brief: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-profit-and-loss/cp/cp0.md`. Part 1: `docs/plans/2026-11-09-report-percent-and-period-pivots-part-1.md`.

Paths are relative to `/Users/nas/tmp/tinycld/books-purchases/tinycld` (branch `books/p5-profit-and-loss`). Go tests: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -run '<Regex>' -count=1`. Vitest, from `/Users/nas/tmp/tinycld/books-purchases/tinycld`: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run <file>`. Typecheck, from `/Users/nas/tmp/tinycld/books-purchases/tinycld/core`: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/tsc --noEmit -p .`. One e2e file, from `/Users/nas/tmp/tinycld/books-purchases/tinycld`, after the port check (`lsof -iTCP -sTCP:LISTEN -nP | grep -E ':(7210|7211)'`): `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- <name>.spec.ts`.

## Global Constraints

- No spec version bump: every new field is optional at version 4.
- Core names no package: Go tests use `zoo_*` collections; e2e use the `salesstub` stub (its shipped reports are in `tests/scripts/reports-stub/server/register.go.txt`).
- B1 (outline): a pivot's cell column needs `aggregate`; the statement writes `amount` = `{ formula: signed, aggregate: "sum" }`.
- `percentOfSection` is a column's whole formula and no other formula reads that column (R2). The builder offers it only at the top of a column formula of a report with layout sections, and offers no percent column to any other formula or to a computed row's cells.
- The Go `Label` of a bucketed label stays the raw key (R8); words come from the client.
- E2E: no raw PocketBase writes (sales go through the stub's UI with `addSale`); `page.goto` only through the existing helpers; desktop and phone (390×844).
- Comments say why, not what. No `any`, no biome-ignore, no `console.*`. Biome: 4 spaces, single quotes. Keep JSX minimal; a part that may not show takes `isVisible` or returns `null`.
- Make no commits: each task's last step hands the diff to the driver.

## Review Focus

1. A percent operand that is not a number or amount column: the builder never writes one, and a read spec whose operand is not a `{ col }` (a `{ value: 5 }`) stays read-only — Task 10 tests `takes only a number or amount column as its operand` and `edits its own shape and keeps any other read-only`, Task 11 test `keeps a percent of a section it cannot write read-only`. `editable` does not check the type of a `{ col }` operand: the engine refuses a text operand at save (Part 1 Task 7), so no saved spec holds one.
2. `partOf` change: `{ subtotal }` alone is still the layout leaf (`field`); a node with `op` and `subtotal` is the operator — Task 10 test `edits its own shape and keeps any other read-only`.
3. A pivot range left from an earlier bucket or field (the author switches the field to a link, or the bucket to Each value): the range is cleared, and a read spec with a range that names no date range param gets an issue, not a silent drop — Task 12 tests `says when a pivot range or the total's place cannot be saved` and the PivotSection clear in `onPick` / `onBucketChange`.
4. The fuzz parity of `percentOfSection` and `pivot.range` checks the engine against PocketBase's list API with integer arithmetic, so a float rounding of the test itself cannot hide or invent a defect — Task 9 `sectionPercentParity`, `periodPivotParity`.
5. The P&L-by-month statement: every month of the period is a column before the total, each month's Net income adds to the total's, and % of income on every row — Task 9 `TestAFictionalProfitAndLossByMonthRunsOnTheMergedEngine`.

---

### Task 9: Parity and the statement shape (last Go task)

**Files:**
- Modify: `core/server/reports/fuzz_test.go` (imports; `FuzzRuleParity` lines 203–207; new `sectionPercentParity`, `periodPivotParity` after `cellsParity`)
- Create: `core/server/reports/periodstatement_test.go`

**Interfaces:**
- Consumes: Part 1's `percentOfSection` (Tasks 7–8), `pivot.range` (Tasks 5–6), `pivot.order: "spec"` (Task 3), `Month` (Task 1); test helpers `newZoo`, `z.run`, `z.listItems`, `z.rec`, `z.save`, `rule`, `loadSpec`, `requireRows`, `pivotIDs`, `requirePivotIDs` (periodpivot_test.go, Task 6), `columnIDs` (pivotorder_test.go, Task 3), `fuzzWords`, `KindSubtotal`, `KindComputed`, `KindGroup`.
- Produces: `(z *zoo) sectionPercentParity(t *testing.T, mux http.Handler, user string)`; `(z *zoo) periodPivotParity(t *testing.T, mux http.Handler, rng *rand.Rand, user string)`; `withProfitZoo(t testing.TB, z *zoo)`; `TestAFictionalProfitAndLossByMonthRunsOnTheMergedEngine`.

This task has no red step: its tests read the engine of Tasks 1–8, which is complete, so they are expected to pass when written. Do not invent a failing run. A failure in Step 2 is a defect of Tasks 1–8: fix it at its source, as Step 2 says.

- [ ] **Step 1: Write the tests**

In `core/server/reports/fuzz_test.go`, add `"strconv"` and `"time"` to the imports. In `FuzzRuleParity`, after `z.cellsParity(t, mux, user)` add:

```go
		z.sectionPercentParity(t, mux, user)
		z.periodPivotParity(t, mux, rng, user)
```

After `cellsParity`, add:

```go
// sectionPercentParity: on a statement with one section per species word, each
// section's total of a percent of the hay section is that section's grams over
// the hay section's grams, as the list API lists the reader's USD feedings and
// the animals they read the species of (an animal the reader cannot list gives
// no species, so its feeding is in no section). The figure is worked out with
// integers, half away from zero, as tc_pct does; no figure when hay has none.
func (z *zoo) sectionPercentParity(t *testing.T, mux http.Handler, user string) {
	t.Helper()
	sections := make([]string, len(fuzzWords))
	for i, w := range fuzzWords {
		sections[i] = fmt.Sprintf(`{"section":"s_%s","title":"%s","key":"%s"}`, w, w, w)
	}
	res := z.run(t, user, `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"},
		{"id":"share","formula":{"op":"percentOfSection","args":[{"col":"grams"}],"subtotal":"s_hay","scale":1}}],
		"filter":{"op":"eq","args":[{"path":[{"field":"currency"}]},{"value":"USD"}]},
		"groups":[{"id":"g","col":"species"}],
		"layout":[`+strings.Join(sections, ",")+`]}`, Options{}, nil)
	species := map[string]string{}
	for _, a := range z.listItems(t, mux, "zoo_animals", user) {
		id, _ := a["id"].(string)
		s, _ := a["species"].(string)
		species[id] = s
	}
	grams := map[string]int64{}
	for _, f := range z.listItems(t, mux, "zoo_feedings", user) {
		if f["currency"] != "USD" {
			continue
		}
		animal, _ := f["animal"].(string)
		g, _ := f["grams"].(float64)
		grams[species[animal]] += int64(g)
	}
	got := map[string]any{}
	for _, r := range res.Rows {
		if r.Kind == KindSubtotal {
			got[r.Label] = r.Cells["share"]
		}
	}
	hay := grams["hay"]
	for _, w := range fuzzWords {
		share, ok := got["Total "+w]
		if !ok {
			t.Fatalf("%s: no total for section %s (every section shows): %v", user, w, got)
		}
		var want any
		if hay != 0 {
			want = float64((2*grams[w]*1000+hay)/(2*hay)) / 10
		}
		if fmt.Sprint(share) != fmt.Sprint(want) {
			t.Fatalf("%s: %s share = %v, list API %d of %d grams = %v", user, w, share, grams[w], hay, want)
		}
	}
}

// periodPivotParity: a month pivot over a random span of 2026 shows every month
// of the span, and each note's month cells and total are the sums of the grams
// of the reader's USD feedings the list API lists in that month (0 in a month
// with none).
func (z *zoo) periodPivotParity(t *testing.T, mux http.Handler, rng *rand.Rand, user string) {
	t.Helper()
	first := 1 + rng.IntN(12)
	last := first + rng.IntN(13-first)
	from := fmt.Sprintf("2026-%02d-01", first)
	to := time.Date(2026, time.Month(last+1), 0, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
	res := z.run(t, user, `{"version":4,"source":"zoo.feedings","params":[{"id":"period","type":"dateRange"}],
		"columns":[{"id":"note","path":[{"field":"note"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"filter":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"field":"currency"}]},{"value":"USD"}]},
			{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"period"}]}]},
		"groups":[{"id":"g","col":"note"}],
		"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"grams","range":"period"}}`,
		Options{}, map[string]string{"period": fmt.Sprintf(`{"from":%q,"to":%q}`, from, to)})
	months := last - first + 1
	ids := make([]string, months)
	for i := range months {
		ids[i] = fmt.Sprintf("grams.p%d=2026-%02d", i, first+i)
	}
	if got := pivotIDs(res); !slices.Equal(got, ids) {
		t.Fatalf("%s %s..%s: pivot columns = %v\nwant %v", user, from, to, got, ids)
	}
	want := map[string][]int64{}
	for _, f := range z.listItems(t, mux, "zoo_feedings", user) {
		fed, _ := f["fed"].(string)
		if f["currency"] != "USD" || len(fed) < 10 || fed[:10] < from || fed[:10] > to {
			continue
		}
		month, err := strconv.Atoi(fed[5:7])
		if err != nil {
			t.Fatalf("fed %q: %v", fed, err)
		}
		note, _ := f["note"].(string)
		if want[note] == nil {
			want[note] = make([]int64, months)
		}
		g, _ := f["grams"].(float64)
		want[note][month-first] += int64(g)
	}
	seen := 0
	for _, r := range res.Rows {
		if r.Kind != KindGroup {
			continue
		}
		seen++
		cells, ok := want[r.Label]
		if !ok {
			t.Fatalf("%s: note %q is not in the list API's feedings %v", user, r.Label, want)
		}
		var total int64
		for i, c := range cells {
			total += c
			if got := r.Cells[fmt.Sprintf("grams.p%d", i)]; !reflect.DeepEqual(got, c) {
				t.Fatalf("%s: note %s month %d = %#v, list API %d", user, r.Label, first+i, got, c)
			}
		}
		if !reflect.DeepEqual(r.Cells["grams"], total) {
			t.Fatalf("%s: note %s total = %#v, list API %d", user, r.Label, r.Cells["grams"], total)
		}
	}
	if seen != len(want) {
		t.Fatalf("%s: %d note rows, list API %d notes", user, seen, len(want))
	}
}
```

Add `"strings"` to the imports too (`sectionPercentParity` joins the sections).

Create `core/server/reports/periodstatement_test.go`:

```go
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
	computed := 0
	for _, r := range res.Rows {
		want, ok := figures[r.Label]
		if !ok {
			continue
		}
		cells := []string{"amount.p0", "amount.p1", "amount.p2", "amount"}
		for i, col := range cells {
			if got := r.Cells[col]; got != want[i] {
				t.Fatalf("%s %s = %#v, want %d", r.Label, col, got, want[i])
			}
		}
		if got := r.Cells["pct"]; got != shares[r.Label] {
			t.Fatalf("%s %% of income = %#v, want %v", r.Label, got, shares[r.Label])
		}
		// A computed row's months add to its total, Net income's included.
		if r.Kind == KindComputed {
			computed++
			if want[0]+want[1]+want[2] != want[3] {
				t.Fatalf("%s months do not add to its total", r.Label)
			}
		}
	}
	if computed != 4 {
		t.Fatalf("checked %d computed rows, want 4", computed)
	}

	// Active shows an account with a posted line in the period, at 0.
	active := map[string]string{"period": quarter["period"], "rows": `"active"`}
	all := z.run(t, "ann", raw, Options{ExpandAll: true}, active)
	var shipping *Row
	for i := range all.Rows {
		if all.Rows[i].Label == "6800 Shipping" {
			shipping = &all.Rows[i]
		}
	}
	if shipping == nil || shipping.Cells["amount"] != int64(0) {
		t.Fatalf("Active: shipping row = %+v", shipping)
	}
}
```

- [ ] **Step 2: Run the tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -run 'FuzzRuleParity|TestAFictionalProfitAndLossByMonth' -count=1`
Expected: PASS. These tests read the engine of Tasks 1–8. A failure is a defect there: find its root cause and fix it at the source (for example `rangeSeries` over a text day in `pivot.go`, `readSectionBases` in `section_percent.go`, a computed row's pivot cells in `layout.go`). Change an expected figure only when the figure contradicts the spec, and then say so in the hand-off.

- [ ] **Step 3: Run the package's Go tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -count=1 && go vet ./reports/`
Expected: PASS, no vet output.

- [ ] **Step 4: Hand the diff to the driver**

Commit message: `test(reports): parity of percentOfSection and pivot.range, and a P&L-by-month statement on zoo_*`

---

### Task 10: Builder — "Percent of a section's total" (formula library)

**Files:**
- Modify: `core/reports/lib/builder/formula-parts.ts` (`WINDOWS`, `SCALED_PARTS`, `ARITY`, `PART_LABELS`, `ReportFormulaContext`, `partOf`)
- Modify: `core/reports/lib/builder/formula-ops.ts` (`editable`, `childContext`, `partsAt`, `withPart`)
- Modify: `core/reports/lib/builder/formula-type.ts` (`formulaType` percent case)
- Modify: `core/reports/lib/builder/formula-context.ts` (`WINDOW_OPS`, `sectionPercentColumns`, `sectionsOf`, every context gets `sections`; no percent column in `values`)
- Modify: `core/reports/lib/builder/__tests__/formula-fixture.ts` (`CTX.sections`, new `WITH_SECTIONS`, `SHARE`)
- Modify: `core/reports/lib/builder/__tests__/formula-ops.test.ts`, `formula-type.test.ts`, `formula-context.test.ts`

**Interfaces:**
- Consumes: `specNodeSchema` with `subtotal` (Part 1 Task 4 keeps it); `hasScale`, `onlyKeys` (`formula.ts`); `settledNode` (`formula-ops.ts`).
- Produces: part `percentOfSection` in `WINDOWS` (so in `REPORT_PARTS`, `WINDOW_SET`) and in `SCALED_PARTS`; `ARITY.percentOfSection = [1, 1]`; `PART_LABELS.percentOfSection = "Percent of a section's total"`; `ReportFormulaContext.sections: readonly { id: string; title: string }[]` and `ReportFormulaContext.onlyColumns?: boolean`; `formulaType` gives `'percent'`; `sectionPercentColumns(columns: readonly ColumnDraft[]): ReadonlySet<string>` (formula-context.ts); `windowColumns` includes percent columns; fixtures `WITH_SECTIONS`, `SHARE`.

- [ ] **Step 1: Write the failing tests**

In `core/reports/lib/builder/__tests__/formula-fixture.ts`, add `sections: [],` to `CTX` after `lookups: true,`, and append:

```ts
/** A column of a report with two layout sections, and a text column beside the numbers. */
export const WITH_SECTIONS: ReportFormulaContext = {
    ...CTX,
    values: [...CTX.values, { key: 'team', label: 'Team', type: 'text' }],
    sections: [
        { id: 'open', title: 'Open sales' },
        { id: 'paid', title: 'Paid sales' },
    ],
}
export const SHARE: SpecNode = {
    op: 'percentOfSection',
    args: [{ col: 'amount' }],
    subtotal: 'open',
    scale: 1,
}
```

In `core/reports/lib/builder/__tests__/formula-ops.test.ts`, add `SHARE` and `WITH_SECTIONS` to the `./formula-fixture` import, and append:

```ts
describe("Percent of a section's total", () => {
    it('is offered at the top of a column formula of a report with sections only', () => {
        expect(partsAt(WITH_SECTIONS, { value: 0 }, [])).toContain('percentOfSection')
        // No section, no running values (a layout row, a label), or inside another part.
        expect(partsAt(CTX, { value: 0 }, [])).not.toContain('percentOfSection')
        const noWindows = { ...WITH_SECTIONS, windows: false }
        expect(partsAt(noWindows, { value: 0 }, [])).not.toContain('percentOfSection')
        const sum: SpecNode = { op: 'add', args: [{ value: 0 }, { value: 1 }] }
        expect(partsAt(WITH_SECTIONS, sum, [0])).not.toContain('percentOfSection')
        const onlyText = {
            ...WITH_SECTIONS,
            values: [{ key: 'team', label: 'Team', type: 'text' as const }],
        }
        expect(partsAt(onlyText, { value: 0 }, [])).not.toContain('percentOfSection')
    })
    it('writes the first section and a number column, with places', () => {
        expect(withPart(WITH_SECTIONS, { col: 'qty' }, [], 'percentOfSection')).toEqual({
            op: 'percentOfSection',
            args: [{ col: 'qty' }],
            subtotal: 'open',
            scale: 2,
        })
        // A text column is no operand: the first number or amount column is.
        expect(withPart(WITH_SECTIONS, { col: 'team' }, [], 'percentOfSection')).toEqual({
            op: 'percentOfSection',
            args: [{ col: 'amount' }],
            subtotal: 'open',
            scale: 2,
        })
        // Another part takes the operand, not the percent, and drops the section.
        expect(withPart(WITH_SECTIONS, SHARE, [], 'runningSum')).toEqual({
            op: 'runningSum',
            args: [{ col: 'amount' }],
        })
    })
    it('takes only a number or amount column as its operand', () => {
        expect(partsAt(WITH_SECTIONS, SHARE, [0])).toEqual(['field'])
        expect(contextAt(WITH_SECTIONS, SHARE, [0]).values.map(v => v.key)).toEqual([
            'amount',
            'cost',
            'qty',
        ])
    })
    it('edits its own shape and keeps any other read-only', () => {
        expect(partOf(SHARE)).toBe('percentOfSection')
        expect(partOf({ subtotal: 'open' })).toBe('field')
        expect(isEditableNode(SHARE, PARAMS)).toBe(true)
        expect(isEditableNode({ ...SHARE, args: [{ value: 5 }] }, PARAMS)).toBe(false)
        expect(isEditableNode({ ...SHARE, args: [{ col: 'amount' }, { col: 'cost' }] }, PARAMS)).toBe(
            false
        )
        expect(
            isEditableNode(
                { op: 'percentOfSection', args: [{ col: 'amount' }], subtotal: 'open' },
                PARAMS
            )
        ).toBe(false)
        expect(
            isEditableNode({ op: 'percentOfSection', args: [{ col: 'amount' }], scale: 1 }, PARAMS)
        ).toBe(false)
        expect(
            isEditableNode(
                { op: 'runningSum', args: [{ col: 'amount' }], subtotal: 'open' },
                PARAMS
            )
        ).toBe(false)
    })
    it('keeps its section when its operand or places change', () => {
        expect(REPORT_TREE.withChildren(SHARE, [{ col: 'qty' }])).toEqual({
            ...SHARE,
            args: [{ col: 'qty' }],
        })
        expect(settleScales(WITH_SECTIONS, SHARE)).toEqual(SHARE)
    })
})
```

In `core/reports/lib/builder/__tests__/formula-type.test.ts`, add `SHARE, WITH_SECTIONS` to the `./formula-fixture` import, and append:

```ts
describe('a percent of a section', () => {
    it('is a percent', () => {
        expect(formulaType(SHARE, typesOf(WITH_SECTIONS))).toBe('percent')
    })
})
```

In `core/reports/lib/builder/__tests__/formula-context.test.ts`, add `sectionPercentColumns` to the `../formula-context` import, and append:

```ts
describe('a percent of a section', () => {
    const share: ColumnDraft = {
        kind: 'formula',
        id: 'share',
        label: 'Share',
        formula: {
            kind: 'editable',
            node: { op: 'percentOfSection', args: [{ col: 'amount' }], subtotal: 'open', scale: 1 },
        },
        aggregate: '',
        scale: null,
        format: '',
        total: 'recompute',
        explicitTotal: false,
        labelField: '',
    }
    const withShare = (): BuilderDraft => {
        const d = draft()
        return { ...d, columns: [...d.columns, share] }
    }

    it("a column formula knows the layout's sections", () => {
        const ctx = formulaContext(withShare(), CATALOG, { kind: 'column', selfKey: 'share' })
        expect(ctx.sections).toEqual([{ id: 'open', title: 'Open' }])
        expect(formulaContext(withShare(), CATALOG, { kind: 'layout', index: 1 }).sections).toEqual([
            { id: 'open', title: 'Open' },
        ])
    })

    // Its cells are filled after the statements run (R2): another formula would read none.
    it('is read by no other formula, and is a percent column no total adds', () => {
        const d = withShare()
        expect(sectionPercentColumns(d.columns)).toEqual(new Set(['share']))
        for (const place of [
            { kind: 'detail' as const },
            { kind: 'column' as const, selfKey: 'team' },
        ]) {
            expect(formulaContext(d, CATALOG, place).values.map(v => v.key)).not.toContain('share')
        }
        expect(windowColumns(d.columns).has('share')).toBe(true)
        expect(rowsTotalColumns(columnChoices(d, CATALOG), d.columns).has('share')).toBe(false)
    })
})
```

- [ ] **Step 2: Run the tests and see them fail**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/builder/__tests__/formula-ops.test.ts core/reports/lib/builder/__tests__/formula-type.test.ts core/reports/lib/builder/__tests__/formula-context.test.ts`
Expected: FAIL — `percentOfSection` is not a part (`partOf(SHARE)` is `'field'`), and `sectionPercentColumns` is not exported.

- [ ] **Step 3: Implement**

In `core/reports/lib/builder/formula-parts.ts`:

```ts
const WINDOWS = ['runningSum', 'percentOfParent', 'percentOfTotal', 'percentOfSection'] as const
```

Add `'percentOfSection',` to `SCALED_PARTS` after `'percentOfTotal',`; add `percentOfSection: [1, 1],` to `ARITY` after `percentOfTotal: [1, 1],`; add `percentOfSection: "Percent of a section's total",` to `PART_LABELS` after `percentOfTotal: 'Percent of total',`.

In `ReportFormulaContext`, after `lookups: boolean`, add:

```ts
    /** The layout's sections: what a Percent of a section's total divides by (layout.go). */
    sections: readonly { id: string; title: string }[]
    /** Inside a Percent of a section's total (set by contextAt): only a number or amount column. */
    onlyColumns?: boolean
```

Replace the `partOf` line `if (node.col !== undefined || node.subtotal !== undefined) return 'field'` with:

```ts
    // A layout row reads a section as { subtotal }; on an operator, subtotal names
    // the section a Percent of a section's total divides by.
    if (node.col !== undefined || (node.subtotal !== undefined && node.op === undefined)) {
        return 'field'
    }
```

In `withPart` (`core/reports/lib/builder/formula-ops.ts`), a percent of a section changed to another part gives that part its operand, not itself: the engine refuses a window inside a window. Replace the `carried` line and the `seed` line with:

```ts
    // A percent of a section is a whole formula: its operand moves into the new part.
    const from =
        partOf(current) === 'percentOfSection' ? (current.args?.[0] ?? { value: null }) : current
    const carried = isCarried(here, from) ? from : { value: null }
```

```ts
    const seed = max === 1 ? [isCarried(here, from) ? from : filler] : seedOf(here, from)
```

In `core/reports/lib/builder/formula-ops.ts`, after `const NUMBER_TYPES …` add:

```ts
// formula.go percentOfSection: the operand is a number or an amount column.
const isSectionOperand = (value: Pick<FormulaValue, 'type'>) =>
    value.type === 'number' || value.type === 'money'
```

and add `type FormulaValue,` to the `./formula-parts` import. Move the `NUMBER_TYPES` and `isSectionOperand` constants above `editable` if the linter asks for use after definition (both are `const`, used only inside functions, so either place compiles).

In `editable`, add a case before `case 'lookup':`:

```ts
        case 'percentOfSection': {
            const [arg] = node.args ?? []
            return (
                onlyKeys(node, ['op', 'args', 'subtotal', 'scale']) &&
                typeof node.subtotal === 'string' &&
                node.args?.length === 1 &&
                hasScale(node) &&
                arg !== undefined &&
                arg.col !== undefined &&
                onlyKeys(arg, ['col'])
            )
        }
```

In `childContext`, before `if (WINDOW_SET.has(part)) return { ...here, windows: false }`:

```ts
    if (part === 'percentOfSection') {
        const values = here.values.filter(isSectionOperand)
        return { ...here, values, totals: false, windows: false, onlyColumns: true }
    }
```

In `partsAt`, after `const slot = slotAt(root, path)`:

```ts
    if (here.onlyColumns) return here.values.length > 0 ? ['field'] : []
```

and in its filter, before `if (WINDOW_SET.has(part)) return here.windows`:

```ts
        // A column's whole formula (bind.go sectionPercent), over a section of the layout.
        if (part === 'percentOfSection') {
            return (
                path.length === 0 &&
                here.windows &&
                here.sections.length > 0 &&
                here.values.some(isSectionOperand)
            )
        }
```

In `withPart`, in the `switch (part)` that holds `case 'if':`, add:

```ts
        case 'percentOfSection': {
            const section = here.sections[0]
            const own = current.col !== undefined ? here.values.find(v => v.key === current.col) : undefined
            const operand = own && isSectionOperand(own) ? own : here.values.find(isSectionOperand)
            if (!section || !operand) return current
            return settledNode(here, {
                op: 'percentOfSection',
                args: [{ col: operand.key }],
                subtotal: section.id,
            })
        }
```

In `core/reports/lib/builder/formula-type.ts`, add `case 'percentOfSection':` after `case 'percentOfTotal':` (before `return 'percent'`).

In `core/reports/lib/builder/formula-context.ts`:

```ts
const WINDOW_OPS: ReadonlySet<string> = new Set([
    'runningSum',
    'percentOfParent',
    'percentOfTotal',
    'percentOfSection',
])
```

After `windowColumns`, add:

```ts
/**
 * The columns that are a Percent of a section's total. The engine fills their cells
 * after the statements run (R1), so no other formula and no computed row cell may
 * read or write them (bind.go sectionPercents).
 */
export function sectionPercentColumns(columns: readonly ColumnDraft[]): ReadonlySet<string> {
    return new Set(
        columns
            .filter(c => c.kind === 'formula' && c.formula.node.op === 'percentOfSection')
            .map(c => c.id)
    )
}

// The layout's sections, which a Percent of a section's total divides by.
function sectionsOf(draft: Pick<BuilderDraft, 'layout'>) {
    return draft.layout.flatMap(item =>
        item.kind === 'section' ? [{ id: item.id, title: item.title }] : []
    )
}
```

Add `sections: sectionsOf(draft),` after `lookups: …` in each of the three returned contexts (`layoutContext`, the heading context and the last one in `formulaContext`). In `formulaContext`, replace the `values` lines with:

```ts
    // Child rows have no single value on the row to read, and a percent of a
    // section has none until the statements have run.
    const percents = sectionPercentColumns(draft.columns)
    const values = columnChoices(draft, catalog)
        .filter(
            choice =>
                choice.key !== selfKey &&
                !choice.isNested &&
                !percents.has(choice.key) &&
                offered(choice.key)
        )
        .map(choice => ({ key: choice.key, label: choice.label, type: choice.type }))
```

- [ ] **Step 4: Run the tests and the typecheck, and see them pass**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/builder/__tests__/`
Expected: PASS (every builder test, including the existing ones that build `CTX`).
Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core && /Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/tsc --noEmit -p .`
Expected: no error. A context literal the compiler names without `sections` gets `sections: []` (or its draft's sections) at that place.

- [ ] **Step 5: Hand the diff to the driver**

Commit message: `feat(reports): builder part Percent of a section's total`

---

### Task 11: Builder — the section picker, the description and the draft round trip

**Files:**
- Modify: `core/reports/lib/builder/formula-describe.ts` (`percentOfSection` line)
- Modify: `core/reports/components/builder/report-formula.tsx` (`SectionLeaf`; `ReportFormulaLeaf`)
- Modify: `core/reports/lib/builder/draft.ts` (`cellColumnIds` leaves percent columns out)
- Modify: `core/reports/lib/builder/__tests__/formula-describe.test.ts`, `draft.test.ts`

**Interfaces:**
- Consumes: Task 10's `percentOfSection` part, `ReportFormulaContext.sections`, `sectionPercentColumns`, `SHARE`, `WITH_SECTIONS`; `PickMenu` / `PickItem` (`@tinycld/core/ui/pick-menu`).
- Produces: `describeNode` writes "Amount as % of Open sales" (`"<subtotal> (missing)"` for an unknown section); `SectionLeaf` with test id `<part testID>-section` sets `subtotal`; `cellColumnIds` never offers a percent-of-section column; a spec `percentOfSection` column round-trips through the draft (editable in its own shape, fixed in any other).

- [ ] **Step 1: Write the failing tests**

In `core/reports/lib/builder/__tests__/formula-describe.test.ts`, add `SHARE, WITH_SECTIONS` to the `./formula-fixture` import, and append:

```ts
describe('a percent of a section', () => {
    it('names the column and the section', () => {
        expect(describeNode(WITH_SECTIONS, SHARE)).toBe('Amount as % of Open sales')
        expect(describeNode(WITH_SECTIONS, { ...SHARE, subtotal: 'gone' })).toBe(
            'Amount as % of gone (missing)'
        )
    })
})
```

In `core/reports/lib/builder/__tests__/draft.test.ts`, append:

```ts
describe('a percent of a section in the draft', () => {
    const SHARE_META = { title: 'Shares', description: '' }
    const shareSpec = (share: SpecNode): ReportSpec => ({
        version: CURRENT_SPEC_VERSION,
        source: SALES_KEY,
        columns: [
            { id: 'status', label: 'Status', path: [{ field: 'status' }] },
            { id: 'amount', label: 'Amount', path: [{ field: 'amount' }], aggregate: 'sum' },
            { id: 'share', label: 'Share', formula: share },
        ],
        groups: [{ id: 'byStatus', col: 'status', subtotals: true }],
        layout: [
            { section: 'open', title: 'Open', key: 'open' },
            { section: 'paid', title: 'Paid', key: 'paid' },
            {
                row: 'gap',
                label: 'Open less paid',
                formula: { op: 'sub', args: [{ subtotal: 'open' }, { subtotal: 'paid' }] },
            },
        ],
    })
    const SHARE_NODE: SpecNode = {
        op: 'percentOfSection',
        args: [{ col: 'amount' }],
        subtotal: 'open',
        scale: 1,
    }

    it('edits a percent of a section and saves it as it was read', () => {
        const spec = shareSpec(SHARE_NODE)
        const draft = draftFromSpec(spec, SHARE_META, CATALOG)
        expect(draft.columns[2]).toMatchObject({ kind: 'formula', formula: { kind: 'editable' } })
        expect(specFromDraft(draft, CATALOG)).toEqual(spec)
    })

    it('keeps a percent of a section it cannot write read-only', () => {
        const spec = shareSpec({ ...SHARE_NODE, args: [{ value: 5 }] })
        const draft = draftFromSpec(spec, SHARE_META, CATALOG)
        expect(draft.columns[2]).toMatchObject({ kind: 'formula', formula: { kind: 'fixed' } })
        expect(specFromDraft(draft, CATALOG)).toEqual(spec)
    })

    // Its cells are filled after the statements run, so a computed row cannot set one.
    it('offers no computed row cell on a percent of a section', () => {
        const draft = draftFromSpec(shareSpec(SHARE_NODE), SHARE_META, CATALOG)
        const types = new Map([
            ['status', 'select' as const],
            ['amount', 'money' as const],
            ['share', 'percent' as const],
        ])
        expect(cellColumnIds(draft, types)).toEqual(['amount'])
    })
})
```

- [ ] **Step 2: Run the tests and see them fail**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/builder/__tests__/formula-describe.test.ts core/reports/lib/builder/__tests__/draft.test.ts`
Expected: FAIL — the description is "a part the editor cannot show", and `cellColumnIds` gives `['amount', 'share']`. (The two round-trip tests may already pass after Task 10; they pin the behavior.)

- [ ] **Step 3: Implement**

In `core/reports/lib/builder/formula-describe.ts`, in `describeNode`'s switch, after the `percentOfParent` / `percentOfTotal` case add:

```ts
            case 'percentOfSection': {
                const section =
                    here.sections.find(s => s.id === n.subtotal)?.title ?? `${n.subtotal} (missing)`
                return `${sub(args[0])} as % of ${section}`
            }
```

In `core/reports/lib/builder/draft.ts`, add `sectionPercentColumns` to the `./formula-context` import, and replace `cellColumnIds`'s body with:

```ts
    const percents = sectionPercentColumns(draft.columns)
    return draft.columns.slice(1).flatMap(column => {
        const type = types.get(column.id)
        const isNumber = type !== undefined && isNumericType(type)
        const isPivotCell = draft.pivot.on && draft.pivot.cell === column.id
        return isNumber && !isPivotCell && !percents.has(column.id) ? [column.id] : []
    })
```

and add to its doc comment: "A Percent of a section's total is filled after the statements run, so no row sets it."

In `core/reports/components/builder/report-formula.tsx`, after `BoundLeaf`, add:

```tsx
// The section whose total a Percent of a section's total divides by (layout.go):
// it sits at the top of a column formula, so the column's context holds the sections.
function SectionLeaf({ ctx, expr, set, id, where }: LeafProps) {
    if (partOf(expr) !== 'percentOfSection') return null
    const items: PickItem[] = ctx.sections.map(section => ({
        id: section.id,
        label: section.title,
        isSelected: section.id === expr.subtotal,
        onSelect: () => set({ ...expr, subtotal: section.id }),
    }))
    const current = ctx.sections.find(section => section.id === expr.subtotal)
    const label = current ? current.title : `${expr.subtotal} (missing)`
    return (
        <PickMenu label={label} purpose={`Section${where}`} items={items} testID={`${id}-section`} />
    )
}
```

and add `<SectionLeaf {...props} />` to `ReportFormulaLeaf` after `<BoundLeaf {...props} />`.

- [ ] **Step 4: Run the tests, the typecheck and biome, and see them pass**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/builder/__tests__/`
Expected: PASS.
Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core && /Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/tsc --noEmit -p .`
Expected: no error.
Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && /Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/biome check core/reports/lib/builder core/reports/components/builder/report-formula.tsx`
Expected: no error.

- [ ] **Step 5: Hand the diff to the driver**

Commit message: `feat(reports): pick the section of a percent of a section in the builder`

---

### Task 12: Builder — pivot range and order

**Files:**
- Create: `core/reports/lib/builder/pivot.ts`
- Modify: `core/reports/lib/builder/draft.ts` (pivot schema lines 245–255; `groupingIssues` pivot block lines 348–358; `NO_PIVOT` line 843; `draftFromSpec` pivot lines 922–931; `specPivot` lines 1262–1273)
- Modify: `core/reports/components/builder/PivotSection.tsx`
- Modify: `core/reports/components/builder/parts.tsx` (`BucketSelectProps.onValueChange` takes the value)
- Modify: `core/reports/lib/builder/__tests__/draft.test.ts`
- Create: `core/reports/lib/builder/__tests__/pivot.test.ts`

**Interfaces:**
- Consumes: Part 1 Task 4's spec pivot fields `range?: string`, `order?: 'spec'` (`lib/spec.ts`); `describePath`, `Catalog` (`columns.ts`); `BuilderDraft`.
- Produces: draft `pivot.range: string` ('' = none), `pivot.totalAfter: boolean`; `specPivot` writes `range` and `order: 'spec'`; `draftFromSpec` reads them; `PivotRangeChoice = { value: string; label: string }`; `pivotRangeChoices(draft: Pick<BuilderDraft, 'source' | 'params' | 'pivot'>, catalog: Catalog): PivotRangeChoice[]` (empty unless the pivot is on, over a date, with a bucket, and the report has a `dateRange` param, or a range left from an earlier choice is set, which gives `''` and `"<range> (missing)"`; else first `{ value: '', label: 'Only the months with rows' }`, then one "Show every month of Period" per date range param); draft issues at `pivot.range` and `pivot.totalAfter`; test ids `report-builder-pivot-range`, `report-builder-pivot-total-after`.

- [ ] **Step 1: Write the failing tests**

Create `core/reports/lib/builder/__tests__/pivot.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { CURRENT_SPEC_VERSION, type ReportSpec } from '../../spec'
import { type BuilderDraft, draftFromSpec } from '../draft'
import { pivotRangeChoices } from '../pivot'
import { CATALOG, SALES_KEY } from './catalog-fixture'

const BY_MONTH: ReportSpec = {
    version: CURRENT_SPEC_VERSION,
    source: SALES_KEY,
    params: [
        { id: 'period', type: 'dateRange', label: 'Period' },
        { id: 'asOf', type: 'asOf', label: 'As of' },
    ],
    columns: [
        { id: 'customer', label: 'Customer', path: [{ field: 'customer' }] },
        { id: 'amount', label: 'Amount', path: [{ field: 'amount' }], aggregate: 'sum' },
    ],
    groups: [{ id: 'byCustomer', col: 'customer' }],
    pivot: { path: [{ field: 'day' }], bucket: 'month', cell: 'amount' },
}
const META = { title: 'By month', description: '' }
const withPivot = (draft: BuilderDraft, pivot: Partial<BuilderDraft['pivot']>): BuilderDraft => ({
    ...draft,
    pivot: { ...draft.pivot, ...pivot },
})

describe('pivotRangeChoices', () => {
    it('offers every bucket of each date range param on a bucketed date pivot', () => {
        const draft = draftFromSpec(BY_MONTH, META, CATALOG)
        expect(pivotRangeChoices(draft, CATALOG)).toEqual([
            { value: '', label: 'Only the months with rows' },
            { value: 'period', label: 'Show every month of Period' },
        ])
        expect(pivotRangeChoices(withPivot(draft, { bucket: 'fiscalYear' }), CATALOG)).toEqual([
            { value: '', label: 'Only the fiscal years with rows' },
            { value: 'period', label: 'Show every fiscal year of Period' },
        ])
    })

    it('offers none without a bucket, off a date, without a date range param or a pivot', () => {
        const draft = draftFromSpec(BY_MONTH, META, CATALOG)
        expect(pivotRangeChoices(withPivot(draft, { bucket: '' }), CATALOG)).toEqual([])
        const byTeam = withPivot(draft, { path: [{ link: 'team' }], bucket: '' })
        expect(pivotRangeChoices(byTeam, CATALOG)).toEqual([])
        const noRange = { ...draft, params: draft.params.filter(p => p.type !== 'dateRange') }
        expect(pivotRangeChoices(noRange, CATALOG)).toEqual([])
        expect(pivotRangeChoices(withPivot(draft, { on: false }), CATALOG)).toEqual([])
    })

    // The draft issue at pivot.range needs a control that can clear it.
    it('keeps a range whose date range param is gone on screen, to clear it', () => {
        const draft = withPivot(draftFromSpec(BY_MONTH, META, CATALOG), { range: 'period' })
        const noRange = { ...draft, params: draft.params.filter(p => p.type !== 'dateRange') }
        expect(pivotRangeChoices(noRange, CATALOG)).toEqual([
            { value: '', label: 'Only the columns with rows' },
            { value: 'period', label: 'period (missing)' },
        ])
    })
})
```

In `core/reports/lib/builder/__tests__/draft.test.ts`, append:

```ts
describe('a pivot over a whole period', () => {
    const PERIOD_META = { title: 'Sales', description: '' }
    const RANGED: ReportSpec = {
        ...RICH,
        pivot: {
            path: [{ field: 'day' }],
            bucket: 'month',
            cell: 'amount',
            maxColumns: 12,
            range: 'period',
            order: 'spec',
        },
    }
    const issueAt = (draft: BuilderDraft, path: string) =>
        draftIssues(draft, CATALOG).find(issue => issue.path.join('.') === path)?.message

    it("keeps a pivot's range and its total after the pivot columns", () => {
        const draft = draftFromSpec(RANGED, PERIOD_META, CATALOG)
        expect(draft.pivot).toMatchObject({ range: 'period', totalAfter: true })
        expect(specFromDraft(draft, CATALOG)).toEqual(RANGED)
        const plain = draftFromSpec(RICH, PERIOD_META, CATALOG)
        expect(plain.pivot).toMatchObject({ range: '', totalAfter: false })
    })

    it('says when a pivot range or the total\'s place cannot be saved', () => {
        const draft = draftFromSpec(RANGED, PERIOD_META, CATALOG)
        expect(issueAt(draft, 'pivot.range')).toBeUndefined()
        expect(issueAt(draft, 'pivot.totalAfter')).toBeUndefined()
        const unknown = { ...draft, pivot: { ...draft.pivot, range: 'gone' } }
        expect(issueAt(unknown, 'pivot.range')).toBe('Choose a date range')
        const noBucket = { ...draft, pivot: { ...draft.pivot, bucket: '' as const } }
        expect(issueAt(noBucket, 'pivot.range')).toBe(
            'Choose By: day, month, quarter, year or fiscal year'
        )
        // The first column labels the rows, so the pivot columns cannot come before it.
        const first = { ...draft, pivot: { ...draft.pivot, cell: 'status' } }
        expect(issueAt(first, 'pivot.totalAfter')).toBe(
            'The first column labels the rows: choose another column for the cells'
        )
    })
})
```

- [ ] **Step 2: Run the tests and see them fail**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/builder/__tests__/pivot.test.ts core/reports/lib/builder/__tests__/draft.test.ts`
Expected: FAIL — `../pivot` does not exist; the draft has no `range` / `totalAfter`.

- [ ] **Step 3: Implement**

Create `core/reports/lib/builder/pivot.ts`:

```ts
import type { Bucket } from '../spec'
import { type Catalog, describePath } from './columns'
import type { BuilderDraft } from './draft'

// "Show every <bucket> of <date range param>" (spec §4 P2): a pivot over a date with
// a bucket may have a column for every bucket a date range param's from..to touches
// (bind.go pivotRange refuses it anywhere else).

export interface PivotRangeChoice {
    value: string
    label: string
}

const BUCKET_WORDS: Record<Bucket, { one: string; many: string }> = {
    day: { one: 'day', many: 'days' },
    month: { one: 'month', many: 'months' },
    quarter: { one: 'quarter', many: 'quarters' },
    year: { one: 'year', many: 'years' },
    fiscalYear: { one: 'fiscal year', many: 'fiscal years' },
}

/**
 * The pivot's range choices: '' (the buckets with rows) and each date range param.
 * Where a range cannot apply, none; but a range left from an earlier choice (its
 * param deleted) keeps the select on screen, so the author can clear the issue.
 */
export function pivotRangeChoices(
    draft: Pick<BuilderDraft, 'source' | 'params' | 'pivot'>,
    catalog: Catalog
): PivotRangeChoice[] {
    const choices = usableChoices(draft, catalog)
    if (choices.length > 0 || !draft.pivot.on || draft.pivot.range === '') return choices
    return [
        { value: '', label: 'Only the columns with rows' },
        { value: draft.pivot.range, label: `${draft.pivot.range} (missing)` },
    ]
}

function usableChoices(
    draft: Pick<BuilderDraft, 'source' | 'params' | 'pivot'>,
    catalog: Catalog
): PivotRangeChoice[] {
    const { pivot } = draft
    if (!pivot.on || pivot.bucket === '') return []
    const type = describePath(catalog, draft.source, pivot.path)?.field.type
    if (type !== 'date' && type !== 'datetime') return []
    const ranges = draft.params.filter(param => param.type === 'dateRange')
    if (ranges.length === 0) return []
    const words = BUCKET_WORDS[pivot.bucket]
    return [
        { value: '', label: `Only the ${words.many} with rows` },
        ...ranges.map(param => ({
            value: param.id,
            label: `Show every ${words.one} of ${param.label ?? param.id}`,
        })),
    ]
}
```

If `lib/spec.ts` has no exported `Bucket` type, use `type Bucket = (typeof BUCKETS)[number]` with `BUCKETS` imported from `../spec` (it is the tuple `draft.ts` builds `BUCKET_CHOICES` from).

In `core/reports/lib/builder/draft.ts`, in the pivot schema after `labelField: z.string(),` add:

```ts
        /** A date range param whose every bucket is a column (spec pivot.range); '' : only the buckets with rows. */
        range: z.string(),
        /** The pivot columns come just before the cell column, which follows as their total (pivot.order "spec"). */
        totalAfter: z.boolean(),
```

In `NO_PIVOT`, add `range: '',` and `totalAfter: false,`. In `draftFromSpec`'s pivot, after `labelField: …` add:

```ts
              range: spec.pivot.range ?? '',
              totalAfter: spec.pivot.order === 'spec',
```

In `specPivot`, after the `labelField` spread add:

```ts
        ...(pivot.range !== '' ? { range: pivot.range } : {}),
        ...(pivot.totalAfter ? { order: 'spec' as const } : {}),
```

In `groupingIssues`, inside `if (draft.pivot.on) { … }` after the cell check add:

```ts
        // bind.go pivotRange and pivot.order refusals.
        if (draft.pivot.range !== '') {
            const range = draft.params.find(param => param.id === draft.pivot.range)
            if (range?.type !== 'dateRange') {
                issues.push({ path: ['pivot', 'range'], message: 'Choose a date range' })
            } else if (draft.pivot.bucket === '') {
                issues.push({
                    path: ['pivot', 'range'],
                    message: 'Choose By: day, month, quarter, year or fiscal year',
                })
            }
        }
        if (draft.pivot.totalAfter && draft.columns[0]?.id === draft.pivot.cell) {
            issues.push({
                path: ['pivot', 'totalAfter'],
                message: 'The first column labels the rows: choose another column for the cells',
            })
        }
```

In `core/reports/components/builder/parts.tsx`, change `BucketSelectProps.onValueChange` to `onValueChange?: (value: string) => void`.

In `core/reports/components/builder/PivotSection.tsx`:
- Import `type Control` with `NumberInput, SelectInput, Toggle, useWatch` from `@tinycld/core/ui/form`, `type BuilderDraft` from `../../lib/builder/draft`, and `pivotRangeChoices, type PivotRangeChoice` from `../../lib/builder/pivot`.
- In `usePivotFields`, after `const pivot = useWatch(…)` add `const params = useWatch({ control: form.control, name: 'params' })`; after `cells` add `const rangeChoices = pivotRangeChoices({ source: base.key, params, pivot }, catalog)`.
- In `onPick`, replace `if (!isDatePath(path)) form.setValue('pivot.bucket', '', { shouldDirty: true })` with:

```ts
        // A range belongs to a bucketed date; one left from it would be refused.
        if (!isDatePath(path)) {
            form.setValue('pivot.bucket', '', { shouldDirty: true })
            form.setValue('pivot.range', '', { shouldDirty: true })
        }
```

- Add, after `setLabelField`:

```ts
    const onBucketChange = (bucket: string) => {
        if (bucket === '') form.setValue('pivot.range', '', { shouldDirty: true })
    }
```

and return `rangeChoices` and `onBucketChange` too.
- Add above `PivotFields`:

```tsx
interface RangeSelectProps {
    choices: PivotRangeChoice[]
    control: Control<BuilderDraft>
}

// Every bucket of a date range, or only those with rows; shown where it applies, or
// where a range left from an earlier choice must be cleared.
function RangeSelect({ choices, control }: RangeSelectProps) {
    if (choices.length === 0) return null
    return (
        <SelectInput
            name="pivot.range"
            control={control}
            label="Columns"
            options={choices}
            testID="report-builder-pivot-range"
        />
    )
}
```

- In `PivotFields`, read `rangeChoices` and `onBucketChange` from `usePivotFields(builder)`; pass `onValueChange={onBucketChange}` to `BucketSelect`; put `<RangeSelect choices={rangeChoices} control={control} />` after `BucketSelect`; after the `pivot.cell` `SelectInput` add:

```tsx
            <Toggle
                name="pivot.totalAfter"
                control={control}
                label="Total after the pivot columns"
                testID="report-builder-pivot-total-after"
                onValueChange={builder.validateLists}
            />
```

- Update the `PivotSection` comment: "… It needs a group level. It may show every bucket of a period, and its total after its columns."

- [ ] **Step 4: Run the tests, the typecheck and biome, and see them pass**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/builder/__tests__/`
Expected: PASS.
Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core && /Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/tsc --noEmit -p .`
Expected: no error. A test or component that builds a whole `BuilderDraft['pivot']` literal without a spread gets `range: ''` and `totalAfter: false`.
Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && /Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/biome check core/reports/lib/builder core/reports/components/builder`
Expected: no error.

- [ ] **Step 5: Hand the diff to the driver**

Commit message: `feat(reports): builder pivot range and total after the pivot columns`

---

### Task 13: E2E — period pivot and percent of a section on the stub

**Files:**
- Modify: `tests/scripts/reports-stub/server/register.go.txt` (two shipped reports before `return nil`; the package comment counts five shipped reports)
- Create: `tests/e2e/reports-period-pivot.spec.ts`
- Modify: `tests/e2e/reports-helpers.ts` (header comment, lines 16–22: the stub's shipped reports)

**Interfaces:**
- Consumes: the engine (Part 1) and the client words (Part 1 Task 2); e2e helpers `createInvitedUser`, `skipWithoutReportsStub`, `waitForTableFit` (`./helpers`), `addSale`, `addTeam`, `cellOf`, `hubRow`, `openReportStub`, `openReportsHub`, `rowOf`, `uniqueName` (`./reports-helpers`); test ids `report-param-period`, `report-period-preset-custom`, `report-period-from`, `report-period-to`, `report-period-popover`, `report-heading-<col>`, `report-cell-<col>-<row>`, `reports-hub-tab-published`, `report-table`.
- Produces: shipped published reports `salesstub.period_statement` ("Stub period statement") and `salesstub.period_pivot` ("Stub period pivot"), revision 1; `test.describe('reports: period pivot', …)` with a desktop statement test, a desktop pivot test and a phone pivot test.

- [ ] **Step 1: Add the shipped reports**

In `tests/scripts/reports-stub/server/register.go.txt`, before `return nil` in `Register`, add:

```go
	// A published statement with a percent of a section (P1): sections by a hidden
	// status column, each customer under its section, and a computed row; every
	// row's share divides its own amount by the Open sales total
	// (tests/e2e/reports-period-pivot.spec.ts).
	reports.RegisterShipped(reports.Shipped{
		Key: "salesstub.period_statement", Title: "Stub period statement", Revision: 1,
		Description: "Each customer's sales by status, as a share of the open sales.",
		Spec: json.RawMessage(`{
			"version": 4,
			"source": "salesstub.sales",
			"columns": [
				{"id": "customer", "label": "Customer", "path": [{"field": "customer"}]},
				{"id": "amount", "label": "Amount", "path": [{"field": "amount"}], "aggregate": "sum"},
				{"id": "share", "label": "% of open", "formula": {"op": "percentOfSection", "args": [{"col": "amount"}], "subtotal": "open", "scale": 1}},
				{"id": "side", "hidden": true, "path": [{"field": "status"}]}
			],
			"groups": [
				{"id": "section", "col": "side", "open": true, "subtotals": true},
				{"id": "byCustomer", "col": "customer"}
			],
			"layout": [
				{"section": "open", "title": "Open sales", "key": "open"},
				{"section": "paid", "title": "Paid sales", "key": "paid"},
				{"row": "all", "label": "All sales", "formula": {"op": "add", "args": [{"subtotal": "open"}, {"subtotal": "paid"}]}}
			],
			"sort": [{"col": "customer"}]
		}`),
	})
	// A published month pivot over every month of the period (P2), with each
	// customer's total after the months (P3) and the months in words (P4)
	// (tests/e2e/reports-period-pivot.spec.ts).
	reports.RegisterShipped(reports.Shipped{
		Key: "salesstub.period_pivot", Title: "Stub period pivot", Revision: 1,
		Description: "Each customer's sales in every month of a period.",
		Spec: json.RawMessage(`{
			"version": 4,
			"source": "salesstub.sales",
			"params": [{"id": "period", "type": "dateRange", "label": "Period", "default": {"preset": "thisFiscalYear"}}],
			"columns": [
				{"id": "customer", "label": "Customer", "path": [{"field": "customer"}]},
				{"id": "amount", "label": "Total", "path": [{"field": "amount"}], "aggregate": "sum"}
			],
			"filter": {"op": "between", "args": [{"path": [{"field": "day"}]}, {"param": "period"}]},
			"groups": [{"id": "byCustomer", "col": "customer"}],
			"pivot": {"path": [{"field": "day"}], "bucket": "month", "cell": "amount", "range": "period", "order": "spec"}
		}`),
	})
```

In the package comment at the top, replace "and three shipped reports (one published, two not)" with "and shipped reports (published and not)".

In `tests/e2e/reports-helpers.ts`, the header comment names only two of the stub's shipped reports. Replace its last three lines:

```ts
// sales), both headed "Stub Sales Co", and the shipped reports "Stub larger side"
// (published) and "Stub team totals" (unpublished) over sales
// (reports-stub/server/register.go.txt).
```

with:

```ts
// sales), both headed "Stub Sales Co", and the shipped reports over sales
// (reports-stub/server/register.go.txt): published "Stub larger side", "Stub
// statement", "Stub period statement" and "Stub period pivot"; unpublished "Stub
// team totals" and "Stub team balance".
```

- [ ] **Step 2: Write the e2e spec**

Create `tests/e2e/reports-period-pivot.spec.ts`:

```ts
import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub, waitForTableFit } from './helpers'
import {
    addSale,
    addTeam,
    cellOf,
    hubRow,
    openReportStub,
    openReportsHub,
    rowOf,
    uniqueName,
} from './reports-helpers'

/**
 * A percent of a section (P1) on the shipped "Stub period statement", and a month
 * pivot over every month of a period (P2) with its total after the months (P3) and
 * the months in words (P4) on the shipped "Stub period pivot"
 * (tests/scripts/reports-stub/server/register.go.txt). A fresh invited member owns
 * every sale it reads, so the figures are exact.
 */

const STATEMENT = 'Stub period statement'
const PIVOT = 'Stub period pivot'
const MONTHS = ['Jan 2026', 'Feb 2026', 'Mar 2026']

interface Sale {
    customer: string
    amount: number
    day: string
    status?: 'open' | 'paid'
}

async function seed(page: Page, label: string, sales: readonly Sale[]) {
    const invited = await createInvitedUser(page, label)
    const p = invited.inviteePage
    const team = uniqueName('Team')
    await openReportStub(p)
    await addTeam(p, team, 'north')
    for (const sale of sales) await addSale(p, { team, cost: 0, ...sale })
    return { p, close: invited.close }
}

async function openShipped(p: Page, title: string) {
    await openReportsHub(p)
    await p.getByTestId('reports-hub-tab-published').click()
    await hubRow(p, title).click()
}

// The first quarter of 2026 as a Custom period; February has no sale. The
// default period is this fiscal year, which holds no 2026 sale from 2027 on,
// so the wait is for the period control, not for a row.
async function openFirstQuarter(p: Page) {
    await openShipped(p, PIVOT)
    await expect(p.getByTestId('report-param-period')).toBeVisible()
    await p.getByTestId('report-param-period').click()
    await p.getByTestId('report-period-preset-custom').click()
    await p.getByTestId('report-period-from').fill('2026-01-01')
    await p.getByTestId('report-period-to').fill('2026-03-31')
    await expect.poll(() => p.url()).toContain('p-period=2026-01-01..2026-03-31')
    await p.keyboard.press('Escape')
    await expect(p.getByTestId('report-period-popover')).toHaveCount(0)
    await expect(p.getByTestId('report-heading-amount.p3')).toHaveCount(0)
}

const QUARTER_SALES: readonly Sale[] = [
    { customer: 'Ann', amount: 1000, day: '2026-01-10' },
    { customer: 'Bo', amount: 2500, day: '2026-03-10' },
]

async function expectMonthHeadings(p: Page) {
    for (const [i, month] of MONTHS.entries()) {
        await expect(p.getByTestId(`report-heading-amount.p${i}`)).toHaveText(month)
    }
}

test.describe('reports: period pivot', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a percent of a section divides every row by the open sales total', async ({ page }) => {
        const { p, close } = await seed(page, 'rptshare', [
            { customer: 'Ann', amount: 1000, day: '2026-01-10' },
            { customer: 'Bo', amount: 2000, day: '2026-01-11' },
            { customer: 'Cy', amount: 1500, day: '2026-01-12', status: 'paid' },
        ])
        try {
            await openShipped(p, STATEMENT)
            await expect(rowOf(p, 'All sales')).toBeVisible()
            const shares: [string, string][] = [
                ['Ann', '33.3%'],
                ['Bo', '66.7%'],
                ['Total Open sales', '100.0%'],
                ['Cy', '50.0%'],
                ['Total Paid sales', '50.0%'],
                // A computed row divides its own amount: 45.00 of 30.00.
                ['All sales', '150.0%'],
            ]
            for (const [label, share] of shares) {
                await expect(await cellOf(p, 'share', label)).toHaveText(share)
            }
            await expect(p.getByTestId('report-heading-side')).toHaveCount(0)
        } finally {
            await close()
        }
    })

    test('a month pivot shows every month of the period, in words, with the total after', async ({
        page,
    }) => {
        const { p, close } = await seed(page, 'rptpd', QUARTER_SALES)
        try {
            await openFirstQuarter(p)
            await expectMonthHeadings(p)
            // February has no sale: a sum reads 0 there.
            await expect(await cellOf(p, 'amount.p1', 'Ann')).toHaveText('0.00')
            await expect(await cellOf(p, 'amount.p1', 'Bo')).toHaveText('0.00')
            await expect(await cellOf(p, 'amount.p0', 'Ann')).toHaveText('10.00')
            await expect(await cellOf(p, 'amount.p2', 'Bo')).toHaveText('25.00')
            await expect(await cellOf(p, 'amount', 'Bo')).toHaveText('25.00')
            // The total column follows the months (pivot.order "spec").
            const march = await p.getByTestId('report-heading-amount.p2').boundingBox()
            const total = await p.getByTestId('report-heading-amount').boundingBox()
            expect(march).not.toBeNull()
            expect(total).not.toBeNull()
            expect(total?.x ?? 0).toBeGreaterThan(march?.x ?? 0)
            expect(await waitForTableFit(p)).toBe('one-line')
        } finally {
            await close()
        }
    })

    test('the month pivot fits a phone with the months in words', async ({ page }) => {
        const { p, close } = await seed(page, 'rptpdphone', QUARTER_SALES)
        try {
            await openFirstQuarter(p)
            await p.setViewportSize({ width: 390, height: 844 })
            // The report has no authored layouts, so core fits one line or
            // its automatic lines (table-lines.ts fitTable).
            const layout = await waitForTableFit(p)
            expect(['one-line', 'auto']).toContain(layout)
            // The FlashList scroller inside the table (ReportTable.tsx) clips its
            // rows sideways, so the table's own scrollWidth cannot see a row wider
            // than the list: measure the table and every scroller in it.
            const overflow = await p.getByTestId('report-table').evaluate(table => {
                const scrollers = [table, ...table.querySelectorAll<HTMLElement>('*')].filter(
                    el => el === table || ['auto', 'scroll'].includes(getComputedStyle(el).overflowY)
                )
                return Math.max(...scrollers.map(el => el.scrollWidth - el.clientWidth))
            })
            expect(overflow).toBeLessThanOrEqual(0)
            expect(await p.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
            await expectMonthHeadings(p)
            await expect(await cellOf(p, 'amount.p1', 'Ann')).toHaveText('0.00')
        } finally {
            await close()
        }
    })
})
```

- [ ] **Step 3: Run the spec**

Run the port check: `lsof -iTCP -sTCP:LISTEN -nP | grep -E ':(7210|7211)'` (kill a leftover server there only if it was started from `/Users/nas/tmp/tinycld/books-purchases`). Then, from `/Users/nas/tmp/tinycld/books-purchases/tinycld`: `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-period-pivot.spec.ts`
Expected: 3 passed. When the run takes more than about 4 minutes, hand back `NEEDS-RUN: E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-period-pivot.spec.ts` to the driver. A failure that points at the server: search `tmp/e2e-serve-7210.log` for the error (never read it whole). Fix the root cause; no timeout bump, no serial mode, no retry.

- [ ] **Step 4: Run biome on the files**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && /Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/biome check tests/e2e/reports-period-pivot.spec.ts tests/e2e/reports-helpers.ts`
Expected: no error.

- [ ] **Step 5: Hand the diff to the driver**

Commit message: `test(reports): e2e period pivot and percent of a section on the stub`

---

### Task 14: Help, docs and the docs check

**Files:**
- Modify: `core/help/reports.md` (the **Group and sort** and **Pivot** bullets, lines 31–35; two new sections after "Statements: sections and computed rows"; the **Formulas** section)
- Modify: `docs/packages.md` ("Fields added in version 4" table, after the `layout[].cells` row)
- Modify: `core/reports/lib/__tests__/statements-docs.test.ts`

**Interfaces:**
- Consumes: the names of Tasks 10–12 ("Percent of a section's total", the section menu that shows the section's title (Task 11 `SectionLeaf`, no visible label), **Columns**, "Show every month of …", "Total after the pivot columns") and the spec fields of Part 1.
- Produces: help headings "Show a row as a percent of a section", "Pivot by month across a whole period"; packages.md rows `columns[].formula` (`percentOfSection`), `pivot.range`, `pivot.order`, `columns[].pivot.labelType`.

- [ ] **Step 1: Write the failing test**

In `core/reports/lib/__tests__/statements-docs.test.ts`, add `'Show a row as a percent of a section',` and `'Pivot by month across a whole period',` to the help headings list; add `'columns[].formula',`, `'pivot.range',`, `'pivot.order',` and `'columns[].pivot.labelType',` to the fields list; and add these tests inside the `describe`:

```ts
    it('names the pivot controls and the bucket words in the help', () => {
        const help = read('core/help/reports.md')
        expect(help).toContain('**Total after the pivot columns**')
        expect(help).toContain("**Percent of a section's total**")
        for (const words of ['"Jan 2026"', '"Q1 2026"', '"FY 2026"']) {
            expect(help).toContain(words)
        }
    })

    it('names the percent of a section operator in the version 4 table', () => {
        const docs = read('docs/packages.md')
        const start = docs.indexOf('**Fields added in version 4.**')
        const table = docs.slice(start, docs.indexOf('\n\n', docs.indexOf('| Field |', start)))
        expect(table).toContain('`percentOfSection`')
    })
```

- [ ] **Step 2: Run the test and see it fail**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/statements-docs.test.ts`
Expected: FAIL — the headings and fields are missing.

- [ ] **Step 3: Write the help and the docs**

In `core/help/reports.md`, replace the **Group and sort** bullet:

```md
   - **Group and sort**: add group levels, each with or without subtotals, and the
     order of the rows.
```

with:

```md
   - **Group and sort**: add group levels, each with or without subtotals, and the
     order of the rows. A level over a date can group by day, month, quarter, year
     or fiscal year; its group rows are named in words, such as "Jan 2026",
     "Q1 2026", "2026" or "FY 2026".
```

and the **Pivot** bullet:

```md
   - **Pivot**: spread a column with a total across one column per value, such as
     one column per month, or one column per linked record, such as one column per
     team. Under **Show as**, choose the record's field that names its columns.
```

with:

```md
   - **Pivot**: spread a column with a total across one column per value, such as
     one column per month, or one column per linked record, such as one column per
     team. Under **Show as**, choose the record's field that names its columns. A
     column of a month is headed in words, such as "Jan 2026". Under **Columns**,
     choose **Show every month of …** to show every month of a period, and turn on
     **Total after the pivot columns** to show the total after them.
```

After the "Statements: sections and computed rows" section (before "## Hide groups with nothing to show"), add:

```md
## Show a row as a percent of a section

In a statement, a column can show each row as a percent of one section's total, such as each expense as a percent of income. To add one, select **Add formula**, choose **Percent of a section's total**, choose the column to divide, then open the menu that follows it, which shows a section's name, and choose the section to divide by. Each row shows its own figure divided by that section's total, times 100: the rows of each section, the group and section totals, and the computed rows. The section's own total shows 100.0%. When the section's total is 0 or empty, the column is empty. When the report has more than one currency, each currency divides by its own section's total. On a pivot report the percent divides the row's total across the pivot columns. This part is offered only as a column's whole formula in a report with sections, and no other formula can read that column.

## Pivot by month across a whole period

A pivot over a date shows one column for each day, month, quarter, year or fiscal year that has rows. To show every month of a period, also a month with no rows, open **Pivot**, choose the date field and **By** **Month**, and under **Columns** choose **Show every month of …** with the date range the report asks for. A month with no rows then shows 0.00 in a sum. When the period has more months than **At most this many columns**, the report shows the first ones and tells you how many it left out. When the period is **All dates**, the report shows only the months that have rows.

Column headings name each bucket in words: a month as "Jan 2026", a quarter as "Q1 2026", a year as "2026", a fiscal year as "FY 2026" (the year in which it ends), and a day in the short day form, such as "Jan 5, 2026".

The pivot columns usually come after all the other columns. Turn on **Total after the pivot columns** to show them just before the column with the total, so that each row's total follows its months.
```

In the **Formulas** section, replace "totals such as **Sum of** and running values such as **Running sum**;" with "totals such as **Sum of** and running values such as **Running sum** and, in a statement, **Percent of a section's total**;".

In `docs/packages.md`, after the `layout[].cells` row of the "Fields added in version 4" table, add these rows (one line each):

```md
| `columns[].formula` `percentOfSection` | `{ op: "percentOfSection", args: [{ col }], subtotal, scale }`, in a layout report: the row's value of a number or amount column ÷ the total of that column in the layout section `subtotal` names × 100, at `scale` places, half away from zero. Type percent. Every detail, group, subtotal, section and computed row divides its own cell; a section's total is read with its `within` rows, also when the section is folded. On a pivot report the argument is the row's total across the pivot. Each currency divides by its own section's total; a total of 0 or blank gives no figure; an opening row stays blank. It is a column's whole formula and no other formula reads that column. Refused outside a layout report, when `subtotal` names no section (422 `unknown_subtotal`), when the argument is not a `{ col }` of a number or amount column, inside any other operator, on a column with `aggregate`, and on a pivot's cell column; `subtotal` on any other operator is refused. |
| `pivot.range` | The id of a `dateRange` param. When the param resolves to both bounds, the pivot has one column for every bucket its `from`..`to` touches, in order, whether or not a row falls in it (a sum reads 0 there; min, max and average stay blank), at most `maxColumns`, then the `pivotCap` notice with the count left out in `leftOut`. A row outside the range counts only in the cell column. On All dates, or with no `range`, the columns are the buckets that have rows. Refused when the param is unknown (422 `unknown_param`) or not a `dateRange`, on a pivot with no `bucket`, and on a pivot not over a date. |
| `pivot.order` | `"spec"`: the result keeps the spec's column order and puts the pivot columns just before their cell column, which follows them as the row's total. Absent: the pivot columns come after every column. Refused for any other value and when the cell column is the spec's first column. |
| `columns[].pivot.labelType` | On a bucketed pivot column (and as `labelType` on a bucketed group row and its subtotal): `month`, `quarter`, `year`, `fiscalYear` or `day`, with the raw key in `labelValue` (`"2026-01"`, `"2026-Q1"`, `"2026"`, the fiscal year's end year as a number, `"2026-01-05"`). `label`, `pivot.key` and the group key stay the raw key; the view writes it in words in the reader's locale ("Jan 2026", "Q1 2026", "2026", "FY 2026", the short day). These types are never a field's type. |
```

- [ ] **Step 4: Run the test and see it pass**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/statements-docs.test.ts`
Expected: PASS.
Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && pnpm run packages:generate` only if no e2e run is in progress (the help topic is a core topic; the generator picks up the changed body).
Expected: exits 0.

- [ ] **Step 5: Hand the diff to the driver**

Commit message: `docs(reports): help and package docs for percent of a section and period pivots`
