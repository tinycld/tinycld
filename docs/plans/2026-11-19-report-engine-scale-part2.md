# Report Engine Query Scaling Implementation Plan — part 2 (tasks 2 and 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A General Ledger run with an Accounts filter takes 1 s or less on a book with 500,000 journal lines, with every report's result unchanged.

**Architecture:** This part makes the first two engine changes the baseline of part 1 points at. Task 2: the report filter also writes bounds on the base table's stored columns, so SQLite reads the report's rows through an index and not every base row (requirement 3). Task 3: the formula operator `startOfMonth`, which task 4 needs to read the stored monthly balances. Each SQL change keeps a `legacySQL` switch and a cross-check on the fictional zoo.

**Tech Stack:** Go, SQLite (PocketBase fork, dbx, modernc.org/sqlite; SQLite 3.54), core report engine `tinycld.org/core/reports`, TypeScript (the report builder's operator lists), vitest.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/from-p2-write-off-report-sql.md` (the root cause; sections "Why report runs, and not other steps, slowed down so much" and "App (contributing…)"). The unit's five requirements, the baseline and the stored-summary decision: part 1, `/Users/nas/tmp/tinycld/books-purchases/tinycld/docs/plans/2026-11-19-report-engine-scale.md`. Outline of all 7 tasks: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md`.

**Repos:** workspace `/Users/nas/tmp/tinycld/books-purchases`. Core is `tinycld/` (branch `books/p5-report-engine-scale`); Books is `books/` (same branch name). Tasks 2 and 3 are core tasks: they may edit `tinycld/core`. Task 2 also changes two Books files (the scale harness of task 1 and the ledger record).

**Order:** task 1 (part 1) is done before this part: `books/server/report_general_ledger_scale_test.go` exists. Then task 2, then task 3. Tasks 4 to 7 are part 3 (section "What is left" at the end).

## Global Constraints

- The bar is never lowered: runs 3, 4 and 5 of `TestGLScale` (the runs with an Accounts filter) take 1 s or less at 500,000 lines, median of 5. No task changes a bar, removes a run, or changes the scale book to pass.
- Every report's rows stay the same. An engine change is proven by a cross-check (new SQL against the `legacySQL` switch) on the zoo, and one test proves the switch writes the old SQL.
- Core names no package: core tests use the fictional zoo only (`zoo.ledger_lines`, `zoo.crates`, `zoo.meals`); no Books slug, collection or route in `tinycld/core`.
- Server logging goes through the package's `log` (`logging.ForPackage`); no manual message prefixes.
- Each change to SQL is recorded with its EXPLAIN plan and its time before and after in `books/docs/plans/general-ledger-ledger.md`, section "Performance (p5-report-engine-scale)", under "Changes".
- The 500k run takes about 13 minutes: an implementer never starts it. The 100k run of one size takes more than 4 minutes with its build (measured by the planner, see task 2 step 7): it is a `NEEDS-RUN` for the driver too.
- Go code is formatted with `gofmt`. TypeScript: 4-space indent, single quotes, no semicolons (biome).

## Review Focus

1. A filter list that names an account the reader cannot see, and a row whose link is blank: the bound must not add or drop a row; the filter still decides. Test: task 2, `TestFilterBoundsKeepADayWithATimeAndLeaveTheFilterToDecide` (ann picks leo and mo and sees only leo's crate; the crate with no animal shows only with no list).
2. A day stored with a time after it (`2026-03-05 23:59:59.999Z`) on the last day of a range, and under `lte`, `lt` and `gte` alone: the bound keeps or drops it as the filter does. Test: task 2, the same test (`leo late`, `mo late`; the `since` and `before` params).
3. An unset param, an empty list, All dates: no bound, or a bound that changes nothing. Test: task 2, the param sets of the same test (`nil`, `{"pets": "[]"}`) and the `allDates` run of `TestFilterBoundsKeepTheResult`.
4. A base that is a union, and a dry compile (Validate, no database): no bound and no read of the database. Tests: task 2, `TestFilterBoundsLeaveAUnionAlone`; the existing `TestAliaserTargetsMatchTheCompiler` compiles `w` with no app and panics when `filterBounds` reads the collection there.
5. `startOfMonth` of a blank day, and of a date-time: blank stays blank; a date-time gives a day. Tests: task 3, the shared case "startOfMonth of a blank day is blank" and the `DateTime` column of `TestCompileFormulaTypes`.

---

### Task 2: The filter bounds the base table (core, requirement 3)

Today `w` (the report's rows) reads every row of the base table, also with an Accounts filter and a period: the filter compares `NULLIF(t2.id, '')` of the joined account and `substr()` of the day, and no index serves either. This task writes, before the filter in `w`'s WHERE, bounds on the base row's own stored columns that the filter implies. The filter stays, so the rows are the same.

What gives a bound (only a term of the filter's top-level `and`, or the filter itself):
- `in(X, list)`, X a column or path of one to-one link that ends at the link, the list a param or a constant → `<base>.<link column> IN <list>`.
- `between(D, range param)`, and `lt`, `lte`, `gt`, `gte(D, day)`, D a `Date` field of the base row itself, the day a `date` or `asOf` param or a bound of a range param → `<base>.<column> >= lo AND <base>.<column> <= (hi || x'ff')` (one side for `lt`/`lte`/`gt`/`gte`).

What gives none: a base that is a view or has union parts, a dry compile, an unset param, All dates, an empty list, a `DateTime` field (its comparison is zoned), a day typed as a constant (the compiler types it as text), a link column that is not a relation field, a day column that is not a text, date or autodate field, any other term (also a term under `or` or `not`).

**Files:**
- Create: `tinycld/core/server/reports/bounds.go`
- Create: `tinycld/core/server/reports/bounds_test.go`
- Modify: `tinycld/core/server/reports/legacy.go` (the struct, line 6-10)
- Modify: `tinycld/core/server/reports/rows.go` (`rowsCTE` struct line 226-241; `rows()` line 351-360; `render()` line 616-626)
- Modify: `books/server/report_general_ledger_scale_test.go` (`TestGLScale`: plans for every size)
- Modify: `books/docs/plans/general-ledger-ledger.md` (append under "### Changes")

**Interfaces:**
- Consumes (existing, core `server/reports`): `legacySQL` (legacy.go); `requireSameAsLegacy(t *testing.T, z *zoo, user, spec string, opts Options, params map[string]string, off *bool)` (aggonce_test.go); `withBalanceZoo(t testing.TB, z *zoo)`, `balanceSpec() string`, `queryPlan(t testing.TB, db *sql.DB, stmt string, args []any) []string` (balance_shape_test.go); `withLedgerScale(t testing.TB, z *zoo, n int)`, `withLedgerLines(t testing.TB, z *zoo, n int, mixed bool)`, `ledgerID(prefix string, i int) string`, `ledgerRuns() []ledgerRun`, `runLedger(z *zoo, params map[string]string, opts Options) (*Result, error)`, const `ledgerPeriod` (ledger_scale_test.go); `observeReads(t *testing.T) *[]readStatement`, `rawStatement(tb testing.TB, stmt string, params dbx.Params) (string, []any)`, `readPool(t *testing.T, z *zoo) *sql.DB` (statement_test.go); `newZoo`, `newUnionZoo`, `(z *zoo) rec/save/id/run`, `rule`, `requireRows`.
- Consumes (Books, task 1): `TestGLScale`, `glScaleRuns`, `glScaleStatements`.
- Produces (for task 6): `dayBounds(col, lo, hi string) string` (`""` leaves a side open; returns `col >= lo AND col <= (hi || x'ff')`), `linkBound(col, list string) string`, `rowFreeLeaf(n *Node) bool`, `(c *compiler) filterBounds(r *rowsCTE) (string, error)`, the switch `legacySQL.noFilterBounds`. Test helpers for tasks 5 and 6: `ledgerAccounts(ids ...int) string`, `requireRowsWhere(t *testing.T, seen []readStatement, starts bool, prefix string)`, `withCrates(t *testing.T, z *zoo)`, `cratesSpec(z *zoo) string`. `TestGLScale` writes `gl-plans.md` for every size that ran, with headings `## <size> lines, run <name>`.

- [ ] **Step 1: Add the switch**

In `tinycld/core/server/reports/legacy.go`, replace

```go
	repeatAggregates bool
}
```

with

```go
	repeatAggregates bool
	// noFilterBounds leaves the bounds on the base table's stored columns out
	// of w's WHERE, as before filterBounds wrote them.
	noFilterBounds bool
}
```

- [ ] **Step 2: Write the failing tests**

Create `tinycld/core/server/reports/bounds_test.go`:

```go
package reports

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// rowsWhere finds the WHERE of w, the report's rows, in a statement.
var rowsWhere = regexp.MustCompile(`(?s)\bw AS (?:MATERIALIZED )?\(SELECT .*?\nWHERE ([^\n]*)`)

// requireRowsWhere requires that w's WHERE starts with prefix (or, with
// starts false, does not) in every statement that has the report's rows, and
// that one or more has them.
func requireRowsWhere(t *testing.T, seen []readStatement, starts bool, prefix string) {
	t.Helper()
	found := 0
	for _, st := range seen {
		m := rowsWhere.FindStringSubmatch(st.sql)
		if m == nil {
			continue
		}
		found++
		if got := strings.HasPrefix(m[1], prefix); got != starts {
			t.Fatalf("w's WHERE starts with %q: %v, want %v:\n%s", prefix, got, starts, m[1])
		}
	}
	if found == 0 {
		t.Fatal("no statement has the report's rows")
	}
}

// ledgerAccounts is an accounts param: the ledger fixture's accounts ids.
func ledgerAccounts(ids ...int) string {
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = fmt.Sprintf("%q", ledgerID("acct", id))
	}
	return "[" + strings.Join(quoted, ",") + "]"
}

// The report filter names an account list and a period: w reads the lines
// through the (account, date) index, not every line.
func TestAFilterOnALinkAndADayBoundsTheBaseTable(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	withLedgerScale(t, z, 40)
	params := map[string]string{"period": ledgerPeriod, "accounts": ledgerAccounts(0, 2, 4)}
	seen := observeReads(t)
	if _, err := runLedger(z, params, Options{Today: "2025-07-15"}); err != nil {
		t.Fatal(err)
	}
	requireRowsWhere(t, *seen, true, "[[t1.date]] >= ")
	db := readPool(t, z)
	// byAccountAndDay counts the statements with the report's rows, and those
	// whose plan reads the lines by account and day range.
	byAccountAndDay := func() (statements, searched int) {
		for _, st := range *seen {
			if !rowsWhere.MatchString(st.sql) {
				continue
			}
			statements++
			raw, args := rawStatement(t, st.sql, st.params)
			plan := strings.Join(queryPlan(t, db, raw, args), "\n")
			if strings.Contains(plan, "SEARCH zoo_ledger_lines USING INDEX idx_zoo_ledger_lines_account_date (account=? AND date>? AND date<?)") {
				searched++
			}
		}
		return statements, searched
	}
	if n, searched := byAccountAndDay(); n == 0 || searched != n {
		t.Fatalf("%d of %d statements read the report's rows by account and day, want every one", searched, n)
	}
	// The switch writes the SQL of before: the cross-checks compare with it.
	legacySQL.noFilterBounds = true
	t.Cleanup(func() { legacySQL.noFilterBounds = false })
	*seen = nil
	if _, err := runLedger(z, params, Options{Today: "2025-07-15"}); err != nil {
		t.Fatal(err)
	}
	requireRowsWhere(t, *seen, false, "[[t1.date]] >= ")
	if n, searched := byAccountAndDay(); n == 0 || searched != 0 {
		t.Fatalf("with the switch on, %d of %d statements read the rows by account and day, want none", searched, n)
	}
}

// Every run of the ledger fixture returns the same pages with the bounds and
// without them: with and without an account list, all dates, a mixed book,
// and a reader with no grant.
func TestFilterBoundsKeepTheResult(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		z := newZoo(t)
		withBalanceZoo(t, z)
		withLedgerLines(t, z, 45, mixed)
		for _, r := range ledgerRuns() {
			opts := r.opts
			opts.Today = "2025-07-15"
			requireSameAsLegacy(t, z, "ann", balanceSpec(), opts, r.params, &legacySQL.noFilterBounds)
		}
		requireSameAsLegacy(t, z, "bob", balanceSpec(), Options{Today: "2025-07-15"},
			map[string]string{"period": ledgerPeriod, "accounts": ledgerAccounts(0, 2)}, &legacySQL.noFilterBounds)
	}
}

// withCrates adds crates that any signed-in reader lists, each for an animal
// (which has its own rule) and packed at a day and time (a PocketBase date
// column): leo's and mo's on 2026-03-05 late in the day, one with no animal
// that day, and leo's again the day after.
func withCrates(t *testing.T, z *zoo) {
	t.Helper()
	crates := core.NewBaseCollection("zoo_crates")
	crates.ListRule, crates.ViewRule = rule(`@request.auth.id != ""`), rule(`@request.auth.id != ""`)
	crates.Fields.Add(
		&core.RelationField{Name: "animal", CollectionId: z.recs["leo"].Collection().Id, MaxSelect: 1},
		&core.DateField{Name: "packed"},
		&core.TextField{Name: "tag"},
	)
	crates.AddIndex("idx_zoo_crates_animal_packed", false, "animal, packed", "")
	z.save(t, crates)
	for _, c := range []struct{ tag, animal, packed string }{
		{"leo late", z.id("leo"), "2026-03-05 23:59:59.999Z"},
		{"mo late", z.id("mo"), "2026-03-05 23:59:59.999Z"},
		{"nobody", "", "2026-03-05 12:00:00.000Z"},
		{"leo next", z.id("leo"), "2026-03-06 00:00:00.000Z"},
	} {
		z.rec(t, c.tag, "zoo_crates", map[string]any{"tag": c.tag, "animal": c.animal, "packed": c.packed})
	}
	RegisterSource(Source{
		Key: "zoo.crates", Collection: "zoo_crates", Label: "Crates",
		Fields: []Field{
			{Key: "tag", Label: "Tag", Type: Text, Aggregates: MinMax},
			{Key: "packed", Label: "Packed", Type: Date, Aggregates: MinMax},
		},
		Links: []Link{{Key: "animal", Label: "Animal", Target: "zoo.animals", Cardinality: ToOne, Field: "animal"}},
	})
}

// cratesSpec lists the crates of a list of animals, packed in a day range and
// up to a day.
func cratesSpec(z *zoo) string {
	return fmt.Sprintf(`{"version":4,"source":"zoo.crates",
	"params":[{"id":"days","type":"dateRange","label":"Days","default":{"preset":"allDates"}},
		{"id":"pets","type":"choice","label":"Pets","multiple":true,"options":[{"value":%q,"label":"Leo"},{"value":%q,"label":"Mo"}]},
		{"id":"day","type":"date","label":"Day"},
		{"id":"since","type":"date","label":"Since"},
		{"id":"before","type":"date","label":"Before"}],
	"columns":[{"id":"tag","path":[{"field":"tag"}]},{"id":"pet","path":[{"link":"animal"}]}],
	"filter":{"op":"and","args":[
		{"op":"between","args":[{"path":[{"field":"packed"}]},{"param":"days"}]},
		{"op":"in","args":[{"col":"pet"},{"param":"pets"}]},
		{"op":"in","args":[{"path":[{"link":"animal"}]},{"param":"pets"}]},
		{"op":"lte","args":[{"path":[{"field":"packed"}]},{"param":"day"}]},
		{"op":"gte","args":[{"path":[{"field":"packed"}]},{"param":"since"}]},
		{"op":"lt","args":[{"path":[{"field":"packed"}]},{"param":"before"}]}]},
	"sort":[{"col":"tag"}]}`, z.id("leo"), z.id("mo"))
}

// A day with a time after it on the last day of a range stays in. A crate of
// an animal the reader cannot see, and a crate with no animal, pass or fail
// as the filter decides, bounded or not. An unset param bounds nothing.
func TestFilterBoundsKeepADayWithATimeAndLeaveTheFilterToDecide(t *testing.T) {
	z := newZoo(t)
	withCrates(t, z)
	spec := cratesSpec(z)
	both := fmt.Sprintf(`[%q,%q]`, z.id("leo"), z.id("mo"))
	const fifth = `{"from":"2026-03-05","to":"2026-03-05"}`
	seen := observeReads(t)
	// ann sees leo and not mo: the bound keeps mo's crate, the filter drops it.
	res := z.run(t, "ann", spec, Options{}, map[string]string{"days": fifth, "pets": both})
	// The last row of each result is the total row, which has no tag.
	requireRows(t, res, "tag", "leo late", nil)
	requireRowsWhere(t, *seen, true, "[[t1.packed]] >= ")
	// With no list, the crate with no animal and the hidden animal's show.
	requireRows(t, z.run(t, "ann", spec, Options{}, map[string]string{"days": fifth}), "tag", "leo late", "mo late", "nobody", nil)
	// lte alone: up to the end of the fifth.
	requireRows(t, z.run(t, "bob", spec, Options{}, map[string]string{"day": `"2026-03-05"`, "pets": both}), "tag", "mo late", nil)
	// gte alone bounds the low side only; lt alone, the high side only.
	*seen = nil
	requireRows(t, z.run(t, "ann", spec, Options{}, map[string]string{"since": `"2026-03-06"`}), "tag", "leo next", nil)
	requireRowsWhere(t, *seen, true, "[[t1.packed]] >= {:")
	*seen = nil
	requireRows(t, z.run(t, "ann", spec, Options{}, map[string]string{"before": `"2026-03-06"`}), "tag", "leo late", "mo late", "nobody", nil)
	requireRowsWhere(t, *seen, true, "[[t1.packed]] <= (")
	for _, user := range []string{"ann", "bob"} {
		for _, params := range []map[string]string{
			nil,
			{"since": `"2026-03-05"`},
			{"since": `"2026-03-06"`, "pets": both},
			{"before": `"2026-03-06"`},
			{"before": `"2026-03-05"`, "pets": both},
			{"since": `"2026-03-05"`, "before": `"2026-03-06"`},
			{"days": fifth},
			{"days": fifth, "pets": both},
			{"pets": both},
			// An empty list.
			{"pets": `[]`, "days": fifth},
			{"pets": both, "day": `"2026-03-05"`},
			{"days": `{"from":"2026-03-06","to":"2026-03-31"}`, "day": `"2026-03-06"`},
		} {
			requireSameAsLegacy(t, z, user, spec, Options{}, params, &legacySQL.noFilterBounds)
		}
	}
}

// A union has no one stored column to bound: w's WHERE is the filter alone.
func TestFilterBoundsLeaveAUnionAlone(t *testing.T) {
	z := newUnionZoo(t)
	spec := `{"version":4,"source":"zoo.meals","params":[{"id":"days","type":"dateRange","label":"Days","default":{"preset":"allDates"}}],
		"columns":[{"id":"type","path":[{"field":"kind"}]}],
		"filter":{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"days"}]},"sort":[{"col":"type"}]}`
	seen := observeReads(t)
	z.run(t, "ann", spec, Options{}, map[string]string{"days": `{"from":"2026-01-01","to":"2026-12-31"}`})
	requireRowsWhere(t, *seen, true, "(substr(")
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run, in `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports`: `go test -run 'TestAFilterOnALink|TestFilterBounds' -count=1 -v .`
Expected: FAIL in two tests, PASS in the two others (they hold on the old SQL too, and must keep holding):

```
    bounds_test.go:56: w's WHERE starts with "[[t1.date]] >= ": false, want true:
--- FAIL: TestAFilterOnALinkAndADayBoundsTheBaseTable
--- PASS: TestFilterBoundsKeepTheResult
    bounds_test.go:175: w's WHERE starts with "[[t1.packed]] >= ": false, want true:
--- FAIL: TestFilterBoundsKeepADayWithATimeAndLeaveTheFilterToDecide
--- PASS: TestFilterBoundsLeaveAUnionAlone
```

- [ ] **Step 4: Write the bounds**

Create `tinycld/core/server/reports/bounds.go`:

```go
package reports

import (
	"fmt"
	"slices"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// filterBounds are bounds on the base table's stored columns that the report
// filter implies, joined with AND, "" when it implies none. The filter reads
// a link through the joined row's id (blank when the reader cannot see the
// row) and a day through substr(), and no index serves either form, so w read
// every base row. A bound names the stored column itself, which an index on
// it serves. The filter stays as it is and still decides each row: a bound
// only leaves out rows the filter refuses.
//
// Only a term of the filter's top-level and (or the filter itself) gives a
// bound: a row the filter keeps passes every such term. The base must be a
// collection that is not a view and has no union parts, as storedLinkCol
// requires of a lookup's source.
func (c *compiler) filterBounds(r *rowsCTE) (string, error) {
	f := c.p.spec.Filter
	// A dry compile (Validate) has no database to read the collection from.
	if f == nil || legacySQL.noFilterBounds || c.dry() || len(c.p.base.Parts) > 0 {
		return "", nil
	}
	col, err := c.app.FindCachedCollectionByNameOrId(c.p.base.Collection)
	if err != nil {
		// Without the bounds the rows are still right, only slower.
		log.Warn("filter not bounded", "collection", c.p.base.Collection, "err", err)
		return "", nil
	}
	if col.IsView() {
		return "", nil
	}
	// stored is true when the collection keeps column name as one of kinds:
	// a bound compares the stored value as text.
	stored := func(name string, kinds ...string) bool {
		field := col.Fields.GetByName(name)
		return field != nil && slices.Contains(kinds, field.Type())
	}
	terms := []*Node{f}
	if f.Op == "and" {
		terms = f.Args
	}
	var bounds []string
	for i, term := range terms {
		if term == nil || len(term.Args) != 2 {
			continue
		}
		at := "filter"
		if f.Op == "and" {
			at = fmt.Sprintf("filter.args.%d", i)
		}
		var bound string
		switch term.Op {
		case "in":
			bound, err = r.listBound(term, at, stored)
		case "between", "lt", "lte", "gt", "gte":
			bound, err = r.dayBound(term, at, stored)
		}
		if err != nil {
			return "", err
		}
		if bound != "" {
			bounds = append(bounds, bound)
		}
	}
	return strings.Join(bounds, " AND "), nil
}

// basePath is the path a filter term's first value reads on the base row: a
// path leaf, or a column that is a path and not a to-many total or a child
// row's. nil for any other value.
func (r *rowsCTE) basePath(n *Node, at string) (*pathPlan, error) {
	switch {
	case n.Op != "" || n.Value != nil || n.Param != "":
		return nil, nil
	case n.Col != "":
		cp := r.c.p.byID[n.Col]
		if cp == nil || cp.path == nil || cp.tm >= 0 || cp.nest > 0 {
			return nil, nil
		}
		return cp.path, nil
	case n.Path != nil:
		return bindValuePath(r.c.p.app, r.c.p.base, n.Path, at, pathValue)
	}
	return nil, nil
}

// rowFreeLeaf is a value that reads nothing of a row: a param (or a bound of
// one) or a constant.
func rowFreeLeaf(n *Node) bool {
	return n.Op == "" && n.Col == "" && n.Path == nil && n.Outer == nil && n.Record == "" && n.Subtotal == "" && (n.Param != "" || n.Value != nil)
}

// listBound is the bound of in(X, list) where X ends at one to-one link of the
// base row: the row's stored relation column is in the list. The filter reads
// the linked row's id through a join on that column, so where it holds the
// column holds that id.
func (r *rowsCTE) listBound(term *Node, at string, stored func(string, ...string) bool) (string, error) {
	p, err := r.basePath(term.Args[0], at+".args.0")
	if err != nil || p == nil || !p.endsAtLink || len(p.hops) != 1 || !rowFreeLeaf(term.Args[1]) {
		return "", err
	}
	if !stored(p.hops[0].link.Field, core.FieldTypeRelation) {
		return "", nil
	}
	list, err := compileFormula(r.join(true), term.Args[1], at+".args.1")
	if err != nil || !list.list || list.unset || list.sql == "()" {
		return "", err
	}
	return linkBound(q(r.root.alias, p.hops[0].link.Field), list.sql), nil
}

// dayBound is the bound of between(D, range) and of lt, lte, gt or gte(D,
// day), where D is a Date field of the base row itself and the other value a
// param's day (a day typed as a constant is text to the compiler and gives no
// bound). The column must hold text or a PocketBase date.
func (r *rowsCTE) dayBound(term *Node, at string, stored func(string, ...string) bool) (string, error) {
	p, err := r.basePath(term.Args[0], at+".args.0")
	if err != nil || p == nil || len(p.hops) != 0 || p.endsAtLink || p.field.Type != Date || !rowFreeLeaf(term.Args[1]) {
		return "", err
	}
	if !stored(p.field.Column, core.FieldTypeText, core.FieldTypeDate, core.FieldTypeAutodate) {
		return "", nil
	}
	v, err := compileFormula(r.join(true), term.Args[1], at+".args.1")
	if err != nil || v.unset || v.all {
		return "", err
	}
	col := q(r.root.alias, p.field.Column)
	switch {
	case term.Op == "between" && v.rng:
		return dayBounds(col, v.lo, v.hi), nil
	case term.Op == "between" || v.rng || v.typ != Date:
		return "", nil
	case term.Op == "lt" || term.Op == "lte":
		return dayBounds(col, "", v.sql), nil
	}
	return dayBounds(col, v.sql, ""), nil
}

// linkBound bounds a stored relation column by a list of ids.
func linkBound(col, list string) string {
	return col + " IN " + list
}

// dayBounds bounds a stored day column by the first (lo) and last (hi) day
// its day may be; "" leaves that side open. The column holds a day
// (2026-07-01) or a day with a time after it (2026-07-01 10:00:00.000Z), and
// its day is its first 10 characters. A day at or after lo is a text at or
// after lo. A day at or before hi is a text at or before hi followed by the
// byte ff, which sorts after every character a time can hold; so the bounds
// hold wherever the comparison of the days does, also for lt and gt, whose
// bounds are only wider.
func dayBounds(col, lo, hi string) string {
	var parts []string
	if lo != "" {
		parts = append(parts, col+" >= "+lo)
	}
	if hi != "" {
		parts = append(parts, col+" <= ("+hi+" || x'ff')")
	}
	return strings.Join(parts, " AND ")
}
```

Notes on this code:
- `c.dry()` and `c.app`: a dry compile (Validate, and `TestAliaserTargetsMatchTheCompiler`) has no database. Do not read the collection through `c.p.app` there: it panics.
- The value side is compiled a second time (`compileFormula(r.join(true), …)`), which binds its values again under new names. That is intended: the filter's own SQL does not change by one character, so the cross-check compares the same filter.
- `lt` and `gt` get the bounds of `lte` and `gte`. They are wider than needed and still right.
- `stored` checks the kind of the collection's column: a bound compares the stored value as text, so a Date field declared over a number column gets none.
- Not covered by a test (read by the reviewer as correct, never run): a reader time zone with these params, a list param that holds a group pick (`groupListSQL`), a `with: required` link hop.

In `tinycld/core/server/reports/rows.go`, three edits.

In the `rowsCTE` struct, replace

```go
	filterMixed string
	// wheres are the lookups' wheres compiled on the report's base rows (in
```

with

```go
	filterMixed string
	// bounds are the bounds on the base table's stored columns that the filter
	// implies (filterBounds), "" for none: w's WHERE holds them before the
	// filter, so SQLite reads the base rows through an index on them.
	bounds string
	// wheres are the lookups' wheres compiled on the report's base rows (in
```

In `rows()`, replace

```go
		r.filter, r.filterMixed = e.sql, e.condMixed
	}
