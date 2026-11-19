# Stored Receivable Items (p5-report-perf, stored part 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The A/R Aging reads a stored table `books_receivable_items` that Books keeps current in each write transaction, and reads only the rows of items open on the As of day.

**Architecture:** One base collection holds the rows the two receivable views compute (one per posted Accounts receivable line, one per application of a posted payment) plus `source` and `settled`. Two Go SQL constants are the one definition of a row; a per-entry / per-payment / per-invoice refresh, a whole-book rebuild and a verify all read them. The refresh runs after the record write inside the write's transaction and calls `changesignal.Touch`. The report source reads the table and the A/R Aging filter adds `settled > asOf`.

**Tech Stack:** Go (PocketBase fork, dbx, SQLite window functions and `UPDATE … FROM`), PocketBase JS migration, core `reports` and `changesignal`.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/stored-receivables-design.md` (approved 2026-11-18) and `/Users/nas/tmp/tinycld/books-purchases/books/docs/specs/ar-aging-design.md` (§3 Live, §4 Data, §9). Outline of all six tasks: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/plan-outline-stored.md`.

**This part:** tasks 1 to 3 of the outline (S1 to S3). Part 2 (`2026-11-18-report-perf-stored-part-2.md`, not written) holds S4 (the views go), S5 (spec and record) and S6 (L profile).

**Repos:** workspace `/Users/nas/tmp/tinycld/books-purchases`; every change is in `books/` (branch `books/p5-report-perf`). No core change. All Go commands run in `/Users/nas/tmp/tinycld/books-purchases/books/server`.

## Global Constraints

