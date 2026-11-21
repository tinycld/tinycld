# Report Engine Scale, Part 28: A Page's Detail Statement Computes the Rows of the Groups It Can Reach (task 28a) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The detail statement of a paged grouped report run computes its select (the sort keys, the running values, the record joins) over the rows of the open groups its page can reach only, so that a page of 201 rows no longer computes the rows of every open group of the page and throws most of them away.

**Architecture:** The core report engine orders a page's detail rows by their open group's place (`gord.__g`) and cuts the page with `LIMIT limit+1`. The groups up to the first one at which the row count reaches `limit+1` hold the whole page. Two new CTEs after `gord`, written by one new function `leafRows`: `gcut` (a scalar: the place of that group, from a count of rows for each open group) and `wl` (the rows of the groups up to it, materialized). The detail select then reads `wl` in place of `w`/`ws`. A group held by a cursor counts as no rows and is always kept. A select with a percent of the whole report, or with a probed condition, is not cut. A test switch `legacySQL.everyLeafRows` writes the statement as before, so tests compare the two results.

**Tech Stack:** Go; core report engine (`tinycld.org/core/reports`, SQLite through `modernc.org/sqlite`); the Books Go package (`tinycld.org/packages/books`) runs once as a guard, and its scale measures are run by the driver only.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md`, section "State after the part 28 planner", and the hand-off note it names, `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/handoff-backend-planner-part28.md`. Bar (human, 2026-11-20): every General Ledger run under 2 s at 500k lines; run "2 all accounts, year" is 2.73 s (`scale-500k-final-gl/gl-results.md`).

## The evidence this plan is written from

All in `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part28-scratch/`. The change was built and run through `go test -overlay`; no repo file was changed.

1. `out-before/parts.md` (500k, the tree as is, medians of 5): General Ledger run 2 is 2.624 s = section rows 355 ms + Total row 391 ms + detail page 1,861 ms. The detail page computes its select over the 94,512 period rows of all 106 open sections, to show 201 rows of the first section (11,833 rows).
2. `out-after/parts.md` (500k, the change, near quiet machine, estimates ± 5 %): run 1 589 → 507 ms, run 2 2.624 → 1.689 s, run 3 236 → 231 ms and run 4 152 → 152 ms (one open group: the same SQL), run 5 1.359 → 1.073 s. Run 2's detail page: 924 ms (`w` 317, the sections again 229, `ws` 31, `gcut` 45, `wl` 76, the page and its cells over 11,833 rows 226).
3. Two facts the build found. `wl` must be `MATERIALIZED`: inline, SQLite flattens it into the page select and reads `gord` with `SCAN gord` for each row (`quick1.log`; `TestAGroupedRunOfManyOpenGroupsReadsItsDetailsOnce` times out). The cut must be a scalar (`gcut`): a CTE that selects from `gord` puts `SCAN gord` in the plan, which `TestAGroupedDetailStatementReadsItsRowsOnce` and Books `TestTransactionDetailReadsItsPeriodRowsOnce` refuse.
4. The built files: `core/run.go`, `core/detail.go`, `core/rows.go`, `core/legacy.go` (each the file after the change), `core/leafrows_test.go` (the new test file). `red1.log` (the switch and the tests, no change: 3 FAIL, 3 pass), `green1.log` (the 6 tests: `ok`), `pkg2.log` (the whole core `reports` package with the change and the new test file: `ok`), `books-pkg2.log` (the whole Books Go package with the core change, 382 s: one FAIL, every other test passes).
5. **OPEN: this plan is NOT ready to run.** The one FAIL of `books-pkg2.log` is Books `TestTransactionDetailReadsItsPeriodRowsOnce` (`books/server/report_statements_scale_test.go:419`, "the rows do not find their section by its keys"). The plan lines it collects hold no `SCAN gord` and no `SEARCH t1` (correct), and three `SEARCH gord USING AUTOMATIC … (g0=? AND g1=?)` (the count, `wl`, the page), but no `SCAN t1`: the test requires a `SCAN t1` or `SCAN d` line, and with the cut the page select reads `wl`. Not yet known: which line now reads the period's rows once (the full plan of the statement was not read). Step 8 as written goes red on this test. The task needs one more step, in Books, that states the test's positive condition for the cut statement, written from the real plan lines; see `handoff-backend-planner-part28b.md`.

## Global Constraints

- Workspace root: `/Users/nas/tmp/tinycld/books-purchases`. Core: `/Users/nas/tmp/tinycld/books-purchases/tinycld`; Books: `/Users/nas/tmp/tinycld/books-purchases/books`; both on branch `books/p5-report-engine-scale`. Written against core `ee405c61`, books `f5ad1e22`. Every line number in this plan is of a file before this plan's edits.
- This is a core task: the implementer edits core `core/server/reports/` and Books `docs/plans/` only. Books builds against the core tree of the workspace, so the Books tests see the core change with no publish step.
- Core names no package: no Books name (`books`, `journal`, `account`, `ledger`, a collection or a report name) in core code, core tests or core comments. The core tests use the zoo fixture only.
- The result of every report run must not change: the same rows, in the same order, on the same pages, with the same cursors (one case is open: Review Focus, the line marked OPEN). `requireSameAsLegacy` and the page comparisons prove it; a difference is a defect of the change. Do not change an expected value of an existing test.
- With `legacySQL.everyLeafRows` on, and for every statement that is not cut, the SQL is the SQL of before, character for character.
- Never raise a time limit to pass (`TestAGroupedRunOfManyOpenGroupsReadsItsDetailsOnce` and `TestAGroupedRunOfAUnionWithManyOpenGroupsReadsItsRowsOnce`, 2 s for each statement).
- The strings `"JOIN gord ON"` and `"gord AS MATERIALIZED ("` stay in the SQL; the plan of the page statement must hold no `SCAN gord`.
- No migration, no client change, no new index.
- The implementer starts no scale run and takes no timing: the scale runs are a `NEEDS-RUN` for the driver (step 10). A timing is valid only on a quiet machine: `pgrep -fl 'chrome-headless-[s]hell'` prints nothing and the 1-minute load is under 4.
- Run the whole core `reports` package once (step 7) and the whole Books Go package once (step 8). Other steps run named tests.
- Do not edit `books/docs` before step 11.
- Steps 9 and 12 commit. When the implementer's prompt does not allow commits, hand the file list of the step to the driver in place of the commit. Never push.

## Review Focus

- A cursor inside a group with a short page: the next page holds that group, shows only its rows after the cursor, and must find them though they are not counted. Pinned by `TestThePagesAreTheSameOverTheGroupsAPageCanReach` (limits 1, 2, 3, 5; it requires that a cut statement with a held group ran).
- A group that moved between pages (its sort value changed): the rows the next page reads by id must be among the rows the statement keeps. Pinned by `TestAMovedGroupIsReadOverTheGroupsAPageCanReach`.
- OPEN (found by the plan review, by reading; not built, not pinned): a held group that is NOT one of the page's leaves. `heldLeaves` drops it (`at < 0`), and `wl` joins `gord`, so its rows are not in `wl`. Before the change `readShownNow` read its row by id over `w`/`ws` and wrote the row's current sort values into `rn.shown`, which `heldAfter` (`assemble.go:517`) copies into the next cursor's `Held`. With the cut the row is not found, so `Page.Next` may differ from the cursor of before on that page (the rows are expected to be the same: a later page where the group is a leaf reads the id again). Input: `byAnimalWeight`, Limit 2, a reader with 5 or more animals; page 1 ends inside the lightest animal; set its weight to 999; page 2 has two or more leaves and the moved animal is not one of them. `TestAMovedGroupIsReadOverTheGroupsAPageCanReach` does not reach it (bob reads 2 animals). This case needs a test (walk with `requireLeafPagesSame`) before step 9; when it is red, the candidate fix is that `readShownNow` reads the shown rows over the table before the cut (`all`), not over `wl`. Neither the test nor the fix is in this plan's steps: see the hand-off note `handoff-backend-planner-part28b.md`. Do not start the task before the driver settles this line.
- A blank group key and one key in two currencies: the count for each group and the kept rows must match the groups as the page does. Pinned by `TestThePagesAreTheSameOverTheGroupsAPageCanReach` (`addOddTreats`, four readers, a union source).
- A running value across pages and a percent of the parent: each page computes only some groups, and the pages must still join into the one-page run. Pinned by `TestPagesOverTheGroupsTheyReachJoinIntoTheWholeRun`.
- A percent of the whole report, and a probed condition, read every open group: their statement must not be cut. Pinned by `TestAPercentOfTheWholeReportKeepsEveryGroupsRows` and `TestAProbedConditionKeepsEveryGroupsRows`.
- Accepted, not pinned by a time: a page that every row fits on (an export, a limit above the row count). `gcut` is NULL and `wl` is a copy of every row, so the statement pays one count and one wide copy more than before. The result is pinned (`TestAPageComputesTheRowsOfTheGroupsItCanReach`, limit 1000: 504 rows; the limit 1000 of the pages test); the cost is not measured. The reviewer reads the 100k `TestStatementsScale` figures of step 10 for a run that became slower.

---

### Task 28a: A page's detail statement computes the rows of the groups it can reach

**Files:**
- Modify: `tinycld/core/server/reports/legacy.go` (the last field of `legacySQL`, lines 30-34)
- Create (test): `tinycld/core/server/reports/leafrows_test.go`
- Modify: `tinycld/core/server/reports/detail.go` (`windowFrame`, line 110; `windowSQL`, line 629)
- Modify: `tinycld/core/server/reports/rows.go` (`compiler`, line 115)
- Modify: `tinycld/core/server/reports/run.go` (`runner`, line 363; `pagedGroupPass`, line 778; `detailPass`, lines 1513-1526 and 1536-1538; `leafFrom`, lines 1803-1809; `detailSelect`, line 2013)
- Modify: `books/docs/plans/general-ledger-ledger.md` (a new record at the end, after the record of task 9b-3, which starts at line 797)

**Interfaces:**
- Consumes (exist, do not change): core `func (c *compiler) leafOrder(s *stmt, leaves [][]any) string` (the CTE `gord`), `func (c *compiler) leafJoin(row string) string`, `func (rn *runner) shownBefore()` (its first result is the held groups, each with a `Group` key), `keyString`, `limitSQL(int64) string`, `(*stmt).bind`, `rn.pageLimit()`, `rn.pagedGroups()`, `c.detailSortMixed`, the switch `legacySQL.plannedLeafJoin`. Core test helpers `newZoo(t)`, `newUnionZoo(t)`, `bulkFeedings(t, z, n)`, `withF7(t, z)`, `withEuroFeeding(t, z)`, `addOddTreats(t, z)`, `observeReads(t) *[]readStatement`, `requireSameAsLegacy(t, z, user, spec, opts, params, off *bool)`, `groupedPages`, `mergedRows`, `z.run`, `z.feeding`, `z.recs`, the specs `pagedBySpecies`, `byNoteClosed`, `byAnimalWeight`, `mealsByAnimal`, `mealsByNote`, the formula `allCosts`, and `mealDays`.
- Produces: core `func (c *compiler) leafRows(s *stmt, table string, held []int, want int) []string` (the CTEs `gcut` and `wl`), `func (c *compiler) leafOn(alias string) string`, `func (rn *runner) heldLeaves(leaves [][]any) []int`, the fields `runner.leafCut bool`, `compiler.detailWholeReport bool`, `windowFrame.wholeReport bool`, and the test switch `legacySQL.everyLeafRows bool`. A later task (28b, only if the driver asks for it) reads `wl`, `leafRows` and `runner.leafCut`.

- [ ] **Step 1: Add the test switch**

In `tinycld/core/server/reports/legacy.go`, add the field after `plannedLeafJoin bool` (line 33), before the closing brace:

```go
	// plannedLeafJoin joins a detail row to its open group with a plain JOIN
	// on the bare keys, whose order SQLite's planner picks, as before leafFrom
	// fixed the order with CROSS JOIN and took the keys' affinity off.
	plannedLeafJoin bool
	// everyLeafRows computes a paged detail statement over the rows of every
	// open group of its page, as before leafRows left out the groups the page
	// cannot reach.
	everyLeafRows bool
}
```

Nothing reads the field yet; it is here so that the tests of step 2 compile.

- [ ] **Step 2: Write the failing tests**

Create `tinycld/core/server/reports/leafrows_test.go`:

```go
package reports

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// pagedBySpeciesHaving is pagedBySpecies with a having on its group, so the
// detail statement reads its rows under the shown groups (ws).
var pagedBySpeciesHaving = strings.Replace(pagedBySpecies, `"opening":{"label":"Start"}}`,
	`"opening":{"label":"Start"},"having":{"op":"gt","args":[{"op":"count","args":[{"path":[{"field":"id"}]}]},{"value":0}]}}`, 1)

