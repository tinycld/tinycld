# Report Engine Scale, Part 31: `in` of a Select With a List Tests the Stored Column Too Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A report filter `in(<Select field>, <list of its option values>)` is served by a plain index on the field's column, and SQLite no longer keeps the LEFT JOIN to the linked row and scans the rows it could search.

**Architecture:** Core's `in` operator (`core/server/reports/formula.go`, `case "in"`) writes `(<value> IN (SELECT +value FROM json_each({:p})))`. For a Select, `<value>` is `NULLIF(c, '')`: no index serves it, and SQLite does not see that a row with no linked row fails it. A new function `selectIn` (new file `selectin.go`, the pair of `selectEq` in `selecteq.go`) adds a second term on the stored column, `c IN ({:p}, {:p}, NULL)`, with one bound placeholder for each distinct value. The `expr` of a list keeps its text values (`listed`) so `selectIn` can bind them. `legacySQL.noSelectIn` turns the term off for the tests that compare the two forms.

**Tech Stack:** Go 1.27, PocketBase (dbx), SQLite (modernc v1.57.0). Core module `tinycld.org/core`, package `reports`.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-expenses-by-vendor/cp3-options.md` (section A "(1)", section B, section C "(a)"), with its probe folder `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-expenses-by-vendor/cp3-options-probe/`, and the human's decision of 2026-11-21 (the note of `/Users/nas/code/tinycld/docs/plans/auto-helix/p5-expenses-by-vendor.md`, copied in Global Constraints). The unit's outline: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md`.

## Global Constraints

- The request, verbatim (lane default, through the human, 2026-11-21): "the core `in` operator (core/server/reports/formula.go:881-891) must add a plain-column NULL-rejecting term with ONE BOUND PLACEHOLDER PER VALUE, written like selectEq (core/server/reports/selecteq.go:22), with tests like the selectEq tests." (The line numbers are those of core main; on this branch `case "in"` is at `formula.go:931`.)
- Every value in SQL is bound (CLAUDE.md). The new term writes no value into the SQL text. Its list ends with the SQL keyword `NULL`, which is no value of the list (see "Facts the planner measured").
- The result of every report is the same as before, for every stored value: a row with no linked row, with a blank Select, and with a linked row the reader cannot see. The term is written wherever its guard holds, also under `or` and `not`, because it changes no value there; the tests prove it for each of those.
- Core names no package: no Books name, slug, collection or route in any core file. The fixture is the zoo.
- No hosting word in core or Books.
- No timing probe. The implementer starts no scale run: the 100k `TestStatementsScale` and the 500k `TestGLScale` are a `NEEDS-RUN` for the driver (step 8).
- Do not push. Commits name no assistant and link no session.
- Paths: core is `/Users/nas/tmp/tinycld/books-purchases/tinycld`, Books is `/Users/nas/tmp/tinycld/books-purchases/books`. Both on branch `books/p5-report-engine-scale`. Tree when this plan was written: core 75b3effc, books ab16fedb.

## Review Focus

