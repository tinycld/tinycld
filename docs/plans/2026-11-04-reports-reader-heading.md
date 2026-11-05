# Report source Heading and FiscalStartMonth run for the reader

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Tests come from a test-writer subagent and are proven RED before any non-test code exists; a separate implementer makes them green without editing the tests; a separate reviewer checks the branch.

**Goal:** A report source's `Heading` and `FiscalStartMonth` run for the reader of the run, so a package can return a value read from a collection that only some users may read, without leaking it to other users.

**Architecture:** Both functions get the reader as the `*core.RequestInfo` that the run's list rules already use (`listRequestInfo(auth, headers)`). Core adds one helper, `FindViewableRecord`, that finds a record by id only when the reader's view rule allows it (PocketBase `app.CanAccessRecord`). Each run calls each function once and uses that one value everywhere.

**Tech Stack:** Go, PocketBase fork (`third_party/pocketbase`), core report engine `tinycld.org/core/reports` (`core/server/reports`).

**Spec:** this file (the human approved the change, "option a", in the auto-helix lane `purchases`, unit `p5-trial-balance`).

## Problem (verified)

- `Source.Heading func(app core.App) string` (registry.go:127) and `Source.FiscalStartMonth func(app core.App) int` (registry.go:123) get only the app.
- run.go:238 calls `Heading(app)`; rows.go:94 `fiscalStartOf(src, app)` calls `FiscalStartMonth(app)`; `fiscalStartOf` runs from `readerCompiler` (run.go:260, the period presets) and again from `newCompiler` (rows.go:83, `fiscalYear` / `fiscalPeriod` formulas).
- The report catalog lists every registered source to every user. A package whose heading or fiscal month comes from a rule-guarded collection therefore shows it to any user who builds a report on that source. docs/packages.md:1143-1147 says `Heading` "must return no data that some readers may not see", which a package that needs a per-reader value cannot meet.

## Who is the reader ("no reader" runs)

- Every read that calls these functions goes through `readerCompiler` (run.go:248): `Run` (run.go:204, from `handleRun`) and `ReadParamOptions` (options.go:234). `readerCompiler` refuses a nil auth and a superuser with `ErrNoReader`. A shared report runs as the signed-in viewer. So the functions are always called with a signed-in, non-superuser reader; there is no "no reader" run to handle.
- `Validate`'s dry compile (run.go:1045 `newCompiler(p, nil, nil, nil)`) and unit tests that pass a nil app or nil info must NOT call the functions: fiscal start stays 1. `CheckSources` / shipped / check paths do not call them (grep: only run.go:238 and rows.go:98 do).
- No caller may pass a nil reader to these functions.

## Global Constraints

- Core must name no package (CLAUDE.md "Core must not name a package"): tests use the fictional zoo package only (`core/server/reports/zoo_test.go`).
- Go style: comments explain why. No `any`-style dodges.
- The run result's wire shape does not change (`heading` stays `json:"heading,omitempty"`); the TS client (`core/reports/...`) needs no change.
- Run from `core/server`: `go test ./reports/...` and `go build ./...`. From `tinycld`: `pnpm run checks`.

## Interfaces (exact)

`core/server/reports/registry.go`:

```go
// FiscalStartMonth (1-12) drives fiscalYear, fiscalPeriod and the fiscal
// period presets when this source is a report's base. nil means 1. It runs
// once per read of a report, for that read's reader (see Heading).
FiscalStartMonth func(app core.App, reader *core.RequestInfo) int
// Heading is the name a report on this base source prints above its title
// (a business name), read once per run for the run's reader. reader is the
// signed-in user the run is for (never nil, never a superuser) with the
// request's headers, as the run's list rules see them. Return only what
// that reader may see: read through FindViewableRecord. nil prints the
// organization's name.
Heading func(app core.App, reader *core.RequestInfo) string
```

New file `core/server/reports/reader.go`:

```go
// FindViewableRecord returns the record of collection with id when the
// reader's view rule allows it. A record that does not exist, or that the
// reader may not view (a null rule included), returns sql.ErrNoRows, as
// PocketBase's view API answers 404 for both, so a caller cannot tell them
// apart. Any other error (a rule that cannot be built, a database fault) is
// returned as is.
func FindViewableRecord(app core.App, reader *core.RequestInfo, collection, id string) (*core.Record, error)
```

Implementation outline: first `app.FindCachedCollectionByNameOrId(collection)`; a collection that does not exist is a wiring mistake in the package, so it returns an error that is NOT `sql.ErrNoRows` (and does not wrap it), e.g. `fmt.Errorf("reports: collection %q does not exist", collection)` — PocketBase answers `sql.ErrNoRows` for an unknown collection, which would otherwise read as "not viewable". Then `app.FindRecordById(col, id)` (`sql.ErrNoRows` when missing; other errors as is), then `app.CanAccessRecord(rec, reader, rec.Collection().ViewRule)` (third_party/pocketbase/core/record_query.go:599); `false` → `nil, sql.ErrNoRows`; an error → that error.