// speciesShares is pagedBySpecies with each feeding's share of its species.
var speciesShares = strings.Replace(pagedBySpecies, `"running","formula":{"op":"runningSum","args":[{"col":"cost"}]}}`,
	`"running","formula":{"op":"runningSum","args":[{"col":"cost"}]}},
	{"id":"ofParent","formula":{"op":"percentOfParent","args":[{"col":"cost"}],"scale":1}}`, 1)

// reportShares is speciesShares with each feeding's share of the whole report.
var reportShares = strings.Replace(speciesShares, `"ofParent","formula":{"op":"percentOfParent","args":[{"col":"cost"}],"scale":1}}`,
	`"ofParent","formula":{"op":"percentOfParent","args":[{"col":"cost"}],"scale":1}},
	{"id":"ofTotal","formula":{"op":"percentOfTotal","args":[{"col":"cost"}],"scale":1}}`, 1)

// reachedRows is how many rows the one page statement of seen computed its
// detail select over (the rows of wl), or -1 when it computed it over the
// rows of every open group (no wl). The page statement joins gord; the read
// of the shown rows and a probe of the conditions join it too.
func reachedRows(t *testing.T, z *zoo, seen []readStatement) int {
	t.Helper()
	found, n := 0, -1
	for _, st := range seen {
		if !strings.Contains(st.sql, "JOIN gord ON") || strings.Contains(st.sql, "WHERE d.__id IN (") || strings.Contains(st.sql, "SELECT EXISTS (") {
			continue
		}
		found++
		at := strings.Index(st.sql, "\nwl AS MATERIALIZED (")
		if at < 0 {
			continue
		}
		end := at + 1 + strings.Index(st.sql[at+1:], "\nSELECT ")
		if err := z.app.DB().NewQuery(st.sql[:end+1] + "SELECT count(*) FROM wl").Bind(st.params).Row(&n); err != nil {
			t.Fatal(err)
		}
	}
	if found != 1 {
		t.Fatalf("%d page statements that join the open groups, want 1", found)
	}
	return n
}

