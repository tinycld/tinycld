# Report Engine Scale, Part 10: Books Declares the Reports That Are Not Live — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Books marks its journal-line sources as not live, keeps the Profit and Loss, the Customer Balance and the Audit Log live, and a Go test pins that exactly six shipped reports are manual.

**Architecture:** Two declarations of core's report engine (task 8a, built): `reports.Source.NotLive` on `books.journal_lines` and `books.basis_lines`, and `reports.Shipped.Live` on the three shipped reports that read the journal lines and stay live. Every other run that reads the journal lines (the six large shipped reports, a copy of any report, a report a user builds) is then manual: no data topics, and the client shows the note and Refresh (task 8b). One test runs every shipped report through `POST /api/reports/run`.

**Tech Stack:** Go, package `books` in `books/server` (module `tinycld.org/packages/books`), core package `tinycld.org/core/reports`, PocketBase test app. Markdown help topic.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/handoff-backend-planner-part7.md` (section "Design proposed", items 2 and 6), the outline `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md` (task 8c), part 8 of this plan (`tinycld/docs/plans/2026-11-20-report-engine-scale-part8.md`, its Interfaces block), and the human's decision of 2026-11-20 in `/Users/nas/code/tinycld/docs/plans/auto-helix/p5-report-engine-scale.md`.

Workspace root: `/Users/nas/tmp/tinycld/books-purchases`. Books is `books/`, on the branch `books/p5-report-engine-scale`. This part changes Books only and holds ONE task (8c). Task 8d (Books e2e of the note and Refresh) is the next plan part; it needs 8b (core client) and 8c both built.

## Global Constraints

- The human's decision (2026-11-20, copied): "the not-live list is every built-in report that reads all journal lines: General Ledger, Transaction Detail, Trial Balance, Balance Sheet, the Journal report and Profit and Loss by Month, plus any user-built report over journal lines. Profit and Loss and smaller reports stay live."
- The six keys that are NOT live, exactly: `books.general_ledger`, `books.transaction_detail`, `books.trial_balance`, `books.balance_sheet`, `books.journal`, `books.profit_and_loss_by_month`.
- `Live: true` goes on exactly three shipped reports: `books.profit_and_loss`, `books.customer_balance`, `books.audit_log`. These are the reports whose run reads `books_journal_lines` and which stay live (measured: with `NotLive` set and no `Live`, the runs of these three are manual; the Audit Log's view `books_audit_log` reads the journal lines). The A/R Aging, the two sales reports and the Sales Tax Liability read no journal line; they get no mark.
- Do not raise a `Revision` and do not change a `Spec`: `Live` is not stored, and the spec hash tests must stay green.
- Core is not edited. Do not run `pnpm install` or `packages:generate`.
- No backward compatibility work: the product is not live.
- Help text: the note's text is `This report does not update by itself. Press Refresh to see new changes.`; the button is **Refresh**; its key is R (no modifier, so no glyph).
- Run only the named Go tests. The driver runs the full checks.

## Review Focus

- A reader keeps a copy of the Profit and Loss with Save as: the copy is not the shipped record, so it is manual (`TestShippedReportsNotLive`, the drafts, step 1).
- A user builds a report over journal lines in the builder: every preview run and the saved report are manual (`TestShippedReportsNotLive`, "a report built over journal lines"; `TestReportSourcesRun`, step 3).
- A later unit ships a 14th report and forgets to decide: the test fails until the key is listed in `shippedNotLive` (`TestShippedReportsNotLive`, the key list, step 1).
- Someone sets `Live` on a seventh report to hide a slow run: `TestShippedLiveDeclarations` names the three (step 1).
- A manual shipped report that an admin shares or unshares while it is open: the run keeps the two saved-report topics (`TestShippedReportsNotLive` compares a manual run's topics with `savedOnly`, step 1).
- The Profit and Loss by Month and the Profit and Loss share one test helper (`runPL`): the helper passes each report's own `Live`, so one is manual and one is live in the same test (`TestProfitAndLossTopics`, step 3).

Known and accepted: a report that a user builds over the Audit log source is manual too, because the view `books_audit_log` reads the journal lines. The help text of step 5 says so.

Not tested here, by design: the note and the Refresh button in a browser (task 8d, the next part).

---

### Task 8c: Books declares which reports are not live

This task changes 13 Go files and one help file. It cannot be split: `NotLive` on the sources turns nine tests red at once, and a task ends green. Every edit but the new test file is 2 to 12 lines.

**Files:**
- Create: `books/server/report_live_test.go`
- Modify: `books/server/report_sources.go` (the `books.journal_lines` source at line 79; `basisLinesSource` at line 232)
- Modify: `books/server/report_profit_and_loss.go` (`profitAndLossShipped`, lines 20-21)
- Modify: `books/server/report_customer_balance.go` (`customerBalanceShipped`, lines 20-21)
- Modify: `books/server/report_audit_log.go` (`auditLogShipped`, lines 17-18)
- Modify (tests): `books/server/report_profit_and_loss_test.go` (`runPL` line 42; `TestProfitAndLossTopics` lines 1155-1170), `books/server/report_customer_balance_test.go` (`runCBErr` line 39), `books/server/report_audit_log_test.go` (`runAuditErr` line 102), `books/server/report_balance_sheet_test.go` (`TestBalanceSheetTopics` line 1401), `books/server/report_general_ledger_test.go` (`TestGeneralLedgerTopics` line 1038), `books/server/report_transaction_detail_test.go` (`TestTransactionDetailTopics` line 742), `books/server/report_trial_balance_test.go` (`TestTrialBalanceTopics` line 801), `books/server/report_journal_test.go` (`TestJournalTopics` line 960), `books/server/report_sources_test.go` (`TestReportSourcesRun` line 985)
- Modify: `books/help/reports.md` (lines 8, 43, 121, 142, 184; a new section before "## Download or email a report" at line 376)

**Interfaces:**
- Consumes (core, task 8a, built): `reports.Source.NotLive bool`, `reports.Shipped.Live bool`, `reports.Request.Live bool`, `reports.Result.Manual bool` (JSON `"manual":true`, absent when false). A manual run's `Topics` is empty from `reports.Run`, and `["collection/report_shares","collection/reports"]` from `POST /api/reports/run` with a `reportId`. `reports.Register(app)` binds the run route at serve; `reports.ReconcileShipped(app)`; `reports.ResetForTesting()`.
- Consumes (Books test helpers that exist): `newRegisteredApp(t) (*tests.TestApp, *pocketbase.PocketBase)` (`registrations_test.go`; it calls Books' `Register`, which registers the sources and the 13 shipped reports), `mkUser(t, app, role)`, `setPkgAccess(t, app, user, access)`, `tokenFor(t, user) string` (`env_test.go`), `reconcileBooksReportShares(app) error` (`report_shares.go`), `mustJSON(v) json.RawMessage`, the constant `debitToDay` (a spec on `books.journal_lines`, `report_sources_test.go:923`), `profitAndLossKey`.
- Produces (task 8d relies on these):
  - The run of the shipped General Ledger, Transaction Detail by Account, Trial Balance, Balance Sheet, Journal and Profit and Loss by Month answers `"manual":true`. The run of every other shipped Books report does not.
  - The run of a draft spec (the builder's Preview, a copy) over `books.journal_lines` or `books.basis_lines` answers `"manual":true`.
  - `requireManual(t *testing.T, res *reports.Result)` and `shippedNotLive map[string]bool` (test helpers, `report_live_test.go`).

- [ ] **Step 1: Write the failing tests**

Create `books/server/report_live_test.go`:

```go
package books

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/reports"
)

