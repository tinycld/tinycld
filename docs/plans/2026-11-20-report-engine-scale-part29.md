# Report Engine Scale, Part 29: Every Value in a Statement Is Bound (task 29) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Each value the human listed on 2026-11-20 (the day of `startOfMonth`, a percent's scale, the placeholder day, the seven row limits of the core report engine, and the values of four Books ledger statements) is a bound parameter and no longer in the SQL text, and every result stays the same.

**Architecture:** In core, a value a statement compares or computes with goes through `(*stmt).bind` (the `bind` of a formula context): the day `startOfMonth` adds, the scale of a percent, the placeholder day of a blank pivot cell, and every row limit; `limitSQL` is removed. In Books, each of the four statements becomes a named constant whose values are `{:name}` placeholders, bound with `dbx.Params`. The `x'ff'` sentinel stays in the text (approved).

**Tech Stack:** Go; core report engine (`tinycld.org/core/reports`, SQLite through `modernc.org/sqlite` v1.57.0, `github.com/pocketbase/dbx`); Books Go package (`tinycld.org/packages/books`, built against the core tree through the workspace `go.work`).

**Spec:** the rule in `/Users/nas/code/tinycld/CLAUDE.md` ("SQL-text literals require explicit human approval. Bind every value in SQL"); the audit `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/sql-literal-audit.md` (places 1, 2, 3, 6, 7, 9, 10, 11); and the HUMAN DECISIONS bullet of 2026-11-20 23:00 in `/Users/nas/code/tinycld/docs/plans/auto-helix/p5-report-engine-scale.md` (line 36), which adds the seven `limitSQL` places.

## The evidence this plan is written from

The planner built every change of this plan once, through a Go overlay on scratch copies (core `ea0055e3`, books `f5ad1e22`; no repo file was changed).

1. Core, both tasks together: `go vet ./reports/` clean; `go test ./reports/ -count=1` is `ok` (162 s) with the test changes of this plan.
2. Core, with no source change: the four new tests fail (RED) with the messages each task names.
3. Core, task 1 alone: its three tests pass and `TestEveryRowLimitIsBound` fails. The tasks do not depend on each other's source.
4. A bound row limit makes SQLite prepare the statement a second time at its first step, and its `EXPLAIN` too (`TestABoundLimitPreparesTheStatementAgain`, in the tree since core main `eef75b78`). Measured in the overlay: each statement with ` LIMIT {:rpN}` reports 1 second prepare, each statement with no `LIMIT` reports 0, and the counted page (`__counted <= {:rpN}`) reports 0. So the existing test `TestEveryReportStatementIsPreparedOnce` fails after task 2 and task 2 changes it (step 5). The human decided to bind the limits; this plan does not treat the second prepare as a question, and the 100k runs of task 3 are a normal no-regression check.
5. Before the change the placeholder day never reached a statement's text (a blank cell's SQL is dropped, its cell is `NULL`). After it the day is a bound param that no SQL reads; dbx accepts a param with no placeholder (the other leaves of a blank part were already bound that way, `drill.go` line 446).
6. Books, its change alone on the core of the tree: `go vet .` clean; `go test -count=1 .` is `ok` (400 s). The test of task 3 step 2 fails (RED) on the state after step 1. Books with its change on the core of tasks 1 and 2 (the three tasks together): `go vet .` clean; `go test -count=1 .` is `ok` (524 s), so no Books test pins a plan line that the bound limits change.
7. The old test name `TestEveryReportStatementIsPreparedOnce` also stays in two old report files under `tinycld/.superpowers/sdd/2026-11-01-report-engine-v2/`; they are history and are not changed.

## Global Constraints

- Workspace root `/Users/nas/tmp/tinycld/books-purchases`. Core is `/Users/nas/tmp/tinycld/books-purchases/tinycld` (Go in `core/server`), Books is `/Users/nas/tmp/tinycld/books-purchases/books` (Go in `server`). Both on branch `books/p5-report-engine-scale`. Read `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/agent-rules.md` first.
- Tasks 1 and 2 are core tasks: they may edit `core/server/reports`. Task 3 edits Books only.
- Core names no package: no Books word, collection or route in core code, tests or comments.
- Bind the values this plan names. The approved `x'ff'` sentinel (`bounds.go`, `lookup.go`) and SQL idioms (`''`, `IS NULL`, `substr` positions, `CASE … THEN 1 ELSE 0`) stay; do not change them.
- Not in this part (the plan review found them; the human has not decided on them, and the driver asks): other values written into core report SQL that are in neither the audit nor the human's list: `LIMIT 1` of a first/last lookup (`lookup.go:389`, `:407`), `'Yes'` / `'No'` (`options.go:43`), the place number `%[3]d` (`options.go:136`), `' '` (`options.go:205`), `'-Q'` (`rows.go:708`), `'%+d days'` (`formula.go:1009`), `'text'` and `'$.k'` (`label.go:135`, `options.go:136`). Do not bind them and do not change them here.
- No result changes: every existing test of both packages passes with no edit, but for the one prepare-count test task 2 names.
- No backward-compatible shim: `limitSQL` is deleted, not kept beside `bind`.
- No plan-line or timing check is added. When an existing test pins a plan line that changes, update that test.
- Change a file with the Edit tool only; never `git checkout` or `git restore` a file. Do not push.
- The implementer starts no scale run: the 100k runs are a `NEEDS-RUN` for the driver (task 3 step 8).
- The gcut limit of plan part 28 (not built) is not in this plan; part 28 is revised to bind it.

## Review Focus

Each line names a condition the spec implies, and the test that pins it.