// A page shows its rows in the groups' order, so it ends in the first open
// group at which the groups up to it hold a page of rows. The detail
// statement computes its select (the sort keys, the running values) over the
// rows of the groups up to that one only: lion's 502 rows of 504, with and
// without a having on the group.
func TestAPageComputesTheRowsOfTheGroupsItCanReach(t *testing.T) {
	for name, spec := range map[string]string{"no having": pagedBySpecies, "a having": pagedBySpeciesHaving} {
		t.Run(name, func(t *testing.T) {
			z := newZoo(t)
			bulkFeedings(t, z, 500)
			seen := observeReads(t)
			res := z.run(t, "ann", spec, Options{Limit: 10}, nil)
			if got := reachedRows(t, z, *seen); got != 502 {
				t.Fatalf("the select was computed over %d rows, want lion's 502", got)
			}
			// The page: lion, its opening and its first 8 feedings.
			details := 0
			for _, r := range res.Rows {
				if r.Kind == KindDetail {
					details++
				}
			}
			if details != 8 {
				t.Fatalf("%d detail rows on the page, want 8", details)
			}
			// A page that every group fits on reads every group.
			*seen = nil
			z.run(t, "ann", spec, Options{Limit: 1000}, nil)
			if got := reachedRows(t, z, *seen); got != 504 {
				t.Fatalf("a page of 1000 computed %d rows, want 504", got)
			}
			legacySQL.everyLeafRows = true
			t.Cleanup(func() { legacySQL.everyLeafRows = false })
			*seen = nil
			z.run(t, "ann", spec, Options{Limit: 10}, nil)
			if got := reachedRows(t, z, *seen); got != -1 {
				t.Fatalf("with the switch on, the statement still has wl (%d rows)", got)
			}
		})
	}
}