```

with

```go
		r.filter, r.filterMixed = e.sql, e.condMixed
		if r.bounds, err = c.filterBounds(r); err != nil {
			return nil, err
		}
	}
```

In `render()`, replace

```go
	if r.filter != "" {
		sql += "\nWHERE " + r.filter
	}
	return sql + ")"
```

with

```go
	switch {
	case r.filter != "" && r.bounds != "":
		sql += "\nWHERE " + r.bounds + " AND " + r.filter
	case r.filter != "":
		sql += "\nWHERE " + r.filter
	}
	return sql + ")"
```

Do not add the bounds in `probeWheres` (rows.go:253): it probes over the rows the filter decides on, and it is not a read the baseline names.

- [ ] **Step 5: Run the tests to verify they pass**

Run, in `tinycld/core/server/reports`: `go vet . && go test -run 'TestAFilterOnALink|TestFilterBounds|TestAliaserTargets' -count=1 -v .`
Expected: `--- PASS` for `TestAliaserTargetsMatchTheCompiler`, `TestAFilterOnALinkAndADayBoundsTheBaseTable`, `TestFilterBoundsKeepTheResult`, `TestFilterBoundsKeepADayWithATimeAndLeaveTheFilterToDecide`, `TestFilterBoundsLeaveAUnionAlone`; `ok`.

Then the package (105 s on the planner's machine; it holds the goldens of the ledger fixture and the statement pins): `go test -count=1 .`
Expected: `ok  	tinycld.org/core/reports`.

- [ ] **Step 6: The scale run writes its plans for every size (Books)**

`TestGLScale` writes `gl-plans.md` only at 500k, so a run of the 100k size alone records no plan. In `/Users/nas/tmp/tinycld/books-purchases/books/server/report_general_ledger_scale_test.go`, in `TestGLScale`, replace

```go
			if !last {
				return
			}
			for _, run := range glScaleRuns(b) {
				statements := glScaleStatements(t, b, run)
				steps := explainStatements(t, b.app, statements)
				fmt.Fprintf(&plans, "\n## Run %s\n", run.name)
```

with

```go
			// Every size writes its plans, so a run of one size
			// (-run 'TestGLScale$/100k$') records them too.
			for _, run := range glScaleRuns(b) {
				statements := glScaleStatements(t, b, run)
				steps := explainStatements(t, b.app, statements)
				fmt.Fprintf(&plans, "\n## %s lines, run %s\n", size.name, run.name)
```

In the comment above `TestGLScale`, replace `(each statement of each run at 500,000 lines: its time, its rows, its plan)` with `(each statement of each run at each size: its time, its rows, its plan)`.

Run, in `books/server`: `go vet . && go test -run 'TestGLScaleBook$|TestGLScale$' -count=1 -v .`
Expected: `--- PASS: TestGLScaleBook`, `--- SKIP: TestGLScale`, `ok`. (The workspace's `go.work` makes Books build against the core tree, so this also builds Books against step 4.)

- [ ] **Step 7: Measure at 100,000 lines and record**

Do not start this run yourself: it takes 203 s plus the build. Do steps 1 to 6 first, name their files, and hand back the line below. The agent that gets the run's log does the ledger append of this step, and step 8.

`NEEDS-RUN: cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task2 go test -run 'TestGLScale$/100k$' -timeout 20m -v .` — needed from it: `scale-task2/gl-results.md` (the five medians) and, from `scale-task2/gl-plans.md`, the section `## 100k lines, run 3 busy account, month` (statements 4, 5 and 6).

With the results, append to `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md`, after the line `(Each later task of the plan adds its change here: the SQL before and after, its plan, its time.)`, this block, with the "after" column and the statement times replaced by the driver's run (the figures below are the planner's run of the same code, 2026-11-19):

```markdown

#### Task 2: the filter bounds the base table (core `server/reports/bounds.go`)

The WHERE of `w` (the report's rows), before: `(posted IS ? AND (substr(NULLIF(substr(t1.date, 1, 10), ''), 1, 10) >= ? AND … <= ?) AND (NULLIF(t2.id, '') IN (SELECT +value FROM json_each(?))))`. After: `t1.date >= ? AND t1.date <= (? || x'ff') AND t1.account IN (SELECT +value FROM json_each(?)) AND <the filter as before>`.

| run, 100,000 lines | before (baseline) | after |
|---|---|---|
| 1 all accounts, month | 4,328 ms | 4,140 ms |
| 2 all accounts, year | 5,208 ms | 5,151 ms |
| 3 busy account, month (filtered) | 2,527 ms | 3,001 ms |
| 4 small account, month (filtered) | 1,434 ms | 1,257 ms |
| 5 three accounts, year (filtered) | 3,472 ms | 3,324 ms |

Plan of `books_journal_lines` in run 3: every read of the report's rows is now `SEARCH books_journal_lines USING INDEX idx_books_jlines_account_date (account=? AND date>? AND date<?)` (1 in statement 4, 7 in statement 5, 7 in statement 6); before, each was a `SCAN`. What stays: the beginning-balance lookups, `SEARCH … (account>? AND account<?)` (3 in statement 4), and the earlier-years P&L lookups, `SCAN books_journal_lines` (2 in statements 4 and 5). Statement times of run 3: 420 ms, 605 ms, 1,237 ms.

The rows are read through the index, but the run times at this size did not fall: the time is in the beginning-balance and earlier-years lookups, which tasks 4 and 5 change.

Open question: run 3 rose from 2,527 ms to 3,001 ms. The baseline has no statement times at 100,000 lines, so the cause is not known (two runs at different times of one machine, or a cost of the bound, for example the list subquery read again for each read of `w`). Task 5's record compares run 3's statement times with the ones above and answers it.
```

When the driver's "after" figures of the filtered runs are higher than the baseline's, keep the last sentence and the open question, and record the figures as they are. When run 3 is at or under the baseline, remove the open question. This task has no time bar; its proof is the plan (step 5) and the cross-check. Change no test to make a figure look better.

- [ ] **Step 8: Commit (both repos)**

Only when your prompt tells you to commit. When it does not, name the files in your hand-back; the driver commits.

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/bounds.go core/server/reports/bounds_test.go core/server/reports/legacy.go core/server/reports/rows.go
git commit -m "perf(reports): the filter bounds the base table's stored columns"
cd /Users/nas/tmp/tinycld/books-purchases/books
git add server/report_general_ledger_scale_test.go docs/plans/general-ledger-ledger.md
git commit -m "test(books): scale plans for every size; record the filter bounds"
```

---

### Task 3: The `startOfMonth` formula operator (core)

A formula operator that turns a date into the first day of its month (`2026-08-15` → `2026-08-01`), type Date. Task 4 uses it to split a period's beginning balance into whole stored months and the lines of the period's first month. It is a report operator only. The shared cases carry no `forms`: the shared table runs in the forms engine only the cases whose operators are add, sub, mul, div, round, sum, min and max (`onlyFormsOps`, formula_cases_test.go:346).

**Files:**
- Modify: `tinycld/core/server/reports/formula.go:955-956` (a case after `addDays`/`addMonths`)
- Modify: `tinycld/core/server/reports/lookup.go:659` (`rowFreeOps`)
- Modify: `tinycld/core/server/reports/formula_test.go:74` (`TestCompileFormulaTypes`)
- Modify: `tinycld/core/server/reports/formula_run_test.go:238` (the count of cases)
- Modify: `tinycld/core/lib/forms/fixtures/formula-cases.json` (two cases after "addMonths holds the month end")
- Modify: `tinycld/core/reports/lib/builder/formula-parts.ts` (`DATES`, `ARITY`, the labels)
- Modify: `tinycld/core/reports/lib/builder/formula-type.ts:166-168`
- Modify: `tinycld/core/reports/lib/builder/formula-describe.ts:42`
- Modify: `tinycld/core/reports/lib/builder/__tests__/formula-ops.test.ts:373`
- Modify: `tinycld/core/help/reports.md` (section "Group by a formula")

`formula-ops.ts` needs no change: it names argument labels only for operators with two or more values.

**Interfaces:**
- Consumes: nothing from task 2.
- Produces (for task 4): the formula operator `{"op":"startOfMonth","args":[<a date or date-time>]}` → a Date, SQL `(substr(<day>, 1, 8) || '01')`, blank for a blank day; allowed inside a lookup that is computed once per group key (`rowFreeOps`). Builder label `First day of the month of`.

- [ ] **Step 1: Write the failing tests**

In `tinycld/core/lib/forms/fixtures/formula-cases.json`, after the case named `addMonths holds the month end` (its closing `},`), add:

```json
        {
            "name": "startOfMonth",
            "types": { "a": "date" },
            "values": { "a": "2026-08-15" },
            "formula": { "op": "startOfMonth", "args": [{ "col": "a" }] },
            "expect": "2026-08-01"
        },
        {
            "name": "startOfMonth of a blank day is blank",
            "types": { "a": "date" },
            "values": { "a": null },
            "formula": { "op": "startOfMonth", "args": [{ "col": "a" }] },
            "expect": null
        },