Each line has its test in Task 1 (the test's name is in brackets).

1. A sale whose stall has a blank kind, no stall, or a stall the reader cannot see: the condition stays unknown, so `not(in(…))` holds, `eq(in(…), false)` does not, and a column shows blank [`TestASelectInAListKeepsItsResult`: "not in", "in is false", "a column", "both forms agree on every stored value"].
2. A list that holds a text that is no option value, a blank text, or a number: the term is not written and the old form answers [`TestASelectInAListKeepsItsResult`: "a list with a text that is no option value", "a list with a blank text", "a list of numbers", "a list of a text and a number"].
3. A list that names one value many times (40,000): one placeholder, not 40,000, which would pass SQLite's 32,766 variables [`TestASelectInAListIsBound`: "one value 40,000 times"].
4. An empty list, and a multiple choice param with no pick: no term; no row, and every row [`TestASelectInAListKeepsItsResult`: "an empty list", "not an empty list", "a param's list"].
5. An `in` under `or`, under `not`, and inside a lookup's `where`: the same rows as the old form, byte for byte [`TestASelectInAListKeepsItsResult`: "or of two lists", "or with another condition", "or with a not", "a lookup's where"]. The same on a Select whose column has another name, of a view source and of a union source [`TestASelectInAListOnOtherSourcesKeepsItsResult`].

## Facts the planner measured (do not find again)

- The cause is in the `in` operator, not in `jsonSelect` (`stmt.go:82`). `eq` has the stored-column term since `selectEq`; `in` had none.
- Both `c IN ({:p}, {:p})` and `c IN ({:p}, {:p}, NULL)` give the same zoo plan, with no statistics and after `ANALYZE`: `SEARCH zoo_stalls USING INDEX idx_zoo_stalls_kind (kind=?)`, then `SEARCH zoo_sales USING INDEX idx_zoo_sales_stall (stall=?)`, no `LEFT-JOIN`. The old form: `SCAN zoo_sales`, then `SEARCH zoo_stalls … (id=?) LEFT-JOIN`.
- Why the list ends with `NULL`: without it a blank `c` gives `NULL AND 0`, which is 0, where the old form gave NULL. `eq(in(…), false)` then holds for a row it did not hold for (planner's mutation: the case "in is false" fails). `x IN (…, NULL)` is NULL for an `x` the list does not hold, so with it the condition has the old value for every stored value. `selectEq` does not need this, because `IS` never gives NULL.
- A column's formula (`{"id":"isFood","formula":{"op":"in",…}}`) reads the Select from the report's rows, not from the stored column (`a.sel.sql != a.sql`), so it gets no term. A filter and a lookup's `where` get it.
- A Bool column's cells are `true`, `false` and `nil` in `detailCells`.
- The Expenses by Vendor plan bars of lane default: `TestExpensesByVendorQueryPlans` in `/Users/nas/tmp/tinycld/books/books/server` (books 3a21a45a + its uncommitted checkpoint 3 tests), built through an overlay of this branch's `reports` files plus this plan's change, has 0 plan-bar failures; it stops only on the missing `testdata/expenses_by_vendor_stat1.json`, as before. Without the change: 21 (cp3-options.md). Log: `part31-scratch/ebv-queryplans.log`.
- In the zoo a statement with the term is prepared once (0 second prepares), with no statistics and after `ANALYZE`; the same holds for `selectEq` there.
- Built once by the planner through a Go overlay (`part31-scratch/overlay.json`; the trees were not changed): `go vet .` clean; `go build ./...` in `core/server` clean; the whole core `reports` package `ok` in 131 s; the three Books Go shards (`books/.github/scripts/go-test-shard.sh <n> 3`, with `GOFLAGS=-overlay=…`) `ok` in 96 s, 91 s and 126 s. No existing core or Books test changed its result.
- RED (the tests on the unchanged engine, with only the `legacySQL.noSelectIn` field added): `TestASelectInAListIsServedByTheColumnIndex`, `TestASelectInAListKeepsItsResult`, `TestASelectInAListOnOtherSourcesKeepsItsResult` and `TestASelectInAListIsBound` fail in 29 subtests; `TestASelectInAListIsPreparedOnceWithoutStatistics` passes (it pins that the new term does not change the prepare count).
- The planner's files (the changed core files, the tests, the logs): `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part31-scratch/`.
- Books reports whose filters get the term (cp3-options.md, section C): Profit and Loss, Trial Balance, Balance Sheet, General Ledger (`account.type` in a list) and Statements (`entry.origin` in a list). None has a Books plan-bar test; steps 7 and 8 are their check.

---

### Task 1: core · `in` of a Select with a list of its option values tests the stored column too

**Files:**
- Create: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/selectin.go`
- Create: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/selectin_test.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/formula.go` (the `expr` struct, field `text` at line 49; `literal`, the `[]any` case at lines 606-625; `case "in"` at lines 931-941)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/params.go` (the multiple choice list, line 599)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/legacy.go` (the last field of `legacySQL`, line 37)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/statement_test.go` (the comment of `TestEveryReportStatementIsPreparedOnce`, lines 342-349)
- Modify (step 9, after the driver's runs): `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md` (section "Performance", a new record at the end of the file)

Match each edit by its text, not by its line number.

**Interfaces:**
- Consumes (they exist; read them before use): `type storedSelect struct{ col, sql string }` and `storedOption(options []Option, text string) bool` (`selecteq.go`); `expr.sel`, `expr.options`, `expr.list`, `expr.sql` (`formula.go`); `c.bind` (the compiler's bind, `func(any) string`, as `literal` uses it at `formula.go:621`); `jsonList` (`stmt.go:65`); `var legacySQL` (`legacy.go`). Test helpers: `newZoo`, `withStalls`, `salesAmounts`, `salesNames`, `kindIs`, `stateIs`, `notOf`, `stmtPlan`, `analyze`, `planSearch` (all in `selecteq_test.go`); `observeReads`, `readStatement`, `rawStatement`, `reprepares`, `readPool` (`statement_test.go`); `detailCells` (`viewsort_test.go`); `addZooNotes()` and `addZooFeedLog(t, z)` (`viewsource_test.go`); `newUnionZoo(t)` (`union_zoo_test.go`); `requireSameAsLegacy(t, z, user, spec, opts, params, off *bool)` (`aggonce_test.go`); `z.run` (`run_test.go`).
- Produces:
  - `func selectIn(bind func(any) string, a, list expr) string` (`selectin.go`): the condition with the stored-column term, or "" when `in` is written as before.
  - `expr.listed []string`: the values of a list that holds texts only.
  - `legacySQL.noSelectIn bool`.
  - Test helpers in `selectin_test.go`: `kindIn(list string) string`, `stateIn(list string) string`, `inOf(path, list string) string`, `const salesOfKinds`, `salesReads(t, z, raw, params) []readStatement`, `var storedIn *regexp.Regexp`, `storedInTerms(seen []readStatement) int`.

- [ ] **Step 1: Add the legacy switch and write the failing tests**

In `core/server/reports/legacy.go`, add the field after `everyLeafRows` (the tests compare the two forms through it, so it comes first):

```go
	everyLeafRows bool
	// noSelectIn writes in of a Select with a list of its option values as
	// the one test of the value read, as before selectIn added the test of
	// the stored column.
	noSelectIn bool
}
```

Create `core/server/reports/selectin_test.go`:

```go
package reports

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
)

// kindIn and stateIn are the filters of these tests: a Select path in a
// literal list (list is the JSON of the list).
func kindIn(list string) string {
	return inOf(`{"path":[{"link":"stall"},{"field":"kind"}]}`, `{"value":`+list+`}`)
}

func stateIn(list string) string {
	return inOf(`{"path":[{"field":"state"}]}`, `{"value":`+list+`}`)
}

func inOf(path, list string) string {
	return `{"op":"in","args":[` + path + `,` + list + `]}`
}

