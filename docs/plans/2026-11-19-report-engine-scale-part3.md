# Report Engine Query Scaling Implementation Plan — part 3 (tasks 3b, 4a, 4a-ts and 4b)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A General Ledger run with an Accounts filter takes 1 s or less on a book with 500,000 journal lines, with every report's result unchanged.

**Architecture:** This part makes the General Ledger's beginning balance read the stored monthly balances (`books_account_balances`) and not the account's whole history of journal lines (requirement 2, the decision of part 1). The beginning balance becomes: the stored months before the period's first month, plus the journal lines of that month before the period. That sum adds amounts of two report sources, which core refuses today, so task 3b first lets a source's currency field declare that it holds the same currency as another source's currency field. Task 4a gives each stored month a day (`month_start`) that a report can compare with the period; task 4b changes the report.

**Tech Stack:** Go, SQLite (PocketBase fork), core report engine `tinycld.org/core/reports`, PocketBase JS migrations, TypeScript fixtures (vitest).

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/from-p2-write-off-report-sql.md` (the root cause). The unit's five requirements, the baseline and the stored-summary decision: part 1, `/Users/nas/tmp/tinycld/books-purchases/tinycld/docs/plans/2026-11-19-report-engine-scale.md`. Tasks 2 and 3: part 2, `/Users/nas/tmp/tinycld/books-purchases/tinycld/docs/plans/2026-11-19-report-engine-scale-part2.md`. Outline of all tasks: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md` (its last section gives the task list this part follows). The General Ledger's design: `/Users/nas/tmp/tinycld/books-purchases/books/docs/specs/general-ledger-design.md` §5.

**Repos:** workspace `/Users/nas/tmp/tinycld/books-purchases`. Core is `tinycld/` (branch `books/p5-report-engine-scale`); Books is `books/` (same branch name). Task 3b is a core task: it may edit `tinycld/core` and `tinycld/docs`. Tasks 4a, 4a-ts and 4b are Books tasks: they edit nothing under `tinycld/`.

**Order:** tasks 1, 2 and 3 (parts 1 and 2) are done before this part: `books/server/report_general_ledger_scale_test.go` exists, and core has `bounds.go` and the `startOfMonth` operator. Then 3b → 4a → 4a-ts → 4b. Tasks 5, 6 and 7 are the next part (section "What is left" at the end).

## Global Constraints

- The bar is never lowered: runs 3, 4 and 5 of `TestGLScale` (the runs with an Accounts filter) take 1 s or less at 500,000 lines, median of 5. No task changes a bar, removes a run, or changes the scale book to pass.
- Every report's rows stay the same. Task 4b proves it by a cross-check: the shipped spec against the same spec with the old beginning balance, compared as the JSON of `res.Rows` (figures and currency codes), not figure by figure.
- **Task 3b extends core money rule R24** (`tinycld/docs/plans/2026-11-06-report-engine-v3.md`, R24: "Amounts read from one currency field combine; a run-time guard blanks a row where their codes differ"). Before: one currency field is one source's field. After: two sources' currency fields are one field when one of them declares it (`Field.SameCurrencyAs`). Nothing else of R24 changes: the per-row guard (`fieldGuard`), the blank cell, the `mixedCurrency` notice, and the refusal for two fields with no declaration all stay. The driver accepted this change under the human's standing rule that an engine gap is fixed in core (2026-11-01); the hand-back of this unit names it so the human sees it.
- Core names no package: core tests use the fictional zoo only (`zoo.ledger_lines`, `zoo.ledger_months`); no Books slug, collection or route in `tinycld/core`.
- Books has no release: its migrations are edited in place (`books/pb-migrations/2100000002_create_books_ledger.js`). No backfill, no upgrade path.
- Each change to SQL is recorded with its plan and its time before and after in `books/docs/plans/general-ledger-ledger.md`, section "Performance (p5-report-engine-scale)", under "Changes".
- The 500k run takes about 13 minutes and the 100k run more than 4 minutes with its build: an implementer starts neither. Each is a `NEEDS-RUN` for the driver.
- The whole Books server package takes about 224 s (the planner's run). When it does not end in 4 minutes, stop it and hand it back as a `NEEDS-RUN`; do not raise a timeout.
- After task 4a and a type generation, the Books TypeScript typecheck is red until task 4a-ts. The review of task 4a runs the Go checks only; the TypeScript checks are those of task 4a-ts.
- Books Go tests read the migrations from disk. Server logging goes through the package's `log`. Go code is formatted with `gofmt`. TypeScript: 4-space indent, single quotes, no semicolons (biome).

## Review Focus

1. A section with no line in the period's first month (retained earnings, a quiet account): its Beginning balance comes from the stored months only and must still carry its currency code. Test: task 4b, `TestGeneralLedgerBeginningBalanceFromStoredMonthsEqualsTheLines` (it compares `res.Rows` as JSON, so a lost `balance@currency` fails it; a figure-only check does not see it).
2. Two currency codes on one row (a stored month in EUR, a line in USD): the amount is blank with no code and the run names the column in a `mixedCurrency` notice; it is never a sum of the two. Tests: task 3b, `TestAmountsOfTwoSourcesThatShareACurrencyCombine` (bank) and `TestABaseRowsAmountCombinesWithASharedCurrencyLookup`. Both test detail rows. No core test has two codes inside a group rollup; the rollup form (a group's sum of a base amount plus a shared-currency lookup) is run with one currency only, by task 4b's cross-check. Look at the rollup path in the review of task 3b.
3. A period that starts on the first day of a month, on the first day of the fiscal year, before every line, and All dates: no month is counted twice and none is left out. Test: task 4b, the seven periods of the same cross-check.
4. A stored month whose `month_start` is blank or wrong (a write past the hooks): verify reports it as a drift and the rebuild repairs it; the report must not read such a row silently. Tests: task 4a, `TestBalances_FirstDayFollowsThePeriod`; task 4b, `requireGLOpeningsFromTheLines` in `TestGLScaleBook` (the report's beginning balance against a direct sum of the lines).
5. A declaration that names nothing (an unregistered source, a field that is not a currency field, a field that declares another): the source is refused at the start check with the reason, and the other sources stay usable. Test: task 3b, `TestCheckSourcesRefusesASameCurrencyThatNamesNoCurrencyField`.

---

### Task 3b: Two sources share one currency (core; extends money rule R24)

Core combines two amounts (`add`, `sub`, `min`, `max`, `if`, `case`, `coalesce`) only when their currencies come from one currency field. The name of that field, `<source key>.<currency field key>`, is made in one place, `currencyField` (`rows.go`). `combined` (`formula.go`) refuses two names; `fieldGuard` (`formula.go`) then checks, for two reads of one name, the codes on each row: two known codes that differ give a blank and the `mixedCurrency` notice, and the amount's currency is the first known code.

A stored summary of a source's rows (monthly totals of ledger lines) is a second source with its own currency field, so its amounts never add to the rows' amounts. This task adds `Field.SameCurrencyAs`: a currency field names another source's currency field, and `currencyField` returns that name for both. Nothing in `combined` or `fieldGuard` changes: the two sources' amounts are then two reads of one field, and the guard checks them on each row as before.

**Files:**
- Modify: `tinycld/core/server/reports/registry.go` (imports; `Field`, after `Currency`; `validateField`)
- Modify: `tinycld/core/server/reports/check.go` (`CheckSources` loop; new `checkSameCurrency`)
- Modify: `tinycld/core/server/reports/rows.go` (`currencyField`, about line 488)
- Create: `tinycld/core/server/reports/sharedcurrency_test.go`
- Modify: `tinycld/docs/packages.md` (two list items, about lines 1332 and 1438)

**Interfaces:**
- Consumes (existing, core `server/reports` tests): `newZoo`, `(z *zoo) rec/save/id/run`, `withBalanceZoo(t testing.TB, z *zoo)` and the const `glNet` (balance_shape_test.go), `ResetForTesting`, `lookupSource`, `pathPlan`.
- Produces (for task 4b): `reports.Field.SameCurrencyAs string` — on a Text field that a Money field of the same source names as its `Currency`: `"<source key>.<field key>"` of another source's currency field. Amounts of the two sources then combine. Test helpers: `withLedgerMonths(t testing.TB, z *zoo, sameAs string)`, `ledgerMonthsSource(sameAs string) Source`, the consts `monthsOf`, `linesOf`.

- [ ] **Step 1: Write the failing tests**

Create `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/sharedcurrency_test.go`:

```go
package reports

import (
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// zoo_ledger_months is a stored summary of zoo_ledger_lines: one row for an
// account and a month, with that month's debit and credit totals and their
// currency. Its source's currency field holds the same currency as the
// lines', which it declares with SameCurrencyAs (sameAs "" declares nothing).
//
//	cash   2026-06  debit 500  USD   (its lines are USD)
//	bank   2026-06  debit 900  EUR   (its line is USD: two codes on one row)
//	petty  2026-06  debit 250  GBP   (it has no line)
//
// loan, sales and rent have lines and no month; idle and re have neither.
func withLedgerMonths(t testing.TB, z *zoo, sameAs string) {
	t.Helper()
	accounts, err := z.app.FindCollectionByNameOrId("zoo_ledger_accounts")
	if err != nil {
		t.Fatal(err)
	}
	months := core.NewBaseCollection("zoo_ledger_months")
	months.ListRule, months.ViewRule = accounts.ListRule, accounts.ViewRule
	months.Fields.Add(
		&core.RelationField{Name: "account", CollectionId: accounts.Id, MaxSelect: 1},
		&core.TextField{Name: "first_day"},
		&core.NumberField{Name: "debit_total"},
		&core.NumberField{Name: "credit_total"},
		&core.TextField{Name: "currency"},
	)
	z.save(t, months)
	for _, m := range []struct {
		key, account, currency string
		debit                  int
	}{
		{"cashJune", "cash", "USD", 500},
		{"bankJune", "bank", "EUR", 900},
		{"pettyJune", "petty", "GBP", 250},
	} {
		z.rec(t, m.key, "zoo_ledger_months", map[string]any{
			"account": z.id(m.account), "first_day": "2026-06-01", "debit_total": m.debit, "credit_total": 0, "currency": m.currency,
		})
	}
	RegisterSource(ledgerMonthsSource(sameAs))
}

func ledgerMonthsSource(sameAs string) Source {
	return Source{
		Key: "zoo.ledger_months", Collection: "zoo_ledger_months", Label: "Ledger months",
		Fields: []Field{
			{Key: "first_day", Label: "First day", Type: Date, TextDay: true, Aggregates: MinMax},
			{Key: "debit_total", Label: "Debit total", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "credit_total", Label: "Credit total", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "currency", Label: "Currency", Type: Text, SameCurrencyAs: sameAs},
		},
		Links: []Link{{Key: "account", Label: "Account", Target: "zoo.ledger_accounts", Cardinality: ToOne, Field: "account"}},
	}
}

// monthsOf and linesOf are the stored months' net and the posted lines' net
// of the account whose id is ACCOUNT, 0 where it has none.
const (
	monthsOf = `{"op":"coalesce","args":[{"op":"lookup","source":"zoo.ledger_months",
		"where":{"op":"eq","args":[{"path":[{"link":"account"}]},ACCOUNT]},
		"value":{"op":"sub","args":[{"path":[{"field":"debit_total"}]},{"path":[{"field":"credit_total"}]}]},"pick":"sum"},{"value":0}]}`
	linesOf = `{"op":"coalesce","args":[{"op":"lookup","source":"zoo.ledger_lines",
		"where":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"link":"account"}]},ACCOUNT]},
			{"op":"eq","args":[{"path":[{"field":"posted"}]},{"value":true}]}]},
		"value":` + glNet + `,"pick":"sum"},{"value":0}]}`
)

// monthsAndLines is a report with one row per ledger account: its number and
// the stored months' net added to the lines' net.
func monthsAndLines() string {
	account := strings.NewReplacer("ACCOUNT", `{"outer":[{"field":"id"}]}`)
	return `{"version":4,"source":"zoo.ledger_accounts","columns":[
		{"id":"number","path":[{"field":"number"}]},
		{"id":"net","formula":{"op":"add","args":[` + account.Replace(monthsOf) + `,` + account.Replace(linesOf) + `]}}],
		"sort":[{"col":"number"}]}`
}

// Two sources' amounts do not combine when neither says its currency is the
// other's: R24 holds for one currency field only.
func TestAmountsOfTwoSourcesDoNotCombineWithNoDeclaration(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	withLedgerMonths(t, z, "")
	spec, _, err := Load([]byte(monthsAndLines()))
	if err != nil {
		t.Fatal(err)
	}
	err = Validate(z.app, spec)
	if err == nil || !strings.Contains(err.Error(), "amounts in different currencies cannot be combined") {
		t.Fatalf("Validate = %v, want the refusal to combine two currencies", err)
	}
}

// With the declaration the months' and the lines' amounts add up. On each row
// the codes are checked: one code on both sides gives the sum in it, a side
// with no row leaves the other side's amount and code, two codes give a blank
// with no code and the run's mixedCurrency notice, and no row on either side
// gives 0 with no code.
func TestAmountsOfTwoSourcesThatShareACurrencyCombine(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	withLedgerMonths(t, z, "zoo.ledger_lines.currency")
	if err := CheckSources(z.app); err != nil {
		t.Fatalf("CheckSources: %v", err)
	}
	res := z.run(t, "ann", monthsAndLines(), Options{}, nil)
	type cell struct{ net, currency any }
	got := map[string]cell{}
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			got[r.Cells["number"].(string)] = cell{r.Cells["net"], r.Cells["net@currency"]}
		}
	}
	want := map[string]cell{
		"1000": {int64(-6500), "USD"}, // cash: month 500, lines -7000
		"1100": {nil, nil},            // bank: month EUR, line USD
		"1200": {int64(250), "GBP"},   // petty: a month and no line
		"1300": {int64(0), nil},       // idle: neither
		"2000": {int64(-4500), "USD"}, // loan: lines and no month
		"3900": {int64(0), nil},       // retained earnings: neither
		"4000": {int64(-500), "USD"},  // sales: lines and no month
		"5000": {int64(10000), "USD"}, // rent: lines and no month
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %v\nwant %v", got, want)
	}
	for number, w := range want {
		if got[number] != w {
			t.Errorf("%s = %#v, want %#v", number, got[number], w)
		}
	}
	if !slices.ContainsFunc(res.Notices, func(n Notice) bool { return n.Code == "mixedCurrency" && slices.Contains(n.Columns, "net") }) {
		t.Fatalf("notices = %+v, want mixedCurrency naming net", res.Notices)
	}
}

// The same holds where one amount is the base row's own and the other a
// looked-up row's of the other source: a line's debit added to its account's
// stored months. The bank line (USD) meets the bank month (EUR) and is blank.
func TestABaseRowsAmountCombinesWithASharedCurrencyLookup(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	withLedgerMonths(t, z, "zoo.ledger_lines.currency")
	months := strings.ReplaceAll(monthsOf, "ACCOUNT", `{"outer":[{"link":"account"}]}`)
	spec := `{"version":4,"source":"zoo.ledger_lines","columns":[
		{"id":"account","path":[{"link":"account"},{"field":"number"}]},
		{"id":"day","path":[{"field":"date"}]},
		{"id":"both","formula":{"op":"add","args":[{"path":[{"field":"debit"}]},` + months + `]}}],
		"filter":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"field":"posted"}]},{"value":true}]},
			{"op":"in","args":[{"col":"account"},{"value":["1000","1100","2000"]}]}]},
		"sort":[{"col":"account"},{"col":"day"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	var got [][3]any
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			got = append(got, [3]any{r.Cells["account"], r.Cells["both"], r.Cells["both@currency"]})
		}
	}
	want := [][3]any{
		{"1000", int64(500), "USD"},  // j1a: debit 0 + month 500
		{"1000", int64(3500), "USD"}, // j2a: debit 3000 + month 500
		{"1100", nil, nil},           // j0a: USD line, EUR month
		{"2000", int64(0), "USD"},    // j2b: debit 0, no month
	}
	if !slices.Equal(got, want) {
		t.Fatalf("rows = %#v\nwant %#v", got, want)
	}
}

