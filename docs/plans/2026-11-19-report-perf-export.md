# Report Export In One Page (p5-report-perf, export) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The CSV export of the A/R Aging at 500,000 journal lines meets its bars (5 s, and at most 15 times its time at S), because core's export reads a report in one page and the detail statement computes its open groups once.

**Architecture:** Core's export (`readWholeReport`) reads the report through `Run` in pages of `runLimits.maxPage` (1,000) rows, and each page's group statement computes every group level over all rows again: pages x rows. Task 1 gives `Request` a `PageCap` that only the export sets, so the export is one `Run` of `maxRows+1` rows; a page that is past `maxRows` or that has a next page is refused as today. Task 2 makes the one detail statement linear: its join to the open groups (`gord`, a `json_each` over one bound list) is written `AS MATERIALIZED`, so SQLite reads the list once and not once per detail row.

**Tech Stack:** Go, PocketBase fork (dbx, SQLite through modernc), core `reports` package; Markdown for the record.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/csv-rootcause.md` (root cause and the proposed change A) and `/Users/nas/tmp/tinycld/books-purchases/books/docs/specs/ar-aging-design.md` §9 (the bars). Outline: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/plan-outline-export.md`. Scale numbers before this plan: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/scale-stored/perf-results.md` and the ledger `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/ar-aging-ledger.md`, section Performance.

**Repos:** workspace `/Users/nas/tmp/tinycld/books-purchases`. Tasks 1 and 2 are core tasks: repo `/Users/nas/tmp/tinycld/books-purchases/tinycld`, branch `books/p5-report-perf`, package `core/server/reports` (Go module `tinycld.org/core`, package `reports`). All Go commands of tasks 1 and 2 run in `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports`. The last step writes in `/Users/nas/tmp/tinycld/books-purchases/books` (branch `books/p5-report-perf`). No push.

## Global Constraints

- The bars of spec §9 are never changed, no run is removed, the dataset is not changed (run 7: at most 5,000 ms at L, and L at most 15 times S).
- Core must not name a package: no `books`, no Books collection or route in any core file or test. The tests use core's fictional zoo fixture (`newZoo`, `newSavedZoo`, `bulkFeedings`).
- No word "hosting", "saas", "tenant", "router" in core or Books.
- The export's answers do not change: the same rows in the same order, the same row refusals (`exportSheetRows`, `exportPDFRows`) for the same reports, one audit row per download. Every existing test of the `reports` package keeps its expectations; only the comment of `TestExportReadsEveryPage` changes.
- One time refusal changes, and the plan does not hide it: the limits stay 10 s for a statement and 60 s for an export, but the export's rows are now one statement under the 10 s limit, where each page of 1,000 rows had 10 s of its own. A report whose one-page statement takes more than 10 s is answered `exportTooLong` even when its pages fit in 60 s today. Measured, no such report was found: one page costs 1/25 to 1/200 of the pages' sum, and its longest statement is about two page statements of today (see the facts). The statement limit is NOT raised for the export in this plan; that is a decision for the driver (see the hand-back).
- `Request.PageCap` is set only by `readWholeReport`. The run endpoint (`endpoint.go:131`) never sets it; a run's page stays at most `runLimits.maxPage` rows.
- Code starts from a failing test that ran RED. Undo an edit with the Edit tool, never `git checkout`/`git restore`. Read `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/agent-rules.md` first.
- A run of more than about 4 minutes is a `NEEDS-RUN` for the driver (agent rules). The last step names one.
- Commits are path-limited (`git add <the files of the task>`), signed as the repo is configured (`commit.gpgsign=true`). When signing fails: stop and report it; never `--no-gpg-sign`, never `--no-verify`.

## Facts the planner measured (do not find again)

The code of tasks 1 and 2 was built into a `go test -overlay` copy of `core/server/reports` (core head `be0811ee`) and run. No repo file was changed.

- The limits of an export, as they are and as they stay:
  - `exportLimits` (`export.go:44-47`): a PDF holds 5,000 rows, a CSV or Excel file 50,000, an export stops after 60 s (`export.go:240`, one context for the whole read and the writer).
  - `runLimits` (`run.go:272`): each statement stops after 10 s (`readRows`, `run.go:607`; `ErrTimeout`, answered as `exportTooLong` by `exportRunError`, `export.go:306-311`); `maxPage` 1,000; `pageSize` 200; `rowCap` 5,000.
  - `readWholeReport` (`export.go:331`) passes `RowCap: maxRows + 1` (`export.go:347`), refuses a report of more than `maxRows` rows (`export.go:366-368`) and a run a cap stopped (`rowCap` or `nestCap` notice, `export.go:376-379`); `buildExport` answers the refusal with status 422 and the writer's `TooMany` (`export.go:247-249`).
- Memory: the export already holds every row of the file in one `[]Row` (at most `maxRows+1` rows and the Totals). With one page, the statement rows of the one run (at most `maxRows+2` group rows and `maxRows+2` detail rows as `map[string]any`) are also held until the run ends, where a page held about 2,000 of them. The row limit of the format bounds both; nothing is unbounded.
- One page is at most `maxRows+1` counted rows (`cutPage`, `assemble.go:456`: group, opening, detail and subtotal rows; a Total is not counted). A page ends one row short when a group row and its opening row do not both fit (`assemble.go:467-471`); it then has `Page.Next`. A next page means a counted row past `maxRows`, so "past `maxRows` or a next page" refuses by row count exactly the reports the loop refused. The loop, `withoutAppendedTotals` and `isWholeReportRow` are therefore removed; `uniqueNotices`, `joinNotices`, `union` and `isSectionRow` stay (other callers).
- The `Page.Next` clause of the new check is a defence that no test reaches: a paged grouped run with any row always ends in a Total (`totalPass`, `run.go:1005-1009`: "it always returns a row unless there are no rows at all"), so a page that is short by one row still holds `maxRows` counted rows and a Total, which is past `maxRows`. A flat, layout or nest page with a next page holds `maxRows+1` detail rows (`detailPass`, `run.go:1534-1547`). The clause stays so that a report with no Total row, if the engine ever writes one, is refused and not cut.
- The time refusal (the one that changes). Whole export and longest statement, this machine, tasks 1 and 2 against today:

  | report | rows | today: statements, sum, longest | one page: statements, sum, longest |
  |---|---|---|---|
  | flat (`feedingsFlat`) | 49,005 | 100, 8.3 s, 149 ms | 2, 0.29 s, 201 ms |
  | layout (`zooStatement`) | 49,012 | 100, 15.3 s, 175 ms | 2, 0.52 s, 370 ms |
  | few groups (`feedingsBySpeciesCSV`) | 40,009 | 123, 12.4 s, 134 ms | 3, 0.50 s, 313 ms |
  | one group per row (`byNoteClosed`) | 12,013 | 39, 19.2 s, 1.7 s | 3, 0.09 s, 40 ms |

  Each page of today computes the whole report again (keyset and Totals over every row), so the one statement costs about what two page statements cost today. An export that is refused after this plan and written today needs a page statement of 5 to 10 s today and at most about 10 pages: a report whose viewer page is itself near the 10 s refusal. Not measured: a nest (child rows are read by one statement per level, keyed by the page's parents as one bound list, `nest.go:58-111`), and a report with lookup columns.
- The run endpoint reads its body strictly: a body with `pageCap` (in `options` or beside it) is answered 400 today and after task 1.
- Statements of an export of 1,350 group, detail and subtotal rows with `maxPage` 100 (spec `byNoteClosed`, 446 bulk feedings): today 14 group-page, 14 Total and 14 detail statements; after task 1, one of each. A flat report of 450 rows: today 10 statements, after task 1, 2.
- Time of an export of a report with one group per row (`byNoteClosed`), this machine (darwin/arm64, 14 CPUs):

  | open groups | rows | today (pages of 1,000) | task 1 alone | tasks 1 and 2 |
  |---|---|---|---|---|
  | 2,000 | 6,013 | 4.4 s | 1.7 s | not measured |
  | 4,000 | 12,013 | 19.2 s | 6.8 s (detail statement 6.75 s) | 91 ms (detail 40 ms) |
  | 8,000 | 24,013 | refused at 60 s | refused at 10 s (the detail statement) | not measured |
  | 16,660 | 49,993 | refused at 60 s | refused at 10 s | 369 ms (group 125 ms, Total 79 ms, detail 165 ms) |

  With tasks 1 and 2 the time is linear: 16,000 / 32,000 / 64,000 open groups give a detail statement of 160 / 325 / 646 ms (the format limit was raised for this reading only).
- So task 2 is needed, and it is not fix (B) of the root-cause file. With task 1 alone the detail statement's time grows with rows x open groups in one statement, and an export between about 4,900 and 7,100 open groups, which takes 28 to 60 s today, would be refused at the 10 s statement limit. Where the time is: the detail statement joins `(…) d JOIN gord ON d.k0 IS gord.g0 …`, and `gord` is `json_each({:list})` read inline. The measured fact is the growth (inline: x4 for x2 groups; `MATERIALIZED`: x2). That SQLite reads the bound list again for each detail row is the planner's reading of it, not a measured fact.
- Few groups, many rows (`feedingsBySpeciesCSV`, 40,009 rows): today 12.4 s (123 statements), with tasks 1 and 2 0.5 s (3 statements).
- With `gord AS MATERIALIZED` and task 1, `go test -count=1 .` of the whole `reports` package is `ok` (99.6 s). `EXPLAIN QUERY PLAN` still prints `SCAN gord` (after `MATERIALIZE gord`), so a test cannot pin the plan text; task 2 pins the behavior (a run under a statement limit) and the statement text.
- Fix (B) (cursor pages share one materialized copy of the level CTEs) is NOT needed for this unit: after task 1 the export sends no cursor page, and runs 1 to 6 are one page each and meet their bars. It stays a possible gain for the viewer's later pages; it is not planned here.
- Not measured: the A/R Aging at L. From the profile (`profile-stored-L.txt`): one group statement 0.4 to 0.6 s, one Total 33 ms, the detail statements of 20 pages 1.75 s in sum (before task 2). The last step measures it.

## Review Focus

1. A report of exactly the format's limit, and of one row more, read in one page, in each format (a PDF keeps the reader's folds and holds 5,000 rows). Expected: the first is written whole, the second is refused with the format's sentence and writes no audit row; never a cut file. Tests: `TestExportOfAGroupedReportStopsAtTheFormatsLimit` (task 1, new); existing, run in task 1: `TestExportRefusesEachFormatPastItsLimit`, `TestExportAnswersAnExcelFile`, `TestExportAnswersAPDFFile`, `TestExportReadsAPivotPastTheEngineCap`. The `Page.Next` clause of the check has no test (see the facts): it is a named defence.
2. A client that asks the run endpoint for more than `maxPage` rows, or sends `pageCap`. Expected: a page of `maxPage` rows; `pageCap` is refused as an unknown field (400). The cap must not become a way to read 50,000 rows in one viewer request. Test: `TestTheRunEndpointKeepsMaxPage` (task 1).
3. A report with one group per row near the format's limit (a customer list with every customer open). Expected: the one detail statement ends well inside the 10 s statement limit. Test: `TestAGroupedRunOfManyOpenGroupsReadsItsDetailsOnce` (task 2).
4. A group with a blank key (a row with no customer), opened. Expected: its detail rows are still under it after `gord` is materialized (the join compares with `IS`). Tests, existing, run by name in task 2: `TestExpandOpensABlankGroup` (`groups_test.go:81`), `TestABlankGroupPagesInItsPlace` (`pagedgroups_test.go:146`).
5. A slow report: its one statement passes the 10 s statement limit, where its pages each had 10 s. Expected: the export's own sentence ("The report took too long to export."), status 408, no file, no audit row; never a partial file. Test: `TestExportPastTheStatementLimitIsATimeout` (task 1, new; it holds today and after). Whether such a report should get the export's 60 s for its one statement is the driver's decision; the plan keeps the limit.

---

### Task 1: The export reads one page

**Files:**
- Create: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/export_onepage_test.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/run.go:75-79` (`Request`), `:285-287` (`runner`), `:373` (`Run`), `:693-700` (`pageLimit`)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/export.go:23-25` (route comment), `:327-381` (`readWholeReport`), `:386-402` (`isWholeReportRow`, `withoutAppendedTotals` removed)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/export_test.go:257-258` (a comment)