// salesOfKinds is salesNames under "the stall's kind is one of the kinds
// picked", a multiple choice param.
const salesOfKinds = `{"version":4,"source":"zoo.sales",
	"params":[{"id":"kinds","type":"choice","multiple":true,"options":[{"value":"food","label":"Food"},{"value":"gift","label":"Gift"}]}],
	"columns":[{"id":"name","path":[{"field":"name"}]},{"id":"amount","path":[{"field":"amount"}],"aggregate":"sum"}],
	"filter":{"op":"in","args":[{"path":[{"link":"stall"},{"field":"kind"}]},{"param":"kinds"}]},
	"sort":[{"col":"name"}]}`

// salesReads runs raw as ann and returns the statements of the run that read
// zoo_sales, failing when there is none.
func salesReads(t *testing.T, z *zoo, raw string, params map[string]string) []readStatement {
	t.Helper()
	seen := observeReads(t)
	z.run(t, "ann", raw, Options{}, params)
	observeRead = nil
	var out []readStatement
	for _, st := range *seen {
		if strings.Contains(st.sql, "zoo_sales") {
			out = append(out, st)
		}
	}
	if len(out) == 0 {
		t.Fatalf("no statement of the run reads zoo_sales (%d reads)", len(*seen))
	}
	return out
}

// storedIn is the stored-column term selectIn writes: a column in a list of
// placeholders that ends with NULL. Its first group is the placeholders.
var storedIn = regexp.MustCompile("\\[\\[\\w+\\.\\w+\\]\\] IN \\(((?:\\{:rp\\d+\\}, )+)NULL\\)")

// storedInTerms is how many stored-column terms the statements hold.
func storedInTerms(seen []readStatement) int {
	n := 0
	for _, st := range seen {
		n += len(storedIn.FindAllString(st.sql, -1))
	}
	return n
}

// An in of a Select field with a list of its option values is served by a
// plain index on the field's column, as eq is (selectEq): the planner
// searches the stalls by kind, then the sales by the stall, with a plain
// join; or it searches the sales by their state. It never scans the sales.
// Without the stored-column term (legacySQL.noSelectIn) the same report
// scans the sales and left-joins the stalls.
func TestASelectInAListIsServedByTheColumnIndex(t *testing.T) {
	byKind := []planSearch{{"zoo_stalls", "idx_zoo_stalls_kind", "kind=?"}, {"zoo_sales", "idx_zoo_sales_stall", "stall=?"}}
	byState := []planSearch{{"zoo_sales", "idx_zoo_sales_state", "state=?"}}
	cases := []struct {
		name   string
		raw    string
		params map[string]string
		want   []planSearch
	}{
		{"a linked select in a list", salesAmounts(kindIn(`["food","gift"]`)), nil, byKind},
		{"a linked select in a list of one", salesAmounts(kindIn(`["food"]`)), nil, byKind},
		{"a linked select in a param's list", salesOfKinds, map[string]string{"kinds": `["food","gift"]`}, byKind},
		{"a select of the report's own row", salesAmounts(stateIn(`["open","paid"]`)), nil, byState},
	}
	z := newZoo(t)
	withStalls(t, z, true)
	scans := func(steps []string) (scan, leftJoin string) {
		for _, step := range steps {
			if strings.HasPrefix(step, "SCAN zoo_sales") {
				scan = step
			}
			if strings.HasPrefix(step, "SEARCH zoo_stalls") && strings.Contains(step, "LEFT-JOIN") {
				leftJoin = step
			}
		}
		return scan, leftJoin
	}
	for _, stats := range []string{"no statistics", "after ANALYZE"} {
		if stats == "after ANALYZE" {
			analyze(t, z)
		}
		for _, c := range cases {
			t.Run(stats+"/"+c.name, func(t *testing.T) {
				for _, st := range salesReads(t, z, c.raw, c.params) {
					steps := stmtPlan(t, z, st)
					scan, leftJoin := scans(steps)
					if scan != "" {
						t.Errorf("the plan scans the sales (%s):\n%s\n%s", scan, strings.Join(steps, "\n"), st.sql)
					}
					if leftJoin != "" {
						t.Errorf("the plan left-joins the stalls (%s):\n%s\n%s", leftJoin, strings.Join(steps, "\n"), st.sql)
					}
					for _, s := range c.want {
						if !s.in(steps) {
							t.Errorf("no step SEARCH %s USING … %s (%s):\n%s\n%s", s.table, s.index, s.cond, strings.Join(steps, "\n"), st.sql)
						}
					}
				}
			})
		}
		t.Run(stats+"/without the stored-column term the sales are scanned", func(t *testing.T) {
			t.Cleanup(func() { legacySQL.noSelectIn = false })
			legacySQL.noSelectIn = true
			for _, st := range salesReads(t, z, salesAmounts(kindIn(`["food","gift"]`)), nil) {
				steps := stmtPlan(t, z, st)
				scan, leftJoin := scans(steps)
				if scan == "" || leftJoin == "" {
					t.Errorf("scan %q, left join %q: the old form no longer scans, so this test proves nothing:\n%s", scan, leftJoin, strings.Join(steps, "\n"))
				}
			}
		})
	}
}

