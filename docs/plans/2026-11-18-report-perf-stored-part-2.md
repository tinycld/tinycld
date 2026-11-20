# Stored Receivable Items (p5-report-perf, stored part 2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The two receivable views are gone, the spec and the ledger say what the stored table is, and the L profile and scale numbers of the A/R Aging over the stored table are recorded.

**Architecture:** Part 1 (tasks 1 to 3) added the base collection `books_receivable_items`, kept it current in each write transaction, and pointed the report at it. This part removes what the table replaced (the two view collections, their change-signal registration, their OAuth entries), moves every test that read a view to the stored rows by `source`, writes the record, and measures at L. No behavior of the report changes in this part.

**Tech Stack:** Go (PocketBase fork, dbx), PocketBase JS migration, core `reports` and `changesignal`, Markdown.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/stored-receivables-design.md` (approved 2026-11-18), `/Users/nas/tmp/tinycld/books-purchases/books/docs/specs/ar-aging-design.md` (§3, §4, §8, §9), and the family-header check `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/family-header-check.md`. Outline: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/plan-outline-stored.md`. Part 1: `/Users/nas/tmp/tinycld/books-purchases/tinycld/docs/plans/2026-11-18-report-perf-stored.md`.

**This part:** tasks 4 to 6 of the outline (S4 to S6). They run after tasks 1 to 3 of part 1 are committed in `books/`. The engine tasks 3, 11 and 4 of `2026-11-18-report-perf-part2.md` and `-part3.md` wait for the L numbers of task 6.

**Repos:** workspace `/Users/nas/tmp/tinycld/books-purchases`; every change is in `books/` (branch `books/p5-report-perf`). No core change. All Go commands run in `/Users/nas/tmp/tinycld/books-purchases/books/server`.

**Human decisions (2026-11-19), written into the spec by task 5:**
- (a) ACCEPTED: `settled` is a `Text` field of the source `books.receivable_items` (not `Date`).
- (b) ACCEPTED: `settled` is per customer and item.
- (c) REJECTED: no rule "the first item of a sub-customer is never settled" and no hook on `books_customers`. The family header (D7) holds exactly through the report: in `arGroups` the family level's `Inline` is a lookup over the to-many link `receivable_items` of `books.customers`, with an `isEmpty` guard for No customer (part 1, task 3).

## Global Constraints

- The bars of spec §9 are never changed, no run is removed, the dataset is not changed.
- Books only. Nothing under `/Users/nas/tmp/tinycld/books-purchases/tinycld` changes. If a step seems to need a core change, stop and report it.
- Migration `2100000026_create_books_receivable_items.js` is unreleased: edit it in place. No new migration file, no backfill, no reader of the old views.
- The definition of a receivable row stays in one place: `receivableLinesSQL` and `receivableApplicationsSQL` in `server/receivable_items.go`. After task 4 the migration holds no copy of that SQL.
- Existing A/R Aging tests keep their expected figures and rows. Only the tests this plan names change.
- No word "hosting", "saas", "tenant", "router" in Books.
- Code starts from a failing test that ran RED. Undo an edit with the Edit tool, never `git checkout`/`git restore`. Read `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/agent-rules.md` first.
- A run of more than about 4 minutes is a `NEEDS-RUN` for the driver (agent rules); task 6 names two.

## Interfaces from part 1 (already in the tree when this part starts)

- `server/receivable_items.go`: consts `receivableItemsCollection = "books_receivable_items"`, `receivableSourceLine = "line"`, `receivableSourceApplication = "application"`, `receivableOpen = "9999-12-31"`, `receivableColumns`, `receivableLinesSQL`, `receivableApplicationsSQL`, `receivableSettledSQL`; `insertReceivables`, `settleReceivables(tx core.App, customers []string) (int64, error)`, `settleReceivablesSQL(where string) string`, `rebuildReceivables(tx core.App) error`, `verifyReceivables(app core.App) ([]ReceivableDrift, error)`. Until task 4 it also holds `receivableLinesView`, `receivableApplicationsView` and `registerReceivableItems()`.
- `server/receivable_refresh.go`: the four hook binders and `registerReceivableWriteGuard`, `repairReceivables`, `msgReceivableItemsReadOnly`.
- `server/receivable_stored_test.go`: `type storedRow struct{ ID, Source, Customer, Entry, Date string; Debit, Credit int; Item, Kind, Number string; Seq int; Due, Currency, Settled string }`, `storedRows(t testing.TB, app core.App) []storedRow`, `requireRebuilt(t testing.TB, app core.App)`, `requireNoReceivableDrift(t testing.TB, app core.App)`, `settledOf(t testing.TB, app core.App, item string) string`.
- `server/receivable_items_test.go`: `newARBook`, `arInvoice`, `arPayment`, `arCreditMemo` (each returns the posted `*core.Record`), `arJournalPrefix`, `arLineOf(t, b, entryID) *core.Record`, `type arRow`, `arRows(t *testing.T, app core.App, view string) []arRow`, `allRows`.
- The report source `books.receivable_items` reads `books_receivable_items`; `writeARDatasetKeys` (the bulk writer of the scale test) ends in `rebuildReceivables(tx)`.

## Facts the planner measured (do not find again)

Tasks 1 to 4 were built from the two plan files into a `go test -overlay` copy of `books/server` and run:

- After task 4 the lines query of a whole-book rebuild and of the verify still starts at the accounts: `SEARCH a USING INDEX idx_books_accounts_type (type=?)`, then `SEARCH l USING INDEX idx_books_jlines_account_date (account=?)`. The refresh of one entry starts at the entry (`idx_books_jlines_entry`). So the migration keeps `idx_books_accounts_type`.
- `server/testdata/ar_aging_stat1.json` holds `[]`: the "L statistics" reading of `TestARAgingQueryPlans` equals "no statistics" until the scale run of task 6 writes the file.
- `help/reports.md` on this branch has no A/R Aging section and names no view: task 5 changes no help file.
- `app.FindAllRecords(receivableItemsCollection, dbx.HashExp{"source": source})` reads the rows of one source; the test helper `arRows` keeps its signature, so its 14 call sites change one word each.

## Review Focus