// requireLeafPagesSame walks every page of spec, each page with the rows of
// every open group and with the rows of the groups it can reach, and requires
// the same page (rows, cursor) both ways. It returns the number of pages.
func requireLeafPagesSame(t *testing.T, z *zoo, user, spec string, opts Options, params map[string]string) (pages int) {
	t.Helper()
	for range 200 {
		pages++
		requireSameAsLegacy(t, z, user, spec, opts, params, &legacySQL.everyLeafRows)
		res := z.run(t, user, spec, opts, params)
		if res.Page == nil || res.Page.Next == "" {
			return pages
		}
		opts.Cursor = res.Page.Next
	}
	t.Fatal("the pages do not end")
	return pages
}

// Every page of every reader is the same whether the select is computed over
// every open group's rows or over the rows of the groups the page can reach:
// running values and a percent of the parent, a having, a union, a blank key
// and one key in two currencies, pages that end inside a group (the next page
// holds that group: its rows count as none, and it is read) and pages that
// every group fits on.
func TestThePagesAreTheSameOverTheGroupsAPageCanReach(t *testing.T) {
	z := newUnionZoo(t)
	addOddTreats(t, z)
	withF7(t, z)
	bulkFeedings(t, z, 12)
	seen := observeReads(t)
	for _, limit := range []int{1, 2, 3, 5, 1000} {
		open := Options{ExpandAll: true, Limit: limit}
		for _, spec := range []string{pagedBySpecies, pagedBySpeciesHaving, speciesShares, byNoteClosed} {
			requireLeafPagesSame(t, z, "ann", spec, open, nil)
		}
		for _, user := range []string{"ann", "bob", "dan", "eve"} {
			requireLeafPagesSame(t, z, user, mealsByAnimal, open, mealDays)
			requireLeafPagesSame(t, z, user, mealsByNote, open, nil)
		}
	}
	cut, held := 0, 0
	for _, st := range *seen {
		if strings.Contains(st.sql, "\nwl AS MATERIALIZED (") {
			cut++
			if strings.Contains(st.sql, "gord.__g NOT IN (") {
				held++
			}
		}
	}
	if cut == 0 || held == 0 {
		t.Fatalf("%d statements over the groups a page can reach, %d of them with a held group: the test compared nothing", cut, held)
	}
}

// Pages of a few rows join into exactly the one-page run: a running value
// goes on across pages and a percent of the parent divides by the whole
// group, though each page computed only the groups it could reach.
func TestPagesOverTheGroupsTheyReachJoinIntoTheWholeRun(t *testing.T) {
	z := newZoo(t)
	withF7(t, z)
	bulkFeedings(t, z, 12)
	for _, spec := range []string{pagedBySpecies, speciesShares, reportShares} {
		whole := z.run(t, "ann", spec, Options{Limit: 1000}, nil)
		for _, limit := range []int{1, 3, 4} {
			if got := mergedRows(t, groupedPages(t, z, "ann", spec, Options{Limit: limit})); !reflect.DeepEqual(got, whole.Rows) {
				a, _ := json.Marshal(got)
				b, _ := json.Marshal(whole.Rows)
				t.Fatalf("pages of %d:\n%s\nwhole:\n%s", limit, a, b)
			}
		}
	}
}