// The stored-column term changes no result. A sale with no stall (S4), with
// a stall of a blank kind (S3) and with a stall the reader cannot see (S5)
// is in no list and is "not in" every list under not; its condition stays
// unknown, so eq with false does not hold for it and a column shows it
// blank. Each case also gives the result of the old form, byte for byte.
func TestASelectInAListKeepsItsResult(t *testing.T) {
	z := newZoo(t)
	withStalls(t, z, false)
	food, both := kindIn(`["food"]`), kindIn(`["food","gift"]`)
	cases := []struct {
		name   string
		filter string
		want   []any
		// terms is how many stored-column terms the filter writes.
		terms int
	}{
		{"in", both, []any{"S1", "S2"}, 1},
		{"in a list of one", food, []any{"S1"}, 1},
		{"not in", notOf(both), []any{"S3", "S4", "S5"}, 1},
		{"not in a list of one", notOf(food), []any{"S2", "S3", "S4", "S5"}, 1},
		// eq writes IS: an unknown condition is not equal to false, so the
		// sales whose kind reads blank stay out.
		{"in is false", `{"op":"eq","args":[` + food + `,{"value":false}]}`, []any{"S2"}, 1},
		{"or of two lists", `{"op":"or","args":[` + food + `,` + kindIn(`["gift"]`) + `]}`, []any{"S1", "S2"}, 2},
		{"or with another condition", `{"op":"or","args":[` + food + `,` + stateIs(`"paid"`) + `]}`, []any{"S1", "S2", "S5"}, 1},
		{"or with a not", `{"op":"or","args":[` + notOf(both) + `,` + stateIs(`"open"`) + `]}`, []any{"S1", "S3", "S4", "S5"}, 1},
		{"and", `{"op":"and","args":[` + both + `,` + stateIn(`["paid"]`) + `]}`, []any{"S2"}, 2},
		{"the same value two times", kindIn(`["food","food"]`), []any{"S1"}, 1},
		{"an empty list", kindIn(`[]`), nil, 0},
		{"not an empty list", notOf(kindIn(`[]`)), []any{"S1", "S2", "S3", "S4", "S5"}, 0},
		{"a list with a text that is no option value", kindIn(`["food","fo'od"]`), []any{"S1"}, 0},
		{"a list with a blank text", kindIn(`["food",""]`), []any{"S1"}, 0},
		{"a list of numbers", kindIn(`[1,2]`), nil, 0},
		{"a list of a text and a number", kindIn(`["food",2]`), []any{"S1"}, 0},
		{"a select of the report's own row", stateIn(`["open"]`), []any{"S1", "S4"}, 1},
		{"not, on the report's own row", notOf(stateIn(`["open"]`)), []any{"S2", "S3", "S5"}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spec := salesNames(c.filter, "")
			reads := salesReads(t, z, spec, nil)
			if got := detailCells(z.run(t, "ann", spec, Options{}, nil), "name"); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("sales %#v, want %#v", got, c.want)
			}
			if got := storedInTerms(reads[:1]); got != c.terms {
				t.Fatalf("%d stored-column terms in the first statement, want %d:\n%s", got, c.terms, reads[0].sql)
			}
			requireSameAsLegacy(t, z, "ann", spec, Options{}, nil, &legacySQL.noSelectIn)
		})
	}

	t.Run("a param's list", func(t *testing.T) {
		for _, c := range []struct {
			picked string
			want   []any
			terms  int
		}{
			{`["food","gift"]`, []any{"S1", "S2"}, 1},
			{`["gift"]`, []any{"S2"}, 1},
			// No pick: the condition matches every sale.
			{``, []any{"S1", "S2", "S3", "S4", "S5"}, 0},
		} {
			var params map[string]string
			if c.picked != "" {
				params = map[string]string{"kinds": c.picked}
			}
			reads := salesReads(t, z, salesOfKinds, params)
			if got := detailCells(z.run(t, "ann", salesOfKinds, Options{}, params), "name"); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("picked %s: sales %#v, want %#v", c.picked, got, c.want)
			}
			if got := storedInTerms(reads[:1]); got != c.terms {
				t.Fatalf("picked %s: %d stored-column terms, want %d:\n%s", c.picked, got, c.terms, reads[0].sql)
			}
			requireSameAsLegacy(t, z, "ann", salesOfKinds, Options{}, params, &legacySQL.noSelectIn)
		}
	})

	// A column's formula reads the kind from the report's rows, not from the
	// stored column, so it gets no term; its values are pinned all the same.
	t.Run("a column", func(t *testing.T) {
		spec := salesNames("", `,{"id":"isFood","formula":`+food+`},
			{"id":"flag","formula":{"op":"if","args":[`+food+`,{"value":1},{"value":0}]}}`)
		if n := storedInTerms(salesReads(t, z, spec, nil)); n != 0 {
			t.Fatalf("%d stored-column terms for a column's formula, want 0", n)
		}
		res := z.run(t, "ann", spec, Options{}, nil)
		if got, want := detailCells(res, "name"), []any{"S1", "S2", "S3", "S4", "S5"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("sales %#v, want %#v", got, want)
		}
		// S2's kind is gift (false); S3, S4 and S5 read a blank kind (unknown).
		if got, want := detailCells(res, "isFood"), []any{true, false, nil, nil, nil}; !reflect.DeepEqual(got, want) {
			t.Fatalf("isFood %#v, want %#v", got, want)
		}
		if got, want := detailCells(res, "flag"), []any{int64(1), int64(0), int64(0), int64(0), int64(0)}; !reflect.DeepEqual(got, want) {
			t.Fatalf("flag %#v, want %#v", got, want)
		}
		requireSameAsLegacy(t, z, "ann", spec, Options{}, nil, &legacySQL.noSelectIn)
	})

	t.Run("a lookup's where", func(t *testing.T) {
		spec := `{"version":4,"source":"zoo.stalls","columns":[{"id":"name","path":[{"field":"name"}]},
			{"id":"sales","formula":{"op":"lookup","source":"zoo.sales","where":` + both + `,"value":{"path":[{"field":"amount"}]},"pick":"sum"}}],
			"sort":[{"col":"name"}]}`
		if storedInTerms(salesReads(t, z, spec, nil)) == 0 {
			t.Fatal("the lookup's where holds no stored-column term")
		}
		got := detailCells(z.run(t, "ann", spec, Options{}, nil), "sales")
		if len(got) == 0 {
			t.Fatal("the report has no detail row")
		}
		for i, v := range got {
			if v != int64(3) {
				t.Fatalf("row %d: sales %#v, want 3 (S1 and S2; S5's stall is hidden)", i, v)
			}
		}
		requireSameAsLegacy(t, z, "ann", spec, Options{}, nil, &legacySQL.noSelectIn)
	})

	t.Run("both forms agree on every stored value", func(t *testing.T) {
		const read = `NULLIF({:c}, '') IN (SELECT +value FROM json_each({:list}))`
		for _, stored := range []any{nil, "", "food", "gift", "k2", "fo'od"} {
			var same int
			err := z.app.DB().NewQuery(`SELECT (` + read + ` AND {:c} IN ({:a}, {:b}, NULL)) IS (` + read + `)`).
				Bind(dbx.Params{"c": stored, "list": `["food","gift"]`, "a": "food", "b": "gift"}).Row(&same)
			if err != nil {
				t.Fatal(err)
			}
			if same != 1 {
				t.Fatalf("stored %#v: the two forms differ", stored)
			}
		}
	})
}