1. A database that applied the migration before this edit keeps the two views and has no table (PocketBase never runs an applied migration again). A person who runs the lane's dev server after task 4 sees report errors until the database is made new. Expected: every test app and the e2e run build their database from the migrations; the implementer's report says that a dev database must be reset. Pinned by `TestReceivableItemsShape` (task 4): a new database has the table and no view.
2. An OAuth token with `books:read` reads the stored rows and has no write scope; the removed views must not stay in the scope table (core refuses an entry for a collection that does not exist at start-up). Test: `TestOAuth_ReceivableItems` (task 4).
3. A user with Books level none lists the table over REST with a `source` filter: zero rows, not an error and not the rows. Test: `TestReceivableItemsAccess` (task 4).
4. A book written past the hooks (the scale dataset) must give the same stored rows as the posters, for each source. Test: `TestARAgingBulkWriterMatchesPosters` reads the table by `source` (task 4).
5. A refused L run (core's 10 s limit) ends the L sub-test, and the scale test then still writes its three files: `ar_aging_stat1.json` holds `null` and names no table. The plan check under L statistics must then not be reported as done, and a plan bar that fails inside the scale run must not be reported as "the bars are met". Task 6, step 3 checks that the new statistics file names `books_receivable_items` before it is copied; step 5 gives "meets the bars" only with `--- PASS: TestARAgingScale`.

---

### Task 4: The views go

**Files:**
- Modify (replace whole): `/Users/nas/tmp/tinycld/books-purchases/books/pb-migrations/2100000026_create_books_receivable_items.js`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/receivable_items.go` (the head of the file)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/register.go:38`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/oauth_scopes.go:86-87`
- Test: `/Users/nas/tmp/tinycld/books-purchases/books/server/receivable_items_test.go`
- Test: `/Users/nas/tmp/tinycld/books-purchases/books/server/receivable_stored_test.go`
- Test: `/Users/nas/tmp/tinycld/books-purchases/books/server/oauth_scopes_test.go:223-247`
- Test: `/Users/nas/tmp/tinycld/books-purchases/books/server/registrations_test.go:922`
- Test: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_ar_aging_scale_test.go:1417, 1680-1683`
- Test: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_ar_aging_profile_test.go:91-98`

**Interfaces:**
- Consumes: the block "Interfaces from part 1" above.
- Produces:
  - no collection `books_receivable_lines` or `books_receivable_applications`; no `receivableLinesView`, `receivableApplicationsView`, `registerReceivableItems`;
  - `arRows(t *testing.T, app core.App, source string) []arRow` reads the stored rows of one source (`receivableSourceLine` or `receivableSourceApplication`); `allRows` reads both;
  - `arProfileFloor` holds three reads of `books_receivable_items` (task 6 reads their `FLOOR` lines): every row, every row grouped by customer and item, the open rows grouped by customer and item.

- [ ] **Step 1: Move the tests to the stored table (the failing tests)**

`server/receivable_items_test.go`, seven edits.

1. Replace the header comment and the variable `receivableSources`

```go
// The two receivable item views (A/R Aging spec §4 Data): one row per posted
// Accounts receivable line (books_receivable_lines) and one per application of
// a posted payment (books_receivable_applications), each given to its open
// item. Rows are read as a superuser by the view's name, as the report engine
// reads them, unless a test says it reads them as a reader.

// receivableSources are the six collections both views read (spec §4).
var receivableSources = []string{
	"books_accounts", "books_invoices", "books_journal_entries", "books_journal_lines",
	"books_payment_applications", "books_payments",
}
```

with

```go
// The stored receivable items (A/R Aging spec §4 Data): one row per posted
// Accounts receivable line (source line) and one per application of a posted
// payment (source application), each given to its item. Rows are read as a
// superuser by their source, unless a test says it reads them as a reader.
```

2. Replace the head of `arRows`

```go
func arRows(t *testing.T, app core.App, view string) []arRow {
	t.Helper()
	records, err := app.FindAllRecords(view)
	if err != nil {
		t.Fatalf("read %s: %v", view, err)
	}
```

with

```go
// arRows are the stored rows of one source (receivableSourceLine or
// receivableSourceApplication).
func arRows(t *testing.T, app core.App, source string) []arRow {
	t.Helper()
	records, err := app.FindAllRecords(receivableItemsCollection, dbx.HashExp{"source": source})
	if err != nil {
		t.Fatalf("read the %s rows: %v", source, err)
	}
```

and change the comment `// arRow is one row of either view.` to `// arRow is one stored receivable row.`

3. Replace `allRows`

```go
// allRows are the rows of both views.
func allRows(t *testing.T, app core.App) []arRow {
	t.Helper()
	return append(arRows(t, app, receivableLinesView), arRows(t, app, receivableApplicationsView)...)
}
```

with

```go
// allRows are the stored rows of both sources.
func allRows(t *testing.T, app core.App) []arRow {
	t.Helper()
	return append(arRows(t, app, receivableSourceLine), arRows(t, app, receivableSourceApplication)...)
}
```

4. Replace the whole function `TestReceivableViewsShape` (from `func TestReceivableViewsShape(t *testing.T) {` to its closing `}`) with:

```go
func TestReceivableItemsShape(t *testing.T) {
	b := newARBook(t)
	col, err := b.app.FindCollectionByNameOrId(receivableItemsCollection)
	if err != nil {
		t.Fatalf("find %s: %v", receivableItemsCollection, err)
	}
	if col.Type != core.CollectionTypeBase {
		t.Errorf("type = %q, want base", col.Type)
	}
	for rule, got := range map[string]*string{"list": col.ListRule, "view": col.ViewRule} {
		if got == nil || *got != booksReadRule {
			t.Errorf("%s rule = %v, want the Books READ rule", rule, got)
		}
	}
	for rule, got := range map[string]*string{"create": col.CreateRule, "update": col.UpdateRule, "delete": col.DeleteRule} {
		if got != nil {
			t.Errorf("%s rule = %q, want nil (only Books writes the rows)", rule, *got)
		}
	}
	want := map[string]string{
		"source":   core.FieldTypeSelect,
		"customer": core.FieldTypeRelation, "entry": core.FieldTypeRelation,
		"debit": core.FieldTypeNumber, "credit": core.FieldTypeNumber,
		"date": core.FieldTypeText, "currency": core.FieldTypeText, "item": core.FieldTypeText,
		"item_kind": core.FieldTypeText, "item_number": core.FieldTypeText, "due_date": core.FieldTypeText,
		"item_seq": core.FieldTypeNumber, "settled": core.FieldTypeText,
	}
	for field, typ := range want {
		f := col.Fields.GetByName(field)
		if f == nil {
			t.Errorf("no %s field", field)
			continue
		}
		if f.Type() != typ {
			t.Errorf("%s type = %q, want %q", field, f.Type(), typ)
		}
	}
	indexes := strings.Join(col.Indexes, "\n")
	for _, index := range []string{
		"idx_books_receivable_items_entry", "idx_books_receivable_items_item",
		"idx_books_receivable_items_customer", "idx_books_receivable_items_settled",
	} {
		if !strings.Contains(indexes, index) {
			t.Errorf("no index %s; indexes:\n%s", index, indexes)
		}
	}

	// The table replaced the two views: a new database has neither, and
	// Register declares neither to the change signal.
	gone := []string{"books_receivable_lines", "books_receivable_applications"}
	for _, name := range gone {
		if _, err := b.app.FindCollectionByNameOrId(name); err == nil {
			t.Errorf("the view %s still exists", name)
		}
	}
	changesignal.ResetForTesting()
	app, _ := newRegisteredApp(t)
	if err := changesignal.CheckViews(app); err != nil {
		t.Fatalf("CheckViews on an app wired by Register: %v", err)
	}
	for _, name := range gone {
		if sources, ok := changesignal.RegisteredViews()[name]; ok {
			t.Errorf("Register still registers the view %s over %v", name, sources)
		}
	}
}
```

5. Replace the whole function `TestReceivableViewsAccess` (from `func TestReceivableViewsAccess(t *testing.T) {` to its closing `}`) with:

```go
func TestReceivableItemsAccess(t *testing.T) {
	b := newARBook(t)
	inv := arInvoice(t, b, b.docs.customer, "2026-08-04", "2026-09-03", 100000)
	arPayment(t, b, b.docs.customer, "2026-09-10", 70000, map[string]int{inv.Id: 55000})

	rows := "/api/collections/" + receivableItemsCollection + "/records"
	lines := rows + "?filter=(source='" + receivableSourceLine + "')"
	apps := rows + "?filter=(source='" + receivableSourceApplication + "')"
	reader, none := tokenFor(t, b.reader), tokenFor(t, b.none)
	apiSeq(t, b.app,
		apiReq{method: http.MethodGet, url: lines, token: reader, want: http.StatusOK, content: []string{`"totalItems":2`}},
		apiReq{method: http.MethodGet, url: apps, token: reader, want: http.StatusOK, content: []string{`"totalItems":1`}},
		apiReq{method: http.MethodGet, url: lines, token: none, want: http.StatusOK, content: []string{`"totalItems":0`}},
		apiReq{method: http.MethodGet, url: apps, token: none, want: http.StatusOK, content: []string{`"totalItems":0`}},
		apiReq{method: http.MethodGet, url: rows, token: none, want: http.StatusOK, content: []string{`"totalItems":0`}},
	)
}
```

6. In the rest of the file, replace each `receivableLinesView` with `receivableSourceLine` (10 places, each inside an `arRows(…)` call) and each `receivableApplicationsView` with `receivableSourceApplication` (4 places). Check: `command grep -c "receivableLinesView\|receivableApplicationsView" receivable_items_test.go` prints `0`.

7. Replace the comment of `arEngineSpec`

```go
// arEngineSpec is the two engine pins of spec §4 Engine reach: a union over two
// view collections, grouped on a text column, with a group label that reads
// min of two text fields.
```

with

```go
// arEngineSpec is the engine pin of spec §4 Engine reach: the stored rows
// grouped on a text column, with a group label that reads min of two text
// fields.
```

`server/receivable_stored_test.go`: replace the comment and the whole function `TestReceivableRebuildMatchesViews` (from `// The rebuild stores exactly the rows of the two views.` to the function's closing `}`) with the test below, and delete `"sort"` from the file's imports (nothing else in the file uses it).

```go
// The hooks and the rebuild store the same rows: one per Accounts receivable
// line and one per application of a posted payment, each given to its item.
func TestReceivableRebuildStoresRows(t *testing.T) {
	b := newARBook(t)
	c := b.docs.customer
	inv := arInvoice(t, b, c, "2026-08-04", "2026-09-03", 100000)
	pay := arPayment(t, b, c, "2026-09-10", 70000, map[string]int{inv.Id: 55000})
	memo := arCreditMemo(t, b, c, "2026-10-20", 63500)
	arJournalPrefix(t, b)
	je := postJournal(t, b.app, "", "2026-09-01", "A/R adjustment",
		jnLine{account: b.receivable, debit: 12000, position: 0},
		jnLine{account: b.income, credit: 12000, position: 1})
	line := arLineOf(t, b, je.Id)

	type row struct {
		Customer, Entry, Date string
		Debit, Credit         int
		Kind, Due, Settled    string
	}
	// By source and item: every item here is open.
	want := map[string]row{
		"line " + inv.Id:        {c.Id, inv.GetString("entry"), "2026-08-04", 100000, 0, "invoice", "2026-09-03", receivableOpen},
		"application " + inv.Id: {c.Id, pay.GetString("entry"), "2026-09-10", 0, 55000, "invoice", "2026-09-03", receivableOpen},
		"line " + pay.Id:        {c.Id, pay.GetString("entry"), "2026-09-10", 0, 15000, "payment", "2026-09-10", receivableOpen},
		"line " + memo.Id:       {c.Id, memo.GetString("entry"), "2026-10-20", 0, 63500, "credit_memo", "2026-10-20", receivableOpen},
		"line " + line.Id:       {"", je.Id, "2026-09-01", 12000, 0, "entry", "2026-09-01", receivableOpen},
	}
	check := func(when string) {
		t.Helper()
		got := storedRows(t, b.app)
		if len(got) != len(want) {
			t.Fatalf("%s: stored %d rows, want %d: %+v", when, len(got), len(want), got)
		}
		for _, g := range got {
			key := g.Source + " " + g.Item
			w, ok := want[key]
			if !ok {
				t.Errorf("%s: a stored row of %q, want one of the five items: %+v", when, key, g)
				continue
			}
			if (row{g.Customer, g.Entry, g.Date, g.Debit, g.Credit, g.Kind, g.Due, g.Settled}) != w {
				t.Errorf("%s: the row of %q = %+v, want %+v", when, key, g, w)
			}
			if g.Number == "" || g.Seq == 0 || g.Currency == "" {
				t.Errorf("%s: the row of %q has no number, sequence or currency: %+v", when, key, g)
			}
		}
	}
	check("after the posts")
	requireRebuilt(t, b.app)
	check("after a rebuild")
}
```

`server/oauth_scopes_test.go`: replace the comment and the whole function `TestOAuth_ReceivableViews` (from `// A/R Aging spec §4: a token only reads the two receivable item views, which` to the function's closing `}`) with:

```go
// A/R Aging spec §4: a token only reads the stored receivable items, which
// only Books writes. The two views they replaced have no entry.
func TestOAuth_ReceivableItems(t *testing.T) {
	collections := oauthPackage().Collections
	got, ok := collections[receivableItemsCollection]
	if !ok {
		t.Fatalf("%s has no OAuth entry", receivableItemsCollection)
	}
	if !reflect.DeepEqual(got.Read, []string{scopeRead}) || len(got.Write) != 0 {
		t.Errorf("%s OAuth access = %+v, want Read [%s] and no Write", receivableItemsCollection, got, scopeRead)
	}
	for _, gone := range []string{"books_receivable_lines", "books_receivable_applications"} {
		if _, ok := collections[gone]; ok {
			t.Errorf("%s still has an OAuth entry; the view is gone", gone)
		}
	}

	t.Cleanup(oauth.ResetRegistry)
	oauth.RegisterPackage(oauthPackage())
	path := "/api/collections/" + receivableItemsCollection + "/records"
	if !oauth.ScopeForRoute(http.MethodGet, path).SatisfiedBy([]string{scopeRead}) {
		t.Errorf("GET %s: books:read must read", path)
	}
	if oauth.ScopeForRoute(http.MethodPatch, path+"/x").SatisfiedBy([]string{scopeRead}) {
		t.Errorf("PATCH %s/x: books:read must not write", path)
	}
}
```

`server/report_ar_aging_scale_test.go`, two edits.

1. In `TestARAgingBulkWriterMatchesPosters`, replace the line

```go
	for _, view := range []string{receivableLinesView, receivableApplicationsView} {
```

with

```go
	// The stored rows of each source: the bulk writer's rebuild against the
	// posters' hooks.
	for _, view := range []string{receivableSourceLine, receivableSourceApplication} {
```

2. In the function that counts the dataset (`openItems:`), replace

```go
		openItems: count("SELECT COUNT(*) FROM (SELECT item FROM (" +
			"SELECT item, date, debit, credit FROM books_receivable_lines UNION ALL " +
			"SELECT item, date, debit, credit FROM books_receivable_applications) " +
			"WHERE date <= {:day} GROUP BY item HAVING SUM(debit - credit) != 0)"),
```

with

```go
		openItems: count("SELECT COUNT(*) FROM (SELECT item FROM books_receivable_items " +
			"WHERE date <= {:day} GROUP BY item HAVING SUM(debit - credit) != 0)"),
```

`server/report_ar_aging_profile_test.go`: replace the comment and the variable `arProfileFloor` (lines 91-98, from `// arProfileFloor are reads of the two views with no report over them: the` to the closing `}` of the list) with:

```go
// arProfileFloor are reads of the stored receivable rows with no report over
// them: the least a run can cost. The first two read the whole table; the
// last reads only the rows of the items that are open (settled 9999-12-31),
// which is what a run of the last day reads.
var arProfileFloor = []string{
	"SELECT count(*), sum(debit), sum(credit), sum(length(item)), sum(length(due_date)), sum(length(item_number)), sum(item_seq) FROM books_receivable_items",
	"SELECT count(*) FROM (SELECT customer, item, sum(debit - credit) FROM books_receivable_items GROUP BY customer, item)",
	"SELECT count(*) FROM (SELECT customer, item, sum(debit - credit) FROM books_receivable_items WHERE settled = '" + receivableOpen + "' GROUP BY customer, item)",
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go vet . && go test -run 'TestReceivableItemsShape|TestOAuth_ReceivableItems|TestReceivableItemsAccess|TestReceivableRebuildStoresRows|TestARAgingBulkWriterMatchesPosters' -count=1 . 2>&1 | command grep -E '^(--- FAIL|FAIL|ok)|still' | cut -c1-200`
Expected: `go vet` prints nothing. FAIL for two tests: `TestReceivableItemsShape` with `the view books_receivable_lines still exists`, `the view books_receivable_applications still exists` and `Register still registers the view …`; `TestOAuth_ReceivableItems` with `books_receivable_lines still has an OAuth entry; the view is gone`. `TestReceivableItemsAccess`, `TestReceivableRebuildStoresRows` and `TestARAgingBulkWriterMatchesPosters` pass here: the table is current since task 2 (they pin that the stored rows are what the views gave).

- [ ] **Step 3: The migration creates only the table**

Replace the whole of `pb-migrations/2100000026_create_books_receivable_items.js` with:

```js
/// <reference path="../../tinycld/server/pb_data/types.d.ts" />
// Books receivable items: the rows of the A/R Aging report, stored. Each row
// is a part of one item (`item`): the invoice, the payment's unapplied part,
// the credit memo, or the manual journal line it belongs to. Debit less credit
// is the row's share of the item in integer cents, so a credit is negative.
// item_number is the item's entry number as shown ("JE-10"); item_seq is the
// integer it formats (server/numbering.go), so items sort by number as an
// integer, not as text.
//
// `source` says what a row is:
// - line: one row per posted journal line on an accounts-receivable account.
//   A reversal's line belongs to the item of the entry it reverses and ages
//   from that item's due day. On the A/R line of a posted payment's own
//   entry, credit is the payment's credited (its unapplied part).
// - application: one row per application of a posted payment, a credit on the
//   invoice it pays.
//
// Together the rows of a customer add up to its books_party_balances row: a
// payment's A/R line is split into its applications and its unapplied part.
//
// `settled` is the day the balance of the row's customer and item becomes
// zero and stays zero, 9999-12-31 while it is open. A report of one day reads
// the rows with settled after that day, so it never reads the paid history.
//
// Only Books writes the rows. The two queries in server/receivable_items.go
// are the one definition of a row; server/receivable_refresh.go runs them
// inside the transaction of each write that changes a row and touches core's
// change signal. Only the report engine reads the rows (the
// books.receivable_items source); no client collection backs them. No
// backfill: a new install has an empty ledger.
//
// Indexes. entry and item find the rows a refresh replaces. (customer, item,
// date) serves the settle of one customer and the report's family lookup.
// (settled, date) serves the report's filter. The same migration indexes
// books_accounts on type: the whole-book rebuild and the verify find the
// accounts-receivable accounts by it and then the lines by account
// (idx_books_jlines_account_date), so the ledger is never scanned whole.
//
// AUTHORIZATION. Every member with read access lists and views the rows.
// Nobody creates, updates or deletes a row over REST: the rules are null, and
// server/receivable_refresh.go refuses a superuser's write too.
//
// The rule constants repeat 2100000012's: a migration cannot import another.
const AUTH = '@request.auth.id != "" && @request.auth.disabled != true'
const ME = '@collection.books_access:me.user ?= @request.auth.id'
const READ = `${AUTH} && ${ME} && @collection.books_access:me.can_read ?= true`

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

const ACCOUNT_TYPE_INDEX = 'CREATE INDEX idx_books_accounts_type ON books_accounts (type)'

migrate(
    app => {
        const accounts = app.findCollectionByNameOrId('books_accounts')
        accounts.indexes.push(ACCOUNT_TYPE_INDEX)
        app.save(accounts)
        app.save(itemsCollection(app))
    },
    app => {
        app.delete(app.findCollectionByNameOrId(ITEMS))
        const accounts = app.findCollectionByNameOrId('books_accounts')
        accounts.indexes = accounts.indexes.filter(index => !index.includes('idx_books_accounts_type'))
        app.save(accounts)
    }
)
```

- [ ] **Step 4: Remove the views' Go parts**

`server/receivable_items.go`: delete the first `const` block and `registerReceivableItems` with its comment, that is this whole text (the `import` block and the comment of `receivableItemsCollection` stay; `changesignal` is still used by `rebuildReceivables`):

```go
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

```

`server/register.go`: delete the line `registerReceivableItems()` (line 38, under `registerPartyBalances()`).

`server/registrations_test.go`: delete the line `registerReceivableItems()` (line 922, under `registerPartyBalances()` in the hand-bound list of `TestRegister_BindsEachPartOnce`).

`server/oauth_scopes.go`: delete the two lines

```go
			"books_receivable_lines":           ro,
			"books_receivable_applications":    ro,
```

(the line `"books_receivable_items":           ro,` of part 1 stays).

- [ ] **Step 5: Run the tests to verify they pass**

Run: `gofmt -l . ; go vet . && go test -run 'Receivable|ARAging|ReportSources|CustomerBalance|TestOAuth|TestRegister_|TestAudit_Covers|TestRebuildRoute' -count=1 . 2>&1 | command grep -E '^(--- FAIL|FAIL|ok)'`
Expected: `gofmt -l` prints nothing; one line `ok  tinycld.org/packages/books`.

Then: `command grep -rn "receivableLinesView\|receivableApplicationsView\|registerReceivableItems\|books_receivable_lines\|books_receivable_applications" /Users/nas/tmp/tinycld/books-purchases/books/server /Users/nas/tmp/tinycld/books-purchases/books/pb-migrations /Users/nas/tmp/tinycld/books-purchases/books/tinycld | cut -c1-160`
Expected: exactly two lines, both in tests that say the views are gone: the `gone` list of `TestReceivableItemsShape` in `server/receivable_items_test.go` and the `gone` list of `TestOAuth_ReceivableItems` in `server/oauth_scopes_test.go`. Remove any other use.

- [ ] **Step 6: Run the package**

Run: `go test -count=1 . 2>&1 | command grep -E '^(--- FAIL|FAIL|ok)'` (about 3 minutes, foreground)
Expected: one line, `ok  tinycld.org/packages/books`.

- [ ] **Step 7: Commit**

In the report, say: a dev or e2e database made before this commit still has the two views and no change to them will come (the migration is edited in place); it must be made new. The e2e command builds its own.

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add pb-migrations/2100000026_create_books_receivable_items.js server/receivable_items.go server/register.go server/oauth_scopes.go server/receivable_items_test.go server/receivable_stored_test.go server/oauth_scopes_test.go server/registrations_test.go server/report_ar_aging_scale_test.go server/report_ar_aging_profile_test.go
git commit -m "refactor(books): drop the receivable views, the stored table replaces them"
```

---

### Task 5: Spec and record

**Files:**
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/docs/specs/ar-aging-design.md` (§3 lines 28-29 and 42; §4 lines 51-58, 63, 66, 71, 77, 79; §8 line 112; §9 lines 119, 137, 158, 168, 170)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/ar-aging-ledger.md` (section Performance, after the Baseline sub-section)

**Interfaces:**
- Consumes: the code of tasks 1 to 4 as committed (names in "Interfaces from part 1").
- Produces: the spec text that task 6 and the engine tasks argue from: the large tables of §9 include `books_receivable_items`; "When a bar fails" names the stored table's queries; the ledger has the sub-section `### Stored receivable items (p5-report-perf)` that task 6 adds its numbers to.

Each edit below replaces one whole bullet or paragraph of the spec, found by its first words. Use the Edit tool; a line number is where the text is before this task.

- [ ] **Step 1: §3 Behavior**

Replace the two sub-bullets of **Rows (Q8)** that start `- A family whose rows name no sub-customer draws no family row` (line 28) and `- A family with a sub-customer draws a header row` (line 29) with:

```markdown
  - A family draws a header row with the parent's name, open when the report opens, one row per customer of the family that has an open item (the parent's own row first, then sub-customers by name), then "Total <parent name>", when a sub-customer of the family that is in the Customer pick has a receivable row dated on or before the As of day. The row counts when its item is paid in full too: a family whose only sub-customer has no open item keeps its header, the parent's row and its total.
  - Every other family draws no family row and no family total: its one customer row is at depth 0 (core `groups[].inline`). The No customer row is always drawn so.
```

Replace the bullet that starts `- **Live.**` (line 42) with:

```markdown
- **Live.** A post or a void of an invoice, a payment, a credit memo or a journal entry, an invoice or payment correction, a customer rename or re-parent, or a Books settings change re-runs an open report. The run's topics are `collection/books_receivable_items`, `collection/books_customers` and `collection/books_settings`. Books refreshes the stored receivable rows inside the transaction of each write that changes one and touches the table's change signal there (core `changesignal.Touch`), so the signal fires after the commit and never for a write that is rolled back. An account's type cannot change while a line uses the account, so it moves no row.
```

- [ ] **Step 2: §4 Data, the collection**

Replace the text from the paragraph that starts `One new migration, ` (line 51) through the paragraph that starts `One new report source, a core union source` (line 58), that is five blocks (the migration paragraph, the two view bullets, the paragraph `Each is one SELECT with joins`, the source paragraph), with:

```markdown
One new migration, `pb-migrations/<number>_create_books_receivable_items.js`. Its number is the next free number in `pb-migrations/` when the checkpoint that adds it commits (2100000026 on 2026-11-18). Unit p3-vendor-pages (another lane) may also add 2100000026: when that unit merges first, the integration renames this file to the next free number; nothing else names the number. An index that §9 needs is created in this same migration. It creates one base collection, `books_receivable_items`, with the Books read rule (`books_access` `can_read`) on list and view, and no create, update or delete rule. Only Books writes it; a REST write is refused, a superuser's too.

The stored table (p5-report-perf, human 2026-11-18) replaced two view collections that computed the same rows on every read. The views were too slow at the L size of §9: a run read every receivable row of the book's history.

Each row is a part of one item. Columns: `source` (select: `line` or `application`), `customer`, `entry`, `date`, `currency`, `debit`, `credit`, `item` (text), `item_kind` (text: `invoice`, `payment`, `credit_memo`, else `entry`), `item_number` (text: the item's entry number), `item_seq` (integer: the item's entry `sequence`, the number that `item_number` formats in `server/numbering.go`), `due_date` (text day), `settled` (text day).

- `source = line` — one row per posted journal line on an `accounts_receivable` account: `id` is the line's id, `item` the document id of the line's entry, else of the entry it reverses, else the line's id; `due_date` the invoice's due date for an invoice item, else the date of the item's first entry, so a reversal ages with what it reverses. On the Accounts receivable line of a posted payment's own entry, `credit` is the payment's `credited` (the unapplied part), not the whole amount.
- `source = application` — one row per `books_payment_applications` row of a posted payment: `id` is the application's id, `customer` and `currency` the payment's, `entry` the payment's entry, `date` the payment's date, `debit` 0, `credit` the application's amount, `item` the invoice id, `item_kind` `invoice`, `item_number` and `item_seq` the invoice entry's, `due_date` the invoice's.
- `settled` — the day the balance of the row's customer and item becomes zero and stays zero; `9999-12-31` while the item is open. It is computed per customer and item (the report's item level is under its customer level), so an item whose rows name two customers is not dropped while one customer's part is open. A payment's void opens its invoice again: zero for a time is not settled. `settled` has no rule for sub-customers, and a customer change moves no `settled` day.

Indexes: `(entry)`, `(item)`, `(customer, item, date)`, `(settled, date)`; and `books_accounts (type)`, which the whole-book queries start from.

The definition of a row is two SQL constants in `server/receivable_items.go` (`receivableLinesSQL`, `receivableApplicationsSQL`); the migration holds no SQL of a row. Three things read them:

- **Refresh** (`server/receivable_refresh.go`), after the record write and inside its transaction, at four hook points: an update of a journal entry that is not a draft (a post of any kind: manual entries, documents, reversals, void reversals, auto-reverse, opening balances); an update of a payment (post, void, replacement by a correction); an update of a payment application (an invoice correction moves it to the correction); an update of an invoice that is not a draft (its due day and its entry reach the rows). A refresh deletes the stored rows of the entry, payment or invoice, stores the rows the queries give, computes `settled` again for each customer that lost or gained a row, and touches the change signal. It refuses to run outside a transaction. There is no hook on customers.
- **Rebuild** (`rebuildReceivables`): every row of the book again, for a book written past the hooks. `POST /api/books/balances/rebuild` runs it when the rows drifted and answers the drifted rows as `receivableDrifts`.
- **Verify** (`verifyReceivables`): the stored rows compared with the queries, both ways, and each `settled` with its computed day. The Go test helper `requireNoDrift` runs it, so every posting test checks the stored rows.

One report source, `books.receivable_items` (`server/report_sources.go`), over the collection `books_receivable_items`: fields `date` and `due_date` (`Date`, `TextDay`, `MinMax`), `debit` and `credit` (`Money`, `SumAll`, `Currency: "currency"`), `currency`, `item` (`Text`), `item_kind` (`Select`: Invoice, Payment, Credit memo, Journal entry), `item_number` (`Text`, `MinMax`), `item_seq` (`Number`, `MinMax`: items sort by it, since "JE-10" sorts before "JE-9" as text), `settled` (`Text`); links `customer` → `books.customers` and `entry` → `books.journal_entries` (to-one); `Heading`, `FiscalStartMonth`, `Currency` and `Watch` as `books.journal_lines`. `settled` is a `Text` field, not a `Date` field: core compares a text field as the bare column, which the index `(settled, date)` serves, and wraps a date field in `substr` and `NULLIF`, which no index serves. A customized copy of the report can show the column; an open item shows `9999-12-31`. The source `books.customers` gains the to-many link `receivable_items` → `books.receivable_items` (`BackField: "customer"`), which the family level reads.
```

- [ ] **Step 3: §4 Data, the report spec, engine reach, files, tests**

Replace the bullet `- filter: ` (line 63) with:

```markdown
- filter: `settled > asOf` and `date <= asOf` and `customerKey in customers`. An item whose balance is zero on the day and stays zero adds nothing to any sum, so its rows are not read; an item paid after the day is read whole;
```

In the bullet `- groups:` (line 66), replace the words `` `inline` when the family has no row whose customer has a parent) `` with:

```markdown
`inline` when the family key is blank (No customer) or a lookup counts 0: the count of `books.customers` rows whose `parent` is the family's record, that are in `customers`, and that have a `receivable_items` row with `date <= asOf` (core `lookup` with `exists` over the to-many link). The lookup reads the stored rows, settled ones too: the spec filter does not apply inside a lookup. The Customer Balance keeps `customerFamilyGroups()`'s row rule; `arGroups()` replaces it for this report only)
```

Replace the paragraph that starts `Engine reach.` (line 71) with:

```markdown
Engine reach. Every part is existing core API: a single-collection source, `diffDays`, `if`, `sum`, `min`, `columns[].blankAbove`, `columns[].detail`, `columns[].hidden`, `groups[].inline` / `hideEmpty` / `muted` / `having` / `label`, a `lookup` with `pick: "count"` and an `exists` over a to-many link inside a level condition, `sort[].formula`, `detail.label`, `detail.open`, `display.layouts`. One use is pinned by the Go test of checkpoint 1 (`CheckSources`, `Load`, `Validate`, a run): a group label that reads `min` of a text field on a group whose key is a text column. (Checkpoint 1 also pinned a union part that is a view collection; the stored table ended that use.) Per the human's rule (2026-11-01), a refusal there is fixed in core as checkpoint 0, not designed around.
```

In the paragraph that starts `Files (Books):` (line 77), replace `` `pb-migrations/<number>_create_books_receivable_items.js`; `` with:

```markdown
`pb-migrations/<number>_create_books_receivable_items.js`; `server/receivable_items.go`, `server/receivable_refresh.go` (+ `receivable_items_test.go`, `receivable_stored_test.go`, `receivable_refresh_test.go`); `server/ledger.go`, `server/payments.go`, `server/sales_documents.go` (one bind line each); `server/balances.go` (the rebuild route); `server/oauth_scopes.go`;
```

At the end of the paragraph that starts `Go test cases (` (line 79), after `a reader without Books access gets no rows.`, add:

```markdown
 For the stored table: the stored rows after each kind of post, with no rebuild; `settled` for a part-paid item, an item paid in two parts, an item paid on its own day, and an item opened again by a payment's void; an invoice correction that moves a live payment's applications; a refresh outside a transaction (refused); a REST write (refused, a superuser's too); verify finds a stale row, a missing row and a wrong `settled`; the rebuild route repairs them; an item paid in full is read whole on a day before its `settled` day and not at all from that day on; the family header with a settled sub-customer, on a day before the sub-customer's first row, and with the sub-customer left out of the Customer pick.
```

- [ ] **Step 4: §5 Cut, §8 Non-goals and §9 Performance acceptance**

§5 Cut (line 90): in the bullet `- A description on the lines of an item: the union source carries no memo; the line's document shows it.`, replace `the union source carries no memo` with `the source carries no memo`.

§8: replace the bullet `- No client collection for the two views: only the report engine reads them.` (line 112) with:

```markdown
- No client collection for `books_receivable_items`: only the report engine reads it.
- No upgrade path from the two views the table replaced: the migration is unreleased and was edited in place.
```

§9, first paragraph (line 119): after the sentence that ends `` and a group label that reads `min` of a text field. `` add the sentence:

```markdown
Since p5-report-perf the source is one stored table (§4), not a union over views; the runs, the dataset and the bars did not change.
```

§9, **Dataset** (line 137): replace the sentence that starts `A "large table" below is a table with 10,000 rows or more in L:` with:

```markdown
A "large table" below is a table with 10,000 rows or more in L: `books_journal_lines`, `books_journal_entries`, `books_invoices`, `books_payments`, `books_payment_applications`, `books_receivable_items` (about 275,000 rows in L: the Accounts receivable lines and the applications). The bulk writer rebuilds the stored rows in its transaction, and the comparison with the real posters reads the stored rows too.
```

§9, the paragraph that starts `A plan check of the same statements at a small size` (line 158): replace `the build runs them one time for each change to the views, the indexes or the spec.` with `the build runs them one time for each change to the stored table, its queries, the indexes or the spec.`

§9, replace the paragraph that starts `**When a bar fails.**` (line 168) with:

```markdown
**When a bar fails.** In this order: add an index in this unit's migration; rework the stored table (a column that lets an index apply or lets the report read fewer rows, as `settled` does) or the two queries that fill it. When the cause is the SQL that core's engine writes for the source, the family lookup or the `min` label, the fix is in core as checkpoint 0 (the human's rule of 2026-11-01); the build stops and reports it. A bar is never changed and a run is never removed to pass.
```

§9, **Record** (line 170): replace `each index added and each view change, with the plan step and the time before and after it;` with `each index added and each change to the stored table or its queries, with the plan step and the time before and after it;`

- [ ] **Step 5: Check the spec**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books && command grep -n "books_receivable_lines\|books_receivable_applications\|RegisterView\|two view collections with\|first item of a sub-customer" docs/specs/ar-aging-design.md | cut -c1-120`
Expected: no line.

Run: `command grep -c "books_receivable_items" docs/specs/ar-aging-design.md`
Expected: `6` (the lines of Live, the migration paragraph, the source paragraph, Files, §8 and the large tables of §9).

Run (after step 6): `command grep -n "union source carries\|change to the views\|view changes" docs/specs/ar-aging-design.md docs/plans/ar-aging-ledger.md | cut -c1-120`
Expected: no line.

Run: `command grep -n -i "A/R Aging\|receivable" help/reports.md help/customers.md | cut -c1-120`
Expected: no line that names a view, the stored table or `settled`. No help file changes: what a person sees and does is the same (the same rows, the same figures, the same family headers). If a line does name one of them, stop and report it.

- [ ] **Step 6: The ledger**

`docs/plans/ar-aging-ledger.md`, section `## Performance`, two small edits first:

- line 55: replace `written by checkpoint 3 and again after each change to the views, the indexes or the spec)` with `written by checkpoint 3 and again after each change to the stored table, its queries, the indexes or the spec)`;
- line 89: replace `- indexes added and view changes: each with the plan step and the time before and after it` with `- indexes added and changes to the stored table or its queries: each with the plan step and the time before and after it`.

Then, in the same section, directly above the line `- machine, date, commit:` (the first line of the empty record list, under the Baseline sub-section) add:

```markdown
### Stored receivable items (p5-report-perf)

Design: lane staging `stored-receivables-design.md`, approved by the human 2026-11-18. Plan: core `docs/plans/2026-11-18-report-perf-stored.md` and `-part-2.md`.

- What changed: the two views `books_receivable_lines` and `books_receivable_applications` are gone. One base collection `books_receivable_items` stores their rows with `source` and `settled`; Books refreshes it in each write transaction at four hook points (entry, payment, payment application, invoice) and touches the change signal; `verifyReceivables` and `rebuildReceivables` stand beside the balance rebuild. The report filter adds `settled > asOf`, so a run of the last day reads only the rows of the items open on that day (8,550 open items at L in the run of 2026-11-18; the count of stored rows is in the numbers below).
- Indexes added: `idx_books_receivable_items_entry (entry)`, `_item (item)`, `_customer (customer, item, date)`, `_settled (settled, date)`. `idx_books_accounts_type` stays: the rebuild and the verify start at the accounts.
- Departures from the approved design, accepted by the human 2026-11-19:
  - (a) `settled` is a `Text` field of the source, not a `Date` field. A date field is written as `NULLIF(substr(col, 1, 10), '')`, which no index serves; with it runs 4, 5 and 6 failed the bar "a bound statement has no SCAN of a large table".
  - (b) `settled` is per customer and item, not per item, because the report's item level is under its customer level.
- Rejected by the human 2026-11-19: a rule "the first item of a sub-customer is never settled" with a fifth hook on customers. In its place the family header (plan decision D7) is kept by the report: `arGroups()` replaces the family level's `Inline` with a lookup that counts the family's sub-customers, in the Customer pick, that have a stored row on or before As of, with an `isEmpty` guard for No customer, over the new to-many link `receivable_items` of `books.customers`. D7 holds exactly; the Customer Balance keeps its row rule (D1). Check: lane staging `family-header-check.md`.
- Plan of the family lookup (small book, both readings): `SEARCH books_customers USING INDEX idx_books_customers_sibling_name (parent>? AND parent<?)` and `SEARCH books_receivable_items USING INDEX idx_books_receivable_items_customer (customer=?)`.
- Numbers at L: below, written by the profile and scale runs.

```

- [ ] **Step 7: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add docs/specs/ar-aging-design.md docs/plans/ar-aging-ledger.md
git commit -m "docs(books): A/R Aging spec and ledger for the stored receivable items"
```

---

### Task 6: L profile, scale run and record

**Files:**
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/ar-aging-ledger.md` (section Performance, sub-section `### Stored receivable items (p5-report-perf)`)
- Modify (only when the scale run wrote it): `/Users/nas/tmp/tinycld/books-purchases/books/server/testdata/ar_aging_stat1.json`
- Create (only when the scale run wrote it): `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/ar-aging-evidence/perf-plans.md`

**Interfaces:**
- Consumes: `TestARAgingProfile` (`BOOKS_PROFILE=<output file>`, `BOOKS_PROFILE_SIZE=L`; it writes the output file again after each run, with a summary table, then `FLOOR` lines for `arProfileFloor` of task 4, then each statement's time, rows, plan and CTE times); `TestARAgingScale` (`BOOKS_SCALE=1`, `BOOKS_SCALE_OUT=<folder>`; when it reaches its end it writes `perf-results.md`, `perf-plans.md` and `ar_aging_stat1.json` there); `TestARAgingQueryPlans` (reads `server/testdata/ar_aging_stat1.json` for its "L statistics" reading). The baseline to compare with: the ledger's Performance section (the failed run of 2026-11-18 and the Baseline sub-section) and lane staging `profile-L-baseline.txt`.
- Produces: the L numbers in the ledger, and one of two hand-backs for the driver: every bar passes, or the numbers with the engine tasks 3, 11 and 4 of `2026-11-18-report-perf-part2.md` / `-part3.md` named as next.

`STAGING` below is `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf`. Both runs take more than 4 minutes: the driver runs them (agent rules). Do not start them.

- [ ] **Step 1: Hand back the two runs**

Before the hand-back, run the short check that the harness still starts: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_PROFILE=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/profile-stored-small.txt BOOKS_PROFILE_SIZE=small go test -run 'TestARAgingProfile$' -count=1 . 2>&1 | command grep -E '^(--- FAIL|FAIL|ok)'`
Expected: `ok  tinycld.org/packages/books`, and `command grep -c "^FLOOR" /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/profile-stored-small.txt` prints `3`.

Then hand back exactly:

```text
NEEDS-RUN: cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_PROFILE=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/profile-stored-L.txt BOOKS_PROFILE_SIZE=L go test -run 'TestARAgingProfile$' -count=1 -timeout 60m . > /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/profile-stored-L.log 2>&1
NEEDS-RUN: cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/scale-stored go test -run 'TestARAgingScale$' -count=1 -timeout 60m -v . > /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/perf-scale-stored.log 2>&1
Needed: both logs and the files they write. Run them one after the other, not at the same time (the times are bars). A fresh agent continues at task 6, step 2.
```

- [ ] **Step 2: Read the profile (a fresh agent, after the runs)**

Run: `command grep -E '^\| |^FLOOR' /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/profile-stored-L.txt | cut -c1-240`
Expected: a table with one line per run (1 to 7) and three `FLOOR` lines. A run line that holds `FAILED` is a refused run (core's 10 s limit): record it as it is.

Run: `command grep -E '^(--- FAIL|--- PASS|FAIL|ok|panic)' /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/profile-stored-L.log /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/perf-scale-stored.log | cut -c1-200`, then `command grep -n -E 'median|or less' /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/perf-scale-stored.log | cut -c1-240`, then `command grep -n -E '_test\.go:[0-9]+:' /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/perf-scale-stored.log | command grep -v 'median' | cut -c1-240 | command grep -m 60 ""`
Expected: the verdict of each test and sub-test (`--- PASS: TestARAgingScale` or `--- FAIL: TestARAgingScale`); the median of each run at S and at L, and one `want … or less` line for each time bar that fails; and each other line the scale test printed. A line of the last command is a failure: a plan bar of an S or L statement (it names the run, the statement and the plan step), a row count outside the §9 table, or a refused run. Read `scale-stored/perf-results.md` with the Read tool when it exists: it is the record in the form spec §9 asks for, with the median of each run at S and at L.

- [ ] **Step 3: The L statistics and the plan check**

Run: `ls /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/scale-stored/ 2>&1; command grep -c '"tbl": "books_receivable_items"' /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/scale-stored/ar_aging_stat1.json`

Case A, the folder holds `ar_aging_stat1.json` and the count is 1 or more (the table has its primary key and four indexes, so 5 is usual):

```bash
cp /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/scale-stored/ar_aging_stat1.json /Users/nas/tmp/tinycld/books-purchases/books/server/testdata/ar_aging_stat1.json
mkdir -p /Users/nas/tmp/tinycld/books-purchases/books/docs/plans/ar-aging-evidence
cp /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/scale-stored/perf-plans.md /Users/nas/tmp/tinycld/books-purchases/books/docs/plans/ar-aging-evidence/perf-plans.md
cd /Users/nas/tmp/tinycld/books-purchases/books/server && go test -run 'TestARAgingQueryPlans' -count=1 . 2>&1 | command grep -E '^(--- FAIL|FAIL|ok)|SCAN|AUTOMATIC' | cut -c1-240
```

Expected: `ok  tinycld.org/packages/books`. This is the plan check under the real L statistics, with statistics for `books_receivable_items`: the open check of part 1, task 3. A failure here is a plan bar that fails under L statistics: do not change the bar or the file; fix it by "When a bar fails" of spec §9 (an index of the stored table first), and when the cause is SQL that core writes, record the statement and its plan step and go to case C of step 5.

Case B, the count is 0 or the file is not there: copy nothing. A count of 0 with the file present is what a refused L run leaves (the L sub-test ended before its `ANALYZE`, and the file holds `null`). No file means the test process ended (the `-timeout`, a panic). In both, `server/testdata/ar_aging_stat1.json` stays `[]`, and the plan check under L statistics is NOT done. Say so in the ledger (step 4) and in the hand-back, with the last 5 lines of the failure from `perf-scale-stored.log`.

A plan bar that fails inside the scale run (a line of step 2 that names a run, a statement and a plan step such as `SCAN` or `AUTOMATIC`), in case A or B: do not change the bar. When the statement reads the stored table or the family lookup and an index can serve it: add the index in the migration (edit in place) and to the index list of `TestReceivableItemsShape`, run step 5 of task 4 again, commit it as `perf(books): index <its name> for the A/R Aging`, record the plan step before it in the ledger, and hand back the two `NEEDS-RUN` lines of step 1 again; a fresh agent then starts again at step 2. When the cause is SQL that core writes, go to case C of step 5.

- [ ] **Step 4: Write the record**

`docs/plans/ar-aging-ledger.md`, in the sub-section `### Stored receivable items (p5-report-perf)`, directly under its line `- Numbers at L: below, written by the profile and scale runs.` add a record with these parts, each filled from the files of step 2 and step 3 (copy the figures; do not round again):

1. One line: machine, date, books commit (`git -C /Users/nas/tmp/tinycld/books-purchases/books rev-parse --short HEAD`), core commit (`git -C /Users/nas/tmp/tinycld/books-purchases/tinycld rev-parse --short HEAD`).
2. The profile's summary table at L (the lines that start `| ` of `profile-stored-L.txt`), as it is.
3. A list "Floor (the stored table with no report)": the three `FLOOR` lines, each as `<time> — <what it reads>`, in this order: every row and column; grouped by customer and item; the open rows grouped by customer and item.
4. A table "Before and after" with one line per run 1 to 7 and the columns: run; L before (the failed run of 2026-11-18 in this section: 10,176 ms, 5,842 ms, 10,399 ms, 4,294 ms, over 10 s, not reached, not reached); L after (the scale run's median, or the profile's wall when the scale run has none); S after; ratio L/S; bar (1,000 ms for runs 1 to 6, 5,000 ms for run 7, ratio 15); pass or fail.
5. One line: the Customer Balance time on the same dataset (no bar).
6. One line for the plan bars: "every plan bar holds with no statistics and under L statistics (`TestARAgingQueryPlans`, statistics file of this run)" for case A, or for case B "the plan bars hold with no statistics; the L-statistics reading was not made: the scale run wrote no statistics file" and the cause.
7. The row counts of S and L from `perf-results.md` when it exists, with the count of `books_receivable_items`.

Then fill the empty record list at the end of the Performance section (`- machine, date, commit:` to `- full plan text:`) only for case A, from `perf-results.md`; for case B leave it empty.

- [ ] **Step 5: Commit and hand back**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add docs/plans/ar-aging-ledger.md
git add server/testdata/ar_aging_stat1.json docs/plans/ar-aging-evidence/perf-plans.md   # case A only
git commit -m "docs(books): A/R Aging L numbers over the stored receivable items"
```

Hand back one of these, with the "Before and after" table:

- Case A, the scale log holds `--- PASS: TestARAgingScale` (every time bar and every plan bar of the S and L runs), and `TestARAgingQueryPlans` passed in step 3: `STORED TABLE MEETS THE BARS`. The engine tasks 3, 11 and 4 are then not needed for spec §9; the driver decides. Without that `--- PASS` line this hand-back is never given.
- A time bar fails (case A or B): `BARS NOT MET AFTER S1-S6`, the failing runs with their L time and bar, the floor times, and the next step: the engine tasks 3, 11 and 4 of `2026-11-18-report-perf-part2.md` and `2026-11-18-report-perf-part3.md`, in the order their outline gives. Do not change a bar, remove a run or change the dataset.
- Case C, a plan bar fails (in the scale run, or in `TestARAgingQueryPlans` under the L statistics) from SQL that core writes: `STOPPED: a plan bar fails from SQL that core writes`, with the statement, the plan step and the reading.

The stop rule of the first outline (the human, 2026-11-18) stays: when the numbers show that a bar is not attainable, the evidence goes in the ledger and the hand-back is `STOPPED: <the question>`.