// A percent of the whole report reads the rows of every group, so its
// statement keeps every open group's rows.
func TestAPercentOfTheWholeReportKeepsEveryGroupsRows(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 500)
	seen := observeReads(t)
	z.run(t, "ann", reportShares, Options{Limit: 10}, nil)
	if got := reachedRows(t, z, *seen); got != -1 {
		t.Fatalf("the select of a percent of the whole report was computed over %d rows, want every group's", got)
	}
}

// A condition that may be unknown on a row is probed over every open group
// of the page (probeConditions), so such a statement keeps every open group's
// rows.
func TestAProbedConditionKeepsEveryGroupsRows(t *testing.T) {
	z := newZoo(t)
	withEuroFeeding(t, z)
	spec := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal","open":true}],
		"sort":[{"formula":` + allCosts + `},{"col":"note"}]}`
	seen := observeReads(t)
	z.run(t, "ann", spec, Options{Limit: 2}, nil)
	if got := reachedRows(t, z, *seen); got != -1 {
		t.Fatalf("the select of a probed condition was computed over %d rows, want every group's", got)
	}
	probes := 0
	for _, st := range *seen {
		if strings.Contains(st.sql, "SELECT EXISTS (") && strings.Contains(st.sql, "__smx0 = 1") && !strings.Contains(st.sql, "__level") {
			probes++
		}
	}
	if probes != 1 {
		t.Fatalf("%d probes of the sort key, want 1: the spec no longer has a probed condition", probes)
	}
}

// A group that moved between pages (its record's sort value changed) is read
// wherever it sorts now: the rows the next page reads by id (readShownNow)
// are among the rows the statement keeps.
func TestAMovedGroupIsReadOverTheGroupsAPageCanReach(t *testing.T) {
	z := newZoo(t)
	z.feeding(t, "m2", z.recs["mo"], 40, "2026-02-20", 5)
	first := z.run(t, "bob", byAnimalWeight, Options{Limit: 2}, nil)
	if first.Page == nil || first.Page.Next == "" {
		t.Fatalf("the first page has no next cursor: %+v", first.Page)
	}
	mo := z.recs["mo"]
	mo.Set("weight", 999)
	if err := z.app.Save(mo); err != nil {
		t.Fatal(err)
	}
	seen := observeReads(t)
	if n := requireLeafPagesSame(t, z, "bob", byAnimalWeight, Options{Limit: 2, Cursor: first.Page.Next}, nil); n < 2 {
		t.Fatalf("%d pages after the move, want 2 or more", n)
	}
	shownRead := 0
	for _, st := range *seen {
		if strings.Contains(st.sql, "WHERE d.__id IN (") && strings.Contains(st.sql, "\nwl AS MATERIALIZED (") {
			shownRead++
		}
	}
	if shownRead == 0 {
		t.Fatal("no read of the shown rows ran over the groups a page can reach: the test compared nothing")
	}
}
```

- [ ] **Step 3: Run the tests to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go vet ./reports/ && go test -count=1 -run 'TestAPageComputesTheRowsOfTheGroupsItCanReach|TestThePagesAreTheSameOverTheGroupsAPageCanReach|TestPagesOverTheGroupsTheyReachJoinIntoTheWholeRun|TestAPercentOfTheWholeReportKeepsEveryGroupsRows|TestAProbedConditionKeepsEveryGroupsRows|TestAMovedGroupIsReadOverTheGroupsAPageCanReach' ./reports/`

Expected: `go vet` prints nothing. Three tests FAIL; the other three pass (they are guards: with no change, no statement is cut, and the switch changes nothing):

```
--- FAIL: TestAPageComputesTheRowsOfTheGroupsItCanReach
        leafrows_test.go:65: the select was computed over -1 rows, want lion's 502
--- FAIL: TestThePagesAreTheSameOverTheGroupsAPageCanReach
    leafrows_test.go:144: 0 statements over the groups a page can reach, 0 of them with a held group: the test compared nothing
--- FAIL: TestAMovedGroupIsReadOverTheGroupsAPageCanReach
    leafrows_test.go:233: no read of the shown rows ran over the groups a page can reach: the test compared nothing
FAIL	tinycld.org/core/reports
```

Both subtests of the first test fail with the same line.

- [ ] **Step 4: Mark a select that reads the whole report**

In `tinycld/core/server/reports/detail.go`, add the field at the end of `windowFrame`, after `windows []string` (line 110):

```go
	paged   bool
	windows []string
	// wholeReport is set when a percent of the whole report was compiled on
	// the frame: its value on a row reads the rows of every group, so no group
	// may be left out below it (leafRows).
	wholeReport bool
}
```

In the same file, `windowSQL` (line 609) ends with the `percentOfParent` case (line 626) and then the return of the percent of the total (line 629). Add one line before that return:

```go
	case "percentOfParent":
		return expr{sql: f.window(fmt.Sprintf("tc_pct(%s, SUM(%s) OVER (%s), %d)", num(e), num(e), partitionBy(f.parent), scale)), typ: Percent, win: true}, nil
	}
	f.wholeReport = true
	return expr{sql: f.window(fmt.Sprintf("tc_pct(%s, SUM(%s) OVER (%s), %d)", num(e), num(e), partitionBy(f.total), scale)), typ: Percent, win: true}, nil
}
```

In `tinycld/core/server/reports/rows.go`, add the field to `compiler` after `detailCells func(page, by string) string` (line 115):

```go
	detailCells func(page, by string) string
	// detailWholeReport is true when the detail select last built holds a
	// percent of the whole report (windowFrame.wholeReport).
	detailWholeReport bool