// The stored-column term is written, and changes no result, on the other
// kinds of source: a Select whose column has another name than its key, a
// Select of a view source, and a Select of a union source. Each case also
// gives the result of the old form, byte for byte.
func TestASelectInAListOnOtherSourcesKeepsItsResult(t *testing.T) {
	in := func(field, list string) string {
		return inOf(`{"path":[{"field":"`+field+`"}]}`, `{"value":`+list+`}`)
	}
	report := func(source, field, filter string) string {
		return `{"version":4,"source":"` + source + `","columns":[{"id":"c","path":[{"field":"` + field + `"}]}],
			"filter":` + filter + `,"sort":[{"col":"c"}]}`
	}
	check := func(t *testing.T, z *zoo, spec string, want []any) {
		t.Helper()
		seen := observeReads(t)
		res := z.run(t, "ann", spec, Options{}, nil)
		observeRead = nil
		if storedInTerms(*seen) == 0 {
			t.Fatalf("no statement holds a stored-column term (%d reads)", len(*seen))
		}
		if got := detailCells(res, "c"); !reflect.DeepEqual(got, want) {
			t.Fatalf("rows %#v, want %#v", got, want)
		}
		requireSameAsLegacy(t, z, "ann", spec, Options{}, nil, &legacySQL.noSelectIn)
	}
	t.Run("a select whose column has another name", func(t *testing.T) {
		z := newZoo(t)
		addZooNotes()
		check(t, z, report("zoo.notes", "kind", in("kind", `["f1","f2"]`)), []any{"f1", "f2"})
		check(t, z, report("zoo.notes", "kind", notOf(in("kind", `["f1"]`))), []any{"f2", "f3", "f4"})
	})
	t.Run("a select of a view source", func(t *testing.T) {
		z := newZoo(t)
		addZooFeedLog(t, z)
		check(t, z, report("zoo.feed_log", "note", in("size", `["big"]`)), []any{"f1", "f2"})
		check(t, z, report("zoo.feed_log", "note", notOf(in("size", `["big"]`))), []any{"f3", "f4", "f5", "f6"})
	})
	t.Run("a select of a union source", func(t *testing.T) {
		z := newUnionZoo(t)
		check(t, z, report("zoo.meals", "kind", in("kind", `["feeding"]`)), []any{"feeding", "feeding", "feeding", "feeding"})
		check(t, z, report("zoo.meals", "kind", notOf(in("kind", `["feeding"]`))), []any{"treat", "treat"})
	})
}

// The stored-column term binds one placeholder for each distinct value of
// the list, and no statement of a run holds a value as SQL text, in any
// quoting. A list that names one value 40,000 times binds it once: a
// placeholder for each item would pass SQLite's 32,766 variables.
func TestASelectInAListIsBound(t *testing.T) {
	z := newZoo(t)
	withStalls(t, z, false)
	many := `["gift"` + strings.Repeat(`,"food"`, 40000) + `]`
	cases := []struct {
		name, raw string
		params    map[string]string
		table     string
		values    []string
	}{
		{"a linked select in a list", salesAmounts(kindIn(`["food","gift"]`)), nil, "zoo_stalls", []string{"food", "gift"}},
		{"a linked select in a param's list", salesOfKinds, map[string]string{"kinds": `["gift","food"]`}, "zoo_stalls", []string{"gift", "food"}},
		{"a select of the report's own row", salesAmounts(stateIn(`["open","paid","s2"]`)), nil, "zoo_sales", []string{"open", "paid", "s2"}},
		{"one value 40,000 times", salesAmounts(kindIn(many)), nil, "zoo_stalls", []string{"gift", "food"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reads := salesReads(t, z, c.raw, c.params)
			terms := 0
			for _, st := range reads {
				for _, v := range c.values {
					for _, quoted := range []string{"'" + v + "'", `"` + v + `"`, "`" + v + "`", "[" + v + "]"} {
						if strings.Contains(st.sql, quoted) {
							t.Errorf("a statement holds %s as SQL text:\n%.2000s", quoted, st.sql)
						}
					}
				}
				for _, m := range storedIn.FindAllStringSubmatch(st.sql, -1) {
					terms++
					var bound []string
					for _, mark := range strings.Split(strings.TrimSuffix(m[1], ", "), ", ") {
						v, ok := st.params[strings.Trim(mark, "{:}")].(string)
						if !ok {
							t.Fatalf("the placeholder %s holds %#v, want a text", mark, st.params[strings.Trim(mark, "{:}")])
						}
						bound = append(bound, v)
					}
					if !reflect.DeepEqual(bound, c.values) {
						t.Errorf("the stored-column term binds %q, want %q", bound, c.values)
					}
				}
				if !strings.Contains(st.sql, c.table) {
					t.Errorf("a statement does not name %s:\n%.2000s", c.table, st.sql)
				}
			}
			if terms == 0 {
				t.Fatalf("no statement holds a stored-column term:\n%.2000s", reads[0].sql)
			}
		})
	}
}