`core/server/reports/rows.go`: `fiscalStartOf(src *Source, app core.App, reader *core.RequestInfo) int` (1 when the source has none or returns a value outside 1-12). `newCompiler`'s signature does not change; it computes the fiscal start only when both `app` and `info` are non-nil. `readerCompiler` calls `FiscalStartMonth` once and uses the value for the presets and the compiler (split an internal constructor that takes the fiscal start; `newCompiler` wraps it). `Run` calls `Heading(app, c.info)` once.

## Review Focus

1. A reader who may not view the settings row: heading is `""` (shell prints the organization's name) and fiscal start is 1; no error, no notice that names the row.
2. A heading/fiscal function that is called twice per run could return two values across a concurrent settings change: presets and `fiscalYear` cells would disagree. Pinned by a call-count test (once per `Run`, once per `ReadParamOptions`).
3. `FindViewableRecord` must not turn a rule-build error or DB fault into "not found" (silent wrong heading); only missing/forbidden become `sql.ErrNoRows`.
4. A view rule that reads `@request.headers.*` sees the run's headers (the reader info carries them).
5. Dry compile (`Validate`) with nil app/info must not call the functions (would panic on a nil reader inside a package).

---

### Task 1: Tests (test-writer subagent; RED before any non-test code)

**Files (tests only):**
- Modify: `core/server/reports/heading_test.go` (update `withHeading` to the new signature; add tests below)
- Modify: `core/server/reports/zoo_test.go` (the zoo.feedings `FiscalStartMonth` registration that reads `zooFiscalStart`) and `core/server/reports/formula_run_test.go:75` to the new signature
- Create: `core/server/reports/reader_test.go` (helper tests)
- Any other `_test.go` that sets `Heading` / `FiscalStartMonth` (grep).

Test cases (assert values only after reading the code that produces them):

- [ ] **Step 1:** Fixture: in the test, add a fictional rule-guarded collection (e.g. `zoo_office`, fields `name` text, `fiscal_month` number, `owner` relation to users; view and list rule `owner = @request.auth.id`), one row owned by `ann`. Register a copy of `zoo.feedings` whose `Heading` and `FiscalStartMonth` read that row with `FindViewableRecord(app, reader, "zoo_office", id)` and fall back to `""` / 1 on `sql.ErrNoRows` (both ann and bob may run zoo.feedings: both keep zed). Restore the registry with `t.Cleanup` as the existing tests do.
- [ ] **Step 2:** Heading follows the reader: `z.run(t, "ann", feedingsByDay, ...)` → `res.Heading == "<row name>"`; `z.run(t, "bob", ...)` → `res.Heading == ""`.
- [ ] **Step 3:** Fiscal presets follow the reader: with `fiscal_month` 7 and a period param on `thisFiscalYear` and `Options.Today` "2026-08-15", ann's `res.Params` resolve 2026-07-01..2027-06-30, bob's 2026-01-01..2026-12-31 (read params.go and an existing period-param test for the exact spec/param shape and the resolved value format). Also a `fiscalYear` formula cell differs between the two readers, or the param-options path (`ReadParamOptions`) uses the reader's month — at least one of these.
- [ ] **Step 4:** The reader passed is the run's: inside the registered functions record the `reader.Auth.Id` seen; it equals the running user's id and is never nil.
- [ ] **Step 5:** Call count: one `Run` calls `Heading` once and `FiscalStartMonth` once; one `ReadParamOptions` calls `FiscalStartMonth` once.
- [ ] **Step 6:** `reader_test.go`: `FindViewableRecord` returns the row for its owner; `sql.ErrNoRows` (`errors.Is`) for another user; `sql.ErrNoRows` for a missing id; `sql.ErrNoRows` for a collection with a nil view rule (zoo_vault, any user); the row for an empty (public) view rule; a view rule that reads `@request.headers.x_zoo` passes when the reader info carries that header and fails without it; a view rule that cannot be built (a field that does not exist) returns an error that is NOT `sql.ErrNoRows`.
- [ ] **Step 7:** Keep `TestARunPrintsItsSourcesHeadingAndWatchesWhatItReads` and `TestCheckSourcesRefusesAWatchedCollectionItCannotSignal` passing in intent (only the signature changes).
- [ ] **Step 8: Run RED.** `cd core/server && go test ./reports/...` → must fail to compile (`FindViewableRecord` undefined, signature mismatch). Record the output.
- [ ] **Step 9: Commit** `test(reports): heading and fiscal start month follow the reader`.

### Task 2: Implementation (implementer subagent; never edits `_test.go`)

**Files:**
- Modify: `core/server/reports/registry.go:121-127` (signatures + doc comments above)
- Create: `core/server/reports/reader.go` (`FindViewableRecord`)
- Modify: `core/server/reports/rows.go:80-102` (`fiscalStartOf` with reader; `newCompiler` unchanged signature; internal constructor with fiscal)
- Modify: `core/server/reports/run.go:238-264` (`Heading(app, c.info)` once; `readerCompiler` computes the fiscal start once)
- Modify: `tests/scripts/reports-stub/server/register.go.txt:106` → `func stubHeading(core.App, *core.RequestInfo) string { return "Stub Sales Co" }`; then re-run `pnpm exec tsx tinycld/tests/scripts/scaffold-reports-stub.ts` from the workspace root so the workspace `salesstub/` copy matches (idempotent; it installs only when the stub is not yet a member).
- Modify: `docs/packages.md:1124` example and `:1141-1150` contract: both functions run once per read for its reader and must return only what that reader may see; show `FindViewableRecord` in a short example (fictional zoo names); keep the `Watch` paragraph.

- [ ] **Step 1:** Implement to the Interfaces section.
- [ ] **Step 2:** `cd core/server && go test ./reports/...` → PASS; `go build ./...` and `go vet ./reports/...` → clean.
- [ ] **Step 3:** From `tinycld`: `pnpm run checks` → green.
- [ ] **Step 4: Commit** `feat(reports): source heading and fiscal start month run for the reader` and `docs(reports): reader-scoped Heading and FiscalStartMonth contract`.

### Task 3: Review (reviewer subagent)

Review the branch diff against this plan: contract, no nil reader, call-once, helper error semantics, docs, no package names in core, test soundness (tests unchanged by the implementer: `git diff <task1-commit>..HEAD -- '*_test.go'` empty).

---

## Part 2: `Source.Currency` runs for the reader (human approved, same PR)

**Problem (verified):** `Source.Currency func(app core.App) string` (registry.go:120) runs with the app only, from seven call sites, each hit once per compiled column, statement or row, so one read calls it many times:

- `assemble.go:74` (result column currency), `assemble.go:290` (`runner.currencyOf`, once per row)
- `formula.go:177` (`currencyCode`), `label.go:102` (`labelCurrencySQL`), `layout.go:277` (section currency), `options.go:137` (`shownCurrencySQL`, the param-options path), `pivot.go:75`

A package that reads a home currency from a rule-guarded collection leaks it, as with Heading.

**Interfaces (exact):**

```go
// Currency is the fixed currency of Money fields that name no Currency
// field, read for the reader (see Heading), at most once per source in one
// read of a report. nil means the currency is unknown to the engine.
Currency func(app core.App, reader *core.RequestInfo) string
```

- Every call site reads the fixed currency through ONE compiler method, `func (c *compiler) fixedCurrency(src *Source) string`, that calls `src.Currency(c.app, c.info)` the first time a source's currency is needed in that read and keeps the value per source key for the rest of the read (the compiler lives for exactly one read: one `Run` or one `ReadParamOptions`). A source whose fixed currency the read never needs is not called.
- With a nil app or nil info (dry compile, `Validate`, unit tests) it never calls `Currency`; the call sites keep today's dry behaviour (`""` in `currencyCode`, `NULL` / `""` elsewhere).
- Stub template `tests/scripts/reports-stub/server/register.go.txt` and the workspace `salesstub/` copy (re-run the scaffold) use the new signature if they set Currency.
- `docs/packages.md`: the `Source.Currency` sentence and the reader bullet say all three functions (`Heading`, `FiscalStartMonth`, `Currency`) get the reader and must return only what that reader may see; Heading and FiscalStartMonth run once per read, Currency at most once per source per read.

**Review Focus (Part 2):** a fixed currency read per row would let one result mix two values across a concurrent settings change (pinned by the call count); a reader who may not view the row must never get the guarded code in a column, a cell's currency, a pivot or a param option label; dry compile must not call it.

### Task 4: Currency tests (fresh test-writer subagent; RED before any non-test code)

- Update every `_test.go` that sets `Currency` to `func(core.App, *core.RequestInfo) string` (check_test.go:39, formula_run_test.go:74, zoo_test.go:181 at least; grep).
- Fixture: a rule-guarded row (reuse the Part 1 office fixture pattern, e.g. a `currency` text field on `zoo_office`) read through `FindViewableRecord`; a fictional source copy whose Money field names no Currency field, so it uses `Source.Currency`; fallback for a reader who may not view: a different code the test chooses (or "").
- Cases: ann (may view) gets the guarded code in the result column's `Currency` (`ResultColumn.Currency`) and in each money row's currency where the run carries it; bob gets the fallback and never the guarded code; the reader passed is the run's (`Auth.Id`, never nil); one `Run` calls Currency once for that source however many money columns and rows it has (use at least two money columns and several rows); one `ReadParamOptions` over a money-labelled option calls it at most once; `Validate` calls it zero times.
- RED: `go test ./reports/...` fails to compile on the old signature. Commit `test(reports): fixed currency follows the reader`.

### Task 5: Currency implementation (fresh implementer; never edits `_test.go`)

- registry.go signature + doc comment; `compiler.fixedCurrency` memo; all seven call sites through it (the runner and stmt reach the compiler; read how each gets `app`); stub template + scaffold re-run; packages.md.
- Checks: `go test ./reports/...`, `go build ./...`, `go vet ./reports/`, `gofmt -l reports` in core/server; `pnpm run checks` in tinycld. Commit `feat(reports): fixed currency runs for the reader` (+ docs commit).

### Task 6: Review (fresh reviewer subagent)

Same checklist as Task 3, over the Part 2 diff.