```

In `tinycld/core/server/reports/run.go`, `detailSelect` ends with these lines (line 2013); add the middle one:

```go
	rows, cells := pagedCells(frame, table, sel, early)
	c.detailCells = cells
	c.detailWholeReport = frame.wholeReport
	return rows, order, nil
}
```

- [ ] **Step 5: Write the cut (`run.go`)**

All in `tinycld/core/server/reports/run.go`.

(a) In `runner`, add the field after the comment and field `shown []held` (line 363), before the comment of `readDetails`:

```go
	shown []held
	// leafCut is true when the page's detail statement may leave out the rows
	// of the open groups its page cannot reach (leafRows): its select holds no
	// percent of the whole report, which reads the rows of every group, and
	// probes no condition, which is probed over every open group of the page.
	leafCut bool
```

(b) In `pagedGroupPass` (line 745), set it right after the `detailSelect(keysOf, "w")` call (lines 776-778). `detailPass` needs the decision before it compiles its own select, because the decision changes the table name that select reads:

```go
	if _, rn.groupKeys, err = c.detailSelect(keysOf, "w"); err != nil {
		return nil, err
	}
	rn.leafCut = !c.detailWholeReport && len(c.detailSortMixed) == 0
	rn.groupFP = groupFingerprint(c.groupLevels(), c.inlineLevels(), levelShape(rn.groupShape), foldPrint(rn.expand), rn.groupKeys)
```

(c) In `detailPass` (it starts at line 1502), `limit` and `paged` move up before the `detailSelect` call, and the table becomes `wl` when the statement is cut. Replace the lines from `table := "w"` (line 1513) to `paged := rn.pagedGroups()` (line 1526):

```go
	table := "w"
	if m > 0 {
		table = "ws"
		shown = append(shown, "ws AS (SELECT w.* FROM w WHERE "+c.inShown(m, "w")+")")
	}
	sel, order, err := c.detailSelect(r, table)
	if err != nil {
		return nil, err
	}
	limit := rn.pageLimit()
	rn.readDetails = true
	ctes := append([]string{r.render()}, shown...)
	grouped := len(c.p.groups) > 0
	paged := rn.pagedGroups()
```

with

```go
	table := "w"
	if m > 0 {
		table = "ws"
		shown = append(shown, "ws AS (SELECT w.* FROM w WHERE "+c.inShown(m, "w")+")")
	}
	limit := rn.pageLimit()
	paged := rn.pagedGroups()
	// A page of two or more open groups computes only the rows of the groups
	// it can reach (leafRows), when its select lets it (leafCut).
	cut, all := paged && rn.leafCut && len(leaves) > 1 && !legacySQL.everyLeafRows, table
	if cut {
		table = "wl"
	}
	sel, order, err := c.detailSelect(r, table)
	if err != nil {
		return nil, err
	}
	rn.readDetails = true
	ctes := append([]string{r.render()}, shown...)
	grouped := len(c.p.groups) > 0
```

(d) In the same function, the `case paged:` of the switch (line 1536) adds the two CTEs after `gord`. Replace

```go
	case paged:
		ctes = append(ctes, c.leafOrder(s, leaves))
		from = c.leafFrom(sel)
```

with

```go
	case paged:
		ctes = append(ctes, c.leafOrder(s, leaves))
		if cut {
			ctes = append(ctes, c.leafRows(s, all, rn.heldLeaves(leaves), limit+1)...)
		}
		from = c.leafFrom(sel)
```

The `case grouped:` (the unpaged grouped case, a layout) is not changed. `readShownNow(s, ctes, sel, order)` below takes the same `ctes` and `sel`, so its read of the shown rows by id runs over `wl` too; a held group that is an open group of the page (a leaf) is always in `wl`. A held group that is not a leaf of the page is not in `wl` (see Review Focus, the line marked OPEN).

(e) Replace the body of `leafFrom` (lines 1803-1809; keep its comment) and add the three functions after it:

```go
func (c *compiler) leafFrom(sel string) string {
	return fmt.Sprintf("(SELECT d.*, gord.__g AS __g FROM (%s) d %s) d", sel, c.leafOn("d"))
}

// leafOn joins the rows read as alias to their open group, as leafFrom
// explains.
func (c *compiler) leafOn(alias string) string {
	join, row := "CROSS JOIN", "+"+alias+"."
	if legacySQL.plannedLeafJoin {
		join, row = "JOIN", alias+"."
	}
	return join + " gord ON " + c.leafJoin(row)
}