// The values of the stored-column term are bound, and SQLite may plan an
// indexed IN by its bound values only when sqlite_stat4 has rows: without
// statistics a statement with the term is prepared once, as every other
// statement of the engine is (TestEveryReportStatementIsPreparedOnce).
func TestASelectInAListIsPreparedOnceWithoutStatistics(t *testing.T) {
	z := newZoo(t)
	withStalls(t, z, true)
	db := readPool(t, z)
	for _, st := range salesReads(t, z, salesAmounts(kindIn(`["food","gift"]`)), nil) {
		raw, args := rawStatement(t, st.sql, st.params)
		if n := reprepares(t, db, raw, args); n != 0 {
			t.Errorf("%d reprepares, want 0:\n%.300s", n, st.sql)
		}
	}
}
```

- [ ] **Step 2: Run the tests to see them fail**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports && go test -run 'TestASelectIn' -count=1 .
```

Expected: FAIL. `TestASelectInAListIsServedByTheColumnIndex` fails in its 8 plan cases ("the plan scans the sales", "the plan left-joins the stalls"); its 2 "without the stored-column term" cases pass. `TestASelectInAListKeepsItsResult` fails in each case whose `terms` is not 0, in "a param's list" ("0 stored-column terms in the first statement, want 1") and in "a lookup's where" ("the lookup's where holds no stored-column term"); every `sales …, want …` line holds, because the rows are the old engine's rows. `TestASelectInAListOnOtherSourcesKeepsItsResult` fails in its 3 cases and `TestASelectInAListIsBound` in its 4 cases ("no statement holds a stored-column term"). 29 failed subtests in all. `TestASelectInAListIsPreparedOnceWithoutStatistics` passes: it pins a count the change must keep. A build failure is not RED: fix the test file first.

- [ ] **Step 3: `selectin.go` — the condition with the stored-column term**

Create `core/server/reports/selectin.go`:

```go
package reports

import "strings"

// selectIn is in of a Select field with a list of texts that are all option
// values of the field, with the same test on the stored column:
//
//	(NULLIF(c, '') IN (SELECT +value FROM json_each({:p})) AND c IN ({:p}, {:p}, NULL))
//
// A Select reads as NULLIF of its column (fieldSQL) and the list is read back
// from one bound JSON array (jsonSelect). No index on c serves that term, and
// SQLite does not see that it fails for a row with no linked row, so it keeps
// the LEFT JOIN to the linked row and scans the rows it could search. The
// second term is one a plain index on c serves and one that is never true
// for a NULL c, so the planner may start at the linked rows.
//
// The second list binds one placeholder for each distinct value. The values
// are option values, so their count is at most the count of the field's
// options, far under SQLite's 32,766 variables. No value is ever written
// into the SQL text.
//
// The NULL at the end of the second list keeps the value of the condition.
// Where c is in the list, both terms are 1. Where c is another text, the
// first term is 0. Where c is NULL (no linked row, or one the reader cannot
// see) or blank, the first term is NULL, and so is the second (x IN (…, NULL)
// is NULL, not 0, for an x the list does not hold): the condition stays
// unknown, as a column and eq with false read it. Without the NULL a blank c
// would give 0.
//
// "" when a is no such Select, or the list holds another value or no value,
// and in is written as before.
func selectIn(bind func(any) string, a, list expr) string {
	if legacySQL.noSelectIn || a.sel.col == "" || a.sel.sql != a.sql || len(list.listed) == 0 {
		return ""
	}
	seen := map[string]bool{}
	for _, v := range list.listed {
		if !storedOption(a.options, v) {
			return ""
		}
		seen[v] = true
	}
	marks := make([]string, 0, len(seen)+1)
	for _, v := range list.listed {
		if seen[v] {
			marks = append(marks, bind(v))
			delete(seen, v)
		}
	}
	marks = append(marks, "NULL")
	return "(" + a.sql + " IN " + list.sql + " AND " + a.sel.col + " IN (" + strings.Join(marks, ", ") + "))"
}
```

The first loop binds nothing: when one value is no option value, the function returns before any placeholder exists, so the statement gets no unused parameter. The second loop binds each value at its first place in the list.

- [ ] **Step 4: `formula.go` and `params.go` — a list keeps its texts, and `in` asks `selectIn`**

In `core/server/reports/formula.go`, in the `expr` struct, after the field `text`:

```go
	// text is a text literal's value.
	text *string
	// listed are the values of a list that holds texts only (a literal list
	// or a multiple choice param); in tests a Select's stored column against
	// them too (selectIn). nil for any other value.
	listed []string
```

In `literal`, the `[]any` case becomes (three changes: the `listed` variable, its `append`, and the return):