```

In `tinycld/core/server/reports/formula_test.go`, in the `cases` map of `TestCompileFormulaTypes` (its `day` column is a `DateTime`), after the `addMonths` line add:

```go
		`{"op":"startOfMonth","args":[{"col":"day"}]}`:                      Date,
```

In `tinycld/core/server/reports/formula_run_test.go`, the table now has two more one-row cases: replace `if detailRuns != 70 || totalRuns != 12 || recordRuns != 2 {` with `if detailRuns != 72 || totalRuns != 12 || recordRuns != 2 {`.

In `tinycld/core/reports/lib/builder/__tests__/formula-ops.test.ts`, in the test `offers every report operator but exists and a list in a column`, replace

```ts
            'fiscalPeriod',
            'diffDays',
```

with

```ts
            'fiscalPeriod',
            'startOfMonth',
            'diffDays',
```

- [ ] **Step 2: Run the tests to verify they fail**

Run, in `tinycld/core/server/reports`: `go test -run 'TestCompileFormulaTypes|TestFormulaCases' -count=1 .`
Expected: FAIL with

```
        formula_cases_test.go:423: compile: reports: formula: unknown operator "startOfMonth"
        formula_run_test.go:201: Run: reports: columns.1.formula: unknown operator "startOfMonth"
```

Run, in `/Users/nas/tmp/tinycld/books-purchases/tinycld/core`: `PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run reports/lib/builder/__tests__/formula-ops.test.ts`
Expected: FAIL in `offers every report operator but exists and a list in a column` (the list has no `startOfMonth`).

- [ ] **Step 3: Write the operator (Go)**

In `tinycld/core/server/reports/formula.go`, in `operatorExpr`, replace

```go
		return expr{sql: "tc_add_months(" + args[0].sql + ", " + count + ")", typ: Date}, nil
	case "diffDays":
```

with

```go
		return expr{sql: "tc_add_months(" + args[0].sql + ", " + count + ")", typ: Date}, nil
	case "startOfMonth":
		if err := argc(1, 1); err != nil {
			return expr{}, err
		}
		if !isDateLike(args[0].typ) {
			return expr{}, invalid(at+".args.0", "startOfMonth needs a date")
		}
		// A day's first 8 characters are its year and month with both dashes.
		// A blank day stays blank: NULL || '01' is NULL.
		return expr{sql: "(substr(" + args[0].sql + ", 1, 8) || '01')", typ: Date}, nil
	case "diffDays":
```

In `tinycld/core/server/reports/lookup.go`, in `rowFreeOps`, replace

```go
	"addDays": true, "addMonths": true, "diffDays": true,
```

with

```go
	"addDays": true, "addMonths": true, "diffDays": true, "startOfMonth": true,
```

- [ ] **Step 4: Write the operator (builder)**

In `tinycld/core/reports/lib/builder/formula-parts.ts`, three edits. In `DATES`, replace

```ts
    'addMonths',
    'diffDays',
] as const
```

with

```ts
    'addMonths',
    'startOfMonth',
    'diffDays',
] as const
```

In `ARITY`, after `    addMonths: [2, 2],` add `    startOfMonth: [1, 1],`. In the labels, after `    addMonths: 'Add months',` add `    startOfMonth: 'First day of the month of',`.

In `tinycld/core/reports/lib/builder/formula-type.ts`, replace

```ts
        case 'addMonths':
            return 'date'
