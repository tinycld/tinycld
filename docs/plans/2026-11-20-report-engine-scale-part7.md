# Report Engine Scale, Part 7: the 500k Result and the Changed Bar — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Record the General Ledger's measured times at 500,000 journal lines with the bar the human set on 2026-11-20, and make the scale measure hold that bar.

**Architecture:** Each run of the scale measure (`glScaleRun`) carries its own bar. A filtered run of one month has 1 s, a filtered run of a year has 1.5 s, a run with no Accounts filter has none. A small test pins the bar of each run without a book. The ledger of the General Ledger records the result and the decision.

**Tech Stack:** Go tests in `books/server` (package `books`). Markdown in `books/docs/plans`.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md` (task 7) and the human's decisions in `/Users/nas/code/tinycld/docs/plans/auto-helix/p5-report-engine-scale.md` (the bullet "2026-11-20 HUMAN DECISION").

Workspace root: `/Users/nas/tmp/tinycld/books-purchases`. Books is `books/`, core is `tinycld/`. Both are on the branch `books/p5-report-engine-scale`. This part changes Books only.

## Global Constraints

- The bar (the human, 2026-11-20, copied): "A filtered run of one month is <= 1 s at 500k (runs 3, 4 pass); a filtered year run may be slightly over (run 5 1.418 s accepted)."
- The figure for "slightly over" is 1.5 s. The planner proposes it; the human has not confirmed the figure. **The driver confirms 1.5 s with the human before this task is built.** If the human gives another figure, change `glYearBar` and the `1500 * time.Millisecond` of `TestGLScaleBars`, and the two places in the ledger text.
- The measured times are never changed and never rounded differently from `scale-task7/gl-results.md`: 582 ms, 2.733 s, 231 ms, 147 ms, 1.418 s.
- No timing run in this task. The 500k measure is already run (`scale-task7.log`, quiet machine, 1-minute load 2.75).
- Options A to D of `run5-500k-options.md` are not built (the human's decision).
- Run only the named Go tests. The driver runs the full checks.

## Review Focus

- A later run added to `glScaleRuns` with an Accounts filter and no bar: the measure would pass it at any time. `TestGLScaleBars` fails for a filtered run with no bar, and for a run it does not name (Task 7, step 1).
- A year run that takes 1.5 s to the nanosecond: it passes (`median > run.bar` fails only above the bar). This is the old comparison, kept.
- A filtered run that core refuses at 500k: it still fails the measure (`last && run.filtered`), bar or no bar. The task does not touch that branch.
- A reader of the ledger who sees "Bar: 1 s" in the section's first paragraph and 1.418 s in the result: the first paragraph is changed in the same commit, with the day of the change (step 5).
- The 10k and 100k sizes: no bar applies there, as before (`last`).

---

### Task 7: The scale measure holds the changed bar, and the ledger records the 500k result

**Files:**
- Modify: `books/server/report_general_ledger_scale_test.go` (the type `glScaleRun` at line 93, `glScaleRuns` at line 105, `TestGLScale` at lines 289-325)
- Create: `books/server/report_general_ledger_scale_bars_test.go`
- Modify: `books/docs/plans/general-ledger-ledger.md` (line 241, and a new section at the end of the file)

**Interfaces:**
- Consumes (in `report_general_ledger_scale_test.go`): `type glScaleBook struct{ arBook; lastDay string; incomes []*core.Record; lines int }`, `glScaleRuns(b glScaleBook) []glScaleRun`, `arBook.receivable *core.Record`, `arBook.docs *paymentEnv`, `ledgerEnv.checking *core.Record` (`paymentEnv` embeds `*ledgerEnv`).
- Produces: `glScaleRun.bar time.Duration` (0: the run has no bar), the constants `glMonthBar = time.Second` and `glYearBar = 1500 * time.Millisecond`. The next part (Trial Balance, Balance Sheet, Transaction Detail) uses the two constants for the same bar.

- [ ] **Step 1: Write the failing test**

Create `books/server/report_general_ledger_scale_bars_test.go`:

```go
package books

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// TestGLScaleBars pins the bar of each run of the scale measure at 500,000
// lines: a filtered run of one month 1 s, a filtered run of a year 1.5 s, a
// run with no Accounts filter none. It reads the runs only; no book is
// written.
func TestGLScaleBars(t *testing.T) {
	account := func() *core.Record { return core.NewRecord(core.NewBaseCollection("accounts")) }
	b := glScaleBook{lastDay: "2026-11-20", incomes: []*core.Record{account()}}
	b.receivable = account()
	b.docs = &paymentEnv{ledgerEnv: &ledgerEnv{checking: account()}}
	want := map[string]time.Duration{
		"1 all accounts, month":  0,
		"2 all accounts, year":   0,
		"3 busy account, month":  time.Second,
		"4 small account, month": time.Second,
		"5 three accounts, year": 1500 * time.Millisecond,
	}
	runs := glScaleRuns(b)
	if len(runs) != len(want) {
		t.Fatalf("the measure has %d runs, want %d", len(runs), len(want))
	}
	for _, run := range runs {
		bar, ok := want[run.name]
		if !ok {
			t.Errorf("run %q has no bar in this test", run.name)
			continue
		}
		if run.bar != bar {
			t.Errorf("run %s: bar %s, want %s", run.name, run.bar, bar)
		}
		if run.filtered != (run.bar > 0) {
			t.Errorf("run %s: filtered is %v, bar is %s: a filtered run has a bar, and no other run has one", run.name, run.filtered, run.bar)
		}
	}
}
```

- [ ] **Step 2: Run the test to see it fail**

Run, in `/Users/nas/tmp/tinycld/books-purchases/books/server`:

```bash
go test -run 'TestGLScaleBars$' .
```

Expected: the build fails with `run.bar undefined (type glScaleRun has no field or method bar)`.

- [ ] **Step 3: Give each run its bar**

In `books/server/report_general_ledger_scale_test.go`, replace the type `glScaleRun`:

```go
// glScaleRun is one run of the report.
type glScaleRun struct {
	name   string
	params map[string]any
	// filtered is a run with an Accounts filter: the unit's bar is its.
	filtered bool
	// bar is the most the run's median may take at 500,000 lines; 0 for a
	// run with no bar.
	bar time.Duration
}