```go
	case []any:
		values := make([]any, 0, len(x))
		var typ FieldType
		var listed []string
		for _, item := range x {
			switch y := item.(type) {
			case string:
				values = append(values, y)
				listed = append(listed, y)
				typ = Text
			case json.Number:
				values = append(values, numberValue(y))
				typ = Number
			default:
				return expr{}, invalid(at, "a value list holds text or numbers")
			}
		}
		list, err := jsonList(c.bind, values)
		if err != nil {
			return expr{}, err
		}
		if len(listed) != len(values) {
			listed = nil
		}
		return expr{sql: list, typ: typ, list: true, listed: listed}, nil
```

In `case "in"`, before the last `return`:

```go
	case "in":
		if err := argc(2, 2); err != nil {
			return expr{}, err
		}
		if !args[1].list {
			return expr{}, invalid(at+".args.1", "in needs a list")
		}
		if args[1].sql == "()" {
			return expr{sql: "0", typ: Bool}, nil
		}
		if sql := selectIn(c.bind, args[0], args[1]); sql != "" {
			return expr{sql: sql, typ: Bool}, nil
		}
		return expr{sql: "(" + args[0].sql + " IN " + args[1].sql + ")", typ: Bool}, nil
```

The empty list returns before `selectIn`, as before. An unset multiple choice param has an empty `v.list`, so its `sql` is `"()"` and it also returns before `selectIn`.

In `core/server/reports/params.go`, the multiple choice list (the one line with `list: true, unset: v.unset`):

```go
			return expr{sql: list, typ: Text, list: true, unset: v.unset, listed: v.list}, nil
```

The list of an option-groups param (`options.go:188`) gets no `listed`: its values are read by a subquery, not known when the statement is written.

- [ ] **Step 5: Run the new tests and their neighbours to see them pass**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports && gofmt -l . && go vet . && go test -run 'TestASelectIn|TestASelectEqualToAText|TestSelectOverTextFiltersWithIn|TestAListComparesAsBoundValues|TestALongChoiceListReadManyTimesRuns|TestEveryReportStatementIsPreparedOnce' -count=1 .
```

Expected: `gofmt` and `go vet` print nothing; then `ok  	tinycld.org/core/reports`.

- [ ] **Step 6: Prove that the result tests catch a changed value**

The mutation is made on a COPY of `selectin.go`, read through `go test -overlay`; the repository file is never changed. Copy `selectin.go` to your scratchpad. In the copy, with the Edit tool, change the last line of `selectIn` to (the stored-column term reads unknown as false):

```go
	return "(" + a.sql + " IN " + list.sql + " AND COALESCE(" + a.sel.col + " IN (" + strings.Join(marks, ", ") + "), 0))"
```

Write `{"Replace":{"/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/selectin.go":"<the copy>"}}` to a JSON file in your scratchpad, then:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports && go test -overlay <the json> -run 'TestASelectInAListKeepsItsResult' -count=1 .
```

