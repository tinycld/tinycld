# Report Engine Scale, Part 20: A Fill Record Shows Under Its Own Parents — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A filled record group whose groups above read only that record puts each record under its own parent rows, also when no base row made that parent, so a statement section with no line in the period shows its accounts and adds them.

**Architecture:** No new spec key. When the first filled group is a record group below one or more groups, and every group above it reads only that record (no bucket, no shown form, no amount key), the compiler sets `compiler.fillOwn`. Level L (the filled group) then emits a CTE `fr<L>`: each fill record with the keys of the groups above computed from the record, under each currency. The keyset of every level 1..L is `its keys in w UNION fr<L>'s keys`. Every other report keeps the present fill (each record under each open parent row).

**Tech Stack:** Go, core report engine `tinycld.org/core/reports` (module at `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server`), SQLite CTEs. Tests run on the fictional zoo (`zoo.feedings`, `zoo.ledger_lines`, `zoo.ledger_months`).

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md`, sections "State after plan part 19" and "State after plan part 20" (task 9c-2a2). The design was built and run before this plan: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/handoff-plan-part20b.md`, and the built files in `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part20-overlay/` (each file there is the wanted end state of the file of the same name; `overlay.json` runs them with `go test -overlay`).

## Global Constraints

- This is a core task: it edits `/Users/nas/tmp/tinycld/books-purchases/tinycld` only. No file of the task names a feature package, its slug, its collections or its routes.
- Work on branch `books/p5-report-engine-scale` of the core repo. Do not push.
- The design is built and run. Do not redesign it; write the code of this plan as given.
- A sum over rows of two currencies is blank with no currency code, and the run carries a `mixedCurrency` notice that names the column. No test may accept a figure that adds two currencies.
- No timing probe and no scale run in this plan.
- Read a file with the Read tool before its first Edit. Never `sed -i` or a scripted replace.
- Run Go commands from `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server`.
- No commit message names an AI tool.

## Review Focus

1. A report whose upper group does not read only the filled record (a column of the base row, a bucket, an amount key, a key with a shown form) must keep the present fill: each record under each open parent. Pinned today by `TestFillWhereReadsABucketKey` (bucket) and `TestFillOnADeeperLevelSitsUnderEachParent` (the first group is filled) in `fill_test.go`, which this plan does not change and which pass with it. NOT pinned by a test of their own: an upper group by a base-row column, by an amount, and by a key with a shown form (the three `nil` branches of `ownParentsOf`), and a closed parent that adds its fill records in a report with a `total: "rows"` column whose fill does NOT name its own parents (`fillsUnder` through `parentKeys`). These four tests are task 9c-2a2-t, plan part 21 (`fill_parents_test.go`), which runs before Task 2 of this part: the planner of part 20 stopped at its token limit before it built them, and the reviewer of this plan named the gap.
2. A record whose own parent key is blank: it sits under `(none)`, as a base row with that blank key does (Task 1, `TestFillWhereReadsTheParentRecordAndParams`), or skips a `blank: "skip"` level (Task 1, `TestAFillRecordSkipsItsOwnBlankParentLevel`, `TestRaggedFillFillsUnderTheRecordsOwnParents`).
3. A fill record in another currency than the report's rows: its parent is blank with no code and the run says so (Task 1, `TestAFillRecordOfAnotherCurrencyBlanksItsOwnParent`).
4. A run with no base row on a source with no currency: the records get no parent, so no section adds several currencies (Task 1, `TestFillRecordsNameTheirParentsWhenNoBaseRowPasses`).
5. A closed parent and a paged run: a closed section adds its fill records, and the pages hold each row once (Task 1, `TestAClosedSectionAddsItsFillRecords`, `TestFillRecordsUnderTheirOwnParentsPage`).

## File Structure

| File | Task | What it is for |
|---|---|---|
| `core/server/reports/fill_parents.go` (new) | 1 | The own-parents plan: when it applies, the record's own keys, `fr<n>`, the keysets that read it. |
| `core/server/reports/fill_parents_test.go` (new) | 1 | The behavior on the ledger zoo: Equity section with no line. |
| `core/server/reports/fill.go` | 1 | `fillKeys` returns the records too; `fillCtx` reads the record's own keys. |
| `core/server/reports/levels.go` | 1 | `level.fillRecords`; the keysets of the levels above; `base` emits `fr<n>`. |
| `core/server/reports/rows.go` | 1 | `compiler.fillOwn`, set after the group keys compile. |
| `core/server/reports/fill_test.go` | 1 | One old test: wanted rows change by design. |
| `core/server/reports/fillplan_test.go` | 1 | The plan check learns `fr<n>`. |
| `core/server/reports/ragged_test.go` | 1 | One old test: replaced, by design. |
| `docs/packages.md` | 2 | Rows `groups[].fill`, `columns[].rollup`, `groups[].blank`. |
| `core/server/reports/rollup_rows_layout_test.go` | 2 | A header comment that the change makes false. |

Two tasks, because the unit's limit is 8 files for a task: the three old tests go red with the code, so they stay in Task 1; the documentation is Task 2.

## Evidence for the three old tests (run in the overlay, 2026-11-20)

- `fill_test.go` `TestFillWhereReadsTheParentRecordAndParams` got `East, North, Leo, Lia, South, Zed, (none), Total`. `East` is a site with an animal (Eli) and no feeding: the designed change. `(none)` is NOT a defect: Kit has no site, so its own site key is NULL, and the where `site.name = {record s}.name` compiles to `[[t7.name]] IS (SELECT [[t8.name]] FROM r2 t8 WHERE [[t8.id]] IS NULLIF([[t7.id]], ''))`. `eq` is `IS` in this engine, so two blanks are equal and Kit passes. A feeding of Kit sits under `(none)` too (`w` has `k0 = NULL` for it), and with the tree's code Kit fills under a `(none)` parent when one exists. So a record with a blank own parent key sits under `(none)`, as its base row does. The test's wanted rows change; no code changes.
- `fillplan_test.go` case "a fill under a group with no fill…": the plan is `MATERIALIZE fr2` with the children `CO-ROUTINE p`, `SCAN p`, `SCAN zoo_animals`, and `f1`, `f2` are `w UNION (SCAN fr2)`. The join is in `fr2`, once, with p outer. The check must read `fr<n>` and pass over a keyset that only reads it.
- `ragged_test.go` `TestRaggedFillFillsUnderTheParentTheRowsReach` wanted Leo and Zed (a lion, a zebra) under "cat". By design a record now sits under its own species and region only. The test is replaced by one that shows the new rule with animals that have no feeding.