// shippedNotLive says, for every report Books ships, whether it is NOT live
// (the human's decision of 2026-11-20): the six reports that read every
// journal line do not update by themselves, and the reader presses Refresh.
// A new shipped report is added here, with its answer.
var shippedNotLive = map[string]bool{
	"books.general_ledger":           true,
	"books.transaction_detail":       true,
	"books.trial_balance":            true,
	"books.balance_sheet":            true,
	"books.journal":                  true,
	"books.profit_and_loss_by_month": true,

	"books.profit_and_loss":     false,
	"books.customer_balance":    false,
	"books.audit_log":           false,
	"books.ar_aging":            false,
	"books.sales_by_customer":   false,
	"books.sales_by_product":    false,
	"books.sales_tax_liability": false,
}

// requireManual fails unless res is a manual run: marked, and with no topic.
func requireManual(t *testing.T, res *reports.Result) {
	t.Helper()
	if !res.Manual || len(res.Topics) != 0 {
		t.Errorf("manual = %v, topics = %v, want a manual run with no topic", res.Manual, res.Topics)
	}
}

// liveRun is what the test reads of a run endpoint answer.
type liveRun struct {
	Manual bool     `json:"manual"`
	Topics []string `json:"topics"`
}

// savedOnly are the topics core adds to the run of a saved report; a manual
// run of one carries only these.
var savedOnly = []string{"collection/report_shares", "collection/reports"}