Expected: FAIL in `TestASelectInAListKeepsItsResult/in_is_false` (planner's run: that one subtest). A run that passes, or `[build failed]`, is a finding: stop and report it. Record the failing test name in your report, and remove the copy and the JSON file.

- [ ] **Step 7: The comment of the prepare test, the whole package, the Books Go tests, commit**

In `core/server/reports/statement_test.go`, the comment of `TestEveryReportStatementIsPreparedOnce` names `selectEq` as the one bound value SQLite plans by. Replace its lines

```go
// prepared once. The engine's own SQL binds one value SQLite plans by: the
// stored-column term c = {:p} of an eq of a Select with an option value
// (selectEq), which SQLite prepares again when sqlite_stat4 has rows. That
// value is bound because the engine never writes a value into the SQL text.
```

with

```go
// prepared once. The engine's own SQL binds one value SQLite plans by: the
// stored-column term c = {:p} of an eq of a Select with an option value
// (selectEq), which SQLite prepares again when sqlite_stat4 has rows. That
// value is bound because the engine never writes a value into the SQL text.
// The term c IN ({:p}, …, NULL) of an in of a Select with a list of option
// values (selectIn) binds its values the same way; its prepare count with
// no statistics is pinned by TestASelectInAListIsPreparedOnceWithoutStatistics.
```

The first four lines stay as they are: three lines are added after them. The planner measured no second prepare for the `selectIn` term in the zoo, with no statistics and after `ANALYZE`, so the comment claims none.

Run the whole core package (about 130 s for the tests):

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go build ./... && cd reports && gofmt -l . && go vet . && go test -count=1 .
```

Expected: no output from the build, `gofmt` and the vet, then `ok  	tinycld.org/core/reports`. The package's `TestMain` (`topics_check_test.go`) fails the package when a statement's topics miss a collection the whole statement reads: `ok` also proves that no statement lost a topic.

Run the Books Go tests as CI runs them, three shards (about 2 to 3 minutes each; the workspace's `go.work` makes Books build against this core tree). One command for each shard, in the foreground:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books/server && ../.github/scripts/go-test-shard.sh 1 3
```

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books/server && ../.github/scripts/go-test-shard.sh 2 3
```

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books/server && ../.github/scripts/go-test-shard.sh 3 3
```

Expected: `ok  	tinycld.org/packages/books` three times. These shards hold the plan tests and figure tests of Profit and Loss, Trial Balance, Balance Sheet, General Ledger and Statements, whose filters now carry the term. A failure there is a finding of this task: find the statement and its plan (the test prints them), and fix the cause in `selectIn` or its guard. Never change a Books test's expected figure or plan bar, and never skip a test. When the cause is not in this task's change, hand back `STOPPED: <test> fails: <the first failure line>`.

Then check that core names no package and commit:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld && ! command grep -n -i -E 'books|hosting|tenant|saas|router' core/server/reports/selectin.go core/server/reports/selectin_test.go core/server/reports/formula.go core/server/reports/params.go core/server/reports/legacy.go core/server/reports/statement_test.go && git add core/server/reports/selectin.go core/server/reports/selectin_test.go core/server/reports/formula.go core/server/reports/params.go core/server/reports/legacy.go core/server/reports/statement_test.go && git commit -m "perf(reports): in of a Select with a list of its option values tests the stored column too"
```

Expected: the grep prints nothing; one commit of six files. When the grep prints a line, no commit is made: remove the word and run the command again. Do not push.

- [ ] **Step 8: Hand the two scale runs to the driver**

Hand back the two `NEEDS-RUN` lines below as they are written, with the quiet-machine condition and the two "Needed from" paragraphs under them, and stop. Step 9 is for the agent the driver starts with the logs. Both runs need a quiet machine (`pgrep -fl 'chrome-headless-[s]hell'` prints nothing and the first number of `sysctl -n vm.loadavg` is under 4).

```
NEEDS-RUN: mkdir -p /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task31-st && sysctl -n vm.loadavg > /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task31-st/load.txt && cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task31-st go test -run 'TestStatementsScale$/100k$' -count=1 -timeout 60m -v . > /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task31-st.log 2>&1
NEEDS-RUN: mkdir -p /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task31-gl && sysctl -n vm.loadavg > /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task31-gl/load.txt && cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task31-gl go test -run 'TestGLScale$/500k$' -count=1 -timeout 120m -v . > /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task31-gl.log 2>&1
```

Needed from the first: PASS; `scale-task31-st/st-results.md` (every median; no run refused; the group rows and rows of each run the same as in `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9e2/st-results.md`, the last 100k round) and `scale-task31-st/st-plans.md`.

Needed from the second: PASS; `scale-task31-gl/gl-results.md` (every median; no run refused; the sections on the page of each run the same as in `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-500k-final-gl/gl-results.md`, the last 500k round) and `scale-task31-gl/gl-plans.md`.

For both: a run over its bar, a refused run, or a changed row count is `STOPPED: run <name> is <ms> at <size>` (or the changed count) with the statement times and the plan of the longest statement from the plans file; the bar is never lowered and the term is not taken out to pass.

- [ ] **Step 9: The ledger record (Books), from the driver's logs**

In `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md`, at the end of the file (after the record "Task 9e-2a: a list rule that reads no row is answered once for each run"), add the record below. Fill each `<…>` from the file named beside it; write no figure that is not in those files.

```markdown
#### Task 31: `in` of a Select with a list of its option values tests the stored column too (core `server/reports/selectin.go`, 2026-11-21)

The cause (found by the Expenses by Vendor unit's plan bars): `in(account.type, [three types])` read the type as `NULLIF(type, '')` and the list from one bound JSON array. No index serves that term and SQLite does not see that a row with no account fails it, so it kept the LEFT JOIN to the accounts and scanned `books_journal_lines` and `books_cash_basis_lines`. `eq` had a stored-column term since `selectEq`; `in` had none.

The change (core): `selectIn` writes `(NULLIF(c, '') IN (<the list>) AND c IN ({:p}, …, NULL))` when the left side is a Select read from a stored column and every value of the list is one of its option values. One placeholder is bound for each distinct value; no value is written into the SQL text. The `NULL` at the end of the list keeps the value of the condition unknown for a blank or missing `c`, as before. A list with another value, an empty list, and a column's formula are written as before.

The tests (core `selectin_test.go`): `TestASelectInAListIsServedByTheColumnIndex`, `TestASelectInAListKeepsItsResult`, `TestASelectInAListOnOtherSourcesKeepsItsResult`, `TestASelectInAListIsBound`, `TestASelectInAListIsPreparedOnceWithoutStatistics`.

Code: core <the commit of step 7, `git -C /Users/nas/tmp/tinycld/books-purchases/tinycld log --oneline -1 -- core/server/reports/selectin.go`>.

Books reports whose filters carry the term: Profit and Loss, Trial Balance, Balance Sheet, General Ledger (`account.type`) and Statements (`entry.origin`). The Books Go tests pass on it (three shards).

The 100k round (`scale-task31-st/st-results.md`; before = `scale-task9e2/st-results.md`):

| lines | run | before | after |
|---|---|---|---|
<one row for each of the eleven runs of `st-results.md`: `| 100000 | <run> | <median in scale-task9e2> | <median in scale-task31-st> |`>

The General Ledger at 500k (`scale-task31-gl/gl-results.md`; before = `scale-500k-final-gl/gl-results.md`):

| lines | run | before | after |
|---|---|---|---|
<one row for each run of `gl-results.md`: `| 500000 | <run> | <median in scale-500k-final-gl> | <median in scale-task31-gl> |`>

<One sentence: PASS or not for each of the two runs, whether a run is refused, whether the rows, group rows and sections are the same, and the 1-minute load at the start of each (the first number in `scale-task31-st/load.txt` and in `scale-task31-gl/load.txt`).>

For the Expenses by Vendor unit: with this change on core `books/p5-report-engine-scale`, its `TestExpensesByVendorQueryPlans` has no plan-bar failure with no statistics (21 before; planner's overlay run, 2026-11-21).
```

Then commit in Books:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books && git add docs/plans/general-ledger-ledger.md && git commit -m "docs(books): ledger record of task 31 (in of a Select tests the stored column)"
```

Do not push.
