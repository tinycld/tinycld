# Report group pick text, package hub, report failure e2e — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Three small core changes the Books Trial Balance needs (human decisions D1 and D2, 2026-11-04), plus core e2e for the report view's failure states.

**Architecture:** D1 adds an optional `optionsFrom.groupLabel` template to a source-backed choice param; the engine validates it, one pure TS helper writes the group pick text for the picker and the chip, and the builder edits it. D2 gives the Reports hub a package view: with `?package=<slug>` the hub drops its tabs and lists, in one list, every report of that package the reader can open (own, shared with them, published), read by three pbtsdb live queries. A new e2e spec covers the report view's error, timeout and options-load Retry.

**Tech Stack:** Go (core/server/reports), TypeScript + zod v4, React Native / react-native-web, pbtsdb + TanStack DB, Playwright.

**Spec:** books `docs/specs/trial-balance-design.md` §3 ("All <type> accounts") and §2/§7.1 (sidebar → filtered hub → Trial Balance); books `docs/plans/trial-balance-ledger.md` phase 4 notes D1, D2.

## Global Constraints

- Core names no package (CLAUDE.md "Core must not name a package"); tests use the salesstub stub.
- Every feature works on web and native and at phone width.
- pbtsdb only; `useMyLiveQuery` for my own rows; `materialize()`, never an `innerJoin` from a user-filtered membership into the collection it makes readable.
- Forms through react-hook-form + zod; no hand-rolled validation.
- E2E: no `page.goto` for in-app navigation (initial load in `login`/`loginAs` only); read-only `page.route` interception; data made through the UI.
- `E2E_PORT=7210 E2E_PORT_2=7211` for every e2e command.

## Design decisions

- **D1 field:** `params[].optionsFrom.groupLabel` (string, optional). `{group}` stands for the group's label. Absent or empty: `All {group}` (today's text). Engine refusals (400, `params.<i>.optionsFrom.groupLabel`): set without `optionsFrom.group`; no `{group}` in it; over 200 bytes (maxLabel). Key order in the spec: `source, value, label, group, groupLabel, where`.
- **D2 hub:** `package` set → `view = 'package'`: no tabs; one list of the union of three live queries, each filtered to the package: Mine (`owner = me`, `source ~ '<slug>.%'`), Shared with me (my `report_shares` rows with the report through `materialize()`, the package filter after the query), Published (`published = true`, `source ~ '<slug>.%'`). Rows dedupe by id, sorted by title then id. Roles come from the share rows. Three condition-specific queries (not one `source ~` query) so a share or publish that is removed takes its row away at once. Unfiltered hub: tabs as today, and the tabs no longer filter by package.

## Review Focus

- A report shared twice (direct + group) and also published shows once — `hub-rows.test.ts`.
- A report from another package whose shared row reaches the reader does not show in the package view — `hub-rows.test.ts`.
- A share removed while the package view is open removes the row — covered by the shared query being the only path for a non-owned, unpublished report (design); e2e asserts a share added while open appears.
- A `groupLabel` typed in the builder without `{group}` shows an inline error and blocks Save — `spec.test.ts` (schema) and builder e2e.
- Clearing "Group the list by" drops `groupLabel`, so the spec stays valid — `filter.test.ts`.

---

### Task 1: Engine — `optionsFrom.groupLabel`

**Files:**
- Modify: `core/server/reports/spec.go` (OptionsFrom), `core/server/reports/params.go` (checkParamDefs)
- Test: `core/server/reports/params_test.go` (TestCheckParamDefsRefusesBadDeclarations)

- [ ] Step 1: add cases `group label without group`, `group label without placeholder`, `long group label` (at `params.0.optionsFrom.groupLabel`), plus an accepted grouped def with `GroupLabel: "All {group} teams"`. Run `go test ./reports -run TestCheckParamDefs` in `core/server`: FAIL (field missing).
- [ ] Step 2: add `GroupLabel string \`json:"groupLabel,omitempty"\`` after `Group`; validate in `checkParamDefs` for a choice param with `OptionsFrom`. Run: PASS.

### Task 2: Client — spec schema, group pick text, picker, chip

**Files:**
- Modify: `core/reports/lib/spec.ts` (optionsFromSchema), `core/reports/lib/param-options.ts` (`groupPickText`), `core/reports/lib/params.ts` (chip), `core/reports/components/SourceChoicePicker.tsx`
- Test: `core/reports/lib/__tests__/spec.test.ts`, `param-options.test.ts`, `params.test.ts`

**Produces:** `groupPickText(def: ParamDef, groupLabel: string): string`; `GROUP_LABEL_TOKEN = '{group}'`.