1. A percent with scale 0: a bound `int64(0)` must round to whole percents, as the written `0` did. `TestAPercentBindsItsScale` requires the bound scales 0 and 2; `TestPercentExportsPrintTheScale` holds the figures (`18` for "Of parent").
2. A value compiled on one statement and read on another would name a param the second statement does not hold (dbx refuses it). The percent in a paged detail cell (`pagedcells_test.go`, `percentOfParent`), the layout and pivot percents (`percent_scale_test.go`) and the fuzz parity runs exercise it: the whole-package run of task 1 step 5.
3. `startOfMonth` inside a filter that becomes an index bound ("month to date", `lookupbounds_test.go` line 112) must still give its bound: the whole-package run of task 1 step 5. A feeding with no day must still give a blank month: `TestStartOfMonthBindsItsDay` (the last row).
4. A row limit bound as `int`, not `int64`, or a limit of 0: `TestEveryRowLimitIsBound` requires a positive `int64` for every limit, on all eight kinds of statement.
5. A Go `true` or `false` bound against a stored `posted` flag (1 or 0) must match it: `TestVerifyLines_ReportsEachDisagreement` (a draft's stamped line and a posted entry's unstamped line), `TestCashBalances_FollowThePostsPaymentsAndVoids` and `TestVerifyCashBalances_ReportsEachDriftAndTheRebuildRepairsIt`, run in task 3 step 5.

---

### Task 1: Core binds the formula values (the month's first day, a percent's scale, the placeholder day)

**Files:**
- Modify: `core/server/reports/formula.go:1019-1022` (`startOfMonth`)
- Modify: `core/server/reports/detail.go:351-361` (`(*rowsCtx).window`), `:605-630` (`windowSQL`)
- Modify: `core/server/reports/levels.go:1559-1565` (`(*levelCtx).window`)
- Modify: `core/server/reports/drill.go:90-105` (`pivotBound`), `:459-463` (the `placeholderDay` comment)
- Create (test): `core/server/reports/boundvalues_test.go`

**Interfaces:**
- Consumes: `bind(v any) string` of the formula context `fctx` (`formula.go:334`), which `rowsCtx`, `levelCtx` and the others implement over `(*stmt).bind` (`stmt.go:45`; it returns `{:rpN}`). Test helpers in the package: `newZoo`, `withClientZoo`, `clientShares`, `newSavedZoo`, `shipDrillTarget`, `drilledByMonth`, `observeReads(t) *[]readStatement` (`statement_test.go:332`; a `readStatement` holds `sql string` and `params dbx.Params`), `(z *zoo) run`.
- Produces: `func windowSQL(op string, e expr, scale int, f *windowFrame, bind func(any) string, at string) (expr, error)` (one new parameter, `bind`). In `boundvalues_test.go`: `var boundParam *regexp.Regexp` (`^\{:(rp\d+)\}$`), which task 2 reads; the file's imports `fmt`, `regexp`, `strings`, `testing`, which task 2's test uses.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/boundvalues_test.go`:

```go
package reports

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// boundParam matches a whole SQL fragment that is one bound parameter of the
// engine, and captures its name.
var boundParam = regexp.MustCompile(`^\{:(rp\d+)\}$`)

// pctScales lists the last argument of every tc_pct call in sql: the scale.
func pctScales(t *testing.T, sql string) []string {
	t.Helper()
	var out []string
	for rest := sql; ; {
		at := strings.Index(rest, "tc_pct(")
		if at < 0 {
			return out
		}
		rest = rest[at+len("tc_pct("):]
		depth, last, end := 0, 0, -1
		for i, r := range rest {
			switch {
			case r == '(':
				depth++
			case r == ')' && depth == 0:
				end = i
			case r == ')':
				depth--
			case r == ',' && depth == 0:
				last = i + 1
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			t.Fatalf("a tc_pct call does not close:\n%.200s", rest)
		}
		out = append(out, strings.TrimSpace(rest[last:end]))
	}
}

// A percent's scale is a bound parameter of its statement, never written
// into the text: the share of the Total (2 places) and the share of the
// parent (0 places) of clientShares. TestPercentColumnCarriesItsScale and
// TestPercentExportsPrintTheScale hold the figures.
func TestAPercentBindsItsScale(t *testing.T) {
	z := newZoo(t)
	withClientZoo(t, z)
	seen := observeReads(t)
	z.run(t, "ann", clientShares, Options{}, nil)
	scales := map[int64]bool{}
	for _, st := range *seen {
		for _, arg := range pctScales(t, st.sql) {
			m := boundParam.FindStringSubmatch(arg)
			if m == nil {
				t.Fatalf("a percent's scale is written into the statement: tc_pct(…, %s)", arg)
			}
			scale, ok := st.params[m[1]].(int64)
			if !ok {
				t.Fatalf("the scale's param %s = %#v, want an int64", m[1], st.params[m[1]])
			}
			scales[scale] = true
		}
	}
	if len(scales) != 2 || !scales[0] || !scales[2] {
		t.Fatalf("the bound scales = %v, want 0 and 2", scales)
	}
}

// monthOfFed lists ann's feedings by day, each with the first day of its
// month.
const monthOfFed = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"fed","path":[{"field":"fed"}]},
	{"id":"month","formula":{"op":"startOfMonth","args":[{"col":"fed"}]}}],
	"sort":[{"col":"fed"}]}`

// addedDay captures the day startOfMonth appends to a day's year and month.
var addedDay = regexp.MustCompile(`NULLIF\(substr\([^)]*, 1, 8\), ''\) \|\| (\S+)\)`)

// startOfMonth binds the day it adds to a day's year and month, and its
// figures stay: the first day of each feeding's month, blank for a feeding
// with no day.
func TestStartOfMonthBindsItsDay(t *testing.T) {
	z := newZoo(t)
	seen := observeReads(t)
	res := z.run(t, "ann", monthOfFed, Options{}, nil)
	var got []string
	for _, r := range res.Rows {
		got = append(got, fmt.Sprint(r.Cells["fed"], "→", r.Cells["month"]))
	}
	want := "2026-01-05→2026-01-01 2026-01-20→2026-01-01 2026-02-10→2026-02-01 2026-03-03→2026-03-01 <nil>→<nil>"
	if strings.Join(got, " ") != want {
		t.Fatalf("the months = %q, want %q", got, want)
	}
	bound := 0
	for _, st := range *seen {
		if strings.Contains(st.sql, "'01'") {
			t.Fatalf("the day is written into the statement:\n%s", st.sql)
		}
		for _, m := range addedDay.FindAllStringSubmatch(st.sql, -1) {
			p := boundParam.FindStringSubmatch(m[1])
			if p == nil || st.params[p[1]] != "01" {
				t.Fatalf("the day after || is %s (%#v), want a param bound to \"01\"", m[1], st.params)
			}
			bound++
		}
	}
	if bound == 0 {
		t.Fatal("no statement holds startOfMonth's day")
	}
}

// The bounds of a blank date's pivot cell are bound like a bucket's days,
// never written into the text. TestAPivotCellOfABlankDateHasNoDrill holds
// the result: the blank column draws no drill.
func TestABlankPivotBoundIsBound(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	nub := s.rec(t, "nub", "zoo_animals", map[string]any{"name": "Nub", "species": "lion", "owner": s.id("ann")})
	s.rec(t, "k9", "zoo_keepers", map[string]any{"user": s.id("ann"), "animal": nub.Id})
	s.rec(t, "f8", "zoo_feedings", map[string]any{"animal": nub.Id, "grams": 5, "cost": 1, "currency": "USD", "note": "f8"})
	seen := observeReads(t)
	s.run(t, "ann", drilledByMonth, Options{}, nil)
	bound := 0
	for _, st := range *seen {
		if strings.Contains(st.sql, placeholderDay) {
			t.Fatalf("the placeholder day is written into the statement:\n%s", st.sql)
		}
		for _, v := range st.params {
			if v == placeholderDay {
				bound++
			}
		}
	}
	if bound != 2 {
		t.Fatalf("%d params hold the placeholder day, want 2 (the blank column's from and to)", bound)
	}
}
```