// currencyField gives one name to a currency field and to a field that
// declares it holds the same currency; a field with no declaration keeps its
// own name, so every report that names no such field compiles as before.
func TestCurrencyFieldNamesTheDeclaredField(t *testing.T) {
	ResetForTesting()
	t.Cleanup(ResetForTesting)
	months := ledgerMonthsSource("zoo.ledger_lines.currency")
	alone := ledgerMonthsSource("")
	for name, c := range map[string]struct {
		src  *Source
		key  string
		want string
	}{
		"declared":       {&months, "debit_total", "zoo.ledger_lines.currency"},
		"not declared":   {&alone, "credit_total", "zoo.ledger_months.currency"},
		"not an amount":  {&months, "first_day", ""},
		"the code field": {&months, "currency", ""},
	} {
		if got := currencyField(&pathPlan{src: c.src, field: c.src.field(c.key)}); got != c.want {
			t.Errorf("%s: currencyField = %q, want %q", name, got, c.want)
		}
	}
}

func TestRegisterSourceRefusesAMalformedSameCurrency(t *testing.T) {
	cases := map[string]struct {
		edit func(*Source)
		want string
	}{
		"on a field no money field names": {func(s *Source) { s.Fields[0].SameCurrencyAs = "zoo.ledger_lines.currency" }, "only a currency field of a money field"},
		"no source key":                   {func(s *Source) { s.Fields[3].SameCurrencyAs = "currency" }, "<source key>.<field key>"},
		"no field key":                    {func(s *Source) { s.Fields[3].SameCurrencyAs = "zoo.ledger_lines." }, "<source key>.<field key>"},
		"its own source":                  {func(s *Source) { s.Fields[3].SameCurrencyAs = "zoo.ledger_months.currency" }, "is a field of this source"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ResetForTesting()
			t.Cleanup(ResetForTesting)
			src := ledgerMonthsSource("")
			c.edit(&src)
			defer func() {
				r := recover()
				if msg, _ := r.(string); r == nil || !strings.Contains(msg, c.want) {
					t.Fatalf("panic %v does not mention %q", r, c.want)
				}
			}()
			RegisterSource(src)
		})
	}
}

// What needs the other source is checked when every source is registered:
// CheckSources refuses the source that declares, and leaves the other usable.
func TestCheckSourcesRefusesASameCurrencyThatNamesNoCurrencyField(t *testing.T) {
	cases := map[string]struct {
		sameAs, want string
		chain        bool
	}{
		"unregistered source":   {"zoo.ledger_weeks.currency", `same-currency source "zoo.ledger_weeks" is not registered`, false},
		"unknown field":         {"zoo.ledger_lines.code", `"zoo.ledger_lines.code" is not the currency field of a money field`, false},
		"not a currency field":  {"zoo.ledger_lines.date", `"zoo.ledger_lines.date" is not the currency field of a money field`, false},
		"a field that declares": {"zoo.ledger_years.currency", `"zoo.ledger_years.currency" names "zoo.ledger_lines.currency" itself: name that field`, true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			z := newZoo(t)
			withBalanceZoo(t, z)
			withLedgerMonths(t, z, c.sameAs)
			if c.chain {
				years := ledgerMonthsSource("zoo.ledger_lines.currency")
				years.Key = "zoo.ledger_years"
				RegisterSource(years)
			}
			err := CheckSources(z.app)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("CheckSources = %v, want it to contain %q", err, c.want)
			}
			if lookupSource(z.app, "zoo.ledger_months") != nil {
				t.Fatal("the refused source is still usable")
			}
			if lookupSource(z.app, "zoo.ledger_lines") == nil {
				t.Fatal("a sound source was refused")
			}
		})
	}
}
```

- [ ] **Step 2: Run the tests to verify they do not build**

Run, in `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server`: `go test -run 'TestAmountsOfTwoSources|TestABaseRowsAmountCombines|TestCurrencyFieldNames|TestRegisterSourceRefusesAMalformedSameCurrency|TestCheckSourcesRefusesASameCurrency' ./reports/`
Expected: FAIL, the build stops with `unknown field SameCurrencyAs in struct literal of type Field`.

- [ ] **Step 3: Add the field and its registration check**

In `tinycld/core/server/reports/registry.go`, add `"strings"` to the imports (after `"sort"`).

In `type Field struct`, replace

```go
	Currency string
	// Options label a Select field's values.