```

with

```ts
        case 'addMonths':
        case 'startOfMonth':
            return 'date'
```

In `tinycld/core/reports/lib/builder/formula-describe.ts`, after `    fiscalPeriod: 'fiscal period of',` add `    startOfMonth: 'first day of the month of',`.

- [ ] **Step 5: Help**

In `tinycld/core/help/reports.md`, in the section `## Group by a formula`, after its paragraph (it ends `…or the cell column of a pivot.`) add a blank line and:

```markdown
To turn a day into the first day of its month in a formula, pick **First day of the month of** and the date. For example, a day of August 15, 2026 gives August 1, 2026. Use it to compare a date with the start of a month, or to group days by month in a formula.
```

- [ ] **Step 6: Run the tests to verify they pass**

Run, in `tinycld/core/server/reports`: `go vet . && go test -run 'TestCompileFormulaTypes|TestFormulaCases' -count=1 .`
Expected: `ok  	tinycld.org/core/reports`.

Run, in `tinycld/core/server/forms`: `go test -run 'TestSharedFormulaCases' -count=1 .`
Expected: `ok` (the count of cases that carry `forms` stays 27: the new cases carry none).

Run, in `tinycld/core`: `PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run reports/lib/builder/__tests__ lib/forms/__tests__/formula-cases.test.ts`
Expected: every file passes.