// Every shipped Books report runs through core's run endpoint as a Books
// reader: exactly the six of shippedNotLive are manual, and every other one
// carries data topics. A copy of a live report and a report built over
// journal lines are manual too.
func TestShippedReportsNotLive(t *testing.T) {
	reports.ResetForTesting()
	t.Cleanup(reports.ResetForTesting)
	app, _ := newRegisteredApp(t)
	reports.Register(app)
	mkUser(t, app, "owner")
	reader := mkUser(t, app, "member")
	setPkgAccess(t, app, reader, "readonly")
	if err := reports.ReconcileShipped(app); err != nil {
		t.Fatalf("ReconcileShipped: %v", err)
	}
	if err := reconcileBooksReportShares(app); err != nil {
		t.Fatalf("reconcileBooksReportShares: %v", err)
	}
	rows, err := app.FindRecordsByFilter("reports", "shipped_key != ''", "shipped_key", 0, 0)
	if err != nil {
		t.Fatalf("read the shipped reports: %v", err)
	}
	var keys []string
	for _, row := range rows {
		keys = append(keys, row.GetString("shipped_key"))
	}
	if want := slices.Sorted(maps.Keys(shippedNotLive)); !slices.Equal(keys, want) {
		t.Fatalf("shipped reports = %v, want %v: list a new report in shippedNotLive", keys, want)
	}

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	err = app.OnServe().Trigger(&core.ServeEvent{App: app, Router: router}, func(e *core.ServeEvent) error {
		mux, err := e.Router.BuildMux()
		if err != nil {
			return err
		}
		run := func(body map[string]any) (liveRun, error) {
			body["options"] = map[string]any{"today": "2026-11-09"}
			req := httptest.NewRequest(http.MethodPost, "/api/reports/run", bytes.NewReader(mustJSON(body)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", tokenFor(t, reader))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			var out liveRun
			if rec.Code != http.StatusOK {
				return out, fmt.Errorf("status %d, want 200; body: %s", rec.Code, rec.Body.String())
			}
			return out, json.Unmarshal(rec.Body.Bytes(), &out)
		}

		for _, row := range rows {
			key := row.GetString("shipped_key")
			got, err := run(map[string]any{"reportId": row.Id})
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			if got.Manual != shippedNotLive[key] {
				t.Errorf("%s: manual = %v, want %v", key, got.Manual, shippedNotLive[key])
			}
			// A manual run follows no data: it keeps only the topics of the
			// saved report itself. A live run follows its data too.
			if hasData := !slices.Equal(got.Topics, savedOnly); hasData == got.Manual {
				t.Errorf("%s: manual = %v with topics %v", key, got.Manual, got.Topics)
			}
		}

		// The Profit and Loss stays live over the journal lines.
		for _, row := range rows {
			if row.GetString("shipped_key") != profitAndLossKey {
				continue
			}
			got, err := run(map[string]any{"reportId": row.Id})
			if err != nil {
				return err
			}
			if !slices.Contains(got.Topics, "collection/books_journal_lines") {
				t.Errorf("Profit and Loss topics = %v, want collection/books_journal_lines", got.Topics)
			}
		}

		// A draft is never the shipped record: the same spec, posted as a
		// copy or from the builder, is manual and follows nothing.
		drafts := map[string]json.RawMessage{
			"a copy of the Profit and Loss":     profitAndLossShipped().Spec,
			"a copy of the Customer Balance":    customerBalanceShipped().Spec,
			"a copy of the Audit Log":           auditLogShipped().Spec,
			"a report built over journal lines": json.RawMessage(debitToDay),
		}
		for name, spec := range drafts {
			got, err := run(map[string]any{"spec": spec})
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			if !got.Manual || len(got.Topics) != 0 {
				t.Errorf("%s: manual = %v, topics = %v, want a manual run with no topic", name, got.Manual, got.Topics)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Only the three reports that read the journal lines and stay live declare
// Live. A report that does not read them needs no mark.
func TestShippedLiveDeclarations(t *testing.T) {
	all := []reports.Shipped{
		trialBalanceShipped(), generalLedgerShipped(), balanceSheetShipped(), journalShipped(),
		profitAndLossShipped(), profitAndLossByMonthShipped(), transactionDetailShipped(),
		salesTaxLiabilityShipped(), customerBalanceShipped(), arAgingShipped(),
		salesByCustomerShipped(), salesByProductShipped(), auditLogShipped(),
	}
	var live []string
	for _, s := range all {
		if s.Live {
			live = append(live, s.Key)
		}
	}
	slices.Sort(live)
	want := []string{"books.audit_log", "books.customer_balance", "books.profit_and_loss"}
	if !slices.Equal(live, want) {
		t.Errorf("shipped reports with Live = %v, want %v", live, want)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run, from `/Users/nas/tmp/tinycld/books-purchases/books/server`:

```bash
go test -run 'TestShipped' .
```

Expected: FAIL. `TestShippedReportsNotLive` prints `books.balance_sheet: manual = false, want true` and the same line for `books.general_ledger`, `books.journal`, `books.profit_and_loss_by_month`, `books.transaction_detail`, `books.trial_balance`, and four `... want a manual run with no topic` lines for the drafts. `TestShippedLiveDeclarations` prints `shipped reports with Live = [], want [books.audit_log books.customer_balance books.profit_and_loss]`.

- [ ] **Step 3: Change the tests that read a run's topics**

With `NotLive` set, nine tests that exist fail. Change them now, so step 4 ends green.

(a) The three run helpers pass the report's own `Live`, as core's run endpoint does for the shipped record. In `report_profit_and_loss_test.go`, `runPL` (line 42):

```go
	res, err := reports.Run(context.Background(), app, reports.Request{
		Spec: spec, Loaded: info, Params: given, Options: opts, Auth: reader, Live: shipped.Live,
	})
```

In `report_customer_balance_test.go`, `runCBErr` (line 39):

```go
	return reports.Run(context.Background(), app, reports.Request{
		Spec: spec, Loaded: info, Params: given, Options: opts, Auth: reader, Live: customerBalanceShipped().Live,
	})
```

In `report_audit_log_test.go`, `runAuditErr` (line 102):

```go
	return reports.Run(context.Background(), app, reports.Request{
		Spec: spec, Loaded: info, Params: given, Options: opts, Auth: reader, Live: auditLogShipped().Live,
	})
```

(b) In each of the five tests below, replace the whole `for _, topic := range []string{ … } { if !slices.Contains(res.Topics, topic) { … } }` loop that follows the named line with the one line `requireManual(t, res)`. The test then ends with that line and `}`.

- `report_balance_sheet_test.go`, `TestBalanceSheetTopics`, after `res := b.bsOpened(t, reports.Options{Today: bsToday}, checking)`
- `report_general_ledger_test.go`, `TestGeneralLedgerTopics`, after `res := runGL(t, b.app, b.reader, map[string]any{"period": glPeriod("2026-08-01", "2026-08-31")}, reports.Options{Today: glToday})`
- `report_transaction_detail_test.go`, `TestTransactionDetailTopics`, after `res := tdRun(t, b.app, b.reader, map[string]any{"period": glPeriod("2026-11-01", "2026-11-30")})`
- `report_trial_balance_test.go`, `TestTrialBalanceTopics`, after `res := runTB(t, b.app, b.reader, map[string]any{"asOf": tbToday}, opts)` (the second `runTB` call of the test)
- `report_journal_test.go`, `TestJournalTopics`, after `res := runJournal(t, b.app, b.reader, map[string]any{"period": jnAugust}, reports.Options{Today: glToday})`

For example, the end of `TestJournalTopics` becomes:

```go
	res := runJournal(t, b.app, b.reader, map[string]any{"period": jnAugust}, reports.Options{Today: glToday})
	requireManual(t, res)
}
```

Change the comment above each of the five tests to: `// The report reads every journal line, so it is not live: its run is manual and follows no topic; the reader presses Refresh.` Every one of the five files still uses `slices` elsewhere, so no import changes.

(c) In `report_profit_and_loss_test.go`, `TestProfitAndLossTopics`, the subtest `by month`: replace everything from the comment `// The by-month report has no opened lines, …` to the end of the `if slices.Contains(res.Topics, "collection/books_journal_entries") { … }` block with:

```go
		// The by-month report reads every journal line of the period, so it
		// is not live: it follows no topic, and the reader presses Refresh.
		requireManual(t, res)
```

The subtest `profit and loss` does not change: the Profit and Loss stays live, and `runPL` now passes its `Live`.

(d) In `report_sources_test.go`, `TestReportSourcesRun` (line 985), replace the `for _, topic := range []string{"collection/books_journal_lines", "collection/books_settings"} { … }` loop with:

```go
	// A report a user builds over journal lines is not live.
	requireManual(t, res)
```

- [ ] **Step 4: Declare the sources and the live reports**

In `books/server/report_sources.go`, the journal lines source (line 79):

```go
		{
			Key: "books.journal_lines", Collection: "books_journal_lines", Label: "Journal lines",
			// The largest Books table: a report over it is not re-run on
			// every change (the reader presses Refresh). A shipped report
			// that stays fast on a large book sets Shipped.Live.
			NotLive: true,
			Fields: []reports.Field{
```

and `basisLinesSource` (line 232):

```go
	return reports.Source{
		Key: "books.basis_lines", Label: "Journal lines by basis",
		// It reads the journal lines, so it is not live either.
		NotLive: true,
		Parts: []reports.SourcePart{
```

In `books/server/report_profit_and_loss.go`, `profitAndLossShipped` only (NOT `profitAndLossByMonthShipped`):

```go
		Revision:    3,
		Unpublished: true,
		// It reads the journal lines (a source that is NotLive) but only
		// their sums, so its run stays fast on a large book: it stays live.
		Live: true,
	}
```

In `books/server/report_customer_balance.go`, `customerBalanceShipped`:

```go
		Revision:    1,
		Unpublished: true,
		// It reads the journal lines (a source that is NotLive) but only
		// their sums, so its run stays fast on a large book: it stays live.
		Live: true,
	}
```

In `books/server/report_audit_log.go`, `auditLogShipped`:

```go
		Revision:    1,
		Unpublished: true,
		// Its view reads the journal lines (a source that is NotLive), but a
		// run reads one page of audit rows, so it stays live.
		Live: true,
	}
```

Run `gofmt -l .` in `books/server`; it must print nothing (gofmt aligns the struct fields; accept its result with `gofmt -w` on the files it names).

- [ ] **Step 5: Run the tests to verify they pass**

Run, from `/Users/nas/tmp/tinycld/books-purchases/books/server`:

```bash
go vet . && go test -run 'Shipped|Topics$|TestReportSourcesRun$|SpecSHA' .
```

Expected: `ok  	tinycld.org/packages/books`. The pattern runs the two new tests, every `…Topics` test, `TestReportSourcesRun`, and the shipped-report tests (spec hash, shipped and shared) that prove no spec or revision changed.

- [ ] **Step 6: Update the Books help topic**

In `books/help/reports.md`:

Line 8, replace the paragraph with:

```markdown
Books reports, such as the Trial Balance, run in the Reports area of the app. A change to a chip or to Customize applies at once. The Accounts filter applies when you press Apply. There is no Run button. Some large reports do not update by themselves when the books change: see "Reports that you refresh" below.
```

Lines 43 (Trial Balance), 121 (Balance Sheet) and 142 (Journal) each start with `The report stays current. When someone posts or voids an entry, …, the report runs again.` Replace each of these three lines with:

```markdown
The report does not update by itself. To see new changes, press **Refresh** (see "Reports that you refresh" below).
```

Add the same line as its own paragraph in two more sections: in "Read the General Ledger", after the paragraph at line 70 (`A large book does not load all its lines at once. …`), and in "See the transactions behind an amount", after the paragraph at line 221 (`Transaction Detail by Account also opens from Reports, …`).

In "Build your own Books report", after the paragraph at line 392 (`To make your own report from Books data, …`), add this paragraph:

```markdown
A report that you build over journal lines does not update by itself (see "Reports that you refresh" above).
```

Line 184 (Profit and Loss), replace with:

```markdown
The Profit and Loss stays current. When someone posts or voids an entry, changes an account, or changes the Books settings, the report runs again. The Profit and Loss by Month does not update by itself: press **Refresh** (see "Reports that you refresh" below).
```

Add this section immediately before `## Download or email a report`:

```markdown
## Reports that you refresh

Six Books reports read every journal line, so they do not run again each time the books change: the General Ledger, Transaction Detail by Account, the Trial Balance, the Balance Sheet, the Journal and the Profit and Loss by Month. Under the chips, each of them shows the note "This report does not update by itself. Press Refresh to see new changes." and a **Refresh** button.

To see the changes made after you opened the report, press **Refresh**, or press the R key. The report runs again and keeps your place. A change that you make to a chip, to a filter or to Customize still applies at once, and that run reads the newest data.

Transaction Detail by Account opened from a figure, in the panel at the side, has its own **Refresh** button.

The Profit and Loss, the Customer Balance, the A/R Aging, the sales reports, the Sales Tax Liability and the Audit Log stay current: they run again when the books change, and show no **Refresh** button. When someone saves a change to the Profit and Loss, the Customer Balance or the Audit Log, that report does not update by itself any more and shows **Refresh**.

A report that you build over journal lines or over the Audit log does not update by itself. A copy that you keep with Save as of one of the six reports above, of the Profit and Loss, of the Customer Balance or of the Audit Log does not update by itself either. Each shows the same note and **Refresh** button. A copy of the A/R Aging, of a sales report or of the Sales Tax Liability stays current.
```

Do not run `packages:generate`: the driver does.

- [ ] **Step 7: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add server/report_live_test.go server/report_sources.go server/report_profit_and_loss.go server/report_customer_balance.go server/report_audit_log.go server/report_profit_and_loss_test.go server/report_customer_balance_test.go server/report_audit_log_test.go server/report_balance_sheet_test.go server/report_general_ledger_test.go server/report_transaction_detail_test.go server/report_trial_balance_test.go server/report_journal_test.go server/report_sources_test.go help/reports.md
git commit -m "feat(reports): the six reports over every journal line are not live"
```

After this task, six Books e2e tests that wait for a post from another session to show in an open report with no action are red until task 8d changes them to press Refresh: `journal-report.spec.ts:1320`, `trial-balance.spec.ts:481`, `balance-sheet.spec.ts:845`, `general-ledger.spec.ts:843`, `report-drill-down.spec.ts:511`, `report-drill-panel.spec.ts:180-200`. Do not run or change them in this task.