```

with

```go
	Currency string
	// SameCurrencyAs, on a Text field that a Money field of this source names
	// as its Currency, names the currency field of another source,
	// "<source key>.<field key>": the two fields hold the code of one currency
	// (a stored summary of rows and the rows themselves, say). Amounts of the
	// two sources then combine as amounts of one currency field do, and the
	// engine still checks the codes on each row: where they differ the amount
	// is blank and the run says so. The named field declares no SameCurrencyAs
	// itself; CheckSources refuses a source whose declaration names no
	// registered currency field.
	SameCurrencyAs string
	// Options label a Select field's values.
```

In `validateField`, replace

```go
	// The engine never adds amounts it cannot tell apart by currency.
```

with

```go
	if f.SameCurrencyAs != "" {
		names := func(m Field) bool { return m.Type == Money && m.Currency == f.Key }
		dot := strings.LastIndex(f.SameCurrencyAs, ".")
		switch {
		case f.Type != Text || !slices.ContainsFunc(src.Fields, names):
			return fmt.Errorf("only a currency field of a money field names another source's currency field")
		case dot < 0 || !sourceKeyRe.MatchString(f.SameCurrencyAs[:dot]) || !memberKeyRe.MatchString(f.SameCurrencyAs[dot+1:]):
			return fmt.Errorf("the same-currency field %q must look like \"<source key>.<field key>\"", f.SameCurrencyAs)
		case f.SameCurrencyAs[:dot] == src.Key:
			return fmt.Errorf("the same-currency field %q is a field of this source", f.SameCurrencyAs)
		}
	}
	// The engine never adds amounts it cannot tell apart by currency.