The arrow in `fmt.Sprint(…, "→", …)` and in `want` is the one character U+2192; the Write tool keeps it. The ellipsis in the first `t.Fatalf` is U+2026.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -count=1 -run 'TestAPercentBindsItsScale|TestStartOfMonthBindsItsDay|TestABlankPivotBoundIsBound'`

Expected: FAIL, three tests:
- `TestAPercentBindsItsScale`: `a percent's scale is written into the statement: tc_pct(…, 2)`
- `TestStartOfMonthBindsItsDay`: `the day is written into the statement:`
- `TestABlankPivotBoundIsBound`: `0 params hold the placeholder day, want 2 (the blank column's from and to)`

- [ ] **Step 3: Bind the three values**

`core/server/reports/formula.go`, in `case "startOfMonth":`, replace

```go
		// empty text into NULL too, so NULL || '01' is NULL.
		return expr{sql: "(NULLIF(substr(" + args[0].sql + ", 1, 8), '') || '01')", typ: Date}, nil
```

with

```go
		// empty text into NULL too, so NULL || the bound day is NULL.
		return expr{sql: "(NULLIF(substr(" + args[0].sql + ", 1, 8), '') || " + c.bind("01") + ")", typ: Date}, nil
```

`core/server/reports/detail.go`, in `(*rowsCtx).window`, replace `return windowSQL(op, e, scale, rc.detail, at)` with

```go
	return windowSQL(op, e, scale, rc.detail, rc.bind, at)
```

`core/server/reports/detail.go`, `windowSQL`: replace the last comment line and the signature

```go
// zero.
func windowSQL(op string, e expr, scale int, f *windowFrame, at string) (expr, error) {
```

with

```go
// zero. bind binds the percent's scale on the statement the value is in.
func windowSQL(op string, e expr, scale int, f *windowFrame, bind func(any) string, at string) (expr, error) {
```

and replace the two percent returns

```go
	case "percentOfParent":
		return expr{sql: f.window(fmt.Sprintf("tc_pct(%s, SUM(%s) OVER (%s), %d)", num(e), num(e), partitionBy(f.parent), scale)), typ: Percent, win: true}, nil
	}
	return expr{sql: f.window(fmt.Sprintf("tc_pct(%s, SUM(%s) OVER (%s), %d)", num(e), num(e), partitionBy(f.total), scale)), typ: Percent, win: true}, nil
```

with

```go
	case "percentOfParent":
		return expr{sql: f.window(fmt.Sprintf("tc_pct(%s, SUM(%s) OVER (%s), %s)", num(e), num(e), partitionBy(f.parent), bind(int64(scale)))), typ: Percent, win: true}, nil
	}
	return expr{sql: f.window(fmt.Sprintf("tc_pct(%s, SUM(%s) OVER (%s), %s)", num(e), num(e), partitionBy(f.total), bind(int64(scale)))), typ: Percent, win: true}, nil
```

The scale is bound only on the two percent paths, so a running sum and a percent of a section add no param.

`core/server/reports/levels.go`, in `(*levelCtx).window`, replace `return windowSQL(op, e, scale, lc.lv.frame, at)` with

```go
	return windowSQL(op, e, scale, lc.lv.frame, lc.bind, at)
```

`core/server/reports/drill.go`, in `pivotBound`, replace

```go
	// A blank bucket's cell is NULL, so its bound is a literal: binding it
	// would add a param that no SQL reads.
	if lc.drill.blank {
		return expr{sql: "'" + placeholderDay + "'", typ: Date}, nil
	}
	day := lc.drill.from
```

with

```go
	// A blank bucket's bounds are placeholderDay, bound as any other day. Its
	// cell is NULL, so no SQL reads that param.
	day := lc.drill.from
```

(`pivotBounds` already returns `placeholderDay` for `from` and `to` of a blank value, `drill.go:471`.) In the comment of `placeholderDay`, replace