**Interfaces:**
- Consumes (existing test helpers, package `reports`): `newSavedZoo(t) *savedZoo` (embeds `*zoo`), `(z *zoo) saved(t, key, owner, title, spec string, published bool) *core.Record`, `bulkFeedings(t *testing.T, z *zoo, n int)` (notes `n00001`…, cost 1), `withLimits(t, func(*limits))`, `keepExportLimits(t)`, `observeReads(t) *[]readStatement` (field `sql`), `(s *savedZoo) buildCSV(t, user, reportID, options string) *exportFile`, `(s *savedZoo) exportAs(t, user, body string) *httptest.ResponseRecorder`, `(s *savedZoo) call(t, user, method, url, body string) (int, string)`, `csvExportBody(reportID string) string`, `csvRecords`, `firstFields`, `requireStatus`, `exportAuditRows`, `isTotalStatement(sql string) bool`, consts `byNoteClosed` (`pagedgroups_test.go:54`), `feedingsFlat` (`export_test.go:29`), `runURL` (`endpoint_saved_test.go:9`).
- Produces: `Request.PageCap int`; `runner.askedPageCap int`; `(rn *runner) pageLimit() int` with the cap; `readWholeReport` as one `Run`. Test helpers in `export_onepage_test.go`: `isGroupPageStatement(sql string) bool`, `isGroupedDetailStatement(sql string) bool`, `byNoteRows(n int) []string`.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/export_onepage_test.go` with exactly this content (`TestPageLimit` is added in step 4: it reads a field that does not exist yet):

```go
package reports

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