```

- [ ] **Step 4: Check the declaration against the other source**

Sources register in any order, so what needs the other source is checked in `CheckSources`. In `tinycld/core/server/reports/check.go`, in `CheckSources`, replace

```go
		if problems := checkSource(app, sources[key]); len(problems) > 0 {
```

with

```go
		if problems := append(checkSource(app, sources[key]), checkSameCurrency(sources[key])...); len(problems) > 0 {
```

and add, before the comment `// checkSource runs under mu (read).`:

```go
// checkSameCurrency checks each SameCurrencyAs of src: it names a text field
// of a registered source that a money field there names as its currency, and
// that field declares none itself, so two fields that share a currency have
// one name (currencyField). It runs under mu (read) and reads no database.
func checkSameCurrency(src *Source) []string {
	var problems []string
	for _, f := range src.Fields {
		if f.SameCurrencyAs == "" {
			continue
		}
		dot := strings.LastIndex(f.SameCurrencyAs, ".")
		target, key := sources[f.SameCurrencyAs[:dot]], f.SameCurrencyAs[dot+1:]
		msg := ""
		switch {
		case target == nil:
			msg = fmt.Sprintf("same-currency source %q is not registered", f.SameCurrencyAs[:dot])
		case target.field(key) == nil || target.field(key).Type != Text ||
			!slices.ContainsFunc(target.Fields, func(m Field) bool { return m.Type == Money && m.Currency == key }):
			msg = fmt.Sprintf("%q is not the currency field of a money field", f.SameCurrencyAs)
		case target.field(key).SameCurrencyAs != "":
			msg = fmt.Sprintf("%q names %q itself: name that field", f.SameCurrencyAs, target.field(key).SameCurrencyAs)
		}
		if msg != "" {
			problems = append(problems, fmt.Sprintf("field %q: %s", f.Key, msg))
		}
	}
	return problems
}
```

(`validateField` has already refused a declaration with no dot, so `dot` is never negative here.)

- [ ] **Step 5: Run the tests to see the rule itself fail**

Run: `go test -run 'TestAmountsOfTwoSources|TestABaseRowsAmountCombines|TestCurrencyFieldNames|TestRegisterSourceRefusesAMalformedSameCurrency|TestCheckSourcesRefusesASameCurrency' ./reports/`
Expected: FAIL in three tests, the others pass:

```
--- FAIL: TestAmountsOfTwoSourcesThatShareACurrencyCombine
    sharedcurrency_test.go:116: Run: reports: columns.1.formula: amounts in different currencies cannot be combined
--- FAIL: TestABaseRowsAmountCombinesWithASharedCurrencyLookup
    sharedcurrency_test.go:163: Run: reports: columns.2.formula: amounts in different currencies cannot be combined
--- FAIL: TestCurrencyFieldNamesTheDeclaredField
    sharedcurrency_test.go:200: declared: currencyField = "zoo.ledger_months.currency", want "zoo.ledger_lines.currency"
```

- [ ] **Step 6: Give the two fields one name**

In `tinycld/core/server/reports/rows.go`, replace

```go
// "<source key>.<currency field key>", "" for a value that is not an amount
// or an amount with no currency field.
func currencyField(p *pathPlan) string {
	if p.field.Type != Money || p.field.Currency == "" {
		return ""
	}
	return p.src.Key + "." + p.field.Currency
```

with

```go
// "<source key>.<currency field key>", "" for a value that is not an amount
// or an amount with no currency field. A currency field that declares
// SameCurrencyAs takes the name of the field it names, so amounts of the two
// sources have one name: they combine (currency.same) and fieldGuard checks
// their codes on each row.
func currencyField(p *pathPlan) string {
	if p.field.Type != Money || p.field.Currency == "" {
		return ""
	}
	if cur := p.src.field(p.field.Currency); cur != nil && cur.SameCurrencyAs != "" {
		return cur.SameCurrencyAs
	}
	return p.src.Key + "." + p.field.Currency
```

Do not change `combined`, `currencyGuard` or `fieldGuard` (`formula.go`): `currencyField` is the only place that makes the name (`rows.go` `pathCurrency`, `join.go` `amountCurrency`, `lookup.go` `path` all call it).

- [ ] **Step 7: Run the tests to verify they pass**

Run: `gofmt -l ./reports/ && go vet ./reports/ && go test -run 'TestAmountsOfTwoSources|TestABaseRowsAmountCombines|TestCurrencyFieldNames|TestRegisterSourceRefusesAMalformedSameCurrency|TestCheckSourcesRefusesASameCurrency' ./reports/`
Expected: no file listed by `gofmt`, `ok  tinycld.org/core/reports`.

Then the whole package, once (about 100 s): `go test ./reports/`
Expected: `ok`. Every existing report compiles to the same SQL: with no `SameCurrencyAs` in the registry, `currencyField` returns what it returned before (`TestCurrencyFieldNamesTheDeclaredField`, "not declared"), and the package's golden tests (`TestABalanceShapedReportOverManyAccountsKeepsItsOutput`) stay green.

- [ ] **Step 8: Document the declaration**

In `/Users/nas/tmp/tinycld/books-purchases/tinycld/docs/packages.md`, after the list item about `TextDay` that ends with `not text.` at line 1335 (the item wraps over two lines), add:

```markdown
- `Field.SameCurrencyAs`, on the text field a money field names as its
  `Currency`, names another source's currency field as
  `"<source key>.<field key>"`: the two fields hold one currency (a stored
  summary of rows and the rows themselves). A formula may then add amounts
  of the two sources. The engine checks the codes on each row: where two
  known codes differ, the amount is blank and the run adds a `mixedCurrency`
  notice. `RegisterSource` refuses a declaration on any other field, or one
  that names its own source; `CheckSources` refuses one that names no
  registered currency field, or a field that declares another.
```

In the same file, replace the list item (about line 1438)

```markdown
- A formula that combines amounts of two currency sources is refused
  ("amounts in different currencies cannot be combined"). This applies to
  `add`, `sub`, `min`, `max`, `if`, `case` and `coalesce`. Each currency
  field is a source, and so is each source's fixed `Source.Currency`, even
  when two sources give the same code. An amount combines with amounts of
  its own source.
```

with

```markdown
- A formula that combines amounts of two currency sources is refused
  ("amounts in different currencies cannot be combined"). This applies to
  `add`, `sub`, `min`, `max`, `if`, `case` and `coalesce`. Each currency
  field is a source, and so is each source's fixed `Source.Currency`, even
  when two sources give the same code. An amount combines with amounts of
  its own source, and with amounts of a source whose currency field
  declares `SameCurrencyAs` its own.
```

- [ ] **Step 9: Commit (core repo)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/registry.go core/server/reports/check.go core/server/reports/rows.go core/server/reports/sharedcurrency_test.go docs/packages.md
git commit -m "feat(reports): two sources' currency fields can hold one currency"
```

---

### Task 4a: `books_account_balances.month_start` (Books, server)

A month row holds its month as `period` (`2026-05`). The report engine compares days, so the row also stores the first day of its month, `month_start` (`2026-05-01`), written by posting and by the rebuild. The column has the name of the report source field task 4b adds, because the Books source tests hold every field to the column of its key (`checkFields`, `report_sources_test.go`). (The outline named the column `first_day`; that name is not used.)

**Files:**
- Modify: `books/pb-migrations/2100000002_create_books_ledger.js` (the `books_account_balances` fields, about line 330)
- Modify: `books/server/balances.go` (`applyToBalances`, `verifyBalances`, `rebuildBalances`; new `firstDayOf`, `newBalanceRow`)
- Create: `books/server/balances_month_start_test.go`
- Modify: `books/server/balances_test.go:72-73`, `books/server/ledger_rules_test.go:35`, `books/server/opening_balances_test.go:305`, `books/server/posting_test.go:447,609` (six rows that save a balance row)
- Modify: `books/server/report_general_ledger_scale_test.go` (`writeGLScaleBook`: the INSERT of task 1 step 3)

**Interfaces:**
- Consumes (existing, Books tests): `newBooksApp`, `mkAccount`, `mkDraft`, `rawPost`, `ln`, `applyToBalances`, `verifyBalances`, `rebuildBalances`.
- Produces (for tasks 4a-ts and 4b): the column `books_account_balances.month_start` (text, required, `YYYY-MM-01`), set by every writer; `firstDayOf(period string) string`; `newBalanceRow(tx core.App, account, period, currency string) (*core.Record, error)`. `verifyBalances` reports a row whose `month_start` is not its month's first day as a drift, and `rebuildBalances` repairs it.

- [ ] **Step 1: Write the failing test**

Create `/Users/nas/tmp/tinycld/books-purchases/books/server/balances_month_start_test.go`:

```go
package books

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// Posting writes each month row's first day, verify finds a row whose first
// day is not its month's, and the rebuild repairs it.
func TestBalances_FirstDayFollowsThePeriod(t *testing.T) {
	app := newBooksApp(t)
	cash := mkAccount(t, app, "Checking", "bank", "checking")
	income := mkAccount(t, app, "Income", "income", "service_income")
	rawPost(t, app, mkDraft(t, app, "2026-05-19", ln{cash, 500, 0}, ln{income, 0, 500}), "posted")
	err := app.RunInTransaction(func(tx core.App) error {
		if err := applyToBalances(tx, cash.Id, "2026-05-19", 500, 0, "USD"); err != nil {
			return err
		}
		return applyToBalances(tx, income.Id, "2026-05-19", 0, 500, "USD")
	})
	if err != nil {
		t.Fatal(err)
	}
	firstDay := func() string {
		t.Helper()
		row, err := app.FindFirstRecordByFilter("books_account_balances", "account = {:a} && period = '2026-05'", map[string]any{"a": cash.Id})
		if err != nil {
			t.Fatal(err)
		}
		return row.GetString("month_start")
	}
	if got := firstDay(); got != "2026-05-01" {
		t.Fatalf("month_start = %q, want 2026-05-01", got)
	}
	if drifts, err := verifyBalances(app); err != nil || len(drifts) != 0 {
		t.Fatalf("a posted ledger drifts: %+v, %v", drifts, err)
	}
	// A write past the hooks (a bulk import) leaves the day blank.
	if _, err := app.DB().NewQuery("UPDATE books_account_balances SET month_start = '' WHERE account = {:a}").Bind(map[string]any{"a": cash.Id}).Execute(); err != nil {
		t.Fatal(err)
	}
	drifts, err := verifyBalances(app)
	if err != nil || len(drifts) != 1 || drifts[0].Account != cash.Id || drifts[0].Period != "2026-05" {
		t.Fatalf("drifts = %+v, %v; want the one row with a blank first day", drifts, err)
	}
	if fixed, err := rebuildBalances(app); err != nil || fixed != 1 {
		t.Fatalf("fixed = %d, err %v; want 1", fixed, err)
	}
	if got := firstDay(); got != "2026-05-01" {
		t.Fatalf("after the rebuild month_start = %q, want 2026-05-01", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run, in `/Users/nas/tmp/tinycld/books-purchases/books/server`: `go test -run 'TestBalances_FirstDayFollowsThePeriod$' -count=1 .`
Expected: FAIL with `month_start = "", want 2026-05-01` (the collection has no such field).

- [ ] **Step 3: Add the column**

In `/Users/nas/tmp/tinycld/books-purchases/books/pb-migrations/2100000002_create_books_ledger.js`, in the `books_account_balances` collection, replace

```js
                    { name: 'period', type: 'text', required: true, max: 7, pattern: '^\\d{4}-\\d{2}$' },
                    { name: 'debit_total', type: 'number', onlyInt: true, min: 0 },
```

with

```js
                    { name: 'period', type: 'text', required: true, max: 7, pattern: '^\\d{4}-\\d{2}$' },
                    // The first day of the month, so a report compares the row with a day.
                    { name: 'month_start', type: 'text', required: true, max: 10, pattern: '^\\d{4}-\\d{2}-01$' },
                    { name: 'debit_total', type: 'number', onlyInt: true, min: 0 },
```

(The migration is edited in place: Books has no release, and every database starts empty.)

- [ ] **Step 4: Write the day in posting, verify and rebuild**

In `/Users/nas/tmp/tinycld/books-purchases/books/server/balances.go`, in `applyToBalances`, replace

```go
	if row == nil {
		col, err := tx.FindCachedCollectionByNameOrId("books_account_balances")
		if err != nil {
			return err
		}
		row = core.NewRecord(col)
		row.Set("account", account)
		row.Set("period", period)
		row.Set("currency", currency)
	}
	debitTotal, creditTotal := row.GetInt("debit_total")+debit, row.GetInt("credit_total")+credit
```

with

```go
	if row == nil {
		if row, err = newBalanceRow(tx, account, period, currency); err != nil {
			return err
		}
	}
	debitTotal, creditTotal := row.GetInt("debit_total")+debit, row.GetInt("credit_total")+credit
```

After the closing brace of `applyToBalances`, add:

```go
// firstDayOf is the first day of a month row's month ("2026-05" gives
// "2026-05-01"). A report reads it as the row's date (the source field
// month_start), because the report engine compares days, not months.
func firstDayOf(period string) string {
	return period + "-01"
}

// newBalanceRow is the month row of account for period, with no totals yet.
// Posting and the rebuild both make a row here, so month_start always agrees
// with period.
func newBalanceRow(tx core.App, account, period, currency string) (*core.Record, error) {
	col, err := tx.FindCachedCollectionByNameOrId("books_account_balances")
	if err != nil {
		return nil, err
	}
	row := core.NewRecord(col)
	row.Set("account", account)
	row.Set("period", period)
	row.Set("month_start", firstDayOf(period))
	row.Set("currency", currency)
	return row, nil
}
```

In `verifyBalances`, replace

```go
		if row.GetInt("debit_total") != w.debit || row.GetInt("credit_total") != w.credit {
```

with

```go
		// A row whose month_start is not its month's first day is a drift too:
		// the totals agree, but a report that reads the row by its day (the
		// General Ledger's beginning balance) leaves it out or misplaces it.
		if row.GetInt("debit_total") != w.debit || row.GetInt("credit_total") != w.credit ||
			row.GetString("month_start") != firstDayOf(key.period) {
```

In `rebuildBalances`, replace

```go
			if row == nil {
				col, err := tx.FindCachedCollectionByNameOrId("books_account_balances")
				if err != nil {
					return err
				}
				row = core.NewRecord(col)
				row.Set("account", d.Account)
				row.Set("period", d.Period)
				row.Set("currency", w.currency)
			}
			row.Set("debit_total", w.debit)
```

with

```go
			if row == nil {
				if row, err = newBalanceRow(tx, d.Account, d.Period, w.currency); err != nil {
					return err
				}
			}
			row.Set("month_start", firstDayOf(d.Period))
			row.Set("debit_total", w.debit)
```

- [ ] **Step 5: Give the day to the test rows that save a balance row**

The column is required, so each of these six `saveRecord` rows fails with `month_start: cannot be blank.` until it has the day. In each, add `"month_start"` after `"period"`:

`books/server/balances_test.go` lines 72-73:

```go
	saveRecord(t, app, "books_account_balances", map[string]any{"account": cash.Id, "period": "2026-01", "month_start": "2026-01-01", "debit_total": 1, "currency": "USD"})
	saveRecord(t, app, "books_account_balances", map[string]any{"account": income.Id, "period": "2026-05", "month_start": "2026-05-01", "credit_total": 499, "currency": "USD"})
```

`books/server/ledger_rules_test.go` line 35:

```go
		"account": account.Id, "period": "2026-05", "month_start": "2026-05-01", "debit_total": 100, "currency": "USD",
```

`books/server/opening_balances_test.go` line 305:

```go
		"account": env.checking.Id, "period": "2025-12", "month_start": "2025-12-01", "debit_total": maxStoredAmount - 50, "currency": "USD",
```

`books/server/posting_test.go` line 447:

```go
		"account": env.checking.Id, "period": "2026-05", "month_start": "2026-05-01", "debit_total": maxStoredAmount - 50, "currency": "USD",
```

`books/server/posting_test.go` line 609:

```go
			"account": env.revenue.Id, "period": "2026-05", "month_start": "2026-05-01", "credit_total": maxStoredAmount - 50, "currency": "USD",
```

- [ ] **Step 6: Write the day in the scale book**

The scale harness stores its balances with a raw INSERT (the bulk writer writes none). In `/Users/nas/tmp/tinycld/books-purchases/books/server/report_general_ledger_scale_test.go`, in `writeGLScaleBook`, replace

```go
		_, err := tx.DB().NewQuery(`INSERT INTO books_account_balances (id, account, period, debit_total, credit_total, currency, created, updated)
SELECT 'b' || substr(lower(hex(randomblob(8))), 1, 14), account, substr(date, 1, 7), sum(debit), sum(credit), min(currency), '', ''
FROM books_journal_lines WHERE posted IS true GROUP BY account, substr(date, 1, 7)`).Execute()
```

with

```go
		_, err := tx.DB().NewQuery(`INSERT INTO books_account_balances (id, account, period, month_start, debit_total, credit_total, currency, created, updated)
SELECT 'b' || substr(lower(hex(randomblob(8))), 1, 14), account, substr(date, 1, 7), substr(date, 1, 7) || '-01', sum(debit), sum(credit), min(currency), '', ''
FROM books_journal_lines WHERE posted IS true GROUP BY account, substr(date, 1, 7)`).Execute()
```

`TestGLScaleBook` already requires `verifyBalances` to find no drift, and verify now counts a wrong `month_start`: a scale book with a blank day fails there.

- [ ] **Step 7: Run the tests to verify they pass**

Run: `gofmt -l . && go vet . && go test -run 'Balances|LedgerRules|OpeningBalance|Posting|TestGLScaleBook$' -count=1 .`
Expected: no file listed by `gofmt`, `ok  tinycld.org/packages/books`.

Then the whole package, once (about 4 minutes): `go test -count=1 .`
Expected: `ok`. No other test saves a balance row (the planner ran the package with these changes).

- [ ] **Step 8: Commit (Books repo)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add pb-migrations/2100000002_create_books_ledger.js server/balances.go server/balances_month_start_test.go server/balances_test.go server/ledger_rules_test.go server/opening_balances_test.go server/posting_test.go server/report_general_ledger_scale_test.go
git commit -m "feat(books): a monthly balance row stores the first day of its month"
```

---

### Task 4a-ts: The client's balance fixtures carry `month_start` (Books, TypeScript)

The schema types are generated from the migrations: after task 4a, `BooksAccountBalances` (`tinycld/core/types/pbSchema.ts`, gitignored) has a required `month_start: string`. No client code writes a balance row (the collection's create rule is null), and no screen reads the new field. Four files build a whole `BooksAccountBalances` and need the field to typecheck.

**Files:**
- Modify: `books/tinycld/books/lib/chart-of-accounts-fixture.ts` (`balance()`, about line 346)
- Modify: `books/tinycld/books/lib/account-register-fixture.ts` (the three rows that end at `] satisfies BooksAccountBalances[]`, about lines 414-443)
- Modify: `books/tests/accounts-query.test.ts` (`extraBalance`, line 12)
- Modify: `books/tests/bank-connections-query.test.ts` (`balance`, line 28)

**Interfaces:**
- Consumes: the column `month_start` of task 4a.
- Produces: nothing a later task uses.

- [ ] **Step 1: Regenerate the schema types and see the typecheck fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && pnpm run packages:generate`
Then: `cd /Users/nas/tmp/tinycld/books-purchases/books && pnpm exec tinycld-pkg typecheck`
Expected: FAIL with `Property 'month_start' is missing in type …` in the four files above. When the typecheck names another file, give that file's balance rows the field in the same way and name the file in your report.

- [ ] **Step 2: Add the field to the three helpers**

In `books/tinycld/books/lib/chart-of-accounts-fixture.ts`, replace

```ts
        id: `bal_${seed.account.slice(-8)}_${seed.period.replace('-', '')}`,
        currency: 'USD',
```

with

```ts
        id: `bal_${seed.account.slice(-8)}_${seed.period.replace('-', '')}`,
        month_start: `${seed.period}-01`,
        currency: 'USD',
```

In `books/tests/accounts-query.test.ts`, in `extraBalance`, replace

```ts
    return {
        currency: 'USD',
        created: '2026-01-01 00:00:00.000Z',
```

with

```ts
    return {
        month_start: `${seed.period}-01`,
        currency: 'USD',
        created: '2026-01-01 00:00:00.000Z',
```

In `books/tests/bank-connections-query.test.ts`, replace

```ts
    return { currency: 'USD', created: STAMP, updated: STAMP, ...seed }
```

with

```ts
    return { month_start: `${seed.period}-01`, currency: 'USD', created: STAMP, updated: STAMP, ...seed }
```

- [ ] **Step 3: Add the field to the register fixture's three rows**

In `books/tinycld/books/lib/account-register-fixture.ts`, the array that ends `] satisfies BooksAccountBalances[]` has three rows, with `period: '2026-08'`, `period: '2026-09'` and `period: '2026-10'`. After each `period` line add the row's day:

```ts
        period: '2026-08',
        month_start: '2026-08-01',
```

```ts
        period: '2026-09',
        month_start: '2026-09-01',
```

```ts
        period: '2026-10',
        month_start: '2026-10-01',
```

- [ ] **Step 4: Run the checks**

Run, in `/Users/nas/tmp/tinycld/books-purchases/books`: `pnpm exec tinycld-pkg typecheck`
Expected: no error.
Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/biome check tinycld/books/lib/chart-of-accounts-fixture.ts tinycld/books/lib/account-register-fixture.ts tests/accounts-query.test.ts tests/bank-connections-query.test.ts`
Expected: no error.
Run: `pnpm exec tinycld-pkg test tests/accounts-query.test.ts tests/bank-connections-query.test.ts tests/register-query.test.ts`
Expected: every test passes.

- [ ] **Step 5: Commit (Books repo)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add tinycld/books/lib/chart-of-accounts-fixture.ts tinycld/books/lib/account-register-fixture.ts tests/accounts-query.test.ts tests/bank-connections-query.test.ts
git commit -m "test(books): balance fixtures carry month_start"
```

---

### Task 4b: The General Ledger's beginning balance reads the stored months (Books, requirement 2)

Today the beginning balance of a section is a sum over every posted line of the account before the period: its cost grows with the account's history, in each of a run's three long statements. After this task it is:

- `months`: the stored months of the account with `month_start < startOfMonth(from)` (an income or expense account: only the months of the period's fiscal year; a fiscal year starts on the first day of a month, so a whole month is in one fiscal year);
- plus `monthToDate`: the posted lines of the account with `startOfMonth(from) <= date < from` (at most one month of one account);
- plus, on the retained-earnings account, `earlierPL`: the stored months of every income and expense account in earlier fiscal years.

With All dates `from` is blank, so no month and no line is before it and the balance is 0, as before. The sum of `months` (source `books.account_balances`) and `monthToDate` (source `books.journal_lines`) needs task 3b: the balances' currency field declares `SameCurrencyAs: "books.journal_lines.currency"`.

**Files:**
- Modify: `books/server/report_sources.go` (the `books.account_balances` source, about line 98)
- Modify: `books/server/report_general_ledger.go` (`generalLedgerShipped` revision; `generalLedgerOpening`; new `glSum`, `glStoredNet`)
- Create: `books/server/report_general_ledger_stored_test.go`
- Modify: `books/server/report_general_ledger_test.go` (revision pins at lines 330 and 1126; `TestGeneralLedgerTopics`, line 1045; `TestGeneralLedgerSpecUnchanged`, line 1272)
- Modify: `books/server/report_sources_test.go` (the fields of `books.account_balances`, line 301)
- Modify: `books/server/report_general_ledger_scale_test.go` (`TestGLScaleBook`; new `requireGLOpeningsFromTheLines`)
- Modify: `books/docs/specs/general-ledger-design.md` (§5, the line "Named pieces", line 101)
- Modify: `books/docs/plans/general-ledger-ledger.md` (append under "### Changes")

**Interfaces:**
- Consumes (core): `reports.Field.SameCurrencyAs` (task 3b); the formula operator `startOfMonth` (task 3). (Books, task 4a): the column `month_start`. (Books tests, existing): `newGLBook`, `glBook.account/post`, `saveSettings`, `mkRetainedEarnings`, `mkDraft`, `setStatus`, `statusVoid`, `ln`, `runGL`, `runGLErr`, `glPeriod`, `glAllDates`, `glToday`, `sectionFor`, `balanceCell`; the spec helpers `op`, `fld`, `link`, `rec`, `lit`, `bound`, `mustJSON`, `profitAndLossTypes`, `glNet`; (task 1) `glScaleBook`, `TestGLScaleBook`.
- Produces (for tasks 5 to 7 and for part 4): the source field `books.account_balances.month_start` (Date, text day); `glSum(source string, value *reports.Node, where ...*reports.Node) *reports.Node` (a sum lookup, 0 when no row passes); `glStoredNet() *reports.Node` (a stored month's debits less its credits); the cross-check helpers `glOpeningFromLines() *reports.Node`, `glSpecFromLines(t *testing.T) json.RawMessage`, `runGLSpec(t *testing.T, app core.App, reader *core.Record, raw json.RawMessage, params map[string]any) string`; the book `newGLStoredBook(t *testing.T) glStoredBook`; `requireGLOpeningsFromTheLines(t *testing.T, b glScaleBook)`. The shipped General Ledger is revision 3 and its topics include `collection/books_account_balances`.

- [ ] **Step 1: Write the failing tests (new file)**

Create `/Users/nas/tmp/tinycld/books-purchases/books/server/report_general_ledger_stored_test.go`:

```go
package books

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/reports"
)

// glOpeningFromLines is the beginning balance as the General Ledger read it
// before it read the stored months: every figure from the journal lines. The
// tests below run the report with it and require the same rows.
func glOpeningFromLines() *reports.Node {
	net := glNet()
	from := bound("period", "from")
	pl := lit(profitAndLossTypes())
	own := &reports.Node{
		Op:     "lookup",
		Source: "books.journal_lines",
		Where: op("and",
			op("eq", link("account"), rec("byAccount", "id")),
			op("eq", fld("posted"), lit(true)),
			op("lt", fld("date"), from),
			op("or",
				op("not", op("in", fld("account", "type"), pl)),
				op("gte", op("fiscalYear", fld("date")), op("fiscalYear", from)),
			),
		),
		Value: mustJSON(net),
		Pick:  "sum",
	}
	earlierPL := &reports.Node{
		Op:     "lookup",
		Source: "books.journal_lines",
		Where: op("and",
			op("eq", fld("posted"), lit(true)),
			op("in", fld("account", "type"), pl),
			op("lt", op("fiscalYear", fld("date")), op("fiscalYear", from)),
		),
		Value: mustJSON(net),
		Pick:  "sum",
	}
	return op("add",
		op("coalesce", own, lit(0)),
		op("if",
			op("eq", rec("byAccount", "detail_type"), lit("retained_earnings")),
			op("coalesce", earlierPL, lit(0)),
			lit(0),
		),
	)
}

// generalLedgerOpeningGolden is mustJSON(generalLedgerOpening()): the stored
// months before the period's first month, the lines of that month before the
// period and, on retained earnings, the stored income and expense months of
// earlier fiscal years. TestGeneralLedgerSpecUnchanged holds the spec to it.
const generalLedgerOpeningGolden = "" +
	`{"op":"add","args":[{"op":"add","args":[` +
	`{"op":"coalesce","args":[{"op":"lookup","value":{"op":"sub","args":[{"path":[{"field":"debit_total"}]},{"path":[{"field":"credit_total"}]}]},` +
	`"where":{"op":"and","args":[{"op":"eq","args":[{"path":[{"link":"account"}]},{"path":[{"field":"id"}],"record":"byAccount"}]},` +
	`{"op":"lt","args":[{"path":[{"field":"month_start"}]},{"op":"startOfMonth","args":[{"param":"period","bound":"from"}]}]},` +
	`{"op":"or","args":[{"op":"not","args":[{"op":"in","args":[{"path":[{"link":"account"},{"field":"type"}]},` +
	`{"value":["income","other_income","cost_of_goods_sold","expense","other_expense"]}]}]},` +
	`{"op":"gte","args":[{"op":"fiscalYear","args":[{"path":[{"field":"month_start"}]}]},{"op":"fiscalYear","args":[{"param":"period","bound":"from"}]}]}]}]},` +
	`"source":"books.account_balances","pick":"sum"},{"value":0}]},` +
	`{"op":"coalesce","args":[{"op":"lookup","value":{"op":"sub","args":[{"path":[{"field":"debit"}]},{"path":[{"field":"credit"}]}]},` +
	`"where":{"op":"and","args":[{"op":"eq","args":[{"path":[{"link":"account"}]},{"path":[{"field":"id"}],"record":"byAccount"}]},` +
	`{"op":"eq","args":[{"path":[{"field":"posted"}]},{"value":true}]},` +
	`{"op":"gte","args":[{"path":[{"field":"date"}]},{"op":"startOfMonth","args":[{"param":"period","bound":"from"}]}]},` +
	`{"op":"lt","args":[{"path":[{"field":"date"}]},{"param":"period","bound":"from"}]}]},` +
	`"source":"books.journal_lines","pick":"sum"},{"value":0}]}]},` +
	`{"op":"if","args":[{"op":"eq","args":[{"path":[{"field":"detail_type"}],"record":"byAccount"},{"value":"retained_earnings"}]},` +
	`{"op":"coalesce","args":[{"op":"lookup","value":{"op":"sub","args":[{"path":[{"field":"debit_total"}]},{"path":[{"field":"credit_total"}]}]},` +
	`"where":{"op":"and","args":[{"op":"in","args":[{"path":[{"link":"account"},{"field":"type"}]},` +
	`{"value":["income","other_income","cost_of_goods_sold","expense","other_expense"]}]},` +
	`{"op":"lt","args":[{"op":"fiscalYear","args":[{"path":[{"field":"month_start"}]}]},{"op":"fiscalYear","args":[{"param":"period","bound":"from"}]}]}]},` +
	`"source":"books.account_balances","pick":"sum"},{"value":0}]},{"value":0}]}]}`

// The stored months' currency is the journal lines' currency (core
// SameCurrencyAs), so a report adds the two.
func TestReportSourcesBalancesShareTheLinesCurrency(t *testing.T) {
	for _, s := range reportSources() {
		if s.Key != "books.account_balances" {
			continue
		}
		for _, f := range s.Fields {
			if f.Key == "currency" && f.SameCurrencyAs == "books.journal_lines.currency" {
				return
			}
		}
		t.Fatalf("the currency field of books.account_balances does not declare SameCurrencyAs books.journal_lines.currency: %+v", s.Fields)
	}
	t.Fatal("no books.account_balances source")
}

// glSpecFromLines is the shipped General Ledger with every beginning balance
// read from the journal lines.
func glSpecFromLines(t *testing.T) json.RawMessage {
	t.Helper()
	shipped := string(mustJSON(generalLedgerSpec()))
	stored := string(mustJSON(generalLedgerOpening()))
	// byNormal writes its value twice, so the Balance formula, its rollup and
	// the opening row hold the beginning balance twice each, and the having once.
	if n := strings.Count(shipped, stored); n != 7 {
		t.Fatalf("the spec holds the beginning balance %d times, want 7", n)
	}
	return json.RawMessage(strings.ReplaceAll(shipped, stored, string(mustJSON(glOpeningFromLines()))))
}

// runGLSpec runs a General Ledger spec as reader and returns its rows as JSON.
func runGLSpec(t *testing.T, app core.App, reader *core.Record, raw json.RawMessage, params map[string]any) string {
	t.Helper()
	spec, info, err := reports.Load(raw)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	given := map[string]json.RawMessage{}
	for id, v := range params {
		given[id] = mustJSON(v)
	}
	res, err := reports.Run(context.Background(), app, reports.Request{
		Spec: spec, Loaded: info, Params: given, Options: reports.Options{Today: glToday}, Auth: reader,
	})
	if err != nil {
		t.Fatalf("Run %v: %v", params, err)
	}
	return string(mustJSON(res.Rows))
}

// glStoredBook is a book whose fiscal year starts in July, with lines in an
// earlier fiscal year, in whole months of the current one, in August before
// the 12th, on the 12th and after it, a void pair in July and one in August,
// and a draft.
type glStoredBook struct {
	glBook
	operating, card, equity, retained, income, rent *core.Record
}

func newGLStoredBook(t *testing.T) glStoredBook {
	t.Helper()
	b := glStoredBook{glBook: newGLBook(t)}
	saveSettings(t, b.app, map[string]any{"fiscal_year_start_month": 7})
	b.operating = b.account(t, "Operating", "1000", "bank", "checking")
	b.card = b.account(t, "Company card", "2100", "credit_card", "credit_card")
	b.equity = b.account(t, "Owner's equity", "3000", "equity", "owners_equity")
	b.retained = mkRetainedEarnings(t, b.app, "3900")
	b.income = b.account(t, "Design income", "4000", "income", "service_income")
	b.rent = b.account(t, "Office rent", "6000", "expense", "rent_lease")
	void := func(entry *core.Record) {
		t.Helper()
		if err := setStatus(b.app, entry.Id, statusVoid, ""); err != nil {
			t.Fatalf("void: %v", err)
		}
	}
	// The fiscal year that ended 2025-06-30, and the one that ended 2026-06-30.
	b.post(t, "2025-03-15", ln{b.operating, 30000, 0}, ln{b.income, 0, 30000})
	b.post(t, "2025-09-10", ln{b.rent, 10000, 0}, ln{b.operating, 0, 10000})
	b.post(t, "2026-06-30", ln{b.rent, 900, 0}, ln{b.card, 0, 900})
	// The period's fiscal year: its first day, and July.
	b.post(t, "2026-07-01", ln{b.operating, 5000, 0}, ln{b.income, 0, 5000})
	void(b.post(t, "2026-07-10", ln{b.rent, 4000, 0}, ln{b.operating, 0, 4000}))
	b.post(t, "2026-07-20", ln{b.rent, 2000, 0}, ln{b.card, 0, 2000})
	// August before the 12th.
	b.post(t, "2026-08-03", ln{b.operating, 100000, 0}, ln{b.equity, 0, 100000})
	void(b.post(t, "2026-08-05", ln{b.rent, 7000, 0}, ln{b.operating, 0, 7000}))
	b.post(t, "2026-08-11", ln{b.rent, 300, 0}, ln{b.card, 0, 300})
	mkDraft(t, b.app, "2026-08-04", ln{b.rent, 999, 0}, ln{b.operating, 0, 999})
	// The 12th and after.
	b.post(t, "2026-08-12", ln{b.operating, 700, 0}, ln{b.income, 0, 700})
	b.post(t, "2026-08-20", ln{b.rent, 60, 0}, ln{b.operating, 0, 60})
	return b
}

// The beginning balance from the stored months and the lines of the period's
// first month equals the one from the lines alone, row for row: a period that
// starts in the middle of a month, on the first day of a month, on the first
// day of the fiscal year, in an earlier fiscal year, before every line, and
// All dates; with every account and with a list.
func TestGeneralLedgerBeginningBalanceFromStoredMonthsEqualsTheLines(t *testing.T) {
	b := newGLStoredBook(t)
	fromLines := glSpecFromLines(t)
	stored := mustJSON(generalLedgerSpec())
	for _, period := range []map[string]any{
		glPeriod("2026-08-12", "2026-08-31"),
		glPeriod("2026-08-01", "2026-08-31"),
		glPeriod("2026-07-01", "2026-08-31"),
		glPeriod("2026-07-15", "2026-07-31"),
		glPeriod("2025-09-10", "2025-09-30"),
		glPeriod("2024-01-01", "2024-12-31"),
		glAllDates,
	} {
		for _, params := range []map[string]any{
			{"period": period, "rows": "all"},
			{"period": period},
			{"period": period, "accounts": []string{b.rent.Id, b.retained.Id}},
		} {
			want := runGLSpec(t, b.app, b.reader, fromLines, params)
			got := runGLSpec(t, b.app, b.reader, stored, params)
			if got != want {
				t.Errorf("params %s: the rows differ\n stored months: %s\n lines:         %s", mustJSON(params), got, want)
			}
		}
	}
	// The figures themselves, for the period from the 12th.
	res := runGL(t, b.app, b.reader, map[string]any{"period": glPeriod("2026-08-12", "2026-08-31")}, reports.Options{Today: glToday})
	for _, w := range []struct {
		account *core.Record
		opening int64
		why     string
	}{
		{b.operating, 125000, "300.00 - 100.00 + 50.00 + 1,000.00; the void pairs cancel"},
		{b.card, 3200, "9.00 + 20.00 + 3.00, credit-normal"},
		{b.retained, 19100, "300.00 income less 100.00 and 9.00 rent of earlier fiscal years"},
		{b.income, 5000, "the line of 2026-07-01, the first day of the fiscal year"},
		{b.rent, 2300, "20.00 of July and 3.00 of August 11"},
	} {
		s := sectionFor(t, res, w.account)
		if s.opening == nil {
			t.Fatalf("%s has no opening row", s.head.Label)
		}
		if got := balanceCell(t, *s.opening); got != w.opening {
			t.Errorf("%s Beginning balance = %d, want %d (%s)", s.head.Label, got, w.opening, w.why)
		}
	}
}

// The whole months before the period come from books_account_balances, not
// from the lines: a stored month changed past the ledger moves the beginning
// balance, and the month of the period's first day is read from its lines.
func TestGeneralLedgerBeginningBalanceReadsTheStoredMonths(t *testing.T) {
	b := newGLStoredBook(t)
	opening := func() int64 {
		t.Helper()
		res := runGL(t, b.app, b.reader, map[string]any{"period": glPeriod("2026-08-12", "2026-08-31")}, reports.Options{Today: glToday})
		s := sectionFor(t, res, b.operating)
		if s.opening == nil {
			t.Fatal("Operating has no opening row")
		}
		return balanceCell(t, *s.opening)
	}
	bump := func(period string) {
		t.Helper()
		_, err := b.app.DB().NewQuery("UPDATE books_account_balances SET debit_total = debit_total + 5 WHERE account = {:a} AND period = {:p}").
			Bind(map[string]any{"a": b.operating.Id, "p": period}).Execute()
		if err != nil {
			t.Fatal(err)
		}
	}
	before := opening()
	bump("2026-07")
	if got := opening(); got != before+5 {
		t.Errorf("after July's stored debit rose by 5, Beginning balance = %d, want %d: July is read from its stored month", got, before+5)
	}
	bump("2026-08")
	if got := opening(); got != before+5 {
		t.Errorf("after August's stored debit rose by 5, Beginning balance = %d, want %d: the period's first month is read from its lines", got, before+5)
	}
}
```

- [ ] **Step 2: Change the pins of the existing tests**

In `/Users/nas/tmp/tinycld/books-purchases/books/server/report_general_ledger_test.go`:

Replace (line 330)

```go
	if shipped.Revision != 2 || !shipped.Unpublished {
		t.Errorf("revision %d, unpublished %v; want revision 2 (detail.open), shipped unpublished", shipped.Revision, shipped.Unpublished)
```

with

```go
	if shipped.Revision != 3 || !shipped.Unpublished {
		t.Errorf("revision %d, unpublished %v; want revision 3 (the beginning balance reads the stored months), shipped unpublished", shipped.Revision, shipped.Unpublished)
```

Replace (line 1126, in `TestGeneralLedgerShippedUnpublished`) `row.GetInt("shipped_revision") != 2 ||` with `row.GetInt("shipped_revision") != 3 ||`.

In `TestGeneralLedgerTopics`, replace

```go
		"collection/books_customers", "collection/books_vendors", "collection/books_settings",
	} {
```

with

```go
		"collection/books_customers", "collection/books_vendors", "collection/books_settings",
		// The beginning balance reads the stored months.
		"collection/books_account_balances",
	} {
```

In `TestGeneralLedgerSpecUnchanged`, replace

```go
		`"detail":{"label":{"path":[{"field":"date"}]},"open":`+string(mustJSON(entryOpenPath()))+`}`, 1)
	got := string(mustJSON(generalLedgerSpec()))
```

with

```go
		`"detail":{"label":{"path":[{"field":"date"}]},"open":`+string(mustJSON(entryOpenPath()))+`}`, 1)
	// The golden holds the beginning balance as the report read it from the
	// journal lines. The report now reads the stored months: that formula is
	// pinned by generalLedgerOpeningGolden.
	fromLines := string(mustJSON(glOpeningFromLines()))
	if n := strings.Count(golden, fromLines); n != 7 {
		t.Fatalf("the golden holds the beginning balance from the lines %d times, want 7", n)
	}
	golden = strings.ReplaceAll(golden, fromLines, generalLedgerOpeningGolden)
	got := string(mustJSON(generalLedgerSpec()))
```

(The long constant `generalLedgerSpecGolden` does not change: the rest of the spec stays pinned byte for byte.)

In `/Users/nas/tmp/tinycld/books-purchases/books/server/report_sources_test.go`, in the `books.account_balances` entry, replace

```go
				{key: "period", typ: reports.Text, aggs: reports.MinMax, label: "Month"},
				money("debit_total"), money("credit_total"), text("currency"),
```

with

```go
				{key: "period", typ: reports.Text, aggs: reports.MinMax, label: "Month"},
				{key: "month_start", typ: reports.Date, textDay: true, aggs: reports.MinMax, label: "Month start"},
				money("debit_total"), money("credit_total"), text("currency"),
```

- [ ] **Step 3: Hold the scale book's beginning balance to its lines**

In `/Users/nas/tmp/tinycld/books-purchases/books/server/report_general_ledger_scale_test.go`, at the end of `TestGLScaleBook`, replace

```go
				t.Fatalf("run %s shows a section of account %s, which it did not pick (%v)", run.name, id, picked)
			}
		}
	}
}
```

with

```go
				t.Fatalf("run %s shows a section of account %s, which it did not pick (%v)", run.name, id, picked)
			}
		}
	}
	requireGLOpeningsFromTheLines(t, b)
}

// requireGLOpeningsFromTheLines holds the report's beginning balance, for a
// period that starts in the middle of a month, to a direct sum of the lines
// before it: the busy account (every earlier line) and one income account
// (the lines of the period's fiscal year, which starts in January in this
// book; credit-normal). verifyBalances proves the stored rows; this proves
// the report reads them and the lines of the first month as one balance. A
// report that read a month twice or left one out would be fast and wrong.
func requireGLOpeningsFromTheLines(t *testing.T, b glScaleBook) {
	t.Helper()
	from := b.lastDay[:8] + "15"
	days := map[string]any{"from": from, "year": from[:5] + "01-01"}
	// The income account with the most lines of the fiscal year before the period.
	var incomeID string
	if err := b.app.DB().NewQuery(`SELECT account FROM books_journal_lines
WHERE posted IS true AND date < {:from} AND date >= {:year} AND account IN (SELECT id FROM books_accounts WHERE name LIKE 'Income ___')
GROUP BY account ORDER BY count(*) DESC, account LIMIT 1`).Bind(days).Row(&incomeID); err != nil {
		t.Fatalf("no income account has a line from %s to %s: %v", days["year"], from, err)
	}
	income := b.incomes[slices.IndexFunc(b.incomes, func(a *core.Record) bool { return a.Id == incomeID })]
	res, err := runGLErr(t, b.app, b.reader, map[string]any{
		"period": glPeriod(from, from), "rows": "all", "accounts": []string{b.receivable.Id, income.Id},
	}, reports.Options{Today: b.lastDay})
	if err != nil {
		t.Fatalf("run from %s: %v", from, err)
	}
	for _, w := range []struct {
		account *core.Record
		sum     string
	}{
		{b.receivable, "SELECT COALESCE(sum(debit - credit), 0) FROM books_journal_lines WHERE posted IS true AND account = {:a} AND date < {:from}"},
		{income, "SELECT COALESCE(sum(credit - debit), 0) FROM books_journal_lines WHERE posted IS true AND account = {:a} AND date < {:from} AND date >= {:year}"},
	} {
		var want int64
		if err := b.app.DB().NewQuery(w.sum).Bind(map[string]any{"a": w.account.Id, "from": from, "year": days["year"]}).Row(&want); err != nil {
			t.Fatal(err)
		}
		if want == 0 {
			t.Fatalf("%s has no net before %s: the check would prove nothing", w.account.GetString("name"), from)
		}
		s := sectionFor(t, res, w.account)
		if s.opening == nil {
			t.Fatalf("%s has no beginning-balance row", s.head.Label)
		}
		if got := balanceCell(t, *s.opening); got != want {
			t.Errorf("%s Beginning balance from %s = %d, want %d (the lines before the period)", s.head.Label, from, got, want)
		}
	}
}
```

This check passes before the report changes too (the old beginning balance is the lines' sum). It is here so that the scale runs of tasks 5 to 7 never measure a fast and wrong report.

- [ ] **Step 4: Run the tests to verify they fail**

Run, in `/Users/nas/tmp/tinycld/books-purchases/books/server`: `go vet . && go test -run 'GeneralLedger|ReportSources' -count=1 .`
Expected: FAIL in `TestGeneralLedgerSpecLoads` (`revision 2 … want revision 3`), `TestGeneralLedgerShippedUnpublished`, `TestGeneralLedgerTopics` (`want collection/books_account_balances`), `TestGeneralLedgerSpecUnchanged` (`the General Ledger spec JSON changed at byte …`), `TestGeneralLedgerBeginningBalanceReadsTheStoredMonths` (the stored month does not move the balance), `TestReportSourcesBalancesShareTheLinesCurrency`, and the `books.account_balances` cases of `TestReportSourcesCatalog` (no field `month_start`). `TestGeneralLedgerBeginningBalanceFromStoredMonthsEqualsTheLines` passes now and after step 6: before step 6 the shipped beginning balance is the formula from the lines, so the two specs are the same. It is a guard on the rows, not the RED of this task.

- [ ] **Step 5: Register the day and the shared currency**

In `/Users/nas/tmp/tinycld/books-purchases/books/server/report_sources.go`, in the `books.account_balances` source, replace

```go
				{Key: "period", Label: "Month", Type: reports.Text, Aggregates: reports.MinMax},
				{Key: "debit_total", Label: "Debit total", Type: reports.Money, Aggregates: reports.SumAll, Currency: "currency"},
				{Key: "credit_total", Label: "Credit total", Type: reports.Money, Aggregates: reports.SumAll, Currency: "currency"},
				{Key: "currency", Label: "Currency", Type: reports.Text},
```

with

```go
				{Key: "period", Label: "Month", Type: reports.Text, Aggregates: reports.MinMax},
				// The first day of the row's month, so a report compares a month
				// with a day (the General Ledger's beginning balance).
				{Key: "month_start", Label: "Month start", Type: reports.Date, TextDay: true, Aggregates: reports.MinMax},
				{Key: "debit_total", Label: "Debit total", Type: reports.Money, Aggregates: reports.SumAll, Currency: "currency"},
				{Key: "credit_total", Label: "Credit total", Type: reports.Money, Aggregates: reports.SumAll, Currency: "currency"},
				// A month row's totals are sums of the account's journal lines, in
				// the lines' currency, so a report adds stored months to lines.
				{Key: "currency", Label: "Currency", Type: reports.Text, SameCurrencyAs: "books.journal_lines.currency"},
```

- [ ] **Step 6: Read the stored months in the beginning balance**

In `/Users/nas/tmp/tinycld/books-purchases/books/server/report_general_ledger.go`, in `generalLedgerShipped`, replace

```go
		// 2: lines open the transaction that posted them (detail.open).
		Revision:    2,
```

with

```go
		// 2: lines open the transaction that posted them (detail.open).
		// 3: the beginning balance reads the stored months.
		Revision:    3,
```

Replace the whole body of `generalLedgerOpening` (keep its comment), from `func generalLedgerOpening() *reports.Node {` to its closing brace, with:

```go
func generalLedgerOpening() *reports.Node {
	from := bound("period", "from")
	monthStart := op("startOfMonth", from)
	pl := lit(profitAndLossTypes())
	ownAccount := op("eq", link("account"), rec("byAccount", "id"))
	// The whole months before the period's first month, from the stored rows.
	// A fiscal year starts on the first day of a month, so every day of a
	// month is in the fiscal year of the month's first day.
	months := glSum("books.account_balances", glStoredNet(),
		ownAccount,
		op("lt", fld("month_start"), monthStart),
		op("or",
			op("not", op("in", fld("account", "type"), pl)),
			op("gte", op("fiscalYear", fld("month_start")), op("fiscalYear", from)),
		),
	)
	// The lines of the period's first month before the period: at most one
	// month of one account. They are in the period's fiscal year.
	monthToDate := glSum("books.journal_lines", glNet(),
		ownAccount,
		op("eq", fld("posted"), lit(true)),
		op("gte", fld("date"), monthStart),
		op("lt", fld("date"), from),
	)
	earlierPL := glSum("books.account_balances", glStoredNet(),
		op("in", fld("account", "type"), pl),
		op("lt", op("fiscalYear", fld("month_start")), op("fiscalYear", from)),
	)
	return op("add",
		op("add", months, monthToDate),
		op("if",
			op("eq", rec("byAccount", "detail_type"), lit("retained_earnings")),
			earlierPL,
			lit(0),
		),
	)
}

// glSum adds value over the rows of source that pass every term of where, 0
// when none does.
func glSum(source string, value *reports.Node, where ...*reports.Node) *reports.Node {
	return op("coalesce", &reports.Node{
		Op:     "lookup",
		Source: source,
		Where:  op("and", where...),
		Value:  mustJSON(value),
		Pick:   "sum",
	}, lit(0))
}

// glStoredNet is a stored month's debits less its credits: the sum of glNet
// over the account's posted lines of that month (books_account_balances).
func glStoredNet() *reports.Node {
	return op("sub", fld("debit_total"), fld("credit_total"))
}
```

(The two adds are nested because the planner built and ran this form; do not change it to one `add` of three values: the spec golden pins the JSON.)

- [ ] **Step 7: Run the tests to verify they pass**

Run: `gofmt -l . && go vet . && go test -run 'GeneralLedger|ReportSources|Balances|TestGLScaleBook$' -count=1 .`
Expected: no file listed by `gofmt`, `ok  tinycld.org/packages/books`. Every figure of `report_general_ledger_test.go` stays as it was; the cross-check compares 21 runs row for row.

Then the whole package, once (about 4 minutes): `go test -count=1 .`
Expected: `ok`.

- [ ] **Step 8: Update the design spec**

In `/Users/nas/tmp/tinycld/books-purchases/books/docs/specs/general-ledger-design.md`, §5, in the line that starts `- Named pieces (inlined):`, replace the text from `` `opening` = `` to the end of the line with:

```markdown
`opening` = `add( add( months, monthToDate ), if( eq({record byAccount [detail_type]}, "retained_earnings"), earlierPL, 0 ) )`, where `monthStart` = `startOfMonth(from)`; `stored` = `sub([debit_total], [credit_total])`; `months` = `coalesce( lookup( source books.account_balances, where and( eq([account], {record byAccount [id]}), lt([month_start], monthStart), or( not(in([account, type], PL)), gte(fiscalYear([month_start]), fiscalYear(from)) ) ), value stored, pick sum ), 0 )`; `monthToDate` = `coalesce( lookup( source books.journal_lines, where and( eq([account], {record byAccount [id]}), eq([posted], true), gte([date], monthStart), lt([date], from) ), value net, pick sum ), 0 )`; `earlierPL` = `coalesce( lookup( source books.account_balances, where and( in([account, type], PL), lt(fiscalYear([month_start]), fiscalYear(from)) ), value stored, pick sum ), 0 )`. The whole months before the period come from the stored monthly balances (`books_account_balances`, kept by posting); only the lines of the period's first month are read (p5-report-engine-scale). The two sources add because the balances' currency field declares `SameCurrencyAs: books.journal_lines.currency`.
```

- [ ] **Step 9: Commit the code (Books repo)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add server/report_sources.go server/report_general_ledger.go server/report_general_ledger_stored_test.go server/report_general_ledger_test.go server/report_sources_test.go server/report_general_ledger_scale_test.go docs/specs/general-ledger-design.md
git commit -m "perf(books): the General Ledger's beginning balance reads the stored months"
```

- [ ] **Step 10: Hand the 100k run to the driver, then record the change**

Do not start this run. Hand back:

`NEEDS-RUN: cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task4 go test -run 'TestGLScale$/100k$' -timeout 20m -v .` — needed from it: `scale-task4/gl-results.md` (the five medians) and, from `scale-task4/gl-plans.md`, the section `## 100k lines, run 3 busy account, month` (statements 4, 5 and 6: their times and their plan lines).

With the results, append to `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md`, at the end of the section "### Changes", this block, with every `<…>` replaced by a figure of the run (the "before" column is the "after" column of the last record under "Changes", task 2's):

```markdown
#### Task 4: the beginning balance reads the stored months (2026-11-19)

Before: each section's beginning balance was a sum over every posted line of the account before the period (`lk1`, in each of a run's three long statements), and over every income and expense line of earlier fiscal years on retained earnings. After: the stored months before the period's first month (`books_account_balances`, by `month_start`), plus the posted lines of that month before the period. Core gained `Field.SameCurrencyAs` so that the two sources' amounts add (core money rule R24, extended). The rows are the same: `TestGeneralLedgerBeginningBalanceFromStoredMonthsEqualsTheLines` compares 21 runs row for row.

| run, 100k lines | before | after |
| --- | --- | --- |
| 1 all accounts, month | <ms> | <ms> |
| 2 all accounts, year | <ms> | <ms> |
| 3 busy account, month | <ms> | <ms> |
| 4 small account, month | <ms> | <ms> |
| 5 three accounts, year | <ms> | <ms> |

Run 3, plan lines of `books_journal_lines` and `books_account_balances`, statements 4, 5 and 6:

<the lines from gl-plans.md>

What is left in the time: <one sentence from the statement times: which statement and which read is now the longest>.
```

When a filtered run (3, 4 or 5) is slower after than before, say so in the block and in your report; do not change the report to hide it. Tasks 5 and 6 (the next plan part) change how the lookups are read.

Then commit the record on its own:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add docs/plans/general-ledger-ledger.md
git commit -m "docs(books): record the stored-months beginning balance at 100k lines"
```

---

## What is left (the next plan part)

Part 4 is `2026-11-19-report-engine-scale-part4.md`, written from the outline (`plan-outline.md`, sections "Tasks" 5 to 7 and "State after the part 3 planner") and from this part's Interfaces blocks:

- **5. core · A sum lookup read as one grouped aggregate (requirement 4).** The beginning-balance lookups it groups are now task 4b's three: `months` and `earlierPL` on `books.account_balances`, `monthToDate` on `books.journal_lines`. `earlierPL` reads no report row except through the `if` around it, so it is a lookup computed once per statement.
- **6. core · Day bounds in a lookup's where.** `monthToDate` has `gte([date], startOfMonth(from))` and `lt([date], from)`: the bound of task 2 (`dayBounds`) for a lookup, so its lines are read by `(account, date)`. `months` compares `month_start`, which has no index: task 6's planner measures whether `books_account_balances` needs one on `(account, month_start)`.
- **7. books · Scale run and record.** The 500k `NEEDS-RUN`, the plan pins in `TestGLScaleBook`, the ledger record; `STOPPED` when a filtered run is over 1 s.
- The human's two decisions (large reports do not live-refresh; the Trial Balance, the Balance Sheet and the Transaction Detail reach the same bar) follow in a later part. A report that adds stored months to lines uses `SameCurrencyAs` as the General Ledger does, and the cross-check pattern of task 4b (`glSpecFromLines`, `runGLSpec`: the shipped spec against the old formula, `res.Rows` compared as JSON).

Facts for the next planner (measured or built by this part's planner, 2026-11-19):

- Tasks 3b, 4a and 4b were built together in an overlay on this tree (with task 3's two Go edits): the whole core `reports` package passes (98 s) and the whole Books server package passes (224 s). `TestGLScaleBook` passes with the stored months. No scale run was made: the time of task 4b is not known until its `NEEDS-RUN`.
- The scratch copies of every changed file are in `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part3-scratch/core/` and `…/part3-scratch/books/`, with the overlay files `overlay-3b.json` and `overlay-4.json`. A Books build of a later task needs the migration's `month_start` line in the tree for the run (Books tests read the migrations from disk); put it back after.
- The TypeScript blocks of task 4a-ts were not compiled by the planner: the type they satisfy is generated from the migration task 4a edits.