Run, in `/Users/nas/tmp/tinycld/books-purchases/tinycld`: `PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg typecheck`
Expected: exit 0, no output. Then `PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome check core/reports/lib/builder core/lib/forms/fixtures/formula-cases.json`: no error.

- [ ] **Step 7: Commit (core repo)**

Only when your prompt tells you to commit. When it does not, name the files in your hand-back; the driver commits.

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/formula.go core/server/reports/lookup.go core/server/reports/formula_test.go core/server/reports/formula_run_test.go core/lib/forms/fixtures/formula-cases.json core/reports/lib/builder/formula-parts.ts core/reports/lib/builder/formula-type.ts core/reports/lib/builder/formula-describe.ts core/reports/lib/builder/__tests__/formula-ops.test.ts core/help/reports.md
git commit -m "feat(reports): the startOfMonth formula operator"
```

---

## What is left (part 3: tasks 4 to 7)

Part 3 is `2026-11-19-report-engine-scale-part3.md`, written from the outline (`plan-outline.md`, sections "Tasks", "Order and dependencies", "Facts for the planner of part 2") and from this part's Interfaces blocks. Tasks: 4 (Books: the beginning balance reads stored months), 5 (core: a sum lookup as one grouped aggregate), 6 (core: day bounds in a lookup's where), 7 (Books: the 500k run and its record; `NEEDS-RUN`, and `STOPPED` when a filtered run is over 1 s).

Facts the planner of this part measured (do not find again):
- **Task 2 alone does not lower the run times at 100,000 lines** (step 7's table: run 3 is 3,001 ms against a baseline of 2,527 ms; run 4 1,257 against 1,434; run 5 3,324 against 3,472). Every read of `w` became a `SEARCH` by account and day, so the time is not in the report's rows at this size: it is in the lookups. After task 2, run 3 keeps `SEARCH … (account>? AND account<?)` (the beginning balance, `linkedEq`'s range form) and `SCAN books_journal_lines` (the earlier-years P&L lookups) in statements 4 and 5. Tasks 4 and 5 carry the unit's bar; part 3 must measure after each.
- The 100k size alone takes 203 s plus the build: it is a `NEEDS-RUN`, not an implementer step. The outline's "under 2 minutes" is wrong.
- `TestGLScale` wrote plans only at 500k; task 2 step 6 changes that. `gl-plans.md` headings are then `## <size> lines, run <name>`. Task 7's plan pins read `glScaleStatements`, not the file.
- A list param compiles to `(SELECT +value FROM json_each(?))`; `col IN <that>` is served by the index (plan: `account=? AND date>? AND date<?`, with `LIST SUBQUERY`). A dateRange param is never half open: `{"to": …}` alone is refused, so `dayBounds` gets one empty side only from `lt`/`lte`/`gt`/`gte`.
- A compiler built with no app (`newCompiler(p, nil, nil, nil)`) still has `c.p.app` set: guard database reads with `c.dry()`.
- The zoo has a ledger fixture with the General Ledger's spec shape and an `(account, date)` index: `withBalanceZoo`, `balanceSpec()`, `withLedgerLines`, `ledgerRuns()`, golden `testdata/ledger_scale.golden.json` (`TestABalanceShapedReportOverManyAccountsKeepsItsOutput`). Use it for the cross-checks of tasks 5 and 6. `requireRows` on a report with no groups ends with the total row (`nil`).
- Core plan code builds with `go test -overlay` on scratch copies. The shared formula table (`formula-cases.json`) and the builder's TS files are read from disk, so the planner tested task 3 with the edits in the tree and removed them again.
- `books/server/report_general_ledger_scale_test.go` did not exist when this part was written (task 1 was planned, not run). To build Books plan code, take the file from part 1's code block.