// isGroupPageStatement is the level-row statement of a paged grouped run: its
// body reads the first level's rows beside their path (pagedGroupStatement).
func isGroupPageStatement(sql string) bool {
	return strings.Contains(sql, "FROM o1 o JOIN pp1 pp ON")
}

// isGroupedDetailStatement is the detail statement of a grouped run: it joins
// the open groups' order (leafOrder).
func isGroupedDetailStatement(sql string) bool {
	return strings.Contains(sql, "JOIN gord ON")
}

// byNoteRows is the first field of every line of byNoteClosed's CSV after the
// heading, over ann's four feedings and n bulk ones: a group row, its detail
// row and its subtotal for each note in order, then the Total.
func byNoteRows(n int) []string {
	notes := []string{"f1", "f2", "f3", "f4"}
	for i := 1; i <= n; i++ {
		notes = append(notes, fmt.Sprintf("n%05d", i))
	}
	out := make([]string, 0, 3*len(notes)+1)
	for _, note := range notes {
		out = append(out, note, note, "Total "+note)
	}
	return append(out, "Total")
}

// Each page of a grouped run computes every group level over every row, so an
// export in pages of maxPage rows costs pages x rows. The export reads its
// rows in one page: one statement of each kind, however many pages of maxPage
// rows the report holds (here 1,350 rows, 14 such pages).
func TestExportReadsAGroupedReportInOnePage(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "byNote", "ann", "Feedings by Note", byNoteClosed, false)
	bulkFeedings(t, s.zoo, 446)
	withLimits(t, func(l *limits) { l.maxPage = 100 })

	seen := observeReads(t)
	file := s.buildCSV(t, "ann", rep.Id, `{"today":"2026-11-16"}`)
	pages, totals, details := 0, 0, 0
	for _, st := range *seen {
		switch {
		case isGroupPageStatement(st.sql):
			pages++
		case isTotalStatement(st.sql):
			totals++
		case isGroupedDetailStatement(st.sql):
			details++
		}
	}
	if pages != 1 || totals != 1 || details != 1 {
		t.Fatalf("%d group, %d Total and %d detail statements, want 1 of each", pages, totals, details)
	}
	if got, want := firstFields(csvRecords(t, file.Bytes)), byNoteRows(446); !slices.Equal(got, want) {
		t.Fatalf("%d rows, want %d: the file is not the report in order\nfirst %q\nlast %q", len(got), len(want), got[:min(len(got), 7)], got[max(0, len(got)-4):])
	}
	if file.Rows != 1351 {
		t.Fatalf("file.Rows = %d, want 1351", file.Rows)
	}
}