---

### Task 1: A fill record shows under its own parents

**Files:**
- Create: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/fill_parents.go`
- Create (test): `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/fill_parents_test.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/fill.go` (`parentKeys`, `fillKeys`, `nextFill`, `fillCtx`)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/levels.go` (`level` struct near line 45, `level()` near line 119, `base()` near line 436)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/rows.go` (`compiler` struct near line 93, `rows()` near lines 322-356)
- Modify (test): `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/fill_test.go:279-299`
- Modify (test): `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/fillplan_test.go`
- Modify (test): `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/ragged_test.go:228-249`

**Interfaces:**
- Consumes (exist in the package): `rowsRollupLayoutSpec(having string) string`, `rowsRollupZoo(t) *zoo`, `rowsRollupLines(res) []rowsRollupLine`, `requireRowsRollupLines(t, got, want)`, `rowsRollupHaving`, `withLedgerCurrency(t, code)`, `withEmptyAnimals(t, z)`, `raggedZoo(t)`, `zooRagged`, `requireRows(t, res, what, want...)`; `(c *compiler) parentKeys`, `noRowCurrency`, `groupTargets`, `groupShown`, `fillSets`; `expansion.mayShowUnder(c, s, depth, "")`; `compileFormula(j, node, at)`.
- Produces: `type ownParents struct{ level int; keys []*Node }`; `compiler.fillOwn *ownParents`; `(c *compiler) ownParentsOf(types []FieldType) *ownParents`; `(c *compiler) ownKeys(r *rowsCTE, sc *scope, target *Source, n int) ([]expr, error)`; `fillRecords(n int) string` (`"fr<n>"`); `(c *compiler) ownFillKeys(s *stmt, m, n int, expand *expansion) string`; `(c *compiler) fillsUnder(s *stmt, depth int, expand *expansion) string`; `(c *compiler) ownParentCurrencies(s *stmt, expand *expansion) string`; `(c *compiler) filled(n int) bool`; `fillKeys` returns `(keyset, records string, err error)`; `level.fillRecords string`. Test helpers `ownParentsSpec(having)`, `ownParentsZoo(t, currency)`, `ownParentsNoLine(spec)`, `ownParentsGroups(res)`.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/fill_parents_test.go`:

```go
package reports

import (
	"slices"
	"strings"
	"testing"
)

// The statement by account type with a third section, Equity. Its one account,
// 3900, has a stored month of June (credit 700) and no line of July, so no
// base row gives the report an Equity row: the account names its own section.

const ownParentsBefore = "Balance before July"

func ownParentsSpec(having string) string {
	return strings.Replace(rowsRollupLayoutSpec(having), `{"row":"owed"`, `{"section":"equity","title":"Equity","key":"equity"},{"row":"owed"`, 1)
}

func ownParentsZoo(t *testing.T, currency string) *zoo {
	t.Helper()
	z := rowsRollupZoo(t)
	z.rec(t, "reJune", "zoo_ledger_months", map[string]any{
		"account": z.id("re"), "first_day": "2026-06-01", "debit_total": 0, "credit_total": 700, "currency": currency,
	})
	return z
}

// ownParentsNoLine moves the period to August, where no line passes.
func ownParentsNoLine(spec string) string {
	return strings.Replace(spec, `"2026-07-01"`, `"2026-08-01"`, 1)
}

// ownParentsGroups drops the detail and opening rows.
func ownParentsGroups(res *Result) []rowsRollupLine {
	var out []rowsRollupLine
	for _, line := range rowsRollupLines(res) {
		if line.Kind != KindDetail && line.Kind != KindOpening {
			out = append(out, line)
		}
	}
	return out
}

// The lines of the Assets and Liabilities sections, as in
// TestARowsRollupTotalsUnderLayoutSections.
func ownParentsUpper() []rowsRollupLine {
	return []rowsRollupLine{
		{KindGroup, "Assets", int64(300), int64(6500), int64(-6200), "USD"},
		{KindGroup, "1000", nil, int64(6500), int64(-6500), "USD"},
		{KindOpening, ownParentsBefore, int64(500), nil, nil, "USD"},
		{KindDetail, "1000", nil, int64(10000), int64(-10000), "USD"},
		{KindDetail, "1000", int64(3000), nil, int64(3000), "USD"},
		{KindGroup, "1300", int64(300), nil, int64(300), "USD"},
		{KindOpening, ownParentsBefore, int64(300), nil, nil, "USD"},
		{KindSubtotal, "Total Assets", int64(300), int64(6500), int64(-6200), "USD"},
		{KindGroup, "Liabilities", int64(0), int64(4500), int64(-4500), "USD"},
		{KindGroup, "2000", nil, int64(4500), int64(-4500), "USD"},
		{KindOpening, ownParentsBefore, nil, nil, nil, "USD"},
		{KindDetail, "2000", nil, int64(4500), int64(-4500), "USD"},
		{KindSubtotal, "Total Liabilities", int64(0), int64(4500), int64(-4500), "USD"},
	}
}

func ownParentsEquity() []rowsRollupLine {
	return []rowsRollupLine{
		{KindGroup, "Equity", int64(0), int64(700), int64(-700), "USD"},
		{KindGroup, "3900", nil, int64(700), int64(-700), "USD"},
		{KindOpening, ownParentsBefore, nil, int64(700), nil, "USD"},
		{KindSubtotal, "Total Equity", int64(0), int64(700), int64(-700), "USD"},
	}
}

// 3900 shows under Equity, a section no line of July reaches, and the section
// and its subtotal add it. The computed row reads Assets and Liabilities only.
func TestAFillRecordShowsUnderItsOwnParentWithNoBaseRow(t *testing.T) {
	z := ownParentsZoo(t, "USD")
	res := z.run(t, "ann", ownParentsSpec(rowsRollupHaving), Options{ExpandAll: true}, nil)
	want := append(ownParentsUpper(), ownParentsEquity()...)
	want = append(want, rowsRollupLine{KindComputed, "Assets and liabilities", int64(300), int64(11000), int64(-10700), "USD"})
	requireRowsRollupLines(t, rowsRollupLines(res), want)
}

// 3900's month is in EUR and the report's rows are in USD: the Equity section
// would show a EUR figure under the USD code, so the account row, the section
// and its subtotal are blank with no code, and the run names the columns.
func TestAFillRecordOfAnotherCurrencyBlanksItsOwnParent(t *testing.T) {
	z := ownParentsZoo(t, "EUR")
	res := z.run(t, "ann", ownParentsSpec(""), Options{ExpandAll: true}, nil)
	got := ownParentsGroups(res)
	i := slices.IndexFunc(got, func(l rowsRollupLine) bool { return l.Label == "Equity" })
	if i < 0 {
		t.Fatalf("no Equity row in %#v", got)
	}
	requireRowsRollupLines(t, got[i:], []rowsRollupLine{
		{KindGroup, "Equity", nil, nil, nil, nil},
		{KindGroup, "3900", nil, nil, nil, nil},
		{KindSubtotal, "Total Equity", nil, nil, nil, nil},
		{KindComputed, "Assets and liabilities", nil, nil, nil, nil},
	})
	for _, col := range []string{"debit", "credit", "gap"} {
		if !slices.ContainsFunc(res.Notices, func(n Notice) bool {
			return n.Code == "mixedCurrency" && slices.Contains(n.Columns, col)
		}) {
			t.Fatalf("notices = %+v, want mixedCurrency naming %s", res.Notices, col)
		}
	}
}

// A closed section reads the same as an open one: it adds its fill records
// (1300 in Assets, 3900 in Equity) as it adds its base rows, and shows no row
// under it.
func TestAClosedSectionAddsItsFillRecords(t *testing.T) {
	z := ownParentsZoo(t, "USD")
	spec := strings.Replace(ownParentsSpec(rowsRollupHaving), `"open":true,`, ``, 1)
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRowsRollupLines(t, rowsRollupLines(res), []rowsRollupLine{
		{KindGroup, "Assets", int64(300), int64(6500), int64(-6200), "USD"},
		{KindSubtotal, "Total Assets", int64(300), int64(6500), int64(-6200), "USD"},
		{KindGroup, "Liabilities", int64(0), int64(4500), int64(-4500), "USD"},
		{KindSubtotal, "Total Liabilities", int64(0), int64(4500), int64(-4500), "USD"},
		{KindGroup, "Equity", int64(0), int64(700), int64(-700), "USD"},
		{KindSubtotal, "Total Equity", int64(0), int64(700), int64(-700), "USD"},
		{KindComputed, "Assets and liabilities", int64(300), int64(11000), int64(-10700), "USD"},
	})
}

// With no line in the period every account row is a fill row. A source with a
// currency gives the records their parents in it: each section adds its
// accounts (2000 has net 0 and is hidden). A source with no currency gives
// them no parent: a section would add months of USD, EUR and GBP as one figure
// with no code and no notice, so the sections stay empty, as before.
func TestFillRecordsNameTheirParentsWhenNoBaseRowPasses(t *testing.T) {
	t.Run("the source has a currency", func(t *testing.T) {
		z := ownParentsZoo(t, "USD")
		withLedgerCurrency(t, "USD")
		res := z.run(t, "ann", ownParentsNoLine(ownParentsSpec(rowsRollupHaving)), Options{ExpandAll: true}, nil)
		want := []rowsRollupLine{
			{KindGroup, "Assets", int64(800), int64(0), int64(800), "USD"},
			{KindGroup, "1000", int64(500), nil, int64(500), "USD"},
			{KindOpening, ownParentsBefore, int64(500), nil, nil, "USD"},
			{KindGroup, "1300", int64(300), nil, int64(300), "USD"},
			{KindOpening, ownParentsBefore, int64(300), nil, nil, "USD"},
			{KindSubtotal, "Total Assets", int64(800), int64(0), int64(800), "USD"},
			{KindGroup, "Liabilities", int64(0), int64(0), int64(0), "USD"},
			{KindSubtotal, "Total Liabilities", int64(0), int64(0), int64(0), "USD"},
		}
		want = append(want, ownParentsEquity()...)
		want = append(want, rowsRollupLine{KindComputed, "Assets and liabilities", int64(800), int64(0), int64(800), "USD"})
		requireRowsRollupLines(t, rowsRollupLines(res), want)
	})
	t.Run("the source has no currency", func(t *testing.T) {
		z := ownParentsZoo(t, "USD")
		res := z.run(t, "ann", ownParentsNoLine(ownParentsSpec("")), Options{ExpandAll: true}, nil)
		requireRowsRollupLines(t, rowsRollupLines(res), []rowsRollupLine{
			{KindGroup, "Assets", int64(0), int64(0), int64(0), nil},
			{KindSubtotal, "Total Assets", int64(0), int64(0), int64(0), nil},
			{KindGroup, "Liabilities", int64(0), int64(0), int64(0), nil},
			{KindSubtotal, "Total Liabilities", int64(0), int64(0), int64(0), nil},
			{KindGroup, "Equity", int64(0), int64(0), int64(0), nil},
			{KindSubtotal, "Total Equity", int64(0), int64(0), int64(0), nil},
			{KindComputed, "Assets and liabilities", int64(0), int64(0), int64(0), nil},
		})
		if len(res.Notices) != 0 {
			t.Fatalf("notices = %+v, want none", res.Notices)
		}
	})
}

// Paged, the rows of the pages are the rows of the whole run in order, less
// the computed row each page repeats: the Equity section and 3900 are rows of
// the last page, not rows a page loses.
func TestFillRecordsUnderTheirOwnParentsPage(t *testing.T) {
	z := ownParentsZoo(t, "USD")
	owed := rowsRollupLine{KindComputed, "Assets and liabilities", int64(300), int64(11000), int64(-10700), "USD"}
	opts := Options{Limit: 1, ExpandAll: true}
	var merged []rowsRollupLine
	pages := 0
	for {
		res := z.run(t, "ann", ownParentsSpec(rowsRollupHaving), opts, nil)
		lines := rowsRollupLines(res)
		if len(lines) == 0 || lines[len(lines)-1] != owed {
			t.Fatalf("page %d ends in %#v, want the computed row", pages+1, lines)
		}
		merged = append(merged, lines[:len(lines)-1]...)
		pages++
		if res.Page == nil || res.Page.Next == "" {
			break
		}
		if pages == 20 {
			t.Fatal("the pages do not end")
		}
		opts.Cursor = res.Page.Next
	}
	requireRowsRollupLines(t, merged, append(ownParentsUpper(), ownParentsEquity()...))
	if pages < 2 {
		t.Fatalf("pages = %d, want more than one", pages)
	}
}

// A skip level between the section and the account: an asset account is
// "Current", any other has a blank class. 3900's own class is blank, so it
// sits directly under Equity, as 2000 sits directly under Liabilities.
func TestAFillRecordSkipsItsOwnBlankParentLevel(t *testing.T) {
	z := ownParentsZoo(t, "USD")
	spec := ownParentsSpec(rowsRollupHaving)
	spec = strings.Replace(spec, `{"id":"day"`, `{"id":"cls","formula":{"op":"if","args":[{"op":"eq","args":[{"col":"kind"},{"value":"asset"}]},{"value":"Current"},{"value":""}]}},{"id":"day"`, 1)
	spec = strings.Replace(spec, `{"id":"g","col":"account"`, `{"id":"c","col":"cls","blank":"skip","open":true},{"id":"g","col":"account"`, 1)
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	requireRowsRollupLines(t, ownParentsGroups(res), []rowsRollupLine{
		{KindGroup, "Assets", int64(300), int64(6500), int64(-6200), "USD"},
		{KindGroup, "Current", int64(300), int64(6500), int64(-6200), "USD"},
		{KindGroup, "1000", nil, int64(6500), int64(-6500), "USD"},
		{KindGroup, "1300", int64(300), nil, int64(300), "USD"},
		{KindSubtotal, "Total Assets", int64(300), int64(6500), int64(-6200), "USD"},
		{KindGroup, "Liabilities", int64(0), int64(4500), int64(-4500), "USD"},
		{KindGroup, "2000", nil, int64(4500), int64(-4500), "USD"},
		{KindSubtotal, "Total Liabilities", int64(0), int64(4500), int64(-4500), "USD"},
		{KindGroup, "Equity", int64(0), int64(700), int64(-700), "USD"},
		{KindGroup, "3900", nil, int64(700), int64(-700), "USD"},
		{KindSubtotal, "Total Equity", int64(0), int64(700), int64(-700), "USD"},
		{KindComputed, "Assets and liabilities", int64(300), int64(11000), int64(-10700), "USD"},
	})
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./reports/ -run 'TestAFillRecord|TestAClosedSection|TestFillRecords'`
Expected: FAIL. Six tests fail on their rows: `TestAFillRecordShowsUnderItsOwnParentWithNoBaseRow` prints `rows =` with no `Equity`, `3900` or `Total Equity` line; `TestAFillRecordOfAnotherCurrencyBlanksItsOwnParent`, `TestAClosedSectionAddsItsFillRecords`, `TestFillRecordsNameTheirParentsWhenNoBaseRowPasses/the_source_has_a_currency`, `TestFillRecordsUnderTheirOwnParentsPage` and `TestAFillRecordSkipsItsOwnBlankParentLevel` fail the same way. The subtest `the_source_has_no_currency` PASSES: it pins that this case stays as it is. (Recorded run: `part20-overlay/red.log`.)

- [ ] **Step 3: Create `fill_parents.go`**

Create `core/server/reports/fill_parents.go`:

```go
package reports

import (
	"fmt"
	"slices"
	"strings"
)

// A filled record group whose every group above reads only that record (the
// account's type above the account) lets each record name its own parents: a
// record with no base row then shows under its own parent rows, also when no
// base row gave the report that parent (a section with no line in the period).
// The parents no longer come from w: fr<L> holds each fill record with the
// keys of the groups above computed from the record, and every level from the
// first to L unites its keys in w with fr<L>'s.

// ownParents is the plan of such a report: level is the filled record group's
// level (2 or more), and keys holds, for each group above it, the group's key
// as a formula on the record.
type ownParents struct {
	level int
	keys  []*Node
}

// ownParentsOf returns the plan when the first filled group is a record group
// below at least one group, and every group above it has no bucket, no shown
// form, is not an amount, and reads only the record. types are the group
// keys' types. nil leaves every fill as it is: under each parent row.
func (c *compiler) ownParentsOf(types []FieldType) *ownParents {
	f := slices.IndexFunc(c.p.groups, func(g *groupPlan) bool { return g.group.Fill != nil })
	if f < 1 || c.groupTargets[f] == nil {
		return nil
	}
	link := c.p.groups[f].col.col.Path
	if len(link) == 0 || slices.ContainsFunc(link, func(h Hop) bool { return h.Link == "" || h.Mode != "" }) {
		return nil
	}
	own := &ownParents{level: f + 1}
	for i, g := range c.p.groups[:f] {
		if g.group.Bucket != "" || c.groupShown[i] || types[i] == Money {
			return nil
		}
		key, ok := c.onRecord(&Node{Col: g.col.col.ID}, link, map[string]bool{})
		if !ok {
			return nil
		}
		own.keys = append(own.keys, key)
	}
	return own
}

// onRecord rewrites a row formula as one on the record link points at: a path
// through link loses that prefix, and a {col} becomes its column's own
// rewrite. It is false for a formula that reads anything but the record,
// params and values: another path, a total, a running value, linked rows, a
// lookup, a group's record, a subtotal.
func (c *compiler) onRecord(n *Node, link Path, seen map[string]bool) (*Node, bool) {
	if n == nil {
		return nil, false
	}
	if n.Record != "" || n.Subtotal != "" || n.Source != "" || n.GroupKey != "" || n.PivotBound != "" ||
		len(n.Link) > 0 || len(n.Outer) > 0 || n.Where != nil {
		return nil, false
	}
	switch n.Op {
	case "sum", "count", "countDistinct", "avg", "min", "max", "exists", "notExists",
		"runningSum", "percentOfParent", "percentOfTotal", "percentOfSection":
		return nil, false
	}
	if n.Col != "" {
		cp := c.p.byID[n.Col]
		if cp == nil || seen[n.Col] || cp.col.Aggregate != "" || cp.nest != 0 {
			return nil, false
		}
		if len(cp.col.Path) > 0 {
			return c.onRecord(&Node{Path: cp.col.Path}, link, seen)
		}
		seen[n.Col] = true
		defer delete(seen, n.Col)
		return c.onRecord(cp.col.Formula, link, seen)
	}
	out := *n
	if len(n.Path) > 0 {
		if len(n.Path) <= len(link) || slices.ContainsFunc(n.Path, func(h Hop) bool { return h.Mode != "" }) {
			return nil, false
		}
		for i, h := range link {
			if n.Path[i].Link != h.Link {
				return nil, false
			}
		}
		out.Path = n.Path[len(link):]
		return &out, true
	}
	out.Args = make([]*Node, 0, len(n.Args))
	for _, a := range n.Args {
		arg, ok := c.onRecord(a, link, seen)
		if !ok {
			return nil, false
		}
		out.Args = append(out.Args, arg)
	}
	return &out, true
}

// ownKeys compiles the keys of the groups above level n on the fill record
// (scope sc), as w holds them: a skip level's blank key is NULL. nil when the
// records of level n do not name their own parents.
func (c *compiler) ownKeys(r *rowsCTE, sc *scope, target *Source, n int) ([]expr, error) {
	if c.fillOwn == nil || c.fillOwn.level != n {
		return nil, nil
	}
	keys := make([]expr, 0, n-1)
	for i, node := range c.fillOwn.keys {
		g := c.p.groups[i]
		j := &joinCtx{r: r, sc: sc, src: target, visiting: map[string]bool{}}
		e, err := compileFormula(j, node, g.at+".col")
		if err != nil {
			return nil, err
		}
		if g.group.Blank == GroupBlankSkip {
			e.sql = "NULLIF(" + e.sql + ", '')"
		}
		keys = append(keys, e)
	}
	return keys, nil
}

// fillRecords names the CTE that holds the fill records of level n with their
// own parents' keys.
func fillRecords(n int) string {
	return fmt.Sprintf("fr%d", n)
}

// ownFillKeys is the keyset of level m (1 to n) when the records of level n
// name their own parents: its keys in w, and those of the fill records whose
// enclosing groups are open. A record under a closed group makes that group's
// row and no row of its own.
func (c *compiler) ownFillKeys(s *stmt, m, n int, expand *expansion) string {
	keys := make([]string, 0, m+len(c.currencies))
	for i := range m {
		keys = append(keys, fmt.Sprintf("k%d", i))
	}
	cols := strings.Join(append(keys, c.currencies...), ", ")
	return fmt.Sprintf("SELECT %s FROM w UNION SELECT %s FROM %s WHERE %s",
		cols, cols, fillRecords(n), c.fillsUnder(s, m-1, expand))
}

// fillsUnder is the condition on a parent row (depth groups deep) under which
// its fill records are computed: the row shows and is open. A report with a
// "rows" column computes them under every parent: a closed group's cell adds
// its rows, as it adds its base rows, so the group reads the same closed and
// open.
func (c *compiler) fillsUnder(s *stmt, depth int, expand *expansion) string {
	if slices.ContainsFunc(c.p.cols, func(cp *colPlan) bool { return cp.col.Total == TotalRows }) {
		return "1"
	}
	return expand.mayShowUnder(c, s, depth, "")
}

// ownParentCurrencies are the currencies the fill records that name their own
// parents sit under: those of the base rows, or the sources' when there is no
// base row (parentKeys). When a currency key has no source currency, a run
// with no base row gives the records no parent: the parent rows would name no
// currency, and their "rows" cells would add the records' months of several
// currencies into one figure with no code and no notice (totalOverFillRows).
func (c *compiler) ownParentCurrencies(s *stmt, expand *expansion) string {
	if slices.ContainsFunc(c.currencies, func(ck string) bool { return c.noRowCurrency(ck) == nil }) {
		return fmt.Sprintf("SELECT DISTINCT %s FROM w", strings.Join(c.currencies, ", "))
	}
	return c.parentKeys(s, 1, expand)
}

// filled is true when level n's rows include keys with no base row: its group
// fills in its records, or a filled group below names it as a parent.
func (c *compiler) filled(n int) bool {
	if n < 1 || n > len(c.p.groups) {
		return false
	}
	return c.p.groups[n-1].group.Fill != nil || (c.fillOwn != nil && n < c.fillOwn.level)
}
```

- [ ] **Step 4: Edit `rows.go`**

(a) In the `compiler` struct, directly after the field `fillSets map[int]string` (line 93), add:

```go
	// fillOwn is set when the first filled group's records name their own
	// parents (ownParentsOf): they show under their own parent rows, also
	// when no base row made that parent.
	fillOwn *ownParents
```

(b) In the function that compiles the group keys, replace

```go
	c.groupOptions, c.groupShown = make([][]Option, len(c.p.groups)), make([]bool, len(c.p.groups))
	for i, g := range c.p.groups {
		e, err := r.rowValue(g.col, g.at+".col", map[string]bool{})
		if err != nil {
			return nil, err
		}
		if g.group.Bucket != "" {
```

with

```go
	c.groupOptions, c.groupShown = make([][]Option, len(c.p.groups)), make([]bool, len(c.p.groups))
	keyTypes := make([]FieldType, len(c.p.groups))
	for i, g := range c.p.groups {
		e, err := r.rowValue(g.col, g.at+".col", map[string]bool{})
		if err != nil {
			return nil, err
		}
		keyTypes[i] = e.typ
		if g.group.Bucket != "" {
```

(c) The same loop ends in `r.add(fmt.Sprintf("k%d", i), e.sql)` and its closing brace. Replace

```go
		r.add(fmt.Sprintf("k%d", i), e.sql)
	}
```

with

```go
		r.add(fmt.Sprintf("k%d", i), e.sql)
	}
	c.fillOwn = c.ownParentsOf(keyTypes)
```

`diff core/server/reports/rows.go part20-overlay/rows.go` must print nothing when the step is done.

- [ ] **Step 5: Edit `fill.go`**

(a) In `parentKeys`, replace

```go
	expanded := expand.mayShowUnder(c, s, n-1, "")
```

with

```go
	expanded := c.fillsUnder(s, n-1, expand)
```

(b) Replace the whole of `fillKeys` (its comment's last paragraph is new) from its signature to its closing brace. Keep the comment above it and add this paragraph at the comment's end:

```go
//
// When the records name their own parents (ownParentsOf), p is the currencies
// only, the keys above are each record's own, and the second result is those
// records (the CTE fr<n>), which the keysets of the levels above read too.
```

The function:

```go
func (c *compiler) fillKeys(r *rowsCTE, n int, expand *expansion, at string) (string, string, error) {
	target := c.groupTargets[n-1]
	if target == nil {
		return "", "", invalid(at, "only a group by a link to one record shows records with no rows")
	}
	sc := newScope(r.s, target)
	own, err := c.ownKeys(r, sc, target, n)
	if err != nil {
		return "", "", err
	}
	where, mixed := "", ""
	if w := c.p.groups[n-1].group.Fill.Where; w != nil {
		j := &joinCtx{r: r, sc: sc, src: target, isFilter: true, visiting: map[string]bool{}}
		e, err := compileWhere(&fillCtx{joinCtx: j, n: n, own: own}, w, at)
		if err != nil {
			return "", "", err
		}
		// A blank condition is NULL, which WHERE drops as it drops false.
		if !e.unset {
			where = "\nWHERE " + e.sql
		}
		if e.condMixed != "" {
			mixed = e.condMixed
		}
	}
	keys := make([]string, 0, n+len(c.currencies))
	picked := make([]string, 0, n+len(c.currencies))
	for i := range n - 1 {
		keys = append(keys, fmt.Sprintf("k%d", i))
		if own != nil {
			picked = append(picked, fmt.Sprintf("%s AS k%d", own[i].sql, i))
			continue
		}
		picked = append(picked, fmt.Sprintf("p.k%d AS k%d", i, i))
	}
	keys = append(keys, fmt.Sprintf("k%d", n-1))
	picked = append(picked, q(sc.alias, "id")+fmt.Sprintf(" AS k%d", n-1))
	for _, ck := range c.currencies {
		keys = append(keys, ck)
		picked = append(picked, "p."+ck+" AS "+ck)
	}
	from := strings.TrimPrefix(sc.render(), "FROM ")
	parents := c.parentKeys(r.s, n, expand)
	if own != nil {
		parents = c.ownParentCurrencies(r.s, expand)
	}
	if mixed != "" {
		c.probe(c.p.groups[n-1].at+".fill.where", fmt.Sprintf("SELECT 1 FROM (%s) p\nCROSS JOIN %s\nWHERE %s", parents, from, mixed))
	}
	records := fmt.Sprintf("SELECT %s\nFROM (%s) p\nCROSS JOIN %s%s", strings.Join(picked, ", "), parents, from, where)
	if own != nil {
		return c.ownFillKeys(r.s, n, n, expand), records, nil
	}
	return fmt.Sprintf("SELECT %s FROM w UNION %s", strings.Join(keys, ", "), records), "", nil
}
```

(c) Replace `nextFill` and its comment with:

```go
// nextFill is true when level n+1 has rows with no base row (filled), so a
// level-n row with no base rows still has rows to open onto.
func (c *compiler) nextFill(n int) bool {
	return c.filled(n + 1)
}
```

(d) Replace the `fillCtx` struct, and add to its comment:

```go
//
// own, when set, holds the keys of the groups above as the record's own
// values (ownKeys): there is no parent row to read them from, and each is
// what the record's row will have.
type fillCtx struct {
	*joinCtx
	n   int
	own []expr
}
```

(e) In `(f *fillCtx) col`, inside the loop, directly after the `if g.col.col.ID != id { continue }` block, add:

```go
		if f.own != nil {
			return f.own[i], nil
		}
```

(f) In `(f *fillCtx) record`, directly after `key := fmt.Sprintf("p.k%d", gi)`, add:

```go
	if f.own != nil {
		key = f.own[gi].sql
	}
```

`diff core/server/reports/fill.go part20-overlay/fill.go` must print nothing when the step is done.

- [ ] **Step 6: Edit `levels.go`**

(a) In the `level` struct (near line 45) replace

```go
	fill  string
	frame *windowFrame
```

with

```go
	fill string
	// fillRecords is the level's fill records with their own parents' keys
	// (fillKeys), "" when they sit under each parent row; base emits it as the
	// CTE fr<L>, which the keysets of this level and the levels above read.
	fillRecords string
	frame       *windowFrame
```

(b) In `level()` (near line 119) replace

```go
		fill, err := c.fillKeys(r, n, expand, c.p.groups[n-1].at+".fill")
```

with

```go
		fill, records, err := c.fillKeys(r, n, expand, c.p.groups[n-1].at+".fill")
```

and, at the end of the same `if` block, replace

```go
		lv.fill = fill
		c.fillSets[n] = fmt.Sprintf("f%d", n)
	}
```

with

```go
		lv.fill, lv.fillRecords = fill, records
		c.fillSets[n] = fmt.Sprintf("f%d", n)
	} else if c.filled(n) {
		// A parent of the filled records: its keys in w and the records' own.
		lv.fill = c.ownFillKeys(r.s, n, c.fillOwn.level, expand)
		c.fillSets[n] = fmt.Sprintf("f%d", n)
	}
```

(c) In `base()`, directly after the line `var ctes []string` and before `if lv.fill != "" {`, add:

```go
	if lv.fillRecords != "" {
		// Materialized: the keyset of every level down to this one reads it.
		ctes = append(ctes, fmt.Sprintf("%s AS MATERIALIZED (%s)", fillRecords(lv.n), lv.fillRecords))
	}
```

`diff core/server/reports/levels.go part20-overlay/levels.go` must print nothing when the step is done.

- [ ] **Step 7: Run the new tests, and see the three old tests fail**

Run: `go vet ./reports/ && go test ./reports/ -run 'TestAFillRecord|TestAClosedSection|TestFillRecords'`
Expected: vet prints nothing; `ok  tinycld.org/core/reports`.

Run: `go test ./reports/ -run 'TestFillWhereReadsTheParentRecordAndParams|TestAFillKeysetLoopsOverItsParentRowsFirst|TestRaggedFillFillsUnderTheParentTheRowsReach'`
Expected: FAIL, three tests: `fill_test.go:296: label = [East North Leo Lia South Zed (none) Total]`; `fillplan_test.go:155: fill keysets in the plans: [MATERIALIZE f1: SCAN fr2 MATERIALIZE f2: SCAN fr2], want 1`; `ragged_test.go:247: under cat: [Kit]`. Each is settled in "Evidence for the three old tests" above.

- [ ] **Step 8: Change the three old tests**

(a) `fill_test.go`, `TestFillWhereReadsTheParentRecordAndParams`. Replace its comment

```go
// A fill's where reads an enclosing group's record and params; a where on an
// unset param fills in every record the reader may list.
```

with

```go
// A fill's where reads an enclosing group's record and params; a where on an
// unset param fills in every record the reader may list. The site group reads
// only the animal, so each animal names its own site: East has an animal (Eli)
// and no feeding, and Kit has no site, so it sits under "(none)", where a
// feeding of Kit would sit ("eq" holds for two blanks).
```

and replace the line

```go
	requireRows(t, all, "label", "North", "Leo", "Lia", "South", "Zed", "Total")
```

with

```go
	requireRows(t, all, "label", "East", "North", "Leo", "Lia", "South", "Zed", "(none)", "Total")
	open := Options{Expand: [][]any{{z.id("east"), "USD"}, {nil, "USD"}}}
	requireRows(t, z.run(t, "ann", spec, open, nil), "label", "East", "Eli", "North", "South", "(none)", "Kit", "Total")
```

The `zebras` lines below it stay as they are.

(b) `fillplan_test.go`. After the `parentLoop` variable add:

```go
// fillRecordsCTE is the fill records of a level whose records name their own
// parents (fr<n>, fill_parents.go): the join of p with the target's records
// is in it, and the keysets f<n> only read it.
var fillRecordsCTE = regexp.MustCompile(`^(?:MATERIALIZE|CO-ROUTINE) fr\d+$`)

// readsFillRecords is the loop of a keyset that reads fr<n>.
var readsFillRecords = regexp.MustCompile(`^SCAN fr\d+(?: |$)`)

// firstLoop is the first table a select loops over, "" when it has no loop.
func firstLoop(rows []planRow) string {
	for _, r := range rows {
		if strings.HasPrefix(r.detail, "SCAN ") || strings.HasPrefix(r.detail, "SEARCH ") {
			return r.detail
		}
	}
	return ""
}
```

In `fillOuterLoops` replace

```go
		for _, f := range plan {
			if !fillCTE.MatchString(f.detail) {
```

with

```go
		for _, f := range plan {
			if fillRecordsCTE.MatchString(f.detail) {
				outer = append(outer, f.detail+": "+firstLoop(children(f.id)))
				continue
			}
			if !fillCTE.MatchString(f.detail) {
```

and replace

```go
			loop := ""
			for _, r := range children(branches[len(branches)-1].id) {
				if strings.HasPrefix(r.detail, "SCAN ") || strings.HasPrefix(r.detail, "SEARCH ") {
					loop = r.detail
					break
				}
			}
			outer = append(outer, f.detail+": "+loop)
```

with

```go
			loop := firstLoop(children(branches[len(branches)-1].id))
			// A keyset that reads fr<n> joins nothing: fr<n> is the one checked.
			if readsFillRecords.MatchString(loop) {
				continue
			}
			outer = append(outer, f.detail+": "+loop)
```

Above the case `{"a fill under a group with no fill, its where reads links and the parent record", 1, func…` add the comment:

```go
		// 1: the animals name their own site, so the join is in fr2 (once), and
		// f1 and f2 read it.
```

The case keeps `1`: the one entry is now `MATERIALIZE fr2: SCAN p`, which `parentLoop` accepts.

(c) `ragged_test.go`. Replace the whole of `TestRaggedFillFillsUnderTheParentTheRowsReach` and its comment (lines 228-249) with:

```go
// A fill on the animal level below the skipped region. The species and the
// region read only the animal, so an animal with no feeding fills in under its
// own species and region: Lia (a lion at North) beside Leo, Eli under a species
// and a region no feeding reaches, and Pip (a cat with no site) directly under
// "cat" beside Kit, where its rows would go. Leo and Zed are not cats, so they
// do not fill in under "cat".
func TestRaggedFillFillsUnderTheRecordsOwnParents(t *testing.T) {
	z := raggedZoo(t)
	withEmptyAnimals(t, z)
	z.rec(t, "pip", "zoo_animals", map[string]any{"name": "Pip", "species": "cat", "owner": z.id("ann"), "active": true})
	raw := strings.Replace(fmt.Sprintf(zooRagged, ""), `{"id":"a","col":"animal","subtotals":true}`, `{"id":"a","col":"animal","subtotals":true,"fill":{}}`, 1)
	res := z.run(t, "ann", raw, Options{}, nil)
	requireRows(t, res, "label", "cat", "Kit", "Pip", "Total cat", "elk", "North", "Eli", "Total North", "Total elk",
		"lion", "North", "Leo", "Lia", "Total North", "Total lion", "zebra", "South", "Zed", "Total South", "Total zebra", "Total")
	requireRows(t, res, "depth", 0, 1, 1, 0, 0, 1, 2, 1, 0, 0, 1, 2, 2, 1, 0, 0, 1, 2, 1, 0, 0)
	requireRows(t, res, "level", 1, 3, 3, 1, 1, 2, 3, 2, 1, 1, 2, 3, 3, 2, 1, 1, 2, 3, 2, 1, 0)
}
```

(Eli's site East has the region `north`, which shows as "North".) `reflect` stays imported: other tests of the file use it.

- [ ] **Step 9: Run the changed tests, then the package**

Run: `go vet ./reports/ && gofmt -l reports/ && go test ./reports/ -run 'TestFillWhereReadsTheParentRecordAndParams|TestAFillKeysetLoopsOverItsParentRowsFirst|TestRaggedFill'`
Expected: vet and gofmt print nothing; `ok`.

Run: `go test ./reports/`
Expected: `ok  tinycld.org/core/reports` (about 130 s; recorded run: `part20-overlay/whole-package-green.log`).

- [ ] **Step 10: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/fill_parents.go core/server/reports/fill_parents_test.go core/server/reports/fill.go core/server/reports/levels.go core/server/reports/rows.go core/server/reports/fill_test.go core/server/reports/fillplan_test.go core/server/reports/ragged_test.go
git commit -m "feat(reports): a fill record shows under its own parents"
```

---

### Task 2: Document the own-parents fill

**Files:**
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/docs/packages.md:1552` (row `groups[].fill`), `:1570` (row `columns[].rollup`), `:1596` (row `groups[].blank`)
- Modify (test comment): `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/rollup_rows_layout_test.go:9-13`

**Interfaces:**
- Consumes: the behavior of Task 1 (its tests are the proof of each sentence below).
- Produces: nothing a later task reads.

- [ ] **Step 1: Check the sentences against the tests**

Run: `go test ./reports/ -run 'TestAFillRecord|TestAClosedSection|TestFillRecords|TestRaggedFill|TestFillWhereReadsTheParentRecordAndParams|TestARowsRollup'`
Expected: `ok`. Each sentence of steps 2-4 states what one of these tests asserts; if a test fails, stop and report, do not change the text to fit.

- [ ] **Step 2: `docs/packages.md` line 1552, row `groups[].fill`**

Replace the first sentence of the cell

```
`{ where? }` lists the target's records that have no rows, on any level, under each parent group.
```

with

```
`{ where? }` lists the target's records that have no rows, on any level, under each parent group that is open. When the first filled group is below other groups and each of those groups reads only the filled group's record (a path through the group's link, or a formula over such paths, params and values; no `bucket`, not an amount, not a key that mixes a select's values with other values), each record sits under its own parent rows only, also when no row gave the report that parent: the parent row then shows with the totals of no rows. A record whose own parent key is blank sits under `(none)`, as its rows do. A `{col}` or `{record}` of a group above in the `where` then reads the record's own parent. In every other report a record sits under each open parent row the `where` admits. In a report with a `total: "rows"` column a parent adds its fill records also while it is closed, with any fill.
```

At the end of the same cell, after `so that Total has no cells.`, add:

```
 In that case (no row at all, a source with no currency) records that name their own parents are not listed.
```

- [ ] **Step 3: `docs/packages.md` line 1570, row `columns[].rollup`**

Replace the two sentences

```
A fill of a group below the section needs a where that ties the record to its section. A fill adds rows only under a section that has a row of its own in the period, so a section with no line in the period shows no account row under it and 0 in its cells.
```

with

```
A filled record group below a section group that reads only that record (the account's type above the account) puts each record under its own section, also a section with no row of its own in the period: the section, its subtotal and a computed row that reads it add the record's cells. A record in another currency than the report's rows makes its section blank, with a `mixedCurrency` notice. When the run reads no row at all and the source has no currency, the sections show no record and 0.
```

- [ ] **Step 4: `docs/packages.md` line 1596, row `groups[].blank`**

Replace the sentence

```
A fill below a skipped level fills under the parent the rows reach.
```

with

```
A fill record that names its own parents (see `groups[].fill`) skips the level when its own key there is blank, as its rows do; any other fill below a skipped level fills under the parent the rows reach.
```

- [ ] **Step 5: `rollup_rows_layout_test.go` header comment**

Replace

```go
// account fills in under its own type, so an account with a stored month and
// no line of July has a row, under a section that has a line of July; a
// section with no line of July shows no account row.
```

with

```go
// account fills in under its own type, so an account with a stored month and
// no line of July has a row, also under a section no line of July reaches
// (fill_parents_test.go).
```

- [ ] **Step 6: Check and commit**

Run: `gofmt -l reports/ && go vet ./reports/ && go test ./reports/ -run 'TestARowsRollup'`
Expected: gofmt and vet print nothing; `ok`.

Run: `command grep -c 'A fill below a skipped level fills under' /Users/nas/tmp/tinycld/books-purchases/tinycld/docs/packages.md`
Expected: `0` (the old sentence of line 1596 is gone).

Run: `command grep -c 'names its own parents' /Users/nas/tmp/tinycld/books-purchases/tinycld/docs/packages.md`
Expected: `1` (the new sentence of line 1596).

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add docs/packages.md core/server/reports/rollup_rows_layout_test.go
git commit -m "docs(reports): a fill record shows under its own parents"
```