```go
// refuses a misfit, and its cell is NULL (R6). pivotBound writes it as a
// literal, never a bound param.
```

with

```go
// refuses a misfit, and its cell is NULL (R6). pivotBound binds it, as it
// binds a bucket's day.
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -count=1 -run 'TestAPercentBindsItsScale|TestStartOfMonthBindsItsDay|TestABlankPivotBoundIsBound|TestPercentColumnCarriesItsScale|TestPercentExportsPrintTheScale|TestAPivotCellOfABlankDateHasNoDrill|TestAPivotCellDrillsToItsBucketCutToTheRange'`

Expected: `ok  	tinycld.org/core/reports`

- [ ] **Step 5: The task's checks**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -l reports && go vet ./reports/ && go test ./reports/ -count=1`

Expected: `gofmt` prints nothing, `go vet` prints nothing, `ok  	tinycld.org/core/reports` (about 3 minutes). A failure here is a result that changed: find its cause; do not edit the failing test.

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && command grep -n "'01'\|\"'\" + placeholderDay" reports/formula.go reports/drill.go; command grep -n 'OVER (%s), %d' reports/detail.go`

Expected: no line.

- [ ] **Step 6: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/formula.go core/server/reports/detail.go core/server/reports/levels.go core/server/reports/drill.go core/server/reports/boundvalues_test.go
git commit -m "fix(reports): a formula binds its day, its percent scale and a blank pivot bound"
```

---

### Task 2: Core binds every row limit; `limitSQL` is removed

**Files:**
- Modify: `core/server/reports/stmt.go:42-58` (the `bind` comment; delete `limitSQL`)
- Modify: `core/server/reports/nest.go:99-101`
- Modify: `core/server/reports/options.go:408-409`, `:476-477`
- Modify: `core/server/reports/pivot.go:205-206`
- Modify: `core/server/reports/run.go:899`, `:1279`, `:1556`
- Modify (test): `core/server/reports/statement_test.go:189-202`, `:342-380`, `:388-392`
- Modify (test): `core/server/reports/boundvalues_test.go` (add one test at the end)

**Interfaces:**
- Consumes: `(*stmt).bind(v any) string` (`stmt.go:45`); from task 1, `boundParam` is not needed here, and the imports `regexp`, `strings`, `testing` of `boundvalues_test.go`. Test helpers: `newZoo`, `raggedZoo`, `observeReads`, the package variable `observeRead`, `sitesWithAnimals`, `animalsParam`, `(z *zoo) options`, `rawStatement`, `reprepares`, `readPool`.
- Produces: no `limitSQL` in the package. The test `TestEveryReportStatementIsPreparedOnce` is renamed `TestAReportStatementIsPreparedAgainOnlyForItsLimit`.

- [ ] **Step 1: Write the failing test**

Add at the end of `core/server/reports/boundvalues_test.go`:

```go
var (
	writtenLimit = regexp.MustCompile(`(LIMIT|__counted <=) \d`)
	boundLimit   = regexp.MustCompile(`(LIMIT|__counted <=) \{:(rp\d+)\}`)
)

// skipLevelSpec groups ann's feedings by species, region and animal; the
// region level skips a blank region, so its page counts the rows that draw.
const skipLevelSpec = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"animal","path":[{"link":"animal"}]},
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"region","path":[{"link":"animal"},{"link":"site"},{"field":"region"}]},
	{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
	"groups":[{"id":"s","col":"species","subtotals":true,"open":true},
		{"id":"r","col":"region","blank":"skip","subtotals":true,"open":true},
		{"id":"a","col":"animal","subtotals":true}],
	"sort":[{"col":"species"}]}`