- The bars of spec §9 are never changed, no run is removed, the dataset is not changed.
- Books only. Nothing under `/Users/nas/tmp/tinycld/books-purchases/tinycld` changes. If a step seems to need a core change, stop and report it.
- The definition of a receivable row is in one place: `receivableLinesSQL` and `receivableApplicationsSQL` in `server/receivable_items.go`. Refresh, rebuild and verify read these constants; no second copy in Go. (The views' SQL in the migration stays until task S4 of part 2 drops the views.)
- Migration `2100000026_create_books_receivable_items.js` is unreleased: edit it in place. No backfill (a new install has an empty ledger). No new migration file.
- Money is integer cents. Days are text columns `YYYY-MM-DD`; an open item's `settled` is `9999-12-31`.
- Every refresh runs inside the transaction of the write that changes the rows (`tx.IsTransactional()`), after the record is written.
- Existing A/R Aging tests keep their expected figures and rows. Only the expectations this plan names change (topics, the source catalog, the plan checks' table names).
- Server logging: the package `log` (`logging.ForPackage`), `*Context` variants where a `ctx` exists; no message prefix.
- No word "hosting", "saas", "tenant", "router" in Books.
- Code starts from a failing test that ran RED. Undo an edit with the Edit tool, never `git checkout`/`git restore`. Read `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/agent-rules.md` first.

## Facts the planner measured (do not find again)

The whole of tasks 1 to 3 was built in a scratch copy of `books/server` and `books/pb-migrations` and run:

- After task 2 the whole package (`go test -count=1 .`, about 185 s) is green with `verifyReceivables` in `requireNoDrift`: no existing test needs a rebuild call.
- The engine writes a `Date`/`TextDay` field in a filter as `NULLIF(substr(col, 1, 10), '')`, which no index serves; with `settled` declared `reports.Date` runs 4, 5 and 6 fail the plan bar "a bound statement has no SCAN of a large table". Declared `reports.Text`, core accepts `gt(settled, asOf)`, writes the bare column, and `TestARAgingQueryPlans` passes every bar. So `settled` is a `Text` field.
- Dropping the rows of settled items changes one result: a family whose sub-customer has only settled items lost its header and total (`TestARAgingFamilies`, D7; the family's `Inline` formula counts rows that name a sub-customer). Fix (in this plan, a change to the approved design): the first item of a sub-customer is open for good, and a customer update settles that customer's rows again (a fifth hook point). With it `TestARAgingFamilies` passes unchanged.
- `settled` is per customer and item (the report's item group is under the customer group).
- The lines query starts `FROM books_journal_lines l JOIN books_accounts a` (the view's `FROM books_accounts a CROSS JOIN books_journal_lines l` would read every A/R line to refresh one entry).
- In a set-operator chain SQLite reads `a EXCEPT b UNION ALL c` left to right: the computed side of the verify is one subquery.
- A refresh settles by customer, so the table has the index `idx_books_receivable_items_customer (customer, item, date)`; `TestReceivableSettleReadsOneCustomer` (task 2) pins that the settle statement of one customer has no `SCAN books_receivable_items`. It passes as soon as task 2 compiles (the index is in task 1): it is a pin, not a RED test.

## Review Focus

1. A payment that paid an invoice in full is voided later: the invoice must show as open again, with no application row left. Test: `TestReceivableRefreshOnVoid` (task 2).
2. A sub-customer whose every item is paid: its family keeps its header and total, and loses them only when the customer leaves the family. Tests: `TestReceivableSettledFollowsParent` (task 2), the unchanged `TestARAgingFamilies` (task 3, step 5).
3. An As of day before, on and after the day an item was paid in full, and after that payment's void: the item shows whole before, not at all from the day on, and whole again after the void. Tests: `TestARAgingSettledItemOnPastDay` (task 3, step 1; the one report-level pin of the strict `settled > asOf` filter), `TestReceivableSettled` pins the stored day (task 1); the unchanged `TestARAgingAsOfDay` and `TestARAgingHiddenItems` (task 3, step 5).
4. An invoice correction while a live payment pays the original: the application row must name the correction. Test: `TestReceivableRefreshOnInvoiceCorrection` (task 2).
5. A book whose rows were written past the hooks (a raw import, a repair): the report would show stale rows with no error. Tests: `TestVerifyReceivablesFindsDrift` (task 1), `TestRebuildRoute_RepairsReceivables` (task 2).

---

### Task 1: The stored table, its rebuild and its verify

**Files:**
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/pb-migrations/2100000026_create_books_receivable_items.js`
- Modify (replace whole): `/Users/nas/tmp/tinycld/books-purchases/books/server/receivable_items.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/oauth_scopes.go:86-87`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/registrations_test.go:31-39`
- Test (create): `/Users/nas/tmp/tinycld/books-purchases/books/server/receivable_stored_test.go`

**Interfaces:**
- Consumes: test helpers of `server/receivable_items_test.go` (`newARBook`, `arInvoice`, `arPayment`, `arCreditMemo`, `arJournalPrefix`, `allRows`, `arRow`), `postJournal` and `jnLine` (`report_journal_test.go`).
- Produces:
  - consts `receivableItemsCollection = "books_receivable_items"`, `receivableSourceLine = "line"`, `receivableSourceApplication = "application"`, `receivableOpen = "9999-12-31"`, `receivableColumns`, `receivableLinesSQL`, `receivableApplicationsSQL`, `receivableSettledSQL` (a format string with one `%s`: a WHERE on the stored rows or "");
  - `insertReceivables(tx core.App, query, where string, params dbx.Params) (int64, error)`;
  - `settleReceivables(tx core.App, customers []string) (int64, error)` (nil: every customer; a blank id: the rows with no customer; returns the rows changed);
  - `rebuildReceivables(tx core.App) error`;
  - `type ReceivableDrift struct{ ID, Entry, Item, Problem string }` (`Problem` is `stale`, `missing` or `settled`), `verifyReceivables(app core.App) ([]ReceivableDrift, error)` (at most 100);
  - `settleReceivablesSQL(where string) string` (the settle statement; `where` is a WHERE on the stored rows or "");
  - test helpers `type storedRow struct{ ID, Source, Customer, Entry, Date string; Debit, Credit int; Item, Kind, Number string; Seq int; Due, Currency, Settled string }`, `storedRows(t testing.TB, app core.App) []storedRow`, `requireRebuilt(t testing.TB, app core.App)`, `requireNoReceivableDrift(t testing.TB, app core.App)`, `settledOf(t testing.TB, app core.App, item string) string`.

- [ ] **Step 1: Write the failing test**

Create `server/receivable_stored_test.go`:

```go
package books

import (
	"sort"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// storedRow is one books_receivable_items row.
type storedRow struct {
	ID       string `db:"id"`
	Source   string `db:"source"`
	Customer string `db:"customer"`
	Entry    string `db:"entry"`
	Date     string `db:"date"`
	Debit    int    `db:"debit"`
	Credit   int    `db:"credit"`
	Item     string `db:"item"`
	Kind     string `db:"item_kind"`
	Number   string `db:"item_number"`
	Seq      int    `db:"item_seq"`
	Due      string `db:"due_date"`
	Currency string `db:"currency"`
	Settled  string `db:"settled"`
}

func storedRows(t testing.TB, app core.App) []storedRow {
	t.Helper()
	var rows []storedRow
	err := app.DB().NewQuery("SELECT id, source, customer, entry, date, currency, debit, credit, item, item_kind, item_number, item_seq, due_date, settled FROM books_receivable_items ORDER BY id").All(&rows)
	if err != nil {
		t.Fatalf("read the stored receivable rows: %v", err)
	}
	return rows
}

// requireRebuilt rebuilds the stored rows and fails when they then drift.
func requireRebuilt(t testing.TB, app core.App) {
	t.Helper()
	if err := app.RunInTransaction(rebuildReceivables); err != nil {
		t.Fatalf("rebuild the receivable rows: %v", err)
	}
	requireNoReceivableDrift(t, app)
}

func requireNoReceivableDrift(t testing.TB, app core.App) {
	t.Helper()
	drifts, err := verifyReceivables(app)
	if err != nil {
		t.Fatalf("verify receivables: %v", err)
	}
	if len(drifts) > 0 {
		t.Fatalf("receivable rows drifted from the book: %+v", drifts)
	}
}

// settledOf is the one settled day of the stored rows of item.
func settledOf(t testing.TB, app core.App, item string) string {
	t.Helper()
	days := map[string]bool{}
	for _, r := range storedRows(t, app) {
		if r.Item == item {
			days[r.Settled] = true
		}
	}
	if len(days) != 1 {
		t.Fatalf("item %s has settled days %v, want exactly one", item, days)
	}
	for day := range days {
		return day
	}
	return ""
}

// The rebuild stores exactly the rows of the two views.
func TestReceivableRebuildMatchesViews(t *testing.T) {
	b := newARBook(t)
	c := b.docs.customer
	inv := arInvoice(t, b, c, "2026-08-04", "2026-09-03", 100000)
	arPayment(t, b, c, "2026-09-10", 70000, map[string]int{inv.Id: 55000})
	arCreditMemo(t, b, c, "2026-10-20", 63500)
	arJournalPrefix(t, b)
	postJournal(t, b.app, "", "2026-09-01", "A/R adjustment",
		jnLine{account: b.receivable, debit: 12000, position: 0},
		jnLine{account: b.income, credit: 12000, position: 1})
	requireRebuilt(t, b.app)

	want := allRows(t, b.app)
	sort.Slice(want, func(i, j int) bool { return want[i].id < want[j].id })
	got := storedRows(t, b.app)
	if len(got) != len(want) || len(got) != 5 {
		t.Fatalf("stored %d rows, the views give %d, want 5", len(got), len(want))
	}
	for i, w := range want {
		g := got[i]
		if g.ID != w.id || g.Customer != w.customer || g.Entry != w.entry || g.Date != w.date ||
			g.Debit != w.debit || g.Credit != w.credit || g.Item != w.item || g.Currency != w.currency ||
			g.Kind != w.kind || g.Number != w.number || g.Seq != w.seq || g.Due != w.due {
			t.Errorf("stored row %d = %+v, the view row is %+v", i, g, w)
		}
	}
}

// settled is the day a customer's item reaches zero for good.
func TestReceivableSettled(t *testing.T) {
	b := newARBook(t)
	c := b.docs.customer
	open := arInvoice(t, b, c, "2026-08-04", "2026-09-03", 100000)
	arPayment(t, b, c, "2026-09-10", 40000, map[string]int{open.Id: 40000})
	paid := arInvoice(t, b, c, "2026-08-05", "2026-09-04", 50000)
	arPayment(t, b, c, "2026-09-11", 20000, map[string]int{paid.Id: 20000})
	arPayment(t, b, c, "2026-09-20", 30000, map[string]int{paid.Id: 30000})
	sameDay := arInvoice(t, b, c, "2026-08-06", "2026-09-05", 7000)
	arPayment(t, b, c, "2026-08-06", 7000, map[string]int{sameDay.Id: 7000})
	requireRebuilt(t, b.app)

	for name, tc := range map[string]struct{ item, want string }{
		"part paid: open":                     {open.Id, receivableOpen},
		"paid in two parts: the last payment": {paid.Id, "2026-09-20"},
		"paid on its own day: that day":       {sameDay.Id, "2026-08-06"},
	} {
		if got := settledOf(t, b.app, tc.item); got != tc.want {
			t.Errorf("%s: settled = %q, want %q", name, got, tc.want)
		}
	}
}

// A payment's void opens the invoice again: zero for a time is not settled.
func TestReceivableSettledReopens(t *testing.T) {
	b := newARBook(t)
	c := b.docs.customer
	inv := arInvoice(t, b, c, "2026-08-04", "2026-09-03", 100000)
	pay := arPayment(t, b, c, "2026-09-10", 100000, map[string]int{inv.Id: 100000})
	requireRebuilt(t, b.app)
	if got := settledOf(t, b.app, inv.Id); got != "2026-09-10" {
		t.Fatalf("paid in full: settled = %q, want 2026-09-10", got)
	}
	pay.Set("status", statusVoid)
	if err := b.app.Save(pay); err != nil {
		t.Fatalf("void the payment: %v", err)
	}
	requireRebuilt(t, b.app)
	if got := settledOf(t, b.app, inv.Id); got != receivableOpen {
		t.Errorf("after the payment's void: settled = %q, want open", got)
	}
}

// verifyReceivables names a stale row, a missing row and a wrong settled day.
func TestVerifyReceivablesFindsDrift(t *testing.T) {
	b := newARBook(t)
	c := b.docs.customer
	inv := arInvoice(t, b, c, "2026-08-04", "2026-09-03", 100000)
	arPayment(t, b, c, "2026-09-10", 100000, map[string]int{inv.Id: 100000})
	requireRebuilt(t, b.app)
	rows := storedRows(t, b.app)
	exec := func(sql string) {
		t.Helper()
		if _, err := b.app.DB().NewQuery(sql).Execute(); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec("UPDATE books_receivable_items SET debit = debit + 1 WHERE id = '" + rows[0].ID + "'")
	exec("DELETE FROM books_receivable_items WHERE id = '" + rows[1].ID + "'")
	drifts, err := verifyReceivables(b.app)
	if err != nil {
		t.Fatalf("verify receivables: %v", err)
	}
	problems := map[string]int{}
	for _, d := range drifts {
		problems[d.Problem]++
	}
	if problems["stale"] != 1 || problems["missing"] != 2 || problems["settled"] == 0 {
		t.Fatalf("drifts = %+v, want 1 stale, 2 missing and a settled drift", drifts)
	}
	requireRebuilt(t, b.app)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run 'TestReceivableRebuildMatchesViews|TestReceivableSettled|TestVerifyReceivablesFindsDrift' -count=1 .`
Expected: FAIL, the build fails with `undefined: rebuildReceivables`, `undefined: verifyReceivables`, `undefined: receivableOpen`.

- [ ] **Step 3: Add the base collection to the migration**

In `pb-migrations/2100000026_create_books_receivable_items.js`, insert this block directly above the line `const VIEWS = [`:

```js
const ITEMS = 'books_receivable_items'

const itemsCollection = app =>
    new Collection({
        id: 'pbc_books_receivable_items',
        name: ITEMS,
        type: 'base',
        listRule: READ,
        viewRule: READ,
        createRule: null,
        updateRule: null,
        deleteRule: null,
        fields: [
            { name: 'source', type: 'select', required: true, maxSelect: 1, values: ['line', 'application'] },
            {
                name: 'customer',
                type: 'relation',
                collectionId: app.findCollectionByNameOrId('books_customers').id,
                cascadeDelete: false,
                maxSelect: 1,
            },
            {
                name: 'entry',
                type: 'relation',
                required: true,
                collectionId: app.findCollectionByNameOrId('books_journal_entries').id,
                cascadeDelete: false,
                maxSelect: 1,
            },
            { name: 'date', type: 'text', required: true },
            { name: 'currency', type: 'text' },
            { name: 'debit', type: 'number', onlyInt: true },
            { name: 'credit', type: 'number', onlyInt: true },
            { name: 'item', type: 'text', required: true },
            { name: 'item_kind', type: 'text', required: true },
            { name: 'item_number', type: 'text' },
            { name: 'item_seq', type: 'number', onlyInt: true },
            { name: 'due_date', type: 'text' },
            { name: 'settled', type: 'text', required: true },
        ],
        indexes: [
            `CREATE INDEX idx_books_receivable_items_entry ON ${ITEMS} (entry)`,
            `CREATE INDEX idx_books_receivable_items_item ON ${ITEMS} (item)`,
            `CREATE INDEX idx_books_receivable_items_customer ON ${ITEMS} (customer, item, date)`,
            `CREATE INDEX idx_books_receivable_items_settled ON ${ITEMS} (settled, date)`,
        ],
    })

```

In the up function, add one line directly above `for (const [name, viewQuery] of VIEWS) {`:

```js
        app.save(itemsCollection(app))
```

In the down function, add one line directly above `for (const [name] of VIEWS) {`:

```js
        app.delete(app.findCollectionByNameOrId(ITEMS))
```

In the file's header comment, add this paragraph directly above the line `// AUTHORIZATION. Every member with read access lists and views the rows; a`:

```js
// books_receivable_items stores the rows of both views, with `source` (line or
// application) and `settled`: the day the balance of the row's customer and
// item becomes zero and stays zero, 9999-12-31 while it is open. Only Books
// writes it, from the two queries in server/receivable_items.go, inside the
// transaction of each write that changes a row. Its rules let a reader list
// and view; nobody creates, updates or deletes a row over REST. No backfill:
// a new install has an empty ledger.
//
```

- [ ] **Step 4: Write the implementation**

Replace the whole of `server/receivable_items.go` with:

```go
package books

import (
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/changesignal"
)

const (
	receivableLinesView        = "books_receivable_lines"
	receivableApplicationsView = "books_receivable_applications"
)

// Both views give A/R lines and payment applications to their open items: a
// post or void moves lines, entries and applications; an account type change
// moves which lines are A/R; a payment's status and credited and an invoice's
// due day are read into the rows.
func registerReceivableItems() {
	sources := []string{
		"books_journal_lines", "books_journal_entries", "books_accounts",
		"books_invoices", "books_payments", "books_payment_applications",
	}
	changesignal.RegisterView(receivableLinesView, sources...)
	changesignal.RegisterView(receivableApplicationsView, sources...)
}

// receivableItemsCollection stores the rows of every receivable item: one row
// per posted Accounts receivable journal line (source line) and one per
// application of a posted payment (source application). Only Books writes it,
// with raw SQL from the two queries below, inside the transaction of the write
// that changes a row. settled is the day the balance of the row's customer
// and item becomes zero and stays zero, or receivableOpen while the item is
// open: the A/R Aging reads only the rows with settled after its As of day.
const (
	receivableItemsCollection   = "books_receivable_items"
	receivableSourceLine        = "line"
	receivableSourceApplication = "application"
	receivableOpen              = "9999-12-31"
)

// receivableColumns are the columns the two queries give, in their order.
const receivableColumns = "id, source, customer, entry, date, currency, debit, credit, item, item_kind, item_number, item_seq, due_date"

// A line's entry names its document (e), or the entry it reverses names it
// (r); a reversal carries the doc_type of what it reverses.
const (
	receivableIsDoc = "COALESCE(e.doc_type, '') IN ('invoice', 'payment', 'credit_memo') AND COALESCE(NULLIF(e.document, ''), NULLIF(r.document, '')) IS NOT NULL"
	receivableDocID = "COALESCE(NULLIF(e.document, ''), r.document)"
	// A manual line's item is the line itself; a reversal's line is the item
	// of the reversed line at the same position.
	receivableLineItem = "CASE WHEN " + receivableIsDoc + " THEN " + receivableDocID +
		" WHEN e.reverses != '' THEN COALESCE((SELECT rl.id FROM books_journal_lines rl WHERE rl.entry = e.reverses AND rl.position = l.position), l.id) ELSE l.id END"
)

// receivableLinesSQL is one row per posted journal line on an
// accounts-receivable account. A reversal's line belongs to the item of the
// entry it reverses and ages from that item's due day. On the A/R line of a
// posted payment's own entry, credit is the payment's credited (its unapplied
// part); its applications are the other query's rows. It ends in its WHERE, so
// a refresh adds " AND l.entry = {:entry}".
const receivableLinesSQL = "SELECT l.id AS id, '" + receivableSourceLine + "' AS source, l.customer AS customer, l.entry AS entry, l.date AS date, l.currency AS currency, l.debit AS debit, " +
	"CAST(CASE WHEN p.id IS NOT NULL AND l.credit > 0 THEN p.credited ELSE l.credit END AS INTEGER) AS credit, " +
	"CAST(" + receivableLineItem + " AS TEXT) AS item, " +
	"CAST(CASE WHEN " + receivableIsDoc + " THEN e.doc_type ELSE 'entry' END AS TEXT) AS item_kind, " +
	"CAST(COALESCE(r.number, e.number) AS TEXT) AS item_number, " +
	"CAST(COALESCE(r.sequence, e.sequence) AS INTEGER) AS item_seq, " +
	"CAST(CASE WHEN " + receivableIsDoc + " AND e.doc_type = 'invoice' THEN i.due_date ELSE COALESCE(r.date, e.date) END AS TEXT) AS due_date " +
	"FROM books_journal_lines l " +
	"JOIN books_accounts a ON a.id = l.account " +
	"JOIN books_journal_entries e ON e.id = l.entry " +
	"LEFT JOIN books_journal_entries r ON r.id = e.reverses " +
	"LEFT JOIN books_payments p ON p.id = e.document AND e.doc_type = 'payment' AND p.entry = e.id AND p.status = 'posted' " +
	"LEFT JOIN books_invoices i ON e.doc_type = 'invoice' AND i.id = " + receivableDocID + " " +
	"WHERE l.posted = TRUE AND a.type = 'accounts_receivable'"

// receivableApplicationsSQL is one row per application of a posted payment, a
// credit on the invoice it pays. It ends in its WHERE, so a refresh adds
// " AND pa.payment = {:payment}" or " AND pa.invoice = {:invoice}".
const receivableApplicationsSQL = "SELECT pa.id AS id, '" + receivableSourceApplication + "' AS source, p.customer AS customer, p.entry AS entry, p.date AS date, p.currency AS currency, " +
	"CAST(0 AS INTEGER) AS debit, pa.amount AS credit, " +
	"CAST(pa.invoice AS TEXT) AS item, " +
	"CAST('invoice' AS TEXT) AS item_kind, " +
	"CAST(ie.number AS TEXT) AS item_number, " +
	"CAST(ie.sequence AS INTEGER) AS item_seq, " +
	"i.due_date AS due_date " +
	"FROM books_payment_applications pa " +
	"JOIN books_payments p ON p.id = pa.payment " +
	"JOIN books_invoices i ON i.id = pa.invoice " +
	"JOIN books_journal_entries ie ON ie.id = i.entry " +
	"WHERE p.status = 'posted'"

// receivableSettledSQL gives, for each customer and item of the stored rows
// (%s: "" for all of them, or a WHERE on the stored rows that keeps or drops
// each customer whole), the day its balance becomes zero and stays zero: the
// first day with no later day (itself included) whose end-of-day balance is
// not zero. An item whose last balance is not zero is open.
//
// The first item of a sub-customer (f: by date, then item id) is open for
// good. The A/R Aging draws a family with its header and total when a row of
// the run names a sub-customer (customerFamilyGroups, Inline), also when that
// sub-customer has no open item (D7); the rows of this one item keep that
// true on every day from the sub-customer's first row on. They add up to the
// item's balance on any day, so a settled first item still shows nothing.
const receivableSettledSQL = `
	WITH d AS (
		SELECT customer, item, date, SUM(debit - credit) AS net
		FROM books_receivable_items %s GROUP BY customer, item, date
	), r AS (
		SELECT customer, item, date,
		       SUM(net) OVER (PARTITION BY customer, item ORDER BY date) AS bal
		FROM d
	), z AS (
		SELECT customer, item, date,
		       SUM(bal != 0) OVER (PARTITION BY customer, item ORDER BY date
		                           ROWS BETWEEN CURRENT ROW AND UNBOUNDED FOLLOWING) AS later
		FROM r
	), f AS (
		SELECT customer, item FROM (
			SELECT d.customer AS customer, d.item AS item,
			       ROW_NUMBER() OVER (PARTITION BY d.customer ORDER BY d.date, d.item) AS n
			FROM d JOIN books_customers c ON c.id = d.customer WHERE c.parent != ''
		) WHERE n = 1
	)
	SELECT z.customer AS customer, z.item AS item,
	       CASE WHEN f.item IS NOT NULL THEN '` + receivableOpen + `'
	            ELSE COALESCE(MIN(CASE WHEN z.later = 0 THEN z.date END), '` + receivableOpen + `') END AS settled
	FROM z LEFT JOIN f ON f.customer = z.customer AND f.item = z.item
	GROUP BY z.customer, z.item`

// insertReceivables stores the rows query gives; where narrows it (" AND
// l.entry = {:entry}"). It returns how many rows it wrote.
func insertReceivables(tx core.App, query, where string, params dbx.Params) (int64, error) {
	res, err := tx.DB().NewQuery("INSERT INTO " + receivableItemsCollection + " (" + receivableColumns + ", settled) SELECT " +
		receivableColumns + ", '" + receivableOpen + "' FROM (" + query + where + ")").Bind(params).Execute()
	if err != nil {
		return 0, fmt.Errorf("books: store receivable rows: %w", err)
	}
	return res.RowsAffected()
}

// receivableCustomerChunk bounds the customer ids of one settle statement,
// under SQLite's limit of bound values.
const receivableCustomerChunk = 500

// settleReceivablesSQL writes settled on the stored rows where names (a WHERE
// on the stored rows, or "") finds the customers: only the rows whose day
// changes are written.
func settleReceivablesSQL(where string) string {
	t := receivableItemsCollection
	return "UPDATE " + t + " SET settled = s.settled FROM (" +
		fmt.Sprintf(receivableSettledSQL, where) + ") s WHERE s.customer = " + t + ".customer AND s.item = " +
		t + ".item AND s.settled != " + t + ".settled"
}

// settleReceivables writes settled on the stored rows of customers (a blank id
// is the rows with no customer), or of every customer when customers is nil.
// It returns how many rows it changed.
func settleReceivables(tx core.App, customers []string) (int64, error) {
	update := func(where string, params dbx.Params) (int64, error) {
		res, err := tx.DB().NewQuery(settleReceivablesSQL(where)).Bind(params).Execute()
		if err != nil {
			return 0, fmt.Errorf("books: settle receivable items: %w", err)
		}
		return res.RowsAffected()
	}
	if customers == nil {
		return update("", nil)
	}
	var changed int64
	for start := 0; start < len(customers); start += receivableCustomerChunk {
		chunk := customers[start:min(start+receivableCustomerChunk, len(customers))]
		params := dbx.Params{}
		names := make([]string, len(chunk))
		for i, id := range chunk {
			name := fmt.Sprintf("c%d", i)
			names[i], params[name] = "{:"+name+"}", id
		}
		n, err := update("WHERE customer IN ("+strings.Join(names, ", ")+")", params)
		if err != nil {
			return 0, err
		}
		changed += n
	}
	return changed, nil
}

// rebuildReceivables makes the stored rows equal the two queries over the
// whole book, for a book written past the hooks (a test's raw rows, a repair).
func rebuildReceivables(tx core.App) error {
	if _, err := tx.DB().NewQuery("DELETE FROM " + receivableItemsCollection).Execute(); err != nil {
		return fmt.Errorf("books: clear receivable rows: %w", err)
	}
	for _, query := range []string{receivableLinesSQL, receivableApplicationsSQL} {
		if _, err := insertReceivables(tx, query, "", nil); err != nil {
			return err
		}
	}
	if _, err := settleReceivables(tx, nil); err != nil {
		return err
	}
	changesignal.Touch(tx, receivableItemsCollection)
	return nil
}

// ReceivableDrift is one stored receivable row that disagrees with the book.
// Problem is "stale" (stored, and the queries give no such row), "missing"
// (the queries give it, and it is not stored so) or "settled" (its settled day
// is not the day its item's balance stays zero from).
type ReceivableDrift struct {
	ID      string `db:"id" json:"id"`
	Entry   string `db:"entry" json:"entry"`
	Item    string `db:"item" json:"item"`
	Problem string `db:"problem" json:"problem"`
}

// maxReceivableDriftsReported bounds what verifyReceivables returns.
const maxReceivableDriftsReported = 100

// verifyReceivables compares the stored rows with the two queries, both ways,
// and each row's settled with the day its item settles. Every write path
// refreshes the rows it changes, so a drift means one missed it.
func verifyReceivables(app core.App) ([]ReceivableDrift, error) {
	computed := "SELECT " + receivableColumns + " FROM (" + receivableLinesSQL + " UNION ALL " + receivableApplicationsSQL + ")"
	stored := "SELECT " + receivableColumns + " FROM " + receivableItemsCollection
	drifts := []ReceivableDrift{}
	err := app.DB().NewQuery(`
		SELECT id, entry, item, problem FROM (
			SELECT id, entry, item, 'stale' AS problem FROM (` + stored + ` EXCEPT ` + computed + `)
			UNION ALL
			SELECT id, entry, item, 'missing' AS problem FROM (` + computed + ` EXCEPT ` + stored + `)
			UNION ALL
			SELECT t.id, t.entry, t.item, 'settled' AS problem FROM ` + receivableItemsCollection + ` t
			JOIN (` + fmt.Sprintf(receivableSettledSQL, "") + `) s ON s.customer = t.customer AND s.item = t.item
			WHERE s.settled != t.settled
		) ORDER BY problem, id LIMIT {:max}`).Bind(dbx.Params{"max": maxReceivableDriftsReported}).All(&drifts)
	if err != nil {
		return nil, fmt.Errorf("books: compare the receivable rows with the book: %w", err)
	}
	return drifts, nil
}
```

- [ ] **Step 5: Give the collection its OAuth entry and its server-only mark**

In `server/oauth_scopes.go`, directly under the line `"books_receivable_applications":    ro,` add:

```go
			"books_receivable_items":           ro,
```

In `server/registrations_test.go`, in `var serverOnly`, directly under the line `"books_invoice_tax_lines":       true,` add:

```go
	"books_receivable_items":        true,
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `gofmt -l . ; go vet . && go test -run 'TestReceivableRebuildMatchesViews|TestReceivableSettled|TestVerifyReceivablesFindsDrift|TestReceivableViewsShape|TestAudit_Covers|TestOAuth_Covers' -count=1 .`
Expected: `gofmt -l` prints nothing; `ok  tinycld.org/packages/books`.

- [ ] **Step 7: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add pb-migrations/2100000026_create_books_receivable_items.js server/receivable_items.go server/receivable_stored_test.go server/oauth_scopes.go server/registrations_test.go
git commit -m "feat(books): stored receivable items with rebuild and verify"
```

---

### Task 2: Refresh in the write transaction, the drift check, the rebuild route

**Files:**
- Create: `/Users/nas/tmp/tinycld/books-purchases/books/server/receivable_refresh.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/ledger.go:25` (end of `registerLedger`)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/payments.go:153-154` (end of `registerPaymentGuards`)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/sales_documents.go:209-211`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/balances.go:254-300`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/env_test.go:375-393` (`requireNoDrift`)
- Test (create): `/Users/nas/tmp/tinycld/books-purchases/books/server/receivable_refresh_test.go`

**Interfaces:**
- Consumes (task 1): `receivableItemsCollection`, `receivableSourceLine`, `receivableSourceApplication`, `receivableLinesSQL`, `receivableApplicationsSQL`, `insertReceivables`, `settleReceivables`, `settleReceivablesSQL`, `rebuildReceivables`, `verifyReceivables`, `storedRows`, `settledOf`, `requireNoReceivableDrift`. Existing: `refuse(format string, args ...any) error` (`errors.go`), `paymentsCollection`, `invoiceKind`, `statusDraft`, `statusVoid`, test helpers `mkSuperuser`, `tokenFor`, `apiSeq`, `apiReq`, `mkCustomer`, `newPaymentEnv`, `pe.paidInvoice`, `pe.correctionOf`, `mustPostInvoice`.
- Produces:
  - `type receivablePart struct{ query, where string }`;
  - `refreshReceivables(tx core.App, stale string, params dbx.Params, parts ...receivablePart) error`;
  - `refreshReceivableEntry(tx core.App, entryID string) error`, `refreshReceivablePayment(tx core.App, payment *core.Record) error`, `refreshReceivableInvoice(tx core.App, invoice *core.Record) error`;
  - `bindReceivableEntryRefresh(app core.App)`, `bindReceivableCustomerRefresh(app core.App)`, `bindReceivablePaymentRefresh(app core.App)`, `bindReceivableInvoiceRefresh(app core.App)`, `registerReceivableWriteGuard(app core.App)`, `repairReceivables(tx core.App) error`, const `msgReceivableItemsReadOnly`;
  - `requireNoDrift` fails on a receivable drift; the route `POST /api/books/balances/rebuild` answers one more key, `receivableDrifts` (the list of `ReceivableDrift` read before the repair, at most 100).

The hook points (design §3, plus the customer one): an entry update whose status is not draft (hook 1); a payment update (hook 2: post, void, replacement); a payment application update (hook 3: an invoice correction moves it); an invoice update whose status is not draft (hook 4); a customer update (hook 5: the sub-customer rule of `receivableSettledSQL`). Each is bound inside the function that binds the collection's serialized guard, after it, so `e.App` is the guard's transaction when the refresh runs and every test env that binds the guard binds the refresh. `TestRegister_BindsEachPartOnce` needs no change: no new part is called from `Register`.

- [ ] **Step 1: Write the failing test**

Create `server/receivable_refresh_test.go`:

```go
package books

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
)

// Each post keeps the stored rows in step: no rebuild runs in this test.
func TestReceivableRefreshOnPost(t *testing.T) {
	b := newARBook(t)
	c := b.docs.customer
	inv := arInvoice(t, b, c, "2026-08-04", "2026-09-03", 100000)
	requireNoReceivableDrift(t, b.app)
	pay := arPayment(t, b, c, "2026-09-10", 70000, map[string]int{inv.Id: 55000})
	requireNoReceivableDrift(t, b.app)
	arCreditMemo(t, b, c, "2026-10-20", 63500)
	arJournalPrefix(t, b)
	postJournal(t, b.app, "", "2026-09-01", "A/R adjustment",
		jnLine{account: b.receivable, debit: 12000, position: 0},
		jnLine{account: b.income, credit: 12000, position: 1})
	requireNoReceivableDrift(t, b.app)

	rows := storedRows(t, b.app)
	if len(rows) != 5 {
		t.Fatalf("stored %d rows, want 5 (invoice, payment, application, credit memo, journal line): %+v", len(rows), rows)
	}
	for _, r := range rows {
		if r.Entry == pay.GetString("entry") && r.Source == receivableSourceLine && r.Credit != 15000 {
			t.Errorf("the payment's own line has credit %d, want its unapplied 15000", r.Credit)
		}
	}
	if got := settledOf(t, b.app, inv.Id); got != receivableOpen {
		t.Errorf("a part-paid invoice has settled %q, want open", got)
	}
}

// A payment in full settles the invoice on the payment's day; the payment's
// void takes its applications away and opens the invoice again.
func TestReceivableRefreshOnVoid(t *testing.T) {
	b := newARBook(t)
	c := b.docs.customer
	inv := arInvoice(t, b, c, "2026-08-04", "2026-09-03", 100000)
	pay := arPayment(t, b, c, "2026-09-10", 100000, map[string]int{inv.Id: 100000})
	requireNoReceivableDrift(t, b.app)
	if got := settledOf(t, b.app, inv.Id); got != "2026-09-10" {
		t.Fatalf("paid in full: settled = %q, want 2026-09-10", got)
	}
	pay.Set("status", statusVoid)
	if err := b.app.Save(pay); err != nil {
		t.Fatalf("void the payment: %v", err)
	}
	requireNoReceivableDrift(t, b.app)
	if got := settledOf(t, b.app, inv.Id); got != receivableOpen {
		t.Errorf("after the payment's void: settled = %q, want open", got)
	}
	for _, r := range storedRows(t, b.app) {
		if r.Source == receivableSourceApplication {
			t.Errorf("a void payment still has an application row: %+v", r)
		}
	}
}

// The first item of a sub-customer stays open for good, so the family keeps
// its header and total (D7); the rule follows the customer's parent.
func TestReceivableSettledFollowsParent(t *testing.T) {
	b := newARBook(t)
	parent := mkCustomer(t, b.app, "Keeper Parent", "")
	sub := mkCustomer(t, b.app, "Keeper sub", parent.Id)
	first := arInvoice(t, b, sub, "2026-08-04", "2026-09-03", 7000)
	arPayment(t, b, sub, "2026-09-10", 7000, map[string]int{first.Id: 7000})
	second := arInvoice(t, b, sub, "2026-08-05", "2026-09-04", 8000)
	arPayment(t, b, sub, "2026-09-11", 8000, map[string]int{second.Id: 8000})
	requireNoReceivableDrift(t, b.app)
	if got := settledOf(t, b.app, first.Id); got != receivableOpen {
		t.Errorf("a sub-customer's first item: settled = %q, want open for good", got)
	}
	if got := settledOf(t, b.app, second.Id); got != "2026-09-11" {
		t.Errorf("a sub-customer's later item: settled = %q, want 2026-09-11", got)
	}

	sub, err := b.app.FindRecordById("books_customers", sub.Id)
	if err != nil {
		t.Fatal(err)
	}
	sub.Set("parent", "")
	if err := b.app.Save(sub); err != nil {
		t.Fatalf("take the sub-customer out of its family: %v", err)
	}
	requireNoReceivableDrift(t, b.app)
	if got := settledOf(t, b.app, first.Id); got != "2026-09-10" {
		t.Errorf("after the customer left its parent: settled = %q, want 2026-09-10", got)
	}
}

// A refresh outside a transaction could commit without the write it follows.
func TestReceivableRefreshNeedsTransaction(t *testing.T) {
	b := newARBook(t)
	inv := arInvoice(t, b, b.docs.customer, "2026-08-04", "2026-09-03", 100000)
	if err := refreshReceivableEntry(b.app, inv.GetString("entry")); err == nil {
		t.Fatal("a refresh outside a transaction passed, want a refusal")
	}
	requireNoReceivableDrift(t, b.app)
}

// Nobody writes a stored row over REST, a superuser neither.
func TestReceivableItemsRefuseWrites(t *testing.T) {
	b := newARBook(t)
	arInvoice(t, b, b.docs.customer, "2026-08-04", "2026-09-03", 100000)
	row := storedRows(t, b.app)[0]
	url := "/api/collections/" + receivableItemsCollection + "/records/" + row.ID
	super, reader := tokenFor(t, mkSuperuser(t, b.app)), tokenFor(t, b.reader)
	apiSeq(t, b.app,
		apiReq{method: http.MethodPatch, url: url, token: super, body: `{"debit":1}`, want: http.StatusBadRequest, content: []string{msgReceivableItemsReadOnly}},
		apiReq{method: http.MethodDelete, url: url, token: super, want: http.StatusBadRequest, content: []string{msgReceivableItemsReadOnly}},
		apiReq{method: http.MethodDelete, url: url, token: reader, want: http.StatusForbidden},
		apiReq{method: http.MethodGet, url: url, token: reader, want: http.StatusOK, content: []string{`"debit":100000`}},
	)
	requireNoReceivableDrift(t, b.app)
}

// The rebuild route lists the stored rows that drifted and repairs them.
func TestRebuildRoute_RepairsReceivables(t *testing.T) {
	b := newARBook(t)
	arInvoice(t, b, b.docs.customer, "2026-08-04", "2026-09-03", 100000)
	row := storedRows(t, b.app)[0]
	if _, err := b.app.DB().NewQuery("UPDATE books_receivable_items SET debit = 1 WHERE id = {:id}").Bind(map[string]any{"id": row.ID}).Execute(); err != nil {
		t.Fatal(err)
	}
	apiReq{
		method: http.MethodPost, url: "/api/books/balances/rebuild",
		token: tokenFor(t, b.docs.users.admin), want: http.StatusOK, before: bindBalanceRoutes,
		content: []string{`"receivableDrifts":[{"id":"` + row.ID + `"`, `"problem":"missing"`, `"problem":"stale"`},
	}.run(t, b.app)
	requireNoReceivableDrift(t, b.app)
}

// An invoice correction moves the applications of the live payments to the
// correction before the correction has its entry: the rows follow.
func TestReceivableRefreshOnInvoiceCorrection(t *testing.T) {
	pe := newPaymentEnv(t)
	inv, _ := pe.paidInvoice(t)
	draft := pe.correctionOf(t, inv, pe.customer, 45000)
	mustPostInvoice(t, pe.app, draft.Id, pe.users.bookkeeper.Id)
	requireNoReceivableDrift(t, pe.app)
	applications := 0
	for _, r := range storedRows(t, pe.app) {
		if r.Source != receivableSourceApplication {
			continue
		}
		applications++
		if r.Item != draft.Id {
			t.Errorf("an application row names item %s, want the correction %s (the original is %s)", r.Item, draft.Id, inv.Id)
		}
	}
	if applications == 0 {
		t.Fatal("the paid invoice's correction has no application row")
	}
}

// A refresh settles one customer: it must find that customer's rows by index,
// not read the whole table inside each write transaction.
func TestReceivableSettleReadsOneCustomer(t *testing.T) {
	b := newARBook(t)
	var steps []struct {
		Detail string `db:"detail"`
	}
	err := b.app.DB().NewQuery("EXPLAIN QUERY PLAN " + settleReceivablesSQL("WHERE customer IN ({:c0})")).
		Bind(dbx.Params{"c0": b.docs.customer.Id}).All(&steps)
	if err != nil {
		t.Fatalf("explain the settle statement: %v", err)
	}
	if len(steps) == 0 {
		t.Fatal("the settle statement has no plan step")
	}
	for _, s := range steps {
		if strings.Contains(s.Detail, "SCAN "+receivableItemsCollection) {
			t.Errorf("the settle of one customer scans the stored rows: %q", s.Detail)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run 'TestReceivableRefresh|TestReceivableSettledFollowsParent|TestReceivableItemsRefuseWrites|TestRebuildRoute_RepairsReceivables' -count=1 .`
Expected: FAIL, the build fails with `undefined: refreshReceivableEntry` and `undefined: msgReceivableItemsReadOnly`.

- [ ] **Step 3: Write the refresh**

Create `server/receivable_refresh.go`:

```go
package books

import (
	"errors"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/changesignal"
)

// receivablePart is one of the two queries narrowed to the rows a refresh
// writes again.
type receivablePart struct{ query, where string }

// refreshReceivables replaces the stored rows that match stale (a condition on
// books_receivable_items) with the rows parts give, then settles every
// customer that lost or gained a row. stale must match every row parts can
// give, so no row is stored twice. It must run in the transaction of the
// write that changed the rows: a refresh outside one could commit without
// that write.
func refreshReceivables(tx core.App, stale string, params dbx.Params, parts ...receivablePart) error {
	if !tx.IsTransactional() {
		return errors.New("books: receivable rows are refreshed only inside the transaction of the write")
	}
	customers := map[string]bool{}
	collect := func() error {
		var ids []string
		err := tx.DB().NewQuery("SELECT DISTINCT customer FROM " + receivableItemsCollection + " WHERE " + stale).Bind(params).Column(&ids)
		if err != nil {
			return fmt.Errorf("books: read the customers of receivable rows: %w", err)
		}
		for _, id := range ids {
			customers[id] = true
		}
		return nil
	}
	if err := collect(); err != nil {
		return err
	}
	res, err := tx.DB().NewQuery("DELETE FROM " + receivableItemsCollection + " WHERE " + stale).Bind(params).Execute()
	if err != nil {
		return fmt.Errorf("books: clear stale receivable rows: %w", err)
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return err
	}
	for _, part := range parts {
		n, err := insertReceivables(tx, part.query, part.where, params)
		if err != nil {
			return err
		}
		changed += n
	}
	if changed == 0 {
		return nil
	}
	if err := collect(); err != nil {
		return err
	}
	ids := make([]string, 0, len(customers))
	for id := range customers {
		ids = append(ids, id)
	}
	if _, err := settleReceivables(tx, ids); err != nil {
		return err
	}
	changesignal.Touch(tx, receivableItemsCollection)
	return nil
}

// refreshReceivableEntry stores the Accounts receivable lines of one entry.
func refreshReceivableEntry(tx core.App, entryID string) error {
	return refreshReceivables(tx, "entry = {:entry} AND source = '"+receivableSourceLine+"'", dbx.Params{"entry": entryID},
		receivablePart{receivableLinesSQL, " AND l.entry = {:entry}"})
}

// refreshReceivablePayment stores the rows of one payment: the line of its
// own entry (its credit is the payment's credited while the payment is
// posted) and its applications (none unless it is posted). A payment with no
// entry was never posted and has no row.
func refreshReceivablePayment(tx core.App, payment *core.Record) error {
	entryID := payment.GetString("entry")
	if entryID == "" {
		return nil
	}
	return refreshReceivables(tx, "entry = {:entry}", dbx.Params{"entry": entryID, "payment": payment.Id},
		receivablePart{receivableLinesSQL, " AND l.entry = {:entry}"},
		receivablePart{receivableApplicationsSQL, " AND pa.payment = {:payment}"})
}

// refreshReceivableInvoice stores the rows that read one invoice: the line of
// its entry (its due day) and the applications that pay it (its number and
// due day; an invoice correction moves the applications of the original to
// it before it has its entry).
func refreshReceivableInvoice(tx core.App, invoice *core.Record) error {
	if entryID := invoice.GetString("entry"); entryID != "" {
		if err := refreshReceivableEntry(tx, entryID); err != nil {
			return err
		}
	}
	return refreshReceivables(tx, "item = {:invoice} AND source = '"+receivableSourceApplication+"'", dbx.Params{"invoice": invoice.Id},
		receivablePart{receivableApplicationsSQL, " AND pa.invoice = {:invoice}"})
}

// bindReceivableRefresh binds refresh after each update of collection. Bind it
// after the collection's serialized guard: e.App is then the guard's
// transaction and the record is written.
func bindReceivableRefresh(app core.App, collection string, refresh func(tx core.App, r *core.Record) error) {
	app.OnRecordUpdate(collection).BindFunc(func(e *core.RecordEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		return refresh(e.App, e.Record)
	})
}

// bindReceivableEntryRefresh keeps the rows of an entry's lines: a post
// stamps the lines and numbers the entry (manual entries, documents,
// reversals, void reversals, auto-reverse, opening balances all post through
// this update). A draft has no row.
func bindReceivableEntryRefresh(app core.App) {
	bindReceivableRefresh(app, "books_journal_entries", func(tx core.App, r *core.Record) error {
		if r.GetString("status") == statusDraft {
			return nil
		}
		return refreshReceivableEntry(tx, r.Id)
	})
}

// bindReceivablePaymentRefresh keeps a payment's rows through its post, its
// void and its replacement by a correction (each an update of the payment),
// and through an invoice correction, which moves its applications to the
// correction (an update of the application).
func bindReceivablePaymentRefresh(app core.App) {
	bindReceivableRefresh(app, paymentsCollection, refreshReceivablePayment)
	bindReceivableRefresh(app, "books_payment_applications", func(tx core.App, r *core.Record) error {
		payment, err := tx.FindRecordById(paymentsCollection, r.GetString("payment"))
		if err != nil {
			return err
		}
		return refreshReceivablePayment(tx, payment)
	})
}

// bindReceivableInvoiceRefresh keeps the rows that read an invoice through
// its post: the entry is posted before the invoice row holds its due day and
// its entry.
func bindReceivableInvoiceRefresh(app core.App) {
	bindReceivableRefresh(app, invoiceKind.Collection, func(tx core.App, r *core.Record) error {
		if r.GetString("status") == statusDraft {
			return nil
		}
		return refreshReceivableInvoice(tx, r)
	})
}

// bindReceivableCustomerRefresh settles a customer's rows again after each
// update of the customer: a customer that gets or loses its parent gains or
// loses the item that stays open for good (receivableSettledSQL).
func bindReceivableCustomerRefresh(app core.App) {
	bindReceivableRefresh(app, "books_customers", func(tx core.App, r *core.Record) error {
		if !tx.IsTransactional() {
			return errors.New("books: receivable rows are settled only inside the transaction of the write")
		}
		changed, err := settleReceivables(tx, []string{r.Id})
		if err != nil {
			return err
		}
		if changed > 0 {
			changesignal.Touch(tx, receivableItemsCollection)
		}
		return nil
	})
}

// msgReceivableItemsReadOnly refuses a REST write of a stored receivable row.
const msgReceivableItemsReadOnly = "Books writes the receivable items itself. They cannot be changed."

// registerReceivableWriteGuard refuses every REST write of the stored rows,
// a superuser's too: the collection's rules stop everyone else, and a row a
// person wrote would disagree with the book until the next rebuild.
func registerReceivableWriteGuard(app core.App) {
	refuseWrite := func(e *core.RecordRequestEvent) error { return refuse(msgReceivableItemsReadOnly) }
	app.OnRecordCreateRequest(receivableItemsCollection).BindFunc(refuseWrite)
	app.OnRecordUpdateRequest(receivableItemsCollection).BindFunc(refuseWrite)
	app.OnRecordDeleteRequest(receivableItemsCollection).BindFunc(refuseWrite)
}

// repairReceivables rebuilds the stored rows when they disagree with the book.
func repairReceivables(tx core.App) error {
	drifts, err := verifyReceivables(tx)
	if err != nil || len(drifts) == 0 {
		return err
	}
	return rebuildReceivables(tx)
}
```

- [ ] **Step 4: Bind the hooks**

`server/ledger.go`, in `registerLedger`, directly under the last line `registerAttachments(app)`:

```go
	bindReceivableEntryRefresh(app)
	bindReceivableCustomerRefresh(app)
	registerReceivableWriteGuard(app)
```

`server/payments.go`, in `registerPaymentGuards`, as its last statement (after the closing `})` of `app.OnRecordDelete(applicationsCollection).BindFunc(…)`, before the function's closing `}`):

```go
	bindReceivablePaymentRefresh(app)
```

`server/sales_documents.go`, in `registerSalesDocGuards`, directly under the statement that ends `}, kind.AfterCreate)`:

```go
	if kind.Collection == invoiceKind.Collection {
		bindReceivableInvoiceRefresh(app)
	}
```

- [ ] **Step 5: The rebuild route repairs and reports the stored rows**

`server/balances.go`, in `rebuildBalances`, replace

```go
			fixed = len(drifts) + documents + cash
			return nil
```

with

```go
			fixed = len(drifts) + documents + cash
			return repairReceivables(tx)
```

and extend the function's comment with the sentence: `It also rebuilds the stored receivable rows when they disagree with the book (repairReceivables); those rows are not in the count.`

In `bindBalanceRoutes`, directly above the line `fixed, err := rebuildBalances(re.App)` add:

```go
		// Read before the rebuild repairs them, as the cash-basis rows above.
		receivableDrifts, err := verifyReceivables(re.App)
		if err != nil {
			return err
		}
		if len(receivableDrifts) > 0 {
			log.ErrorContext(re.Request.Context(), "receivable rows disagree with the book", "count", len(receivableDrifts))
		}
```

and in the JSON answer, directly under the line `"cashDrifts": cashListed, "cashDriftCount": cashCount,` add:

```go
			"receivableDrifts": receivableDrifts,
```

- [ ] **Step 6: `requireNoDrift` checks the stored rows**

`server/env_test.go`, in `requireNoDrift`, after the `if len(lines) > 0 { … }` block and before the function's closing `}` add:

```go
	receivables, err := verifyReceivables(app)
	if err != nil {
		t.Fatalf("verify receivables: %v", err)
	}
	if len(receivables) > 0 {
		t.Fatalf("receivable rows drifted from the book: %+v", receivables)
	}
```

and change the function's comment to: `// requireNoDrift fails when a month row disagrees with the ledger, a line's posting stamp disagrees with its entry, or a stored receivable row disagrees with the book.`

- [ ] **Step 7: Run the tests to verify they pass**

Run: `gofmt -l . ; go vet . && go test -run 'TestReceivable|TestRebuildRoute|TestVerifyReceivables|TestRegister_|TestInvoiceCorrection' -count=1 .`
Expected: `gofmt -l` prints nothing; `ok  tinycld.org/packages/books`.

- [ ] **Step 8: Run the package, for every `requireNoDrift` site**

Run: `go test -count=1 . 2>&1 | command grep -E '^(--- FAIL|FAIL|ok)'` (about 3 minutes, foreground)
Expected: one line, `ok  tinycld.org/packages/books`. A `receivable rows drifted from the book` failure names a write path with no refresh: find the write (the drift's `entry` and `item`), and add the refresh at that write inside its transaction. Do not add a rebuild call to a test whose writes go through the hooks. A test that writes ledger, payment, application or invoice rows with raw SQL calls `requireRebuilt(t, app)` before `requireNoDrift` (the planner's run needed none).

- [ ] **Step 9: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add server/receivable_refresh.go server/receivable_refresh_test.go server/ledger.go server/payments.go server/sales_documents.go server/balances.go server/env_test.go
git commit -m "feat(books): receivable rows refresh in the write transaction"
```

---

### Task 3: The report reads the stored table

**Files:**
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_sources.go:251-293`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_ar_aging.go:24-37`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_ar_aging_scale_test.go:211-212, 1159, 1322`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_ar_aging_test.go:1213-1222`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/receivable_items_test.go:743-752`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_sources_test.go:492-523`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/tinycld/books/lib/ar-aging-fixture.ts:700-706`

**Interfaces:**
- Consumes (tasks 1, 2): `receivableItemsCollection`, `rebuildReceivables(tx core.App) error`, the hooks of task 2 (every test book that posts through the guards has current rows).
- Produces: the source `books.receivable_items` is a single-collection source over `books_receivable_items` with one more field, `settled` (`reports.Text`); the A/R Aging filter is `settled > asOf and date <= asOf and customerKey in customers`; a run's topics hold `collection/books_receivable_items` and no topic of a ledger table. The two views and `registerReceivableItems` still exist and nothing reads them through a report (part 2, task S4, drops them).

- [ ] **Step 1: Pin the strict filter, and change the expectations (the failing tests)**

`server/report_ar_aging_test.go`: add this test directly above `func TestARAgingHiddenItems(t *testing.T) {`. It passes before step 3 too (the views give the same rows): it is the pin that the filter of step 4 drops an item only from its settled day on.

```go
// An item paid in full is not read from its settled day on, and is read whole
// on a day before it: the settled filter is strict (settled > As of). A void
// of the payment opens the item again on every day.
func TestARAgingSettledItemOnPastDay(t *testing.T) {
	b := newARBook(t)
	c := mkCustomer(t, b.app, "Settled Co", "")
	inv := arInvoice(t, b, c, "2026-10-01", "2026-10-31", 40000)
	pay := arPayment(t, b, c, "2026-10-05", 40000, map[string]int{inv.Id: 40000})

	before, item := b.openItem(t, map[string]any{"asOf": "2026-10-04"}, c, inv.Id)
	if v, _ := agedCell(t, item, "total"); v != 40000 {
		t.Errorf("as of 2026-10-04 (before the payment) the invoice item = %d, want 40000", v)
	}
	for _, l := range arLinesOf(t, before, item) {
		if strings.Contains(l.Label, "Payment") {
			t.Errorf("as of 2026-10-04 the invoice item has the line %q, want no payment line", l.Label)
		}
	}
	for _, day := range []string{"2026-10-05", "2026-10-06"} {
		res := b.run(t, map[string]any{"asOf": day})
		if rows := cbCustomerRows(res); len(rows) != 0 {
			t.Errorf("as of %s (paid in full) the report has customer rows %q, want none", day, bsLabels(res))
		}
	}

	pay.Set("status", statusVoid)
	if err := b.app.Save(pay); err != nil {
		t.Fatalf("void the payment: %v", err)
	}
	_, item = b.openItem(t, map[string]any{"asOf": "2026-10-06"}, c, inv.Id)
	if v, _ := agedCell(t, item, "total"); v != 40000 {
		t.Errorf("as of 2026-10-06, after the payment's void, the invoice item = %d, want 40000", v)
	}
}

```

`server/report_ar_aging_test.go`, `TestARAgingTopics`: replace the topic list

```go
	for _, topic := range []string{
		"collection/books_journal_lines",
		"collection/books_journal_entries",
		"collection/books_accounts",
		"collection/books_invoices",
		"collection/books_payments",
		"collection/books_payment_applications",
		"collection/books_customers",
		"collection/books_settings",
	} {
```

with

```go
	for _, topic := range []string{
		"collection/books_receivable_items",
		"collection/books_customers",
		"collection/books_settings",
	} {
```

`server/receivable_items_test.go`, `TestReceivableItemsEngine`: replace

```go
	// Core lists a view's base tables, never the view's own topic
	// (core/server/changesignal/topics.go): SQLite reads the base tables.
	for _, topic := range []string{
		"collection/books_journal_lines",
		"collection/books_journal_entries",
		"collection/books_accounts",
		"collection/books_invoices",
		"collection/books_payments",
		"collection/books_payment_applications",
	} {
```

with

```go
	// The run reads the stored table: its topic is the one a refresh touches.
	for _, topic := range []string{"collection/books_receivable_items"} {
```

`server/report_sources_test.go`, the `books.receivable_items` entry of `expectedCatalog`: replace its two comment lines and its key line

```go
			// A union of the two receivable item views (A/R Aging spec §4):
			// each part maps the same keys to the view's own columns.
			key: "books.receivable_items", label: "Receivable items",
```

with

```go
			// The stored receivable items (A/R Aging spec §4).
			key: "books.receivable_items", collection: "books_receivable_items", label: "Receivable items",
```

add, directly under its field line `{key: "item_seq", typ: reports.Number, aggs: reports.MinMax},`:

```go
				{key: "settled", typ: reports.Text},
```

and delete its whole `parts: []wantPart{ … },` block (the two parts `books_receivable_lines` and `books_receivable_applications`).

`server/report_ar_aging_scale_test.go`, three edits:

1. In `writeARDatasetKeys`, replace

```go
		w.finish()
		return nil
```

with (the bulk writer writes past the hooks, so it rebuilds the stored rows in its transaction)

```go
		w.finish()
		return rebuildReceivables(tx)
```

2. Replace the line that starts `arLargeTables  = []string{` with

```go
	arLargeTables  = []string{"books_journal_lines", "books_journal_entries", "books_invoices", "books_payments", "books_payment_applications", "books_receivable_items"}
```

3. In `arCheckRuns`, in the line `if slices.ContainsFunc(own, func(p planStep) bool { return slices.Contains(p.Tables, "books_journal_lines") }) {`, replace `"books_journal_lines"` with `receivableItemsCollection`. Then rename the local `readsLines` to `readsItems` in that function and change the message of its failure from `reads books_journal_lines` to `reads books_receivable_items`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -run 'TestARAgingTopics|TestReceivableItemsEngine|TestReportSourcesCatalog|TestARAgingQueryPlans' -count=1 . 2>&1 | command grep -E '^(--- FAIL|FAIL|ok)|want collection/books_receivable_items|reads books_receivable_items' | cut -c1-220`
Expected: FAIL for the four tests: `topics = […], want collection/books_receivable_items`; the catalog's `collection, label = "", …`; `none of its 6 SELECTs reads books_receivable_items`.

- [ ] **Step 3: The source reads the table**

`server/report_sources.go`: delete the comment and the variable `receivableItemColumns` (lines 251-255), and replace the comment and the head of `receivableItemsSource` down to the `Fields:` line

```go
// receivableItemsSource is every part of every open receivable item (A/R
// Aging spec §4), one union of the two receivable item views (core
// SourcePart): the posted A/R lines, with a payment's own line at its
// unapplied part, and the applications of posted payments. item is the open
// item a row belongs to; a report groups by it to add up an item's open
// amount.
func receivableItemsSource() reports.Source {
	return reports.Source{
		Key: "books.receivable_items", Label: "Receivable items",
		Parts: []reports.SourcePart{
			{Collection: receivableLinesView, Columns: sameNames(receivableItemColumns...)},
			{Collection: receivableApplicationsView, Columns: sameNames(receivableItemColumns...)},
		},
		Fields: []reports.Field{
```

with

```go
// receivableItemsSource is every part of every receivable item (A/R Aging
// spec §4), the stored rows of books_receivable_items: the posted A/R lines,
// with a payment's own line at its unapplied part, and the applications of
// posted payments. item is the item a row belongs to; a report groups by it
// to add up an item's open amount. settled is the day the item's balance
// stays zero from (9999-12-31 while it is open): a report of one day reads
// the rows with settled after that day and never the paid history.
func receivableItemsSource() reports.Source {
	return reports.Source{
		Key: "books.receivable_items", Collection: receivableItemsCollection, Label: "Receivable items",
		Fields: []reports.Field{
```

and add, directly under the field line `{Key: "due_date", Label: "Due date", Type: reports.Date, TextDay: true, Aggregates: reports.MinMax},`:

```go
			// Text, not Date: the engine compares a text field as the bare
			// column, so idx_books_receivable_items_settled serves the filter.
			// A Date field is wrapped in substr and NULLIF, which no index serves.
			{Key: "settled", Label: "Settled", Type: reports.Text},
```

- [ ] **Step 4: The A/R Aging reads only the items open on the day**

`server/report_ar_aging.go`, in `arAgingSpec`, replace

```go
		Filter:  op("and", op("lte", fld("date"), prm("asOf")), op("in", col("customerKey"), prm("customers"))),
```

with

```go
		// settled after the day: an item whose balance is zero on the day and
		// stays zero adds nothing to any sum, so its rows are not read (the
		// first item of a sub-customer is never settled: receivableSettledSQL).
		Filter: op("and",
			op("gt", fld("settled"), prm("asOf")),
			op("lte", fld("date"), prm("asOf")),
			op("in", col("customerKey"), prm("customers")),
		),
```

Keep `Revision: 1` in `arAgingShipped` (the report is unreleased).

- [ ] **Step 5: Run the A/R Aging and source tests**

Run: `gofmt -l . ; go vet . && go test -run 'ARAging|Receivable|ReportSources|TestRebuildRoute' -count=1 . 2>&1 | command grep -E '^(--- FAIL|FAIL|ok)'`
Expected: `gofmt -l` prints nothing; one line `ok  tinycld.org/packages/books`. `TestARAgingFamilies`, `TestARAgingAsOfDay`, `TestARAgingHiddenItems`, `TestARAgingQueryPlans` and `TestARAgingRunsAgreeWithSmallBook` pass with no expected figure, row or bar changed. If `TestARAgingQueryPlans` reports `SCAN books_receivable_items` in a bound run, the `settled` field is not `reports.Text` or the index `idx_books_receivable_items_settled` is missing: fix that, do not change a bar.

- [ ] **Step 6: The fixture's topics**

`tinycld/books/lib/ar-aging-fixture.ts`, in `arAgingResult`, replace

```ts
        topics: [
            'collection/books_receivable_lines',
            'collection/books_receivable_applications',
            'collection/books_customers',
            'collection/books_journal_entries',
            'collection/books_settings',
        ],
```

with

```ts
        topics: [
            'collection/books_receivable_items',
            'collection/books_customers',
            'collection/books_settings',
        ],
```

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books && command grep -rl "ar-aging-fixture" tests tinycld | command grep -m 20 ""` and, for each vitest file it lists under `tests/`, `PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test <file>`; then `PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg typecheck`.
Expected: the tests pass and the typecheck prints no error.

- [ ] **Step 7: Run the package**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go test -count=1 . 2>&1 | command grep -E '^(--- FAIL|FAIL|ok)'` (about 3 minutes, foreground)
Expected: one line, `ok  tinycld.org/packages/books`.

- [ ] **Step 8: The e2e spec (live re-run through the change signal)**

`tests/e2e/books-ar-aging.spec.ts` must pass unchanged: it posts an invoice and a payment through the UI and reads the report again, which now depends on `changesignal.Touch` of `books_receivable_items`. The run takes more than 4 minutes: hand back
`NEEDS-RUN: cd /Users/nas/tmp/tinycld/books-purchases && lsof -iTCP -sTCP:LISTEN -nP | grep -qE ':(7210|7211) ' && { echo PORTS-BUSY; exit 1; }; E2E_PORT=7210 E2E_PORT_2=7211 E2E_IMAP_PORT=1210 E2E_IMAP_PORT_2=1211 pnpm exec tinycld-pkg test:e2e books/tests/e2e/books-ar-aging.spec.ts`
with what is needed from it: every test of the file passes. A failure is fixed at its cause (a missing refresh or touch), never in the spec.

- [ ] **Step 9: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add server/report_sources.go server/report_ar_aging.go server/report_ar_aging_scale_test.go server/report_ar_aging_test.go server/receivable_items_test.go server/report_sources_test.go tinycld/books/lib/ar-aging-fixture.ts
git commit -m "feat(books): A/R Aging reads the stored receivable items open on the day"
```

---

## What part 2 holds

- **S4 The views go.** The migration without the two views and `VIEWS`; `receivableLinesView`, `receivableApplicationsView`, `registerReceivableItems` (also in `register.go:38` and `registrations_test.go:922`), the two OAuth entries and `TestOAuth_ReceivableViews`; `server/receivable_items_test.go` reads the table by `source` (`arRows`, `allRows`, `TestReceivableViewsShape`, `TestReceivableViewsAccess`); `TestReceivableRebuildMatchesViews` of this part compares with the views and is rewritten to literal rows; `report_ar_aging_scale_test.go:1417` and `:1681`; `report_ar_aging_profile_test.go:93-98` (`arProfileFloor`).
- **S5 Spec and record.** `docs/specs/ar-aging-design.md` §3 Live, §4 Data, §8, §9 (the large tables, "When a bar fails"); the ledger. The spec text must say the sub-customer rule and the fifth hook point.
- **S6 L profile and record.** The `NEEDS-RUN` of `TestARAgingProfile` at L and of the scale test (which writes `testdata/ar_aging_stat1.json` again), the numbers in the ledger, and the stop rule.