// leafRows is the CTEs of the rows a page's detail statement computes: wl,
// the rows of table under the open groups the page can reach. A page shows
// its rows in the groups' order and holds at most want of them, so it ends in
// the first group at which the groups up to it hold want rows (gcut, NULL
// when all of them hold fewer: every group is read). The rows of the groups
// after it would be sorted, and their running values computed, to be thrown
// away. A group in held (its place in gord) shows only its rows after a
// cursor: its rows count as none, and it is read wherever it is, so the rows
// readShownNow asks for are there.
//
// What the select reads of other rows is the same over wl: a running value
// and a percent of the parent read the rows of the row's own group, all of
// which wl holds, and a keyed layer reads its group keys. A percent of the
// whole report reads every group's rows, and a probed condition is probed
// over every open group: neither select is cut (runner.leafCut).
func (c *compiler) leafRows(s *stmt, table string, held []int, want int) []string {
	on := c.leafOn("w")
	counted, kept := "", ""
	if len(held) > 0 {
		places := make([]string, len(held))
		for i, at := range held {
			places[i] = s.bind(int64(at))
		}
		in := strings.Join(places, ", ")
		counted, kept = " WHERE gord.__g NOT IN ("+in+")", " OR gord.__g IN ("+in+")"
	}
	return []string{
		fmt.Sprintf("gcut AS MATERIALIZED (SELECT MIN(c.__g) AS __g FROM (SELECT n.__g AS __g, SUM(n.n) OVER (ORDER BY n.__g ROWS UNBOUNDED PRECEDING) AS seen FROM (SELECT gord.__g AS __g, COUNT(*) AS n FROM %s w %s%s GROUP BY gord.__g) n) c WHERE c.seen >= %s)",
			table, on, counted, limitSQL(int64(want))),
		fmt.Sprintf("wl AS MATERIALIZED (SELECT w.* FROM %s w %s WHERE gord.__g <= COALESCE((SELECT __g FROM gcut), gord.__g)%s)", table, on, kept),
	}
}

// heldLeaves are the places in leaves of the groups whose first details
// earlier pages showed (shownBefore).
func (rn *runner) heldLeaves(leaves [][]any) []int {
	shown, _ := rn.shownBefore()
	var out []int
	for _, h := range shown {
		if at := slices.IndexFunc(leaves, func(k []any) bool { return keyString(k) == keyString(h.Group) }); at >= 0 {
			out = append(out, at)
		}
	}
	return out
}
```

Three rules of this SQL, each found by a failed build; do not change them:
- `wl` is `MATERIALIZED`. Inline, SQLite flattens it into the page select and reads `gord` with a scan for each row.
- The cut is the scalar `(SELECT __g FROM gcut)`. A CTE that selects the kept groups from `gord` puts `SCAN gord` in the plan.
- `gcut` and `wl` join `gord` through `leafOn("w")`: the same `CROSS JOIN` and unary `+` as the page's own join, so the rows are read once and find their group through SQLite's automatic index on `gord`.

`leafFrom` with the switch `plannedLeafJoin` on writes the same SQL as before this step (`JOIN gord ON d.k0 IS gord.g0 …`).

- [ ] **Step 6: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go vet ./reports/ && go test -count=1 -run 'TestAPageComputesTheRowsOfTheGroupsItCanReach|TestThePagesAreTheSameOverTheGroupsAPageCanReach|TestPagesOverTheGroupsTheyReachJoinIntoTheWholeRun|TestAPercentOfTheWholeReportKeepsEveryGroupsRows|TestAProbedConditionKeepsEveryGroupsRows|TestAMovedGroupIsReadOverTheGroupsAPageCanReach|TestAGroupedRunOf|TestAGroupedDetailStatement|TestTheOpenGroupJoin' -v ./reports/`

Expected: `go vet` prints nothing; PASS for the six new tests (`TestAPageComputesTheRowsOfTheGroupsItCanReach` with its two subtests) and for the tests of the open-group join (`TestAGroupedRunOfManyOpenGroupsReadsItsDetailsOnce`, `TestAGroupedRunOfAUnionWithManyOpenGroupsReadsItsRowsOnce`, `TestAGroupedDetailStatementReadsItsRowsOnce`, `TestAGroupedDetailStatementOfACollectionFindsItsGroupByAnIndex`, `TestTheOpenGroupJoinOrderKeepsTheResult`, `TestTheOpenGroupJoinMatchesAKeyOfEveryKind`, `TestTheOpenGroupJoinOrderKeepsEveryPage`).

If one of the two `TestAGroupedRunOf…` tests fails on its time limit, or a plan test finds `SCAN gord`, one of the three rules of step 5 is broken: read the statement's SQL, do not raise the limit.

- [ ] **Step 7: Run the core reports package once**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -count=1 ./reports/`

Expected: `ok  tinycld.org/core/reports` (about 150 s). A failure of a test that compares rows, pages or cursors is a result the cut changed: find which group's rows the statement left out (print `SELECT count(*) FROM wl` as `reachedRows` does); do not change the expected rows.

- [ ] **Step 8: Run the Books Go package once**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go test -count=1 -timeout 30m .`