// The one page ends at the format's limit: a report of exactly the limit is
// written whole, and one row more is refused, never cut.
func TestExportOfAGroupedReportStopsAtTheFormatsLimit(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "byNote", "ann", "Feedings by Note", byNoteClosed, false)
	bulkFeedings(t, s.zoo, 446)
	keepExportLimits(t)

	exportLimits.sheetRows = 1351
	whole := s.exportAs(t, "ann", csvExportBody(rep.Id))
	requireStatus(t, whole, http.StatusOK)
	if got, want := firstFields(csvRecords(t, whole.Body.Bytes())), byNoteRows(446); !slices.Equal(got, want) {
		t.Fatalf("%d rows at the limit, want %d", len(got), len(want))
	}

	exportLimits.sheetRows = 1350
	requireStatus(t, s.exportAs(t, "ann", csvExportBody(rep.Id)), http.StatusUnprocessableEntity,
		`"message":"This report has more than 50,000 rows. Narrow the period."`)
	if n := len(exportAuditRows(t, s.app)); n != 1 {
		t.Fatalf("%d audit rows, want 1: the refused export wrote one", n)
	}
}

// A flat report reads in one page too: its rows and its Total, two
// statements, where pages of maxPage rows took two for each page.
func TestExportReadsAFlatReportInOnePage(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	bulkFeedings(t, s.zoo, 446)
	withLimits(t, func(l *limits) { l.maxPage = 100 })

	seen := observeReads(t)
	file := s.buildCSV(t, "ann", rep.Id, `{"today":"2026-11-16"}`)
	if n := len(*seen); n != 2 {
		t.Fatalf("%d statements, want 2 (the Total and the rows)", n)
	}
	rows := firstFields(csvRecords(t, file.Bytes))
	if len(rows) != 451 || rows[0] != "f1" || rows[449] != "n00446" || rows[450] != "Total" {
		t.Fatalf("%d rows, first %q, want 451 from f1 to n00446 and the Total", len(rows), rows[0])
	}
}

// The one page is read under the statement limit, where each page had a
// limit of its own: a statement past it ends the export with the export's
// sentence, and no file and no audit row.
func TestExportPastTheStatementLimitIsATimeout(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "byNote", "ann", "Feedings by Note", byNoteClosed, false)
	withLimits(t, func(l *limits) { l.timeout = time.Nanosecond })
	requireStatus(t, s.exportAs(t, "ann", csvExportBody(rep.Id)), http.StatusRequestTimeout,
		`"message":"The report took too long to export."`)
	if n := len(exportAuditRows(t, s.app)); n != 0 {
		t.Fatalf("a refused export wrote %d audit rows", n)
	}
}