// Every row limit the engine computes is a bound int64, never a number in
// the text: the page of group rows (with LIMIT, and counted on a report with
// a skip level), the group statement, the detail page, the child rows, the
// pivot's columns, the options and the option groups. No spec here has a
// first or last lookup, whose one-row read is not a computed limit.
func TestEveryRowLimitIsBound(t *testing.T) {
	z := newZoo(t)
	seen := observeReads(t)
	z.run(t, "ann", `{"version":4,"source":"zoo.animals","columns":[{"id":"name","path":[{"field":"name"}]},{"id":"w","path":[{"field":"weight"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"name"}],"sort":[{"col":"name"}]}`, Options{ExpandAll: true}, nil)
	z.run(t, "ann", sitesWithAnimals, Options{}, nil)
	z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[{"id":"name","path":[{"link":"animal"},{"field":"name"}]},{"id":"c","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"name"}],"pivot":{"path":[{"field":"note"}],"cell":"c"}}`, Options{}, nil)
	z.options(t, "ann", animalsParam, OptionsRequest{Param: "animals"})
	observeRead = nil
	ragged := raggedZoo(t)
	seenRagged := observeReads(t)
	ragged.run(t, "ann", skipLevelSpec, Options{}, nil)
	kinds := map[string]bool{}
	for _, st := range append(*seen, *seenRagged...) {
		if m := writtenLimit.FindString(st.sql); m != "" {
			t.Fatalf("a row limit is written into the statement (%s):\n%.300s", m, st.sql)
		}
		for _, m := range boundLimit.FindAllStringSubmatch(st.sql, -1) {
			if n, ok := st.params[m[2]].(int64); !ok || n < 1 {
				t.Fatalf("the limit's param %s = %#v, want a positive int64", m[2], st.params[m[2]])
			}
			switch {
			case m[1] != "LIMIT":
				kinds["counted page"] = true
			case strings.Contains(st.sql, "__parent"):
				kinds["nest"] = true
			case strings.Contains(st.sql, "COUNT(*) OVER () AS __n"):
				kinds["pivot"] = true
			case strings.Contains(st.sql, "GROUP BY o.g"):
				kinds["option groups"] = true
			case strings.Contains(st.sql, "o.l <> ''"):
				kinds["options"] = true
			case strings.Contains(st.sql, "__p1"):
				kinds["page"] = true
			case strings.Contains(st.sql, "__level"):
				kinds["group"] = true
			default:
				kinds["detail"] = true
			}
		}
	}
	for _, k := range []string{"page", "counted page", "group", "detail", "nest", "pivot", "options", "option groups"} {
		if !kinds[k] {
			t.Errorf("no %s statement bound a row limit (%v)", k, kinds)
		}
	}
}
```

The eight kinds are the seven places of this task: `run.go:899` gives two (the page with `LIMIT`, and the counted page with `__counted <=`), `run.go:1279` the group statement, `run.go:1556` the detail page, `nest.go:101` the child rows, `pivot.go:206` the pivot, `options.go:409` the options, `options.go:477` the option groups.

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -count=1 -run 'TestEveryRowLimitIsBound'`

Expected: FAIL with `a row limit is written into the statement (LIMIT 2):`

- [ ] **Step 3: Bind the seven limits**

Each place has the statement's `*stmt` in the variable `s`.

`core/server/reports/nest.go` (line 101): replace `limitSQL(int64(limit+1)))` with

```go
		strings.Join(append(order, "__id"), ", "), s.bind(int64(limit+1)))
```

(the whole line; only its last argument changes).

`core/server/reports/options.go` (line 409): replace `limitSQL(int64(limit+1)))` with `s.bind(int64(limit+1)))`, so the line reads

```go
		cols, ranked, strings.Join(conds, " AND "), strings.Join(optionOrder(keys, typ), ", "), s.bind(int64(limit+1)))
```

`core/server/reports/options.go` (line 477): replace `limitSQL(int64(maxOptionGroups+1)))` with `s.bind(int64(maxOptionGroups+1)))`, so the line reads

```go
		ranked, strings.Join(where, " AND "), strings.Join(keys, ", "), s.bind(int64(maxOptionGroups+1)))
```

`core/server/reports/pivot.go` (line 206): replace `limitSQL(int64(pv.max+1)))` with `s.bind(int64(pv.max+1)))`, so the line reads

```go
		label, cur, distinct, order, s.bind(int64(pv.max+1)))
```

`core/server/reports/run.go` (line 899): replace

```go
	rows := limitSQL(limit + 1 + int64(anchors))
```

with

```go
	rows := s.bind(limit + 1 + int64(anchors))
```

(`limit` is an `int64` there; `rows` is written once, into the `LIMIT` of the page or into `__counted <=` of the counted page.)

`core/server/reports/run.go` (line 1279, `groupStatement`): replace `limitSQL(limit))}` with `s.bind(limit))}`, so the line reads

```go
	body := []string{fmt.Sprintf("SELECT %s FROM (%s\nORDER BY __level, __ord LIMIT %s)", first, strings.Join(legs, "\nUNION ALL\n"), s.bind(limit))}
```

`core/server/reports/run.go` (line 1556): replace `limitSQL(int64(limit+1)))` with `s.bind(int64(limit+1)))`, so the line reads

```go
	body := fmt.Sprintf("SELECT * FROM %s WHERE %s ORDER BY %s LIMIT %s", from, where, by, s.bind(int64(limit+1)))
```

- [ ] **Step 4: Remove `limitSQL`**

`core/server/reports/stmt.go`: replace the comment of `bind`

```go
// bind adds a value to the statement's parameters and returns its
// placeholder. Values are always bound, never written into the SQL; the one
// literal is a row limit (limitSQL).
```

with

```go
// bind adds a value to the statement's parameters and returns its
// placeholder. Values are always bound, never written into the SQL: a row
// limit too.
```

and delete the whole of `limitSQL` with its comment (lines 51-58, from `// limitSQL writes a statement's row limit as a literal, never a bound` to `func limitSQL(n int64) string { return strconv.FormatInt(n, 10) }`, and the blank line after). `strconv` stays imported: `sqlJSON` uses it.

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && command grep -rn 'limitSQL' core/server`

Expected: no line.

- [ ] **Step 5: Update the prepare-count test**

A bound `LIMIT` makes SQLite prepare the statement again once (`TestABoundLimitPreparesTheStatementAgain`), so `TestEveryReportStatementIsPreparedOnce` now fails for each statement with a `LIMIT`. The human decided to bind the limits; the test now pins that the bound limit is the only cause.

`core/server/reports/statement_test.go`: replace the comment of `TestABoundLimitPreparesTheStatementAgain`

```go
// SQLite marks a statement whose LIMIT is a bound parameter to be prepared
// again once a value is bound (sqlite3ExprIsInteger sets the statement's
// varmask), so binding the limit costs a second prepare at the first step. A
// literal limit does not.
```

with

```go
// SQLite marks a statement whose LIMIT is a bound parameter to be prepared
// again once a value is bound (sqlite3ExprIsInteger sets the statement's
// varmask), so a bound limit costs a second prepare at the first step. A
// literal limit does not. The engine binds its limits all the same: it
// writes no value into a statement's text (TestEveryRowLimitIsBound).
```

Replace the comment and the name of `TestEveryReportStatementIsPreparedOnce`

```go
// Every statement the engine sends (group, detail, nest, pivot, options and
// option groups), and the EXPLAIN that finds a run's change topics, is
// prepared once. The engine's own SQL binds nothing SQLite plans by; a rule
// CTE is PocketBase's SQL (TestAPocketBaseRuleLikePreparesTheStatementAgain).
func TestEveryReportStatementIsPreparedOnce(t *testing.T) {
```

with

```go
// Every statement the engine sends (group, detail, nest, pivot, options and
// option groups), and the EXPLAIN that finds a run's change topics, is
// prepared again at most once, and only for its bound row limit
// (TestABoundLimitPreparesTheStatementAgain): a statement with no LIMIT is
// prepared once. The engine's own SQL binds nothing else SQLite plans by; a
// rule CTE is PocketBase's SQL
// (TestAPocketBaseRuleLikePreparesTheStatementAgain).
func TestAReportStatementIsPreparedAgainOnlyForItsLimit(t *testing.T) {
```

In that test, replace

```go
		raw, args := rawStatement(t, st.sql, st.params)
		if n := reprepares(t, db, "EXPLAIN "+raw, args); n != 0 {
			t.Errorf("EXPLAIN %d reprepares, want 0:\n%.300s", n, st.sql)
		}
		if n := reprepares(t, db, raw, args); n != 0 {
			t.Errorf("%d reprepares, want 0:\n%.300s", n, st.sql)
		}
```

with

```go
		want := 0
		if strings.Contains(st.sql, " LIMIT {:rp") {
			want = 1
		}
		raw, args := rawStatement(t, st.sql, st.params)
		if n := reprepares(t, db, "EXPLAIN "+raw, args); n != want {
			t.Errorf("EXPLAIN %d reprepares, want %d:\n%.300s", n, want, st.sql)
		}
		if n := reprepares(t, db, raw, args); n != want {
			t.Errorf("%d reprepares, want %d:\n%.300s", n, want, st.sql)
		}
```

In the comment of `TestAPocketBaseRuleLikePreparesTheStatementAgain`, replace

```go
// one known exception to TestEveryReportStatementIsPreparedOnce.
```

with

```go
// other cause of a second prepare beside a bound row limit
// (TestAReportStatementIsPreparedAgainOnlyForItsLimit).
```

(the line before it ends "this pins the", so the sentence reads "this pins the other cause of a second prepare beside a bound row limit").

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -count=1 -run 'TestEveryRowLimitIsBound|TestAReportStatementIsPreparedAgainOnlyForItsLimit|TestABoundLimitPreparesTheStatementAgain|TestAPocketBaseRuleLikePreparesTheStatementAgain'`

Expected: `ok  	tinycld.org/core/reports`

- [ ] **Step 7: The task's checks**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -l reports && go vet ./reports/ && go build ./... && go test ./reports/ -count=1`

Expected: `gofmt` and `go vet` print nothing, the build is clean, `ok  	tinycld.org/core/reports` (about 3 minutes). A failure of any other test is a result or a page that changed: find its cause; do not edit that test. When a failing test pins a query-plan line (`EXPLAIN QUERY PLAN` text) that the bound limit changed, update the pinned line to the new one and name the test in the report.

- [ ] **Step 8: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/stmt.go core/server/reports/nest.go core/server/reports/options.go core/server/reports/pivot.go core/server/reports/run.go core/server/reports/statement_test.go core/server/reports/boundvalues_test.go
git commit -m "fix(reports): every row limit is a bound parameter"
```

---

### Task 3: Books binds the values of its ledger statements; the 100k runs and the ledger record

**Files:**
- Modify: `books/server/cash_balances.go:32-49` (`cashMonthTotals`), `:60-73` (`refreshCashBalance`)
- Modify: `books/server/balances.go:177-195` (`verifyLines`)
- Modify: `books/server/cash_basis.go:173-190` (`documentEntryLines`)
- Create (test): `books/server/bound_values_test.go`
- Modify: `books/docs/plans/general-ledger-ledger.md` (a new record at the end of the file, after the record `#### Task 9b-3`, which starts at line 797)

**Interfaces:**
- Consumes: the constants `statusDraft`, `statusPosted`, `statusVoid`, `originVoidReversal`, `originDocument` (`books/server/entries.go:14-23`); `dbx.Params` (each of the three files imports `github.com/pocketbase/dbx`). The core of tasks 1 and 2 (the workspace `go.work` builds Books against the core tree).
- Produces: the constants `cashMonthTotalsSQL`, `cashMonthSumSQL` (`cash_balances.go`), `verifyLinesSQL` (`balances.go`), `documentEntryLinesSQL` (`cash_basis.go`), each a `string`.

The human named `posted = 1` (`cash_balances.go:48`, `:72`, `cash_basis.go:184`) and `status = 'draft'` / `posted = 1` (`balances.go:194`). The same two statements also write `'posted'`, `'void'` and `0` (`balances.go:193`) and `'document'`, `'void_reversal'` (`cash_basis.go:185-188`). This task binds those too: a statement with half its values bound does not meet the rule, and the test below reads the whole statement.

- [ ] **Step 1: Name the four statements (no change of text)**

Move each statement, as it is now, into a constant, so the test of step 2 can read it. Indent the text with one tab.

`books/server/cash_balances.go`: add before the comment `// cashMonthTotals sums every posted cash-basis row by account and month: what`

```go
// cashMonthTotalsSQL sums every posted cash-basis row by account and month.
const cashMonthTotalsSQL = `
	SELECT account, substr(date, 1, 7) AS period,
	       CAST(SUM(debit) AS INTEGER) AS debit,
	       CAST(SUM(credit) AS INTEGER) AS credit,
	       MAX(currency) AS currency
	FROM books_cash_basis_lines
	WHERE posted = 1
	GROUP BY account, substr(date, 1, 7)`

// cashMonthSumSQL sums the posted cash-basis rows of one account between two
// days.
const cashMonthSumSQL = `
	SELECT COUNT(*) AS n,
	       COALESCE(CAST(SUM(debit) AS INTEGER), 0) AS debit,
	       COALESCE(CAST(SUM(credit) AS INTEGER), 0) AS credit,
	       COALESCE(MAX(currency), '') AS currency
	FROM books_cash_basis_lines
	WHERE account = {:account} AND posted = 1 AND date >= {:from} AND date <= {:to}`

```

In `cashMonthTotals`, replace the call from ``err := app.DB().NewQuery(` `` to ``GROUP BY account, substr(date, 1, 7)`).All(&rows)`` with

```go
	err := app.DB().NewQuery(cashMonthTotalsSQL).All(&rows)
```

In `refreshCashBalance`, replace the call from ``err := tx.DB().NewQuery(` `` to `.One(&sum)` with

```go
	err := tx.DB().NewQuery(cashMonthSumSQL).
		Bind(dbx.Params{"account": key.account, "from": firstDayOf(key.period), "to": key.period + "-31"}).One(&sum)
```

`books/server/balances.go`: add before the comment `// verifyLines reports every line whose posted flag or date disagrees with its`

```go
// verifyLinesSQL selects every line whose posted flag or date disagrees with
// its entry, and every line whose entry row is gone.
const verifyLinesSQL = `
	SELECT l.id AS line, l.entry AS entry, COALESCE(e.status, '') AS entry_status,
	       l.posted AS posted, l.date AS date, COALESCE(e.date, '') AS entry_date
	FROM books_journal_lines l
	LEFT JOIN books_journal_entries e ON e.id = l.entry
	WHERE e.id IS NULL
	   OR (e.status IN ('posted', 'void') AND (l.posted = 0 OR l.date != e.date))
	   OR (e.status = 'draft' AND (l.posted = 1 OR l.date != ''))
	ORDER BY l.entry, l.position, l.id`

```

In `verifyLines`, replace the call from ``err := app.DB().NewQuery(` `` to ``ORDER BY l.entry, l.position, l.id`).All(&drifts)`` with

```go
	err := app.DB().NewQuery(verifyLinesSQL).All(&drifts)
```

`books/server/cash_basis.go`: add before the comment `// documentEntryLines reads the posted lines of the document's entries (origin`

```go
// documentEntryLinesSQL selects the posted lines of one document's entries
// and of their void reversals, in date order.
const documentEntryLinesSQL = `
	SELECT l.entry AS entry, e.status AS status, e.origin AS origin, e.number AS number,
	       l.account AS account, CAST(l.debit AS INTEGER) AS debit, CAST(l.credit AS INTEGER) AS credit,
	       l.date AS date, CAST(l.position AS INTEGER) AS position,
	       l.vendor AS vendor, l.customer AS customer, l.currency AS currency
	FROM books_journal_lines l
	JOIN books_journal_entries e ON e.id = l.entry
	WHERE l.posted = 1 AND e.doc_type = {:type} AND (
		(e.origin = 'document' AND e.document = {:doc})
		OR (e.origin = 'void_reversal' AND e.reverses != '' AND e.reverses IN (
			SELECT id FROM books_journal_entries
			WHERE document = {:doc} AND doc_type = {:type} AND origin = 'document')))
	ORDER BY l.date, e.sequence, e.origin, l.entry, l.position, l.id`

```

In `documentEntryLines`, replace the call from ``err := tx.DB().NewQuery(` `` to `.All(&lines)` with

```go
	err := tx.DB().NewQuery(documentEntryLinesSQL).
		Bind(dbx.Params{"type": d.DocType, "doc": docID}).All(&lines)
```

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && gofmt -l . && go vet .`

Expected: no output.

- [ ] **Step 2: Write the failing test**

Create `books/server/bound_values_test.go`:

```go
package books

import (
	"regexp"
	"strings"
	"testing"
)

// writtenValue matches a value written into a statement's text: a number a
// column is compared with, or a quoted word.
var writtenValue = regexp.MustCompile(`(=|<|>)\s*\d|'[^']+'`)

// The ledger statements bind every value they compare a column with: the
// posted flag, an entry's status and its origin. The empty text (the blank of
// COALESCE and of a blank-text test) is SQL's own idiom, not a value, so the
// test takes it out first.
func TestLedgerStatementsBindTheirValues(t *testing.T) {
	for name, c := range map[string]struct {
		sql   string
		binds []string
	}{
		"cashMonthTotals":    {cashMonthTotalsSQL, []string{"posted = {:posted}"}},
		"cashMonthSum":       {cashMonthSumSQL, []string{"posted = {:posted}"}},
		"verifyLines":        {verifyLinesSQL, []string{"e.status IN ({:posted}, {:void})", "l.posted = {:unstamped}", "e.status = {:draft}", "l.posted = {:stamped}"}},
		"documentEntryLines": {documentEntryLinesSQL, []string{"l.posted = {:posted}", "e.origin = {:document}", "e.origin = {:reversal}", " origin = {:document}"}},
	} {
		text := strings.ReplaceAll(c.sql, "''", "")
		if m := writtenValue.FindString(text); m != "" {
			t.Errorf("%s writes a value into its text: %q", name, m)
		}
		for _, b := range c.binds {
			if !strings.Contains(c.sql, b) {
				t.Errorf("%s does not hold %q:\n%s", name, b, c.sql)
			}
		}
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go test -count=1 -run 'TestLedgerStatementsBindTheirValues' .`

Expected: FAIL; for each of the four names a line `<name> writes a value into its text: …` (`"= 1"` for the cash statements and `documentEntryLines`, `"'posted'"` for `verifyLines`) and a line `<name> does not hold …` for each placeholder. (The planner ran this red on the state after step 1.)

- [ ] **Step 4: Bind the values**

`books/server/cash_balances.go`: in `cashMonthTotalsSQL` replace `WHERE posted = 1` with `WHERE posted = {:posted}`; in `cashMonthSumSQL` replace `AND posted = 1 AND` with `AND posted = {:posted} AND`. The two calls become

```go
	err := app.DB().NewQuery(cashMonthTotalsSQL).Bind(dbx.Params{"posted": true}).All(&rows)
```

```go
	err := tx.DB().NewQuery(cashMonthSumSQL).
		Bind(dbx.Params{"account": key.account, "posted": true, "from": firstDayOf(key.period), "to": key.period + "-31"}).One(&sum)
```

`books/server/balances.go`: in `verifyLinesSQL` replace the two `OR` lines with

```go
	   OR (e.status IN ({:posted}, {:void}) AND (l.posted = {:unstamped} OR l.date != e.date))
	   OR (e.status = {:draft} AND (l.posted = {:stamped} OR l.date != ''))
```

and the call becomes

```go
	err := app.DB().NewQuery(verifyLinesSQL).Bind(dbx.Params{
		"posted": statusPosted, "void": statusVoid, "draft": statusDraft,
		"stamped": true, "unstamped": false,
	}).All(&drifts)
```

`books/server/cash_basis.go`: in `documentEntryLinesSQL` replace the `WHERE` block (five lines) with

```go
	WHERE l.posted = {:posted} AND e.doc_type = {:type} AND (
		(e.origin = {:document} AND e.document = {:doc})
		OR (e.origin = {:reversal} AND e.reverses != '' AND e.reverses IN (
			SELECT id FROM books_journal_entries
			WHERE document = {:doc} AND doc_type = {:type} AND origin = {:document})))
```

and the call becomes

```go
	err := tx.DB().NewQuery(documentEntryLinesSQL).Bind(dbx.Params{
		"type": d.DocType, "doc": docID, "posted": true,
		"document": originDocument, "reversal": originVoidReversal,
	}).All(&lines)
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go test -count=1 -run 'TestLedgerStatementsBindTheirValues|TestVerifyLines|TestRebuildRoute|TestCashBalances|TestVerifyCashBalances|TestDocDeferral|TestCashBasis' .`

Expected: `ok  	tinycld.org/packages/books`

- [ ] **Step 6: The task's checks**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && gofmt -l . && go vet . && go test -count=1 .`

Expected: `gofmt` and `go vet` print nothing; `ok  	tinycld.org/packages/books`. This run builds Books against the core of tasks 1 and 2, so it also proves every Books report on the bound core statements. A failing test that pins a query-plan line the bound limit changed: update the pinned line and name the test in the report. Any other failure is a result that changed: find its cause; do not edit that test.

- [ ] **Step 7: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add server/cash_balances.go server/balances.go server/cash_basis.go server/bound_values_test.go
git commit -m "fix(ledger): the ledger statements bind the posted flag, the status and the origin"
```

The runs of step 8 are then of committed trees; note the core hash (`git -C /Users/nas/tmp/tinycld/books-purchases/tinycld rev-parse --short HEAD`) and the Books hash for the ledger record.

- [ ] **Step 8: Hand the 100k runs to the driver**

Hand back, and stop until the driver gives the logs:

```
NEEDS-RUN: (quiet machine: `pgrep -fl 'chrome-headless-[s]hell'` prints nothing and the first number of `sysctl -n vm.loadavg` is under 4) cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task29-st go test -run 'TestStatementsScale$/100k$' -count=1 -timeout 60m -v .
NEEDS-RUN: (quiet machine, as above) cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task29-gl go test -run 'TestGLScale$/100k$' -count=1 -timeout 60m -v .
```

Needed from the first: PASS; `scale-task29-st/st-results.md` (every median, no run refused; the group rows and rows of each run as in `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9b3/st-results.md`).

Needed from the second: PASS; `scale-task29-gl/gl-results.md` (the five medians and the sections on the page, as in `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9b3-gl/gl-results.md`).

The figures before are the "after" column of the ledger record `#### Task 9b-3` (line 797): TB 1 345 ms, TB 2 153 ms, TB 3 181 ms, BS 1 455 ms, BS 2 504 ms, TD 1 62 ms, TD 2 232 ms, TD 3 30 ms, TD 4 19 ms, TD 5 135 ms, TD 6 34 ms, GL 1 322 ms, GL 2 694 ms, GL 3 182 ms, GL 4 164 ms, GL 5 396 ms (that machine was not quiet, so they may be high).

No regression means: both runs PASS (the tests' own bars), no run is refused, and the row counts and sections are the same. The medians are recorded beside the figures before, with no threshold of this plan's own. When a run fails or is refused, hand back `STOPPED` with the failing lines of its log; change no bar.

- [ ] **Step 9: Write the ledger record**

Read the end of `books/docs/plans/general-ledger-ledger.md` (from line 797) and append at the end of the file, in the form of the record `#### Task 9b-3`, a record `#### Task 29: every value in a statement is bound (core `server/reports`, Books `server`, 2026-11-20)` with:

- The cause: the rule "bind every value in SQL" and the audit (`sql-literal-audit.md`); the human's decision of 2026-11-20 (bind the audit's places and the seven row limits; the `x'ff'` sentinel stays).
- The change, one line each: `startOfMonth` binds `"01"`; a percent binds its scale (2 places); a blank pivot cell's bounds are bound; the seven row limits are bound and `limitSQL` is gone; the four Books statements bind the posted flag, the status and the origin.
- One fact the next reader needs: SQLite prepares a statement with a bound `LIMIT` a second time at its first step, so `TestEveryReportStatementIsPreparedOnce` became `TestAReportStatementIsPreparedAgainOnlyForItsLimit`.
- The tests: core `TestAPercentBindsItsScale`, `TestStartOfMonthBindsItsDay`, `TestABlankPivotBoundIsBound`, `TestEveryRowLimitIsBound`, `TestAReportStatementIsPreparedAgainOnlyForItsLimit`; Books `TestLedgerStatementsBindTheirValues`.
- The 100k figures of step 8 beside those before, in a table of the columns `| lines | run | before | after |`: every run of `st-results.md` and of `gl-results.md`. The core and Books hashes of step 7, and whether the machine was quiet.
- Not done here: the gcut limit of plan part 28, bound when part 28 is revised.

- [ ] **Step 10: Commit the record**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add docs/plans/general-ledger-ledger.md
git commit -m "docs(ledger): the record of task 29, every value in a statement is bound"
```