- [ ] Step 1: tests — schema accepts `groupLabel` with group, refuses it without group / without `{group}` / over 200 bytes, accepts `''`; `groupPickText` gives `All North` by default and `All North teams` for `All {group} teams` (every `{group}` replaced); chip `Teams: N1, All North teams`. Run vitest: FAIL.
- [ ] Step 2: implement; the picker's `AllOfGroup` and `choiceLabels` call `groupPickText`. Run: PASS.

### Task 3: Builder edits the group pick text

**Files:**
- Modify: `core/reports/lib/builder/filter.ts` (`withOptionsGroup` keeps order and `groupLabel`, drops it with the group; `tidyParam` drops an empty `groupLabel`), `core/reports/lib/builder/draft.ts` (`specFromDraft` writes tidied params), `core/reports/components/builder/ConditionValues.tsx` (`OptionsGroup` shows a `TextInput` "Whole group text" bound to `params.<i>.optionsFrom.groupLabel`, placeholder `All {group}`, only while grouped)
- Test: `core/reports/lib/builder/__tests__/filter.test.ts`, `draft.test.ts`

- [ ] Step 1: tests — `withOptionsGroup` keeps `groupLabel` in key order and drops it on `null`; `specFromDraft` drops `groupLabel: ''` and keeps a set one in key order. FAIL.
- [ ] Step 2: implement. PASS.

### Task 4: Hub package view

**Files:**
- Create: `core/reports/lib/hub-rows.ts` (`sharedReports`, `packageReports`)
- Modify: `core/reports/hooks/useReportsHub.ts`, `core/reports/components/hub/ReportsHub.tsx`
- Test: `core/reports/lib/__tests__/hub-rows.test.ts`

**Produces:** `sharedReports<R extends HubRecord>(rows: readonly { role: ShareRole; report: R | undefined }[]): { reports: R[]; roles: Map<string, ShareRole[]> }` (newest change first); `packageReports<R extends HubRecord>(input: { prefix: string; mine: readonly R[]; shared: readonly R[]; published: readonly R[] }): R[]` (prefix match, deduped, title then id). `useReportsHub()` gains `view: HubTab | 'package'`; `ReportsHubView` hides the tabs for `'package'`.

- [ ] Step 1: tests for both helpers (dedupe direct+group+published, other package dropped, order). FAIL.
- [ ] Step 2: implement helpers and the hook (queries gated by view; package state = error if any errors, ready when all ready; empty text names the package). PASS.

### Task 5: Help and package docs

- Modify: `core/help/reports.md` (hub paragraph: a package link lists every report of that package you can open; filters that list records: Whole group text), `docs/packages.md` (the `params[].optionsFrom` row).

### Task 6: E2E

**Files:**
- Create: `tests/e2e/reports-failure-states.spec.ts`
- Modify: `tests/e2e/helpers.ts` (`loginAs(page, id, pw, { startAt })`, `signInAsCollaborator(page, { startAt })`), `tests/e2e/reports-share.spec.ts` (package view test), `tests/e2e/reports-source-params.spec.ts` (group pick text in the picker, chip and builder), `tests/scripts/reports-stub/server/register.go.txt` (`groupLabel: "All {group} teams"` on Stub team totals, revision 2)

- [ ] Failure states (shipped owner, Stub team totals, a team + sale made through the stub's UI): run route answers 500 while failing (the run retries a 5xx twice: `useReportRun` `retry`) → `report-error` "Could not load this report" → stop failing, Retry → row shows; run route answers 408 once → `report-timeout` "This report took too long to run" → Retry → row shows; param-options route answers 500 while failing → `report-filter-load-error` → stop failing, `report-filter-retry` → the test's team option shows.
- [ ] Package view (owner = e2e user makes a salesstub report; collaborator starts at `/a/reports?package=salesstub` through `signInAsCollaborator(page, { startAt })`): no tab pills; the published stub report shows; the owner shares the report → its row appears in the collaborator's list without a tab press or reload; at 390×844 the row and the strip still show.
- [ ] Group pick text: the picker's group pick reads `All North teams`, the chip `Teams: <stem> N1, All North teams`; the builder: grouped ask shows "Whole group text"; typing `Every {group} team` makes the chip `Team: Every North team`; typing text with no `{group}` shows the field error.
- [ ] Run: `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-failure-states.spec.ts reports-share.spec.ts reports-source-params.spec.ts`.

### Task 7: Checks, commit, PR

- [ ] `pnpm run checks`, core vitest (`pnpm exec vitest run core/reports`), `go test ./reports/...` in `core/server`.
- [ ] Commit (conventional), merge `origin/main` if it moved, push, open the PR, watch CI.