Expected: `ok  tinycld.org/packages/books` (about 7 minutes). Every paged Books report with sections runs through the cut. A failure of a test that compares rows is a result the cut changed: fix it in core as in step 7; do not change the expected rows and do not edit a Books test.

- [ ] **Step 9: Commit the core change**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/run.go core/server/reports/detail.go core/server/reports/rows.go core/server/reports/legacy.go core/server/reports/leafrows_test.go
git commit -m "perf(reports): a page's detail statement computes the rows of the groups it can reach"
```

The runs of step 10 are then of a committed tree; write the hash in the ledger record.

- [ ] **Step 10: Hand the scale runs to the driver**

Hand back (the driver runs them, one after the other, and sends an agent back for steps 11 and 12 with the files):

```
NEEDS-RUN: (quiet machine: `pgrep -fl 'chrome-headless-[s]hell'` prints nothing and the first number of `sysctl -n vm.loadavg` is under 4) cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task28-gl go test -run 'TestGLScale$/500k$' -count=1 -timeout 120m -v .
NEEDS-RUN: (quiet machine, as above) cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task28-st go test -run 'TestStatementsScale$/100k$' -count=1 -timeout 60m -v .
```

Needed from the first: PASS (the test holds the 2 s bar of every run at 500k); `scale-task28-gl/gl-results.md`: the five medians and the sections on the page, beside `scale-500k-final-gl/gl-results.md` (run 1 614 ms, run 2 2.73 s, run 3 246 ms, run 4 164 ms, run 5 1.422 s; that run's load was 7.8 to 9.3). The planner's probe gave run 2 about 1.69 s and run 5 about 1.07 s. From `scale-task28-gl/gl-plans.md`, run "2 all accounts, year", the statement that has `gord` lines: no `SCAN gord`.

Needed from the second: PASS; `scale-task28-st/st-results.md`: every median, no run refused, the group rows and rows of each run as in `scale-task9b3/st-results.md`. No run slower than there on a quiet machine: TB 1 345 ms, TB 2 153 ms, TB 3 181 ms, BS 1 455 ms, BS 2 504 ms, TD 1 62 ms, TD 2 232 ms, TD 3 30 ms, TD 4 19 ms, TD 5 135 ms, TD 6 34 ms. Transaction Detail is paged and grouped, so its runs with two or more open sections take the cut; Trial Balance and Balance Sheet are layouts, with the same SQL as before.

A run with a higher median than before on a quiet machine is a result: record it, and do not change the code in this step. When run 2 of the first passes the bar with less than about 10 % to spare, say so in the hand-back: the outline holds two further tasks (28b, 28c) for that case.

- [ ] **Step 11: Write the ledger record**

Read the end of `books/docs/plans/general-ledger-ledger.md` (the record `#### Task 9b-3: a page's detail rows find their open section through an index (core `server/reports/run.go`, 2026-11-20)`, line 797 at books `f5ad1e22`, runs to the end of the file) and append after it, in the same form, `#### Task 28a: a page's detail statement computes the rows of the sections it can reach (core `server/reports/run.go`, 2026-11-20)` with:

- The cause, from this plan's section "The evidence this plan is written from", item 1: at 500k, run 2 was section rows 355 ms + Total row 391 ms + detail page 1,861 ms; the detail page computed its select (the sort keys, the running balance, the record joins) over the 94,512 period rows of all 106 open sections, to show 201 rows of the first section (11,833 rows).
- The change: core `leafRows` writes two CTEs after `gord`: `gcut` (the place of the first open section at which the row count reaches the page size + 1) and `wl` (the rows of the sections up to it, materialized), and the detail select reads `wl`. A section held by a cursor counts as no rows and is always kept. Not cut: a select with a percent of the whole report, a select with a probed condition, a run with one open section, a layout.
- The two facts of item 3 (`wl` must be `MATERIALIZED`; the cut must be a scalar).
- The tests: the six core tests of `leafrows_test.go` by name.
- The 500k figures of step 10 beside those before, in a table of the columns `| lines | run | before | after |`: every run of `scale-task28-gl/gl-results.md` (before: `scale-500k-final-gl/gl-results.md`). The 100k figures of every run of `scale-task28-st/st-results.md` in a second table of the same columns (before: `scale-task9b3/st-results.md`). The core hash of step 9, the Books hash the runs ran on, and whether the machine was quiet (the load at the start of each run).
- Whether every General Ledger run is under the 2 s bar at 500k, and run 2's margin. What is left of run 2 (from item 2: the section rows statement, the Total row statement, and in the detail page `w` and the sections computed again), with the two options not built: the Total row read in the section rows statement (about -345 ms, an estimate), and no `shown` CTEs under `wl` (about -230 ms, an estimate).

- [ ] **Step 12: Commit the record**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add docs/plans/general-ledger-ledger.md
git commit -m "docs: record the page's reachable sections cut and its 500k General Ledger run"
```