// The unit's bar at 500,000 lines (the human, 2026-11-20): a filtered run of
// one month takes 1 s or less; a filtered run of a year may be slightly
// over, which this measure holds to 1.5 s.
const (
	glMonthBar = time.Second
	glYearBar  = 1500 * time.Millisecond
)
```

In `glScaleRuns`, the three filtered runs get their bar. The returned list becomes:

```go
	return []glScaleRun{
		{name: "1 all accounts, month", params: map[string]any{"period": month}},
		{name: "2 all accounts, year", params: map[string]any{"period": year}},
		{name: "3 busy account, month", params: map[string]any{"period": month, "accounts": []string{b.receivable.Id}}, filtered: true, bar: glMonthBar},
		// Rows: All, so the section shows also in a month with no line of it.
		{name: "4 small account, month", params: map[string]any{"period": month, "rows": "all", "accounts": []string{b.incomes[0].Id}}, filtered: true, bar: glMonthBar},
		{name: "5 three accounts, year", params: map[string]any{"period": year, "accounts": three}, filtered: true, bar: glYearBar},
	}
```

In the comment above `TestGLScale`, replace the lines

```go
// (median of 5) and holds the unit's bar: a run with an Accounts filter takes
// 1 s or less at 500,000 lines. A run core refuses (its time limit) is
```

with

```go
// (median of 5) and holds the unit's bar at 500,000 lines: a run with an
// Accounts filter takes its bar or less (glMonthBar for a month, glYearBar
// for a year). A run core refuses (its time limit) is
```

In `TestGLScale`, delete the line `const bar = time.Second`, and replace

```go
				if last && run.filtered && median > bar {
					t.Errorf("run %s at %s lines: median %s, want %s or less", run.name, size.name, median, bar)
				}
```

with

```go
				if last && run.bar > 0 && median > run.bar {
					t.Errorf("run %s at %s lines: median %s, want %s or less", run.name, size.name, median, run.bar)
				}