// The run endpoint has no page cap: a body that asks for more than maxPage
// rows gets a page of maxPage rows, and a body that names a page cap is
// refused as any unknown field is.
func TestTheRunEndpointKeepsMaxPage(t *testing.T) {
	s := newSavedZoo(t)
	bulkFeedings(t, s.zoo, 446)
	withLimits(t, func(l *limits) { l.maxPage = 100 })
	run := func(rest string) (int, string) {
		return s.call(t, "ann", http.MethodPost, runURL, `{"spec":`+feedingsFlat+`,"params":{},`+rest+`}`)
	}
	status, body := run(`"options":{"limit":50001}`)
	if status != http.StatusOK || !strings.Contains(body, `"size":100`) {
		t.Fatalf("status %d, want 200 and a page of 100 rows:\n%.300s", status, body)
	}
	for _, rest := range []string{`"options":{"limit":50001,"pageCap":50001}`, `"options":{"limit":50001},"pageCap":50001`} {
		if status, body := run(rest); status != http.StatusBadRequest {
			t.Fatalf("%s: status %d, want 400\n%.300s", rest, status, body)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify the two one-page tests fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports && go test -run 'TestExportReadsAGroupedReportInOnePage|TestExportOfAGroupedReportStopsAtTheFormatsLimit|TestExportReadsAFlatReportInOnePage|TestExportPastTheStatementLimitIsATimeout|TestTheRunEndpointKeepsMaxPage' -count=1 -v . 2>&1 | command grep -E '^\s*(--- |FAIL|ok)|_test\.go:[0-9]+:' | cut -c1-200`

Expected (measured):

```text
    export_onepage_test.go:63: 14 group, 14 Total and 14 detail statements, want 1 of each
--- FAIL: TestExportReadsAGroupedReportInOnePage
--- PASS: TestExportOfAGroupedReportStopsAtTheFormatsLimit
    export_onepage_test.go:107: 10 statements, want 2 (the Total and the rows)
--- FAIL: TestExportReadsAFlatReportInOnePage
--- PASS: TestExportPastTheStatementLimitIsATimeout
--- PASS: TestTheRunEndpointKeepsMaxPage
FAIL
```

The three tests that pass are guards: they hold today and must hold after step 3.

- [ ] **Step 3: Write the implementation**

`run.go`, type `Request` (line 75): after the `RowCap int` field add

```go
	// PageCap is the most rows a page holds, in place of maxPage: a caller
	// that needs every row reads them in one page, as each page of a grouped
	// run computes every group level again. 0: maxPage. Only the export read
	// sets it; the run endpoint never does.
	PageCap int
```

`run.go`, type `runner` (line 285): after the `askedRowCap int` field add

```go
	// askedPageCap is the request's PageCap (0: runLimits.maxPage); read it
	// through pageLimit.
	askedPageCap int
```

`run.go`, func `Run` (line 373): replace

```go
		ctx: ctx, app: app, c: c, opts: req.Options, expand: expand, askedRowCap: req.RowCap,
```

with

```go
		ctx: ctx, app: app, c: c, opts: req.Options, expand: expand, askedRowCap: req.RowCap, askedPageCap: req.PageCap,
```

`run.go`, func `pageLimit` (line 693): replace the comment and the function with

```go
// pageLimit is the most rows a page holds: options.limit up to maxPage (up
// to the request's PageCap when it is set), else pageSize.
func (rn *runner) pageLimit() int {
	if rn.opts.Limit <= 0 {
		return runLimits.pageSize
	}
	if rn.askedPageCap > 0 {
		return min(rn.opts.Limit, rn.askedPageCap)
	}
	return min(rn.opts.Limit, runLimits.maxPage)
}
```

`export.go`, the route comment (line 24): replace `the whole report, read page by page as that reader with` with `the whole report, read in one page as that reader with`.

`export.go`: replace the comment and the whole function `readWholeReport` (lines 327-381) with

```go
// readWholeReport runs the report as the reader in one page of maxRows+1
// rows (Request.PageCap). It never reads a second page: each page of a
// grouped run computes every group level again, so pages of maxPage rows
// cost pages x rows. It returns nil with no error when the report holds more
// rows than the writer takes (the page is past maxRows, or a row follows
// it), or when a cap stopped the run, so no file is ever cut short.
func readWholeReport(ctx context.Context, re *core.RequestEvent, spec *Spec, loaded *LoadInfo, body exportBody, writer exportWriter) (*Result, error) {
	maxRows := writer.MaxRows()
	opts := body.Options
	opts.Limit = maxRows + 1
	opts.Cursor = ""
	if writer.ExpandAll {
		opts.ExpandAll = true
		opts.Expand, opts.Collapse = nil, nil
	}
	headers := ruleHeaders(re.Request.Header)
	res, err := Run(ctx, re.App, Request{
		Spec: spec, Loaded: loaded, Params: body.Params, Options: opts,
		Auth: re.Auth, Headers: headers, RowCap: maxRows + 1, PageCap: maxRows + 1,
	})
	if err != nil {
		return nil, err
	}
	// A page ends one row short of its limit when a group row and its opening
	// row do not both fit (cutPage): a next page means a row past maxRows.
	if len(res.Rows) > maxRows || (res.Page != nil && res.Page.Next != "") {
		return nil, nil
	}
	res.Notices = uniqueNotices(res.Notices)
	capped := slices.ContainsFunc(res.Notices, func(n Notice) bool { return n.Code == "rowCap" || n.Code == "nestCap" })
	if capped {
		return nil, nil
	}
	return res, nil
}
```

The `res.Page.Next` clause is a defence no test reaches (see "Facts the planner measured"): do not write a test that forces it with a hand-made `Result`, and do not remove it.

`export.go`: delete the two functions that only the loop used, with their comments: `isWholeReportRow` and `withoutAppendedTotals` (lines 386-402). Keep `isSectionRow` (line 383-384; `exportRows` reads it), `uniqueNotices`, `union` and `joinNotices`.

`export_test.go` (lines 257-258): replace the comment

```go
// The server reads the whole report page by page: a build that reads only
// the first page fails on the count.
```

with

```go
// The file holds every row of a report longer than a page of the viewer
// (maxPage): a build that stops at a viewer's page fails on the count.
```

- [ ] **Step 4: Add the `pageLimit` test**

This test reads `runner.askedPageCap`, which step 3 adds: in step 1 it would stop the package from compiling and hide the RED of the other tests. Its RED is that compile error (`unknown field askedPageCap in struct literal of type runner`); the behavior it pins is proven RED by the two one-page tests of step 2.

In `export_onepage_test.go`, before `TestTheRunEndpointKeepsMaxPage`, add:

```go
// The page cap is the export's alone: a run with none keeps maxPage, a run
// with no limit keeps pageSize, and a cap never raises a smaller limit.
func TestPageLimit(t *testing.T) {
	for _, c := range []struct {
		name               string
		limit, asked, want int
	}{
		{"no limit", 0, 0, runLimits.pageSize},
		{"no limit, a cap", 0, 50001, runLimits.pageSize},
		{"a limit under maxPage", 50, 0, 50},
		{"a limit over maxPage", 50001, 0, runLimits.maxPage},
		{"a limit at the cap", 50001, 50001, 50001},
		{"a limit over the cap", 60000, 50001, 50001},
		{"a limit under the cap", 300, 50001, 300},
	} {
		rn := &runner{opts: Options{Limit: c.limit}, askedPageCap: c.asked}
		if got := rn.pageLimit(); got != c.want {
			t.Fatalf("%s: pageLimit = %d, want %d", c.name, got, c.want)
		}
	}
}
```

- [ ] **Step 5: Run the new tests and every export test**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports && gofmt -l . ; go vet . && go test -run 'TestExport|TestCSV|TestXLSX|TestPDF|TestEmail|TestPageLimit|TestTheRunEndpointKeepsMaxPage' -count=1 . 2>&1 | command grep -E '^(--- FAIL|FAIL|ok|panic)|_test\.go:[0-9]+:' | cut -c1-200`

Expected: `gofmt -l` prints nothing, `go vet` prints nothing, then `ok  	tinycld.org/core/reports` (about 13 s). The pattern `TestExport` runs the four new export tests of step 1. This run also holds the existing proofs that the answers did not change: `TestCSVHoldsTheWholeGroupedReport` (the bytes of a grouped file), `TestCSVFollowsTheReadersSort`, `TestExportReadsEveryPage` (450 rows past `maxPage`), `TestExportRefusesMoreRowsThanTheFormatHolds`, `TestExportRefusesEachFormatPastItsLimit`, `TestExportReadsAPivotPastTheEngineCap`, `TestExportStopsAtTheTimeLimit`, `TestExportAnswersAnExcelFile`, `TestExportAnswersAPDFFile`, `TestExportWritesOneAuditRowPerDownload`.

- [ ] **Step 6: Run the package**

`pageLimit` serves every run, so the whole package runs once (about 100 s, under the 4-minute rule):

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports && go test -count=1 . 2>&1 | command grep -E '^(--- FAIL|FAIL|ok|panic)|_test\.go:[0-9]+:' | cut -c1-200`

Expected: `ok  	tinycld.org/core/reports`.

- [ ] **Step 7: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/run.go core/server/reports/export.go core/server/reports/export_test.go core/server/reports/export_onepage_test.go
git commit -m "perf(reports): the export reads a report in one page"
```

---

### Task 2: The detail statement computes its open groups once

**Files:**
- Create: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/leaforder_test.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/run.go` (func `leafOrder`, at line 1716 before task 1 and about 10 lines lower after it; find it by name)

**Interfaces:**
- Consumes (existing test helpers): `newZoo(t) *zoo`, `bulkFeedings(t, z, n)`, `withLimits(t, func(*limits))` (fields `timeout time.Duration`, `maxPage int`), `(z *zoo) try(t, user, raw string, opts Options, params map[string]string) (*Result, error)`, `(z *zoo) run(…) *Result`, `observeReads(t)`, consts `byNoteClosed`, `zooStatement` (`layout_test.go:17`, one `%s`), `KindDetail`. Nothing from task 1: the tests call `Run`, not the export.
- Produces: `(c *compiler) leafOrder(s *stmt, leaves [][]any) string` returns `gord AS MATERIALIZED (SELECT … FROM json_each(…))`. Same signature, same columns (`__g`, `g0`…), same bound value.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/leaforder_test.go` with exactly this content:

```go
package reports

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// The detail statement of a grouped run joins each detail row to its open
// group (gord, a json_each over one bound list). Measured on darwin/arm64,
// 14 CPUs: with gord read inline the statement's time grows with rows x open
// groups (4,000 open groups 6.7 s, 8,000 about 27 s); with gord MATERIALIZED
// it grows with the rows (16,000 open groups 0.16 s). The limit of 2 s is for
// each statement of the run: the group statement (0.13 s at 16,660 groups)
// and the Total (0.08 s) are under it too. It is 13 times under the one
// reading and over 10 times above the other, so this fails only on the
// quadratic read. A failure is a result: never raise the limit to pass.
func TestAGroupedRunOfManyOpenGroupsReadsItsDetailsOnce(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 8000)
	withLimits(t, func(l *limits) {
		l.timeout = 2 * time.Second
		l.maxPage = 30000
	})
	res, err := z.try(t, "ann", byNoteClosed, Options{ExpandAll: true, Limit: 30000}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	details := 0
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			details++
		}
	}
	if details != 8004 {
		t.Fatalf("%d detail rows, want 8004", details)
	}
}

// Both grouped paths write the join: a run that pages its groups, and a
// layout, which does not. Each computes the open groups once.
func TestTheDetailStatementComputesItsOpenGroupsOnce(t *testing.T) {
	for name, spec := range map[string]string{"paged groups": byNoteClosed, "a layout": fmt.Sprintf(zooStatement, "")} {
		t.Run(name, func(t *testing.T) {
			z := newZoo(t)
			seen := observeReads(t)
			z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
			found := 0
			for _, st := range *seen {
				if !strings.Contains(st.sql, "JOIN gord ON") {
					continue
				}
				found++
				if !strings.Contains(st.sql, "gord AS MATERIALIZED (SELECT key AS __g") {
					t.Fatalf("the open groups are read inline:\n%s", st.sql)
				}
			}
			if found != 1 {
				t.Fatalf("%d grouped detail statements, want 1", found)
			}
		})
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports && go test -run 'TestAGroupedRunOfManyOpenGroupsReadsItsDetailsOnce|TestTheDetailStatementComputesItsOpenGroupsOnce' -count=1 -v . 2>&1 | command grep -E '^\s*(--- |FAIL|ok)|_test\.go:[0-9]+:' | cut -c1-160`

Expected (measured; the first test stops at its 2 s limit):

```text
    leaforder_test.go:28: Run: reports: the report took too long to run
--- FAIL: TestAGroupedRunOfManyOpenGroupsReadsItsDetailsOnce (2.16s)
    leaforder_test.go:56: the open groups are read inline:
    leaforder_test.go:56: the open groups are read inline:
--- FAIL: TestTheDetailStatementComputesItsOpenGroupsOnce
    --- FAIL: TestTheDetailStatementComputesItsOpenGroupsOnce/paged_groups
    --- FAIL: TestTheDetailStatementComputesItsOpenGroupsOnce/a_layout
FAIL
```

- [ ] **Step 3: Write the implementation**

`run.go`, func `leafOrder`: replace its comment and its last line. The comment

```go
// leafOrder is the CTE gord: one row per open innermost group, __g its place
// in document order (json_each's index), g<j> its key values (group keys,
// then currency keys), bound as one JSON list as expansion.where binds keys.
```

becomes

```go
// leafOrder is the CTE gord: one row per open innermost group, __g its place
// in document order (json_each's index), g<j> its key values (group keys,
// then currency keys), bound as one JSON list as expansion.where binds keys.
// It is MATERIALIZED: read inline, the detail statement's time grows with
// rows x open groups (4,000 open groups: 6.7 s inline, 40 ms materialized).
```

and the line

```go
	return fmt.Sprintf("gord AS (SELECT %s FROM json_each(%s))", strings.Join(cols, ", "), s.bind("["+strings.Join(encoded, ",")+"]"))
```

becomes

```go
	return fmt.Sprintf("gord AS MATERIALIZED (SELECT %s FROM json_each(%s))", strings.Join(cols, ", "), s.bind("["+strings.Join(encoded, ",")+"]"))
```

Nothing else changes: `leafJoin`, the two `JOIN gord ON` selects in `detailPass` and `materializeShared` (`rows.go:638`, which marks only `w`) stay as they are.

- [ ] **Step 4: Run the new tests and the blank-key tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports && gofmt -l . ; go vet . && go test -run 'TestAGroupedRunOfManyOpenGroupsReadsItsDetailsOnce|TestTheDetailStatementComputesItsOpenGroupsOnce|TestExpandOpensABlankGroup|TestABlankGroupPagesInItsPlace|TestExportReadsAGroupedReportInOnePage|TestCSVHoldsTheWholeGroupedReport' -count=1 -v . 2>&1 | command grep -E '^(--- |FAIL|ok|panic)|_test\.go:[0-9]+:' | cut -c1-160`

Expected: `gofmt -l` and `go vet` print nothing; six `--- PASS` lines (the first test in about 0.25 s) and `ok  	tinycld.org/core/reports`.

- [ ] **Step 5: Run the package**

Every grouped run with an open group sends this statement, so the whole package runs once (about 100 s):

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports && go test -count=1 . 2>&1 | command grep -E '^(--- FAIL|FAIL|ok|panic)|_test\.go:[0-9]+:' | cut -c1-200`

Expected: `ok  	tinycld.org/core/reports`. A failure of a test that compares a statement's text with `gord AS (` is a text pin to update to `gord AS MATERIALIZED (` (the planner's run found none); any other failure is a result that changed: stop and report it with the test's output.

- [ ] **Step 6: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/run.go core/server/reports/leaforder_test.go
git commit -m "perf(reports): a grouped detail statement computes its open groups once"
```

---

### Last step: the scale run and the record (after tasks 1 and 2 are committed)

This is a measurement and a record, not a code task. `STAGING` is `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf`.

**Files:**
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/ar-aging-ledger.md` (section `## Performance`: a new sub-section before `### Record (spec §9)`, and the lines of `### Record (spec §9)`)
- Modify (only when the scale run passes): `/Users/nas/tmp/tinycld/books-purchases/books/server/testdata/ar_aging_stat1.json`, `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/ar-aging-evidence/perf-plans.md`

- [ ] **Step 1: Check that Books builds against the changed core, then hand back the run**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go vet . && go test -run 'TestARAging' -count=1 . 2>&1 | command grep -E '^(--- FAIL|FAIL|ok|panic)' | cut -c1-200`
Expected: `ok  	tinycld.org/packages/books` in about 9 s (measured by the planner against the code of tasks 1 and 2). The pattern runs every A/R Aging test: the plan test and `TestARAgingCSVOpensEveryGroup` (`report_ar_aging_test.go:1390`), which exports the report through core's export route, are among them, and the scale and profile tests skip without their environment variables. So Books reads the changed core and the file's rows are as before.

The scale run takes more than 4 minutes: the driver runs it (agent rules). Hand back exactly:

```text
NEEDS-RUN: cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/scale-stored-2 go test -run 'TestARAgingScale$' -count=1 -timeout 60m -v . > /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/perf-scale-stored-2.log 2>&1
Needed: the log and the files the run writes in scale-stored-2 (perf-results.md, perf-plans.md, ar_aging_stat1.json). Nothing else runs on the machine during it (the times are bars). A fresh agent continues at step 2 of the last step.
```

- [ ] **Step 2: Read the run (a fresh agent, after the run)**

Run: `command grep -E '^(--- FAIL|--- PASS|FAIL|ok|panic)' /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/perf-scale-stored-2.log | cut -c1-200`, then `command grep -n -E 'median|or less' /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/perf-scale-stored-2.log | cut -c1-240`.
Expected: `--- PASS: TestARAgingScale` and no `want … or less` line. Then read the `## Runs` table of `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/scale-stored-2/perf-results.md` with the Read tool (lines 1 to 36): one line per run with its statements, S median, L median, ratio, bar and result. Run 7 before this plan: 76 statements, S 469.842ms, L 14.995203s, ratio 31.9, fail. Expected now: about 36 statements (the 33 before the first page, then one group, one Total and one detail statement); a count near 76 means the export still pages, which is a defect of task 1 to report, not to record as a result.

Then check that run 7's detail statement computes its open groups once at L, where the statistics of the book are loaded: `command grep -c 'MATERIALIZE gord' /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/scale-stored-2/perf-plans.md`. Expected: 1 or more. A count of 0 is not a failed bar; record it in the ledger and in the hand-back, because the linear time of task 2 was measured on core's fixture with no statistics, not on this book.

- [ ] **Step 3: The statistics file and the plans (only with `--- PASS: TestARAgingScale`)**

```bash
cp /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/scale-stored-2/ar_aging_stat1.json /Users/nas/tmp/tinycld/books-purchases/books/server/testdata/ar_aging_stat1.json
cp /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/scale-stored-2/perf-plans.md /Users/nas/tmp/tinycld/books-purchases/books/docs/plans/ar-aging-evidence/perf-plans.md
cd /Users/nas/tmp/tinycld/books-purchases/books/server && go test -run 'TestARAgingQueryPlans' -count=1 . 2>&1 | command grep -E '^(--- FAIL|FAIL|ok)|SCAN|AUTOMATIC' | cut -c1-240
```

Expected: `ok  	tinycld.org/packages/books`. When the scale run did not pass, copy nothing.

- [ ] **Step 4: Write the record**

In `docs/plans/ar-aging-ledger.md`, directly before the line `### Record (spec §9)`, add a sub-section `### Export in one page (p5-report-perf)` with these parts, each filled from the files of step 2 (copy the figures; do not round again):

1. One line: machine, date, books commit (`git -C /Users/nas/tmp/tinycld/books-purchases/books rev-parse --short HEAD`), core commit (`git -C /Users/nas/tmp/tinycld/books-purchases/tinycld rev-parse --short HEAD`).
2. What changed, two lines: "core's export reads a report in one page (`Request.PageCap`, set only by the export), where it read pages of 1,000 rows and each page's group statement computed every group level again" and "a grouped detail statement computes its open groups once (`gord AS MATERIALIZED`), where the statement's time grew with rows x open groups". Plan: core `docs/plans/2026-11-19-report-perf-export.md`. No Books code changed.
3. A table with one line per run 1 to 7 and the columns: run; statements; L before (the column "L after" of the table "Before and after" in the sub-section `### Stored receivable items (p5-report-perf)`); L after; S after; ratio L/S; bar; pass or fail.
4. One line: the Customer Balance time on the same dataset (no bar).
5. One line for the plan bars, from step 3: "every plan bar holds with no statistics and under L statistics (`TestARAgingQueryPlans`, statistics file of this run)", or what failed.

Then, only when the run passed, replace the figures in the list under `### Record (spec §9)` (`- machine, date, commit:` to `- full plan text:`) with those of this run (run 7's statement count and result, the Customer Balance times), and point its "Before and after" references at the new table.

- [ ] **Step 5: Commit and hand back**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add docs/plans/ar-aging-ledger.md
git add server/testdata/ar_aging_stat1.json docs/plans/ar-aging-evidence/perf-plans.md   # only when the run passed
git commit -m "docs(books): A/R Aging scale numbers with the export in one page"
```

Hand back one of:

- Every bar passes (`--- PASS: TestARAgingScale`): "run 7: L `<median>`, S `<median>`, ratio `<ratio>`; all seven runs meet their bars", with the path of `scale-stored-2/perf-results.md`.
- A bar still fails: `STOPPED: run <n> fails its bar` with the run's line of `perf-results.md`, the `want … or less` lines, and for run 7 its statement count, its S and L medians and the `MATERIALIZE gord` count of step 2. Do not change a bar, a run or the dataset. The scale run gives no time for one statement: the time of run 7's detail statement at L comes only from the profile named in the next sentence. When run 7 fails only the ratio bar while L is under 5 s, say so: the S time fell too (2 pages to 1), and the next step is a profile of run 7 at S and L (`TestARAgingProfile`, `BOOKS_PROFILE_SIZE=S` and `=L`), which is a `NEEDS-RUN` for the driver.