```

Leave the refusal branch (`if last && run.filtered { t.Errorf("run %s at %s lines was refused: …` ) as it is.

- [ ] **Step 4: Run the tests to see them pass**

Run, in `books/server`:

```bash
go vet . && go test -run 'TestGLScaleBars$|TestGLScaleBook$|TestGLScale$' -v .
```

Expected: `--- PASS: TestGLScaleBars`, `--- PASS: TestGLScaleBook`, `--- SKIP: TestGLScale` (it starts only with `BOOKS_SCALE=1`), `ok`. The planner built this code in an overlay on books 9a68a199: the same three lines, 2.5 s.

Do not start `TestGLScale` with `BOOKS_SCALE=1`: the 500k size takes many minutes and is the driver's run. With the recorded medians the changed check passes: 231 ms and 147 ms are under 1 s, 1.418 s is under 1.5 s.

- [ ] **Step 5: Record the result and the changed bar in the ledger**

In `books/docs/plans/general-ledger-ledger.md`, line 241, replace the last sentence of the paragraph

```
Bar: a run with an Accounts filter (runs 3, 4, 5) takes 1 s or less at 500,000 journal lines, median of 5.
```

with

```
Bar, as the human changed it on 2026-11-20 (section "Task 7" below): at 500,000 journal lines, median of 5, a run of one month with an Accounts filter (runs 3, 4) takes 1 s or less, and a run of a year with an Accounts filter (run 5) may be slightly over 1 s; the measure holds it to 1.5 s. Until 2026-11-20 the bar was 1 s or less for runs 3, 4 and 5.
```

Append at the end of the file (after the Task 5d section):

```markdown

#### Task 7: the result at 500,000 lines, and the changed bar (2026-11-20)

Measured on a quiet machine (1-minute load 2.75), core 10029574, books 9a68a199, `BOOKS_SCALE=1`, the 500k size. Median of 5.

| run, 500k lines | baseline (2026-11-19) | now | bar |
| --- | --- | --- | --- |
| 1 all accounts, month | refused (time limit) | 582 ms | none |
| 2 all accounts, year | refused (time limit) | 2.733 s | none |
| 3 busy account, month | 13,772 ms | 231 ms | 1 s |
| 4 small account, month | 7,219 ms | 147 ms | 1 s |
| 5 three accounts, year | refused (time limit) | 1.418 s | 1.5 s |

The long statements, one sample each (statement 4 the section rows, 5 the Total row, 6 the detail page): run 3 34.6 / 30.6 / 144.6 ms; run 4 25.1 / 19.5 / 80.5 ms; run 5 192.9 / 203.7 / 986.2 ms; run 1 113 / 108.3 / 342.5 ms; run 2 346.7 / 379.4 ms / 1.9985 s. Plan, the detail statement of run 5 (201 rows, 42,435 chars), reads of `books_journal_lines`: 7 × `SEARCH … idx_books_jlines_account_date (account=? AND date>? AND date<?)`, 6 × `(account>? AND account<?)`, 2 × `idx_books_jlines_entry (entry=?)`, 2 × `(entry>? AND entry<?)`; no `SCAN`. The detail statement of run 2 (no Accounts filter, no bar) has 1 `SCAN books_journal_lines`.

Runs 3 and 4 meet the first bar (1 s or less). Run 5 is 1.418 s: over it. The human decided on 2026-11-20: the bar is changed, not the engine. A filtered run of one month takes 1 s or less at 500,000 lines; a filtered run of a year may be slightly over, and run 5's 1.418 s is accepted. The four further cuts for run 5 that were written up (options A to D) are not built. The measure (`TestGLScale`) now holds each run to its own bar: `glMonthBar` 1 s for runs 3 and 4, `glYearBar` 1.5 s for run 5 (the figure for "slightly over"), none for runs 1 and 2; `TestGLScaleBars` pins them. The same bar applies to the Trial Balance, the Balance Sheet and Transaction Detail.

A separate decision of the human on 2026-11-19 (not built yet; a later part of this unit) takes the reports that read all journal lines, the General Ledger among them, off live refresh: once it is built, a year run over 1 s costs the reader one wait, when the reader asks for it.
```

Check the numbers against `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task7/gl-results.md` (the five medians) and `scale-task7/gl-plans.md` (the statement headings `### statement <n>: <time>` under each `## 500k lines, run …`) before the commit. When a number differs, the file wins.

- [ ] **Step 6: Commit (in the Books repo)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add server/report_general_ledger_scale_test.go server/report_general_ledger_scale_bars_test.go docs/plans/general-ledger-ledger.md
git commit -m "test(books): the General Ledger scale measure holds the changed bar at 500k lines"
```

---

## What is left (not in this part)

The planner of this part reached its context limit after task 7. Not written:

1. **No live refresh for large reports** (the human's decision of 2026-11-19). The design the planner reached, with the code facts behind it, is in `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/handoff-backend-planner-part7.md`. No code of it is built.
2. **The plan pins of outline task 7** (`TestGLScaleBook`: no `SCAN books_journal_lines` in a long statement of a filtered run). The driver's scope for this part did not name them. The 500k plans show the forms to pin (step 5 above).
3. **Trial Balance, Balance Sheet, Transaction Detail scaling** (a later part).
