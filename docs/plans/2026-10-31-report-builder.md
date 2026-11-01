# Reports Hub, Report View and Builder Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Core reports UI: the hub at `/a/reports`, the report view (the report shell moved from Books into `@tinycld/core/reports` and fed by the engine's run result), the builder (columns, filter, group and sort, pivot, formulas, layout), the share dialog with publish, live re-runs, and the core help topic.

**Architecture:** Pure TypeScript modules in `core/reports/lib/` hold every rule that is not drawing (spec schema, access, cell format, row model, params and URL, run client, re-run scheduler, builder draft conversions); each has vitest tests. Hooks in `core/reports/hooks/` read saved reports and shares through pbtsdb and run reports through React Query against `POST /api/reports/run` (an approved read endpoint), subscribing to the result's change-signal topics. Components in `core/reports/components/` draw the hub, the view, the builder panel and the share dialog; routes under `app/a/(app)/reports/` re-export the screens. Two small Go changes: a source catalog endpoint, and a run result that echoes resolved params and says which group rows can open. Core e2e runs against a scaffolded stub package (`salesstub`) that registers two report sources, as the forms-host and search stubs do.

**Tech Stack:** TypeScript, React Native + react-native-web (Expo Router 58), Uniwind classes, react-hook-form + zod, pbtsdb 2.1.x (`useLiveQuery`, `useMyLiveQuery`, `materialize`, `subscribeTopic`), TanStack Query 5, Zustand; Go (PocketBase fork v0.40.4, `tinycld.org/core/reports`); Playwright; vitest; Biome.

**Spec:** `/Users/nas/code/tinycld/docs/superpowers/specs/2026-10-30-report-engine-design.md`, section "## 4. Reports UI (core)" (this unit, all of it), with "## Decisions", "## 2. Report engine", "## 3. Saved reports and sharing", "## Errors", "## Security", "## Testing" and "## Books changes" as context. Server contracts: `docs/plans/2026-10-30-report-sharing.md` section "Contracts for p5-report-builder" (this repo). Engine: `docs/plans/2026-10-30-report-engine.md`. Change signal: `docs/plans/2026-10-30-change-signal.md`, `core/lib/change-signal.ts`. The shell being moved: `/Users/nas/tmp/tinycld/books-purchases/books/tinycld/books/components/reports/`, `lib/report-*.ts`, `hooks/useReport*.ts`, design `books/docs/specs/report-shell-design.md`. Read-only: never edit anything under `/Users/nas/code/tinycld`, never edit the `books` member in this unit.

---

## For review

This section is for the human before anything runs (core UI has no design gate).

### Screens and their layout per width

Widths are core's breakpoints (`useBreakpoint`): phone (`mobile`), tablet, desktop.

**1. Reports hub, `/a/reports`** (optional `?tab=mine|shared|published`, `?package=<slug>`)
- Desktop: page title "Reports" with a "New report" button at the right; a row of three tab pills (Mine · Shared with me · Published); a bordered list with a heading row Title · Source · Owner · Last run · (menu). A row is 44pt or taller: the title (semibold) with its description under it (one line), a red "Broken" badge when the report cannot run, the source label, the owner's name, the last run on this device ("2 hours ago", "Never"), and a "⋯" menu (Open, Duplicate, Share, Publish or Unpublish, Delete — each only when allowed). A press on the row opens the report.
- Tablet: the same list without the Owner column.
- Phone: the same title and tabs; each report is a card (title, "Source · Owner", last run) with its "⋯" menu (a sheet).
- With `?package=<slug>`: a strip "Showing reports from <package name>" with "Show all reports"; the list and New report offer only that package's sources.
- New report opens a dialog (a sheet on phone) "Choose what to report on" listing the sources; with none: "No report sources are installed."
- States: loading, "Could not load reports" with Retry, and an empty message per tab.

**2. Report view, `/a/reports/<id>`** (search params: one `p-<paramId>` per param, `div1000`, `nocents`, `negative`, `red`, `hide`, `edit=1`)
- Header (outside the scroller): back link "Reports", the title, and a toolbar (core `ResponsiveToolbar`, which folds what does not fit into "⋯"): Collapse all / Expand all (only when the result has group rows that can open), Customize, Share (owner, editor), Edit report (owner, editor), Save as… , Copy link.
- Control row: one chip per param (the first date param first), then "Add filter". Each chip opens a popover on desktop and tablet and a bottom sheet on phone; on phone the row scrolls sideways and never wraps.
- Notices strip: row cap, pivot cap, mixed currencies, spec upgraded, renamed fields, and "Updating…" while a changed run loads behind the shown figures.
- Document header: organization name, title, the period in words, "Amounts in thousands".
- Table (the moved shell): heading row fixed above the rows; desktop and tablet: label column flexible (min 220pt), amount columns 128pt, sideways scroll when they do not fit; phone: one or two amount columns at 108pt with no sideways scroll, three or more with a 168pt label column and the amount columns scrolling sideways.
- Customize: desktop an inline drawer at the right (360pt) so the table stays visible; tablet an overlay drawer from the right; phone a bottom sheet.
- States: loading, "Could not load this report" with Retry, timeout with Retry, empty ("No rows for <period>" with Clear filters or Show this fiscal year), "You no longer have access to this report", and a missing-parts state that names each missing source, link or field (with Edit report for owners and editors).

**3. Builder** (the report view with `edit=1`; a new report at `/a/reports/new?source=<key>`)
- Desktop: an inline drawer at the right (440pt) beside the live preview; the report view behind it runs the draft.
- Tablet: an overlay drawer from the right (440pt) over the preview.
- Phone: a full-height sheet with a Design | Preview switch at the top; Preview shows the draft's table.
- Folding sections: Details (title, description), Columns, Filter, Group and sort, Pivot, Layout. Footer: Save (owner, editor, new report), Save as… ; the close button asks "Discard changes?" when the draft changed.

**4. Share dialog** (a sheet on phone): "Share “<title>”"; a people search with a Viewer / Editor role picker (a pick adds the share at once); "People with access" rows (name, role menu, remove); Groups (core `GroupShareSection`); "Publish to everyone" for an admin who can edit; the line "Each person sees only the data they can access."; Done.

### Routes

| Route | File | Screen |
|---|---|---|
| `/a/reports` | `app/a/(app)/reports/index.tsx` | `ReportsHubScreen` |
| `/a/reports/new` | `app/a/(app)/reports/new.tsx` | `NewReportScreen` |
| `/a/reports/<id>` | `app/a/(app)/reports/[id].tsx` | `ReportScreen` |
| layout | `app/a/(app)/reports/_layout.tsx` | `Slot` + `DocumentTitle pkg="Reports"` |

The area is app-owned like Settings and Help: a rail button above Help (desktop, tablet), a "Reports" row in the More sheet (phone), a `reports` screen in `PackageTabs`, chrome handling in `ActivePkgSync`, and the generator keeps the route dir.

### File layout under `@tinycld/core/reports` (`tinycld/core/reports/`)

```
lib/        types.ts spec.ts access.ts money.ts dates.ts format.ts rows.ts params.ts param-forms.ts
            view-url.ts run-client.ts rerun-scheduler.ts
lib/builder/ columns.ts formula.ts filter.ts draft.ts errors.ts
lib/__tests__/ and lib/builder/__tests__/   (vitest)
stores/     report-ui-store.ts report-recent-store.ts (+ __tests__/report-recent-store.test.ts)
hooks/      useReportCatalog.ts useLiveTopics.ts useReportRun.ts useReportRecord.ts useReportView.ts
            useReportsHub.ts useHubActions.ts useReportShares.ts useReportBuilder.ts useBackToReports.ts
components/ report-table-layout.ts use-row-window.ts ReportTable.tsx ReportRow.tsx ReportHeader.tsx
            ReportDocumentHeader.tsx ReportStates.tsx ReportNotices.tsx ReportControls.tsx
            ParamChip.tsx ParamPeriodControl.tsx ParamPickers.tsx ReportCustomizePanel.tsx ReportView.tsx
            __tests__/ (report-table-layout and row-window pure functions)
            hub/ (ReportsHub.tsx HubList.tsx HubRowMenu.tsx NewReportDialog.tsx)
            share/ (ReportShareDialog.tsx)
            builder/ (ReportBuilderPanel.tsx BuilderSection.tsx DetailsSection.tsx ColumnsSection.tsx
                      ColumnPicker.tsx FilterSection.tsx GroupSortSection.tsx PivotSection.tsx
                      LayoutSection.tsx)
screens/    ReportsHubScreen.tsx ReportScreen.tsx NewReportScreen.tsx
```

`core/package.json` gains the export `"./reports/*"`.

### How core e2e gets a report source

Core registers no package and must not name one, and app-shell CI installs no feature package. So the e2e drives a stub package, `salesstub`, written by a new scaffolder `tests/scripts/scaffold-reports-stub.ts` (the pattern of `scaffold-forms-host-stub.ts` and `scaffold-search-stubs.ts`, which CI already runs before the bundle build). The stub has:
- a migration with `salesstub_teams` (any signed-in user reads and writes) and `salesstub_sales` (each user reads and writes only their own rows: `owner = @request.auth.id`), so "the other user sees only their data" is real;
- a Go module that registers `salesstub.sales` (customer, amount and cost as money with a currency field, day, status select; to-one link `team`) and `salesstub.teams` (name; to-many link `sales`), so the builder has a to-one link, a to-many link, money, a date, and a select;
- a landing screen with two small forms (New team, New sale) so the specs set data up through the UI.

Specs that assert exact rows sign in a fresh member made by the existing `createInvitedUser` helper, so their sales are theirs alone and the table never holds another spec's rows. Specs that need "somebody else" use the seeded collaborator. One spec (the broken-report state) seeds one report row by a superuser write, because a broken report comes from removing a package and restarting the server, which CI cannot do (R22).

Why not a core source over core collections: that would ship user-facing report sources only so tests have something to read.

### Driver rulings

- **R1. E2E source:** the `salesstub` scaffolded stub package (above). CI gains a "Scaffold reports stub package" step beside the other three. Its slug must not start with `reports`: `navigateToPackage` finds rail links by href prefix, so `/a/reportsx…` would also match `/a/reports`.
- **R2. Source catalog endpoint:** `GET /api/reports/sources` (signed-in users, session-only like the run route) returns the registry (`reports.Catalog()`): source key and label, fields (key, label, type, aggregates, select options, currency field), links (key, label, target, cardinality). It reads the in-memory registry, never a collection, so the CLAUDE.md rule against new data endpoints (whose intent is "no hand-rolled reads of collection rows") is not touched; the engine plan already assigned "the source catalog the builder's column tree reads" to this unit. See open question Q1.
- **R3. Run result additions:** `params` (each param as the run resolved it: the value, or the from/to days of a preset) so the chip and document header show the period the server ran, without a second copy of the fiscal-year rules in the client; and `canExpand` on group rows (false for a pivot's innermost group), so the client never repeats that rule. Both are additive JSON fields.
- **R4. Row kinds:** the engine's `group` replaces the shell's `section`. `computed` rows draw like subtotal rows (semibold, `border-t border-border`, currency symbol). A group row followed by its own subtotal row shows no figures (the shell's rule: a figure is never shown twice); a group with no subtotal row keeps its figures.
- **R5. Folding is the engine's lazy expansion:** a press on a group row adds or removes its group key in `options.expand`; Expand all sends `expandAll`. The state lives in the reports Zustand store per report for the session (not in the URL), as the shell kept folded sections.
- **R6. Customize** has Number format (Divide by 1000, Without cents, Negative numbers, red negatives) and Columns (a Show toggle per column after the first). The shell's Rows choice, Display columns by, column toggles and method (basis) toggle have no engine counterpart: a report expresses them as params and filters in its spec (Books' Trial Balance does so in `p5-trial-balance`). See Q5.
- **R7. Params are chips:** a date param (`date`, `asOf`, `dateRange`) uses the period control (presets with the shell's labels, plus Custom); a `choice` param uses a picker (single: a press applies; multiple: checkboxes and Apply); `text` and `number` use a one-field form with Apply. A param with a default is always a chip and cannot be removed; a param without a default is offered under "Add filter" until set, and its chip has a remove control (removed = unset = filters nothing, the engine's rule).
- **R8. URL:** `p-<paramId>` per set param, `div1000`, `nocents`, `negative`, `red`, `hide` (column ids), `edit=1`, written with `router.setParams` (no history entry). A value equal to the default is left out; an invalid value falls back to the default. "Copy link" copies the full link (server address + path + query) on web and native alike.
- **R9. Runs go through React Query**, because the run endpoint is not a collection. Expected refusals (400, 403, 408, 422) come back as data, not thrown, so the global query error handler does not send them to Sentry; any other failure throws and retries twice when transient (status 0 or 5xx).
- **R10. Live:** the view subscribes to every topic of the result with pbtsdb `subscribeTopic`; a message re-runs after 250 ms of quiet; messages during a run queue one follow-up run. A paged report re-fetches its loaded pages in order (TanStack infinite query refetch).
- **R11. Paging:** a report without groups loads its next page (200 rows) near the end of the scroll; the merged rows keep each page's detail rows and the newest page's total row.
- **R12. "Last run" is per device:** a persisted Zustand store keyed by user and report, written when a saved report's first page answers; the run endpoint stays read-only as the spec requires. See Q2.
- **R13. Hub actions** follow the contracts: Share for owner and editors; Delete for the owner, or an admin on a published report, never for a shipped report (Unpublish instead); Publish or Unpublish for an admin who is the owner or an editor (Ruling P4); Duplicate for everyone, which creates "Copy of <title>" owned by the reader and opens it.
- **R14. Builder placement and preview** as in "Screens" above; the preview runs the draft spec (`{ spec }`) 400 ms after the last valid change. Opening the builder: `edit=1` on the view (honoured only for owners and editors); a new report has its own route.
- **R15. Formulas reuse the forms engine's `FormulaEditor`** (Number, Column, Add, Subtract, Multiply, Divide, Round, Smallest of, Largest of). `round(div(a, b), p)` maps to `div` with scale p; a bare Divide gets scale 0 for an amount divided by a number, else 4. A stored formula using any other operator (for example `if` in a shipped report) is shown read-only with a Remove button ("This formula uses parts the builder cannot edit"), and is saved unchanged. The same holds for a filter condition the builder cannot represent.
- **R16. Filter builder:** groups "All of / Any of / None of", conditions by field type, "has / has no related …" over a to-many link with its own conditions, and "Ask when run", which turns the value into a param (dateRange for a date between, asOf for "on or before", date for on/before/after, number, text, choice for a select; money and yes/no fields cannot be asked, because a param carries no currency or flag). Server errors under `filter.*` show at the top of the Filter section.
- **R17. Save:** owners and editors save in place; "Save as…" (a title prompt, default "Copy of <title>") creates a copy owned by the reader, from the builder and, for anyone, from the view toolbar (the spec's "Save as when the user cannot edit"). Save closes the builder. Spec errors from the server map to builder fields by spec path (`columns.N…` to column N, `groups.N…`, `sort.N…`, `pivot…`, `layout.N…`, `params.N…`; `title`, `description`); anything else goes to the toast of `handleMutationErrorsWithForm`.
- **R18. Share dialog:** the people search reads `users` (name prefix, not disabled, up to 20; guests included because the contracts allow sharing with a guest; the owner and people who already have a share are left out). A pick adds the share at once with the chosen role. Refusals (`user.owner`, `report.too_many_shares`, unique) show as a toast.
- **R19. App-owned area:** `reports` joins `help` and `settings` everywhere the app lists its own areas (PackageTabs, ActivePkgSync, the generator's kept route dirs), and the generator refuses a package whose slug is an app-owned route, because its emit step would delete core's route files.
- **R20. Package filter:** `/a/reports?package=<slug>` keeps reports and sources whose key starts with `<slug>.`; core stores no package name.
- **R21. Test ids:** the moved shell keeps Books' `report-*` test ids, so later Books specs port unchanged; new parts use `reports-hub-*`, `reports-new-*`, `report-builder-*`, `report-share-*`.
- **R22. One superuser-seeded fixture:** `reports-share.spec.ts` seeds one report whose source no longer exists, with its `broken` mark, by a superuser write (a superuser request passes every report hook unchanged, Ruling P6). See Q3.
- **R23. Unsaved changes:** closing the builder with changes asks "Discard changes?"; leaving the route with a dirty builder uses the forms builder's `useLeaveGuard`.
- **R24. Document header** names the organization (`useOrgInfo`), where the Books shell named the Books business.
- **R25. Empty state:** a result with no detail row and no group row is empty (computed and total rows alone do not count). Message "No rows for <period words>", "No rows as of <day>", or "No rows"; action "Clear filters" when an optional param is set, else "Show this fiscal year" ("Show end of last fiscal year" for an as-of param) when the report has that param and is not on that preset.
- **R26. Money display** copies the shell's exact-cents formatter into core (`roundedQuotient`, digit-built groups); each money cell's currency is the column's fixed currency or the row's `<column>@currency` cell.
- **R27. Books is not changed by this unit.** Its shell keeps working until `p5-trial-balance` removes it; this unit's last tasks run Books' checks and its report e2e.
- **R28. pbtsdb pin:** the lane workspace's `node_modules/pbtsdb` is 2.1.0 while core pins 2.1.1 (core main 22ba87be). Task 0 stops for the driver to run the workspace-root `pnpm install` before any check runs.
- **R29. A changed run keeps the shown figures** (`keepPreviousData`) and shows "Updating…" until the new figures arrive; only the first load shows the loading state. Expanding a group therefore never blanks the table.
- **R30. No access:** a report the reader cannot see (record not visible, or a run answers 403) shows "You no longer have access to this report". A report id that does not exist reads the same, because the server answers both alike (Ruling P9).
- **R31. Date conditions compare the day:** a condition on a date or date-time field compares `addDays(field, 0)` (the stored value's day) with the typed or asked day, because PocketBase stores dates as `YYYY-MM-DD HH:MM:SS.sssZ` and a plain text compare would miss the day itself; a date range param already reads the day (the engine's `between`). Only shapes the builder writes are read back as editable (R15).

### [platform] web-only or native-only items

None new. Every control is a core component that runs on web and native (Popover, Sheet, Drawer inline and overlay, Dialog, Menu, DateField, ButtonGroup, Toggle, SelectInput, Switch, ScrollView). Existing web-only behaviours are reused unchanged: `DocumentTitle` renders on web only; `web:[scrollbar-gutter:stable]` on the table scrollers is a web class variant that changes nothing on native; `useLeaveGuard`'s browser Back/Forward half is web-only (native has no browser history; its in-app Back is guarded on both).

### Open questions (recommended answer first)

**Answered by the human, 2026-11-01: plan approved; every recommended answer accepted (Q1 yes, Q2 per device, Q3 yes, Q4 above Help, Q5 spec filter, Q6 the switch).** The Q3 CLAUDE.md fixture-exception entry is the human's to add (workspace-root CLAUDE.md, outside this repo); the driver lists it at the phase end.

- **Q1. Catalog endpoint (R2).** Approve `GET /api/reports/sources` as part of the reports exception? **Recommended: yes** — the builder cannot offer fields without it, and it serves no collection data.
- **Q2. "Last run" (R12).** Keep it per device, or record it on the server (a new `report_runs` collection written by the client after a run, with a migration and rules)? **Recommended: per device now**; a server record is a separate change if people ask for "last run anywhere".
- **Q3. Superuser seed in `reports-share.spec.ts` (R22).** Approve it as a CLAUDE.md fixture exception (the workspace-root CLAUDE.md list is yours to update)? **Recommended: yes**; the alternative is no e2e for the broken state.
- **Q4. Rail placement.** Reports above Help in the rail's bottom group (beside the other app-owned areas), or in the package list? **Recommended: above Help.**
- **Q5. Rows "Non-zero" (R6).** Trial Balance shows non-zero accounts by default. Keep core free of a row-hiding display option and let `p5-trial-balance` express it in the spec (a filter on the balance source), or add a "Hide rows that are all zero" Customize toggle in core? **Recommended: spec filter** (a display-only hide can disagree with totals and paging).
- **Q6. Phone builder (R14).** Design | Preview switch inside the full-height sheet, or the preview stacked under the design? **Recommended: the switch** (a phone cannot show both legibly).

---

## Global Constraints

- Core names no package: no feature slug, scope, collection name or `/api/<slug>/` route in `core/`, `app/`, `cli/`, `scripts/`, `server/`. `PATH=$WS/node_modules/.bin:$PATH tsx scripts/check-core-isolation.ts` (in `$WS/tinycld`) stays at 0 references. The stub lives under `tests/`, which the check does not scan.
- All screens support web and native at desktop, tablet and phone widths.
- Data rules (CLAUDE.md): pbtsdb for collection data (`useLiveQuery` object form with an explicit `queryKey`, `useMyLiveQuery` for my own rows, `materialize()` for related rows, `useMutation` from `@tinycld/core/lib/mutations` with generator functions); no raw PocketBase writes in components, hooks or e2e (except R22); row types derive from queries; Zustand only for shared UI state; no `useState` + `useEffect` sync (an effect is used only to subscribe to change-signal topics, an external system); every form uses zod through `zodResolver`; JSX stays minimal with `isVisible` props; semantic colour tokens only; no `any`, no `biome-ignore`, no `console.*` (use `log` from `@tinycld/core/lib/logger`).
- `POST /api/reports/run` body: `{ reportId, params, options }` or `{ spec, params, options }`; refusals 400 (field errors), 403 (`reportId.no_access`), 408 (timeout), 422 (unknown reference); topics include `collection/reports` and `collection/report_shares` for a saved run.
- Saved reports: `reports` (`owner`, `title` 1–200, `description` ≤ 2000, `spec` json ≤ 1 MiB, server-owned `source`, `published_by`, `broken`, `shipped_key`, `shipped_revision`), `report_shares` (`report`, `user` or "", `group` or "", `role` viewer|editor). Who sees and does what: the contracts table in `docs/plans/2026-10-30-report-sharing.md`.
- Money cells are integer cents; display rounds once, half away from zero, with integer-exact arithmetic; above 2^53-1 a cell shows "Too large".
- Presets (engine names): range `today thisMonth thisMonthToDate thisQuarter thisQuarterToDate thisFiscalYear thisFiscalYearToDate lastMonth lastQuarter lastFiscalYear last12Months`; as-of `today endOfLastMonth endOfLastQuarter endOfLastFiscalYear`.
- Engine caps the builder respects: 100 columns, 4 link hops per path, 10 sort keys, 6 group levels, 200 layout items, labels ≤ 200, pivot `maxColumns` 1–200 (default 50), ids `^[A-Za-z][A-Za-z0-9_]{0,63}$`, 20 params, text params ≤ 500.
- Front-end behaviour is tested by Playwright at desktop and at phone 390×844 where the phone path runs different code; vitest only for non-UI logic (every `lib/` module, the recent store). Go code keeps Go tests (`-tags no_ui`).
- E2E: never `page.goto()` for in-app moves (`login`, then `navigateToPackage`); set `E2E_PORT=7210 E2E_PORT_2=7211` before every e2e command; no timeouts bumped, no forced-serial runs.
- In this workspace run tools from `$WS/node_modules/.bin` (`PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH`); `pnpm exec` starts a workspace install. `$WS` = `/Users/nas/tmp/tinycld/books-purchases`.
- Commits: core repo `$WS/tinycld`, branch `books/p5-report-builder`; messages never mention Claude.
- The plan's code blocks are not wrapped to Biome's 100-column width. Before each task's lint step, run `PATH=$WS/node_modules/.bin:$PATH biome format --write <the task's new and changed files>` from `$WS/tinycld`; only formatting changes. (The `lib/` modules and their tests were run in a probe outside the repos: vitest green, `tsc` clean, `biome lint` clean, format only.)

## Review Focus

1. A share is removed while the reader has the report open → the view changes to "You no longer have access to this report" without a reload (Task 18 spec "a viewer sees only their own rows, cannot edit, saves a copy, and loses a removed share at once").
2. Change-signal messages arrive in a burst, and again while a run is in flight → exactly one run after the burst, plus one follow-up, and the figures shown are the newest (Task 9 `rerun-scheduler.test.ts`; Task 17 spec "a sale added in another browser shows in the open report").
3. A copied link carries a param value the report no longer offers (a removed choice option, a bad date, an unknown preset) → that param falls back to its default and the report still runs (Task 8 `view-url.test.ts` "falls back").
4. Amounts near 2^53, negative zero, and two currencies in one column → exact text, no "-0.00", each cell in its own currency (Task 6 `format.test.ts`).
5. A shipped report whose spec uses formula operators the builder cannot edit (`if`, `case`) is opened in the builder and saved after a rename → the formula and filter are saved unchanged (Task 11 `draft.test.ts` "keeps parts the builder cannot edit").

## Deferrals

Deferrals: none. Every part of spec §4 is built here: the shell move (frame, period and param chips, filter chips, Customize, table, folding, live apply, the row renderer), the hub with its tabs, rows and actions and the package filter, the report view with Edit report and per-run params and Customize kept by a copied link, the builder (columns through to-one and to-many links, filter with and/or/not, params and exists, group and sort with subtotals, pivot, formulas with the forms engine's editor, layout sections and computed rows, Save and Save as), the share dialog with publish, live re-runs, and the core help topic. The Books copy of the shell is removed by the planned unit `p5-trial-balance` (spec "Books changes"), not deferred.

## What was probed

Every probe ran against core `6eb0def3` in the lane workspace.

- `cd $WS/tinycld/core && PATH=$WS/node_modules/.bin:$PATH vitest run lib/change-signal.test.ts` runs core's vitest config from `core/` (2 passed); `core/vitest.config.ts` includes `**/*.test.{ts,tsx}`, so tests under `core/reports/**/__tests__/` are picked up.
- `cd $WS/tinycld && PATH=… vitest run scripts/__tests__/gen-routes.test.ts` runs the generator tests from the app-shell root (6 passed).
- `cd $WS/tinycld/core && PATH=… tinycld-pkg test -- lib/change-signal.test.ts` passes arguments after `--` straight to vitest; `tinycld-pkg test:e2e -- <file>` does the same for Playwright (`package-scripts/src/runners.ts`: `playwright test --config <member>/playwright.config.ts …passthrough`).
- `cd $WS/tinycld/core/server && go test -tags no_ui -count=1 -run 'TestRunEndpoint$' ./reports/` passes (0.7 s).
- `scripts/generate.ts` prunes every directory under `app/a/(app)/` that is not a present package slug or in `APP_OWNED_ORG_ROUTE_DIRS` (`help`, `settings`): an `app/a/(app)/reports/` dir would be deleted on the next generate unless it joins that set (Task 3).
- `core/types/pbSchema.ts` already has `Reports` and `ReportShares` (spec and broken typed `any`); `server/pb_migrations/2090000008_create_reports.js` is linked.
- `node_modules/pbtsdb/package.json` is 2.1.0; `core/package-versions.json` pins 2.1.1 (R28). `subscribeTopic(pb, topic, onMessage, { signal })` returns `Promise<() => Promise<void>>`.
- The engine's result: `Result{columns, rows, notices, topics, page}`; group rows carry `groupKey`, `expanded`, `section`; every detail page ends in the total of all rows (`paging_test.go`); a layout always emits its section rows and subtotal rows; `tc_pct` returns a percent (× 100).
- `tests/e2e/helpers.ts` `navigateToPackage` treats `settings` and `help` as chrome (waits for their heading); CI runs three stub scaffolders before the bundle build (`.github/workflows/ci.yml`).

---

## File Structure

| File | Responsibility |
|---|---|
| `core/server/reports/catalog.go` (+ `catalog_test.go`) | `Catalog()`, `handleSources` |
| `core/server/reports/endpoint.go` | binds `GET /api/reports/sources` |
| `core/server/reports/run.go`, `params.go`, `assemble.go` (+ `result_extras_test.go`) | `Result.Params`, `ResolvedParam`, `Row.CanExpand` |
| `scripts/gen-routes.ts`, `scripts/generate.ts` (+ test) | app-owned `reports` route dir, slug guard |
| `tests/scripts/scaffold-reports-stub.ts`, `tests/scripts/reports-stub/*.txt` | the e2e stub package |
| `.github/workflows/ci.yml` | stub scaffold step |
| `tests/e2e/helpers.ts`, `tests/e2e/reports-helpers.ts` | skip guard, chrome heading, report e2e helpers |
| `tests/e2e/reports-*.spec.ts` | feature e2e |
| `core/lib/pocketbase.ts` | registers `reports`, `report_shares` |
| `core/package.json` | `./reports/*` export |
| `core/reports/**` | the reports UI (layout above) |
| `app/a/(app)/reports/*.tsx` | routes |
| `app/a/(app)/_layout.tsx`, `core/components/workspace/PackageTabs.tsx`, `PackageRail.tsx`, `MoreDrawer.tsx` | app-owned area wiring |
| `core/help/reports.md` | help topic |
| `docs/packages.md` | the catalog endpoint and the hub's `?package=` link for package authors |

---

## Tasks

### Task 0: Preflight

**Files:** none.

- [ ] **Step 1: Confirm the branch and a clean tree**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && git branch --show-current && git status --short`
Expected: `books/p5-report-builder`; only this plan file committed ahead of `6eb0def3`; no other change.

- [ ] **Step 2: Confirm the pbtsdb install matches the pin (R28)**

Run: `cd /Users/nas/tmp/tinycld/books-purchases && node -e "console.log(require('./node_modules/pbtsdb/package.json').version)" && grep '"pbtsdb"' tinycld/core/package-versions.json`
Expected: both say `2.1.1`. If the install says `2.1.0`, STOP and ask the driver to run `pnpm install` at the workspace root (an executor never installs); then repeat this step.

- [ ] **Step 3: Baseline checks are green**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Expected: Biome, tsc and vitest pass. A failure here is fixed at its root before Task 1 (global rule), never skipped.

---

### Task 1: The source catalog endpoint

**Files:**
- Create: `core/server/reports/catalog.go`
- Create: `core/server/reports/catalog_test.go`
- Modify: `core/server/reports/endpoint.go` (`Register`: bind the route)

**Interfaces:**
- Consumes: `Sources()`, `Source`, `Field`, `Link`, `Option`, `CountOnly` (registry.go); the zoo fixture (`newZoo`, `(*zoo).token`, `RegisterSource`, `CheckSources`).
- Produces: `type CatalogSource struct{Key, Label string; Fields []CatalogField; Links []CatalogLink}` (JSON `key`, `label`, `fields`, `links`); `type CatalogField struct{Key, Label string; Type FieldType; Aggregates []Aggregate; Options []Option; Currency string}` (JSON `key`, `label`, `type`, `aggregates`, `options,omitempty`, `currency,omitempty`); `type CatalogLink struct{Key, Label, Target string; Cardinality Cardinality}` (JSON `key`, `label`, `target`, `cardinality`); `func Catalog() []CatalogSource`; route `GET /api/reports/sources` → `{"sources": [...]}`. The TS mirror is `CatalogSource` in `core/reports/lib/types.ts` (Task 5).

- [ ] **Step 1: Write the failing tests**

`core/server/reports/catalog_test.go`:

```go
package reports

import (
	"net/http"
	"slices"
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/oauth"
)

func catalogSource(t *testing.T, cat []CatalogSource, key string) CatalogSource {
	t.Helper()
	for _, s := range cat {
		if s.Key == key {
			return s
		}
	}
	t.Fatalf("catalog has no %s", key)
	return CatalogSource{}
}

func catalogField(t *testing.T, s CatalogSource, key string) CatalogField {
	t.Helper()
	for _, f := range s.Fields {
		if f.Key == key {
			return f
		}
	}
	t.Fatalf("%s has no field %s", s.Key, key)
	return CatalogField{}
}

func catalogLink(t *testing.T, s CatalogSource, key string) CatalogLink {
	t.Helper()
	for _, l := range s.Links {
		if l.Key == key {
			return l
		}
	}
	t.Fatalf("%s has no link %s", s.Key, key)
	return CatalogLink{}
}

func TestCatalogListsUsableSourcesWithTheirFieldsAndLinks(t *testing.T) {
	newZoo(t)
	cat := Catalog()
	keys := make([]string, len(cat))
	for i, s := range cat {
		keys[i] = s.Key
	}
	if !slices.Equal(keys, []string{"zoo.animals", "zoo.feedings", "zoo.keepers", "zoo.people", "zoo.sites", "zoo.vault"}) {
		t.Fatalf("catalog keys = %v", keys)
	}
	animals := catalogSource(t, cat, "zoo.animals")
	if animals.Label != "Animals" || animals.Fields[0].Key != "id" {
		t.Fatalf("animals = %+v", animals)
	}
	if f := catalogField(t, animals, "weight"); f.Type != Number || !slices.Equal(f.Aggregates, SumAll) {
		t.Fatalf("weight = %+v", f)
	}
	// A field registered with no aggregates offers what the engine allows it: counts.
	if f := catalogField(t, animals, "active"); !slices.Equal(f.Aggregates, CountOnly) {
		t.Fatalf("active = %+v", f)
	}
	if l := catalogLink(t, animals, "feedings"); l.Target != "zoo.feedings" || l.Cardinality != ToMany || l.Label != "Feedings" {
		t.Fatalf("feedings link = %+v", l)
	}
	if f := catalogField(t, catalogSource(t, cat, "zoo.feedings"), "cost"); f.Type != Money || f.Currency != "currency" {
		t.Fatalf("cost = %+v", f)
	}
	if f := catalogField(t, catalogSource(t, cat, "zoo.sites"), "region"); len(f.Options) != 2 || f.Options[0].Label != "North" {
		t.Fatalf("region = %+v", f)
	}
}

// The catalog hands out copies: a caller that edits one cannot change the registry.
func TestCatalogCopiesTheRegistry(t *testing.T) {
	newZoo(t)
	first := catalogField(t, catalogSource(t, Catalog(), "zoo.animals"), "weight")
	first.Aggregates[0] = Count
	if f := catalogField(t, catalogSource(t, Catalog(), "zoo.animals"), "weight"); f.Aggregates[0] != Sum {
		t.Fatalf("the registry changed: %+v", f)
	}
}

func TestCatalogLeavesOutARefusedSource(t *testing.T) {
	z := newZoo(t)
	RegisterSource(Source{Key: "zoo.ghosts", Collection: "zoo_ghosts", Label: "Ghosts", Fields: []Field{{Key: "name", Label: "Name", Type: Text}}})
	if err := CheckSources(z.app); err == nil {
		t.Fatal("CheckSources accepted zoo.ghosts, which has no collection")
	}
	for _, s := range Catalog() {
		if s.Key == "zoo.ghosts" {
			t.Fatal("the catalog offers a refused source")
		}
	}
}

func (z *zoo) get(t *testing.T, name, url, token string, status int, want []string, notWant []string) {
	t.Helper()
	headers := map[string]string{}
	if token != "" {
		headers["Authorization"] = token
	}
	sc := tests.ApiScenario{
		Name: name, Method: http.MethodGet, URL: url, Headers: headers,
		ExpectedStatus: status, ExpectedContent: want, NotExpectedContent: notWant,
		TestAppFactory:        func(testing.TB) *tests.TestApp { return z.app },
		DisableTestAppCleanup: true,
	}
	sc.Test(t)
}

func TestSourcesEndpoint(t *testing.T) {
	z := newZoo(t)
	Register(z.app)
	z.get(t, "a member reads the catalog", "/api/reports/sources", z.token(t, "ann"), http.StatusOK,
		[]string{`"sources":[`, `"key":"zoo.animals"`, `"cardinality":"toMany"`, `"aggregates":["sum","avg","min","max","count","countDistinct"]`},
		// The catalog names sources, never their collections or columns.
		[]string{`zoo_feedings`, `"column"`, `"collection"`})
	z.get(t, "signed out", "/api/reports/sources", "", http.StatusUnauthorized, []string{`"status":401`}, nil)
}

// The catalog describes every package's sources; an OAuth token's scopes cover only
// some, so the route stays session-only, as the run route does.
func TestSourcesEndpointIsNotOpenToOAuthTokens(t *testing.T) {
	if got := oauth.ScopeForRoute(http.MethodGet, "/api/reports/sources"); len(got) != 0 {
		t.Fatalf("GET /api/reports/sources resolved to scopes %v, want default-deny", got)
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'Catalog|SourcesEndpoint' ./reports/`
Expected: FAIL to build: `undefined: CatalogSource`, `undefined: Catalog`.

- [ ] **Step 3: Write the catalog**

`core/server/reports/catalog.go`:

```go
package reports

import (
	"net/http"
	"slices"

	"github.com/pocketbase/pocketbase/core"
)

// CatalogSource is a report source as the builder offers it: its fields and
// links by key and label. It never names the source's collection or columns.
type CatalogSource struct {
	Key    string         `json:"key"`
	Label  string         `json:"label"`
	Fields []CatalogField `json:"fields"`
	Links  []CatalogLink  `json:"links"`
}

// CatalogField is a field the builder can add as a column, filter or group on.
type CatalogField struct {
	Key        string      `json:"key"`
	Label      string      `json:"label"`
	Type       FieldType   `json:"type"`
	Aggregates []Aggregate `json:"aggregates"`
	Options    []Option    `json:"options,omitempty"`
	// Currency is the key of the field that holds each row's currency, for a
	// money field that has one.
	Currency string `json:"currency,omitempty"`
}

// CatalogLink is a link the builder can follow.
type CatalogLink struct {
	Key         string      `json:"key"`
	Label       string      `json:"label"`
	Target      string      `json:"target"`
	Cardinality Cardinality `json:"cardinality"`
}

// Catalog lists the sources a report may use, sorted by key, as copies. A
// field registered with no aggregates offers CountOnly, as the engine reads it.
func Catalog() []CatalogSource {
	all := Sources()
	out := make([]CatalogSource, 0, len(all))
	for _, src := range all {
		cs := CatalogSource{
			Key:    src.Key,
			Label:  src.Label,
			Fields: make([]CatalogField, 0, len(src.Fields)),
			Links:  make([]CatalogLink, 0, len(src.Links)),
		}
		for _, f := range src.Fields {
			aggregates := f.Aggregates
			if aggregates == nil {
				aggregates = CountOnly
			}
			cs.Fields = append(cs.Fields, CatalogField{
				Key: f.Key, Label: f.Label, Type: f.Type,
				Aggregates: slices.Clone(aggregates), Options: slices.Clone(f.Options), Currency: f.Currency,
			})
		}
		for _, l := range src.Links {
			cs.Links = append(cs.Links, CatalogLink{Key: l.Key, Label: l.Label, Target: l.Target, Cardinality: l.Cardinality})
		}
		out = append(out, cs)
	}
	return out
}

// handleSources answers GET /api/reports/sources with the catalog. It reads
// the registry in memory and no collection, so it shows no record data.
func handleSources(re *core.RequestEvent) error {
	return re.JSON(http.StatusOK, map[string]any{"sources": Catalog()})
}
```

In `core/server/reports/endpoint.go`, `Register`, add the route after the run route, and name it in the doc comment:

```go
		e.Router.POST("/api/reports/run", handleRun).
			Bind(apis.RequireAuth("users"), apis.BodyLimit(maxRunBody))
		e.Router.GET("/api/reports/sources", handleSources).
			Bind(apis.RequireAuth("users"))
```

and change the comment line `// signed-in users: a draft spec, or a saved report the caller can view.` to:

```go
// signed-in users: a draft spec, or a saved report the caller can view; and
// GET /api/reports/sources, the catalog the builder offers (Catalog).
```

- [ ] **Step 4: Run the tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'Catalog|SourcesEndpoint' ./reports/`
Expected: `ok  	tinycld.org/core/reports`.

- [ ] **Step 5: Run the package, vet and the composition counts**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -w reports && go test -tags no_ui -count=1 ./reports/... ./coreserver/... && go vet -tags no_ui ./reports/...`
Expected: `ok` for each; the route is bound inside the existing `OnServe` handler, so `TestRegisterBindsTheRecordedHandlerCounts` does not move.

- [ ] **Step 6: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/catalog.go core/server/reports/catalog_test.go core/server/reports/endpoint.go
git commit -m "feat(core): report source catalog for the builder"
```

---

### Task 2: The run result echoes resolved params and says which groups open

**Files:**
- Modify: `core/server/reports/run.go` (`Result`, `Row`, `Run`)
- Modify: `core/server/reports/params.go` (`ResolvedParam`, `resolvedParams`)
- Modify: `core/server/reports/assemble.go` (`groupWithLabel`)
- Create: `core/server/reports/result_extras_test.go`

**Interfaces:**
- Consumes: `paramValue` and `resolveParams` (params.go); `costBySpeciesAndMonth` (pivot_test.go), `feedingsByDay` (run_test.go), `(*zoo).run`.
- Produces: `Result.Params []ResolvedParam` (JSON `params`, always a list); `type ResolvedParam struct{ID, Type string; Unset bool; Value any; List []string; From, To string}` (JSON `id`, `type`, `unset,omitempty`, `value,omitempty`, `list,omitempty`, `from,omitempty`, `to,omitempty`); `Row.CanExpand bool` (JSON `canExpand,omitempty`). TS mirrors: `ResolvedParam`, `ResultRow.canExpand` (Task 5).

- [ ] **Step 1: Write the failing tests**

`core/server/reports/result_extras_test.go`:

```go
package reports

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const threeParams = `{"version":1,"source":"zoo.feedings",
	"params":[
		{"id":"period","type":"dateRange","default":{"preset":"thisMonth"}},
		{"id":"t","type":"text"},
		{"id":"n","type":"number","default":5},
		{"id":"kinds","type":"choice","multiple":true,"options":[{"value":"a","label":"A"},{"value":"b","label":"B"}]}],
	"columns":[{"id":"note","path":[{"field":"note"}]}]}`

func TestARunEchoesEachParamAsItResolved(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", threeParams, Options{Today: "2026-02-15"}, map[string]string{"t": `"lion"`, "kinds": `["b"]`})
	want := []ResolvedParam{
		{ID: "period", Type: "dateRange", From: "2026-02-01", To: "2026-02-28"},
		{ID: "t", Type: "text", Value: "lion"},
		{ID: "n", Type: "number", Value: int64(5)},
		{ID: "kinds", Type: "choice", List: []string{"b"}},
	}
	if !reflect.DeepEqual(res.Params, want) {
		t.Fatalf("params = %+v\nwant     %+v", res.Params, want)
	}
}

func TestAParamWithNoValueAndNoDefaultIsEchoedUnset(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", threeParams, Options{Today: "2026-02-15"}, nil)
	if got := res.Params[1]; got.ID != "t" || !got.Unset || got.Value != nil {
		t.Fatalf("t = %+v", got)
	}
	if got := res.Params[3]; got.ID != "kinds" || !got.Unset || got.List != nil {
		t.Fatalf("kinds = %+v", got)
	}
}

func TestARunWithoutParamsEchoesAnEmptyList(t *testing.T) {
	z := newZoo(t)
	b, err := json.Marshal(z.run(t, "ann", feedingsByDay, Options{}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"params":[]`) {
		t.Fatalf("result JSON lacks an empty params list: %s", b)
	}
}

func TestAGroupRowSaysWhetherItCanOpen(t *testing.T) {
	z := newZoo(t)
	pivot := z.run(t, "ann", fmt.Sprintf(costBySpeciesAndMonth, ""), Options{}, nil)
	if r := pivot.Rows[0]; r.Kind != KindGroup || r.CanExpand {
		t.Fatalf("a pivot's innermost group = %+v, want a group that cannot open", r)
	}
	grouped := z.run(t, "ann", `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species","subtotals":true}]}`, Options{}, nil)
	if r := grouped.Rows[0]; r.Kind != KindGroup || !r.CanExpand {
		t.Fatalf("a group with detail rows = %+v, want one that can open", r)
	}
	if r := grouped.Rows[len(grouped.Rows)-1]; r.Kind != KindTotal || r.CanExpand {
		t.Fatalf("the total = %+v", r)
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'Echo|CanOpen' ./reports/`
Expected: FAIL to build: `undefined: ResolvedParam`, `res.Params undefined`, `r.CanExpand undefined`.

- [ ] **Step 3: Add the fields and fill them**

In `run.go`, `Result`, add after `Page`:

```go
	// Params are the report's params as this run resolved them, in spec
	// order, so the shell shows the period the server ran.
	Params []ResolvedParam `json:"params"`
```

In `run.go`, `Row`, add after `Expanded`:

```go
	// CanExpand says a group row has rows to open (false for a pivot's
	// innermost group), so the client never repeats that rule.
	CanExpand bool `json:"canExpand,omitempty"`
```

In `run.go`, `Run`, replace

```go
	if err := rn.run(); err != nil {
		return nil, err
	}
	return rn.res, nil
```

with

```go
	if err := rn.run(); err != nil {
		return nil, err
	}
	rn.res.Params = resolvedParams(p.spec.Params, params)
	return rn.res, nil
```

In `params.go`, after `resolveParams`, add:

```go
// ResolvedParam is one param as a run resolved it: the posted value, else its
// default, else unset. A preset is given as its days, so the shell shows the
// period the server ran without repeating the fiscal-year rules.
type ResolvedParam struct {
	ID    string   `json:"id"`
	Type  string   `json:"type"`
	Unset bool     `json:"unset,omitempty"`
	Value any      `json:"value,omitempty"`
	List  []string `json:"list,omitempty"`
	From  string   `json:"from,omitempty"`
	To    string   `json:"to,omitempty"`
}

// resolvedParams lists defs in order with the values a run resolved for them.
func resolvedParams(defs []ParamDef, values map[string]paramValue) []ResolvedParam {
	out := make([]ResolvedParam, 0, len(defs))
	for _, d := range defs {
		v := values[d.ID]
		rp := ResolvedParam{ID: d.ID, Type: d.Type, Unset: v.unset}
		if !v.unset {
			rp.Value, rp.List, rp.From, rp.To = v.value, v.list, v.from, v.to
		}
		out = append(out, rp)
	}
	return out
}
```

In `assemble.go`, `groupWithLabel`, change the pushed group row to carry `canOpen`:

```go
	rn.push(Row{ID: id, Kind: KindGroup, Depth: n - 1, Label: label, Section: section, GroupKey: key, Expanded: expanded, CanExpand: canOpen, Cells: rn.cells(row)})
```

- [ ] **Step 4: Run the tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'Echo|CanOpen' ./reports/`
Expected: `ok`.

- [ ] **Step 5: Run the package and vet**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -w reports && go test -tags no_ui -count=1 ./reports/... && go vet -tags no_ui ./reports/...`
Expected: `ok`. (`gofmt` realigns the struct tags of `Result` and `Row`. Endpoint tests match substrings, so the added `params` key does not break them; the plan writer ran Tasks 1–2 in a scratch copy of `core/server` and the whole `reports` and `coreserver` packages passed.)

- [ ] **Step 6: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/run.go core/server/reports/params.go core/server/reports/assemble.go core/server/reports/result_extras_test.go
git commit -m "feat(core): report results echo resolved params and mark groups that open"
```

---

### Task 3: The generator keeps the reports area

**Files:**
- Modify: `scripts/gen-routes.ts` (export `APP_OWNED_ORG_ROUTE_DIRS`, `assertSlugNotAppOwned`; guard `emitRoutes`; `pruneOrphanRouteDirs` takes a `ReadonlySet`)
- Modify: `scripts/generate.ts` (import the set; guard `emitOrgRoutes` before its `rmSync`)
- Test: `scripts/__tests__/gen-routes.test.ts`

**Interfaces:**
- Produces: `export const APP_OWNED_ORG_ROUTE_DIRS: ReadonlySet<string>` (`help`, `reports`, `settings`); `export function assertSlugNotAppOwned(slug: string): void`.

- [ ] **Step 1: Write the failing tests**

Append to `scripts/__tests__/gen-routes.test.ts` (and add `APP_OWNED_ORG_ROUTE_DIRS` to its import from `'../gen-routes'`):

```ts
describe('app-owned org route dirs', () => {
    let base: string
    beforeEach(() => {
        base = fs.mkdtempSync(path.join(os.tmpdir(), 'tcld-owned-'))
        for (const dir of ['help', 'reports', 'settings', 'gone']) fs.mkdirSync(path.join(base, dir))
    })
    afterEach(() => fs.rmSync(base, { recursive: true, force: true }))

    it('keeps the reports, help and settings areas when pruning', () => {
        const pruned = pruneOrphanRouteDirs(base, new Set(), APP_OWNED_ORG_ROUTE_DIRS)
        expect(pruned).toEqual(['gone'])
        for (const dir of ['help', 'reports', 'settings'])
            expect(fs.existsSync(path.join(base, dir))).toBe(true)
    })

    it('refuses a package whose slug is an app route', () => {
        expect(() =>
            emitRoutes({
                packageName: '@tinycld/reports',
                slug: 'reports',
                packageDir: base,
                routesDir: 'screens',
                importSubpath: 'screens',
                routesBase: base,
            })
        ).toThrow("package slug 'reports' is an app route")
    })
})
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run scripts/__tests__/gen-routes.test.ts`
Expected: FAIL: `APP_OWNED_ORG_ROUTE_DIRS` is not exported (`undefined` passed to prune), and the slug test does not throw.

- [ ] **Step 3: Implement**

In `scripts/gen-routes.ts`, after `const ROUTE_EXTS = …`, add:

```ts
// Direct children of the org routes dir (app/a/(app)) that the app owns. They are
// never pruned, and no package may take one as its slug: emitting that package's
// routes would first delete the app's own route files.
export const APP_OWNED_ORG_ROUTE_DIRS: ReadonlySet<string> = new Set(['help', 'reports', 'settings'])

export function assertSlugNotAppOwned(slug: string): void {
    if (APP_OWNED_ORG_ROUTE_DIRS.has(slug)) {
        throw new Error(`[generate] package slug '${slug}' is an app route (/a/${slug}); choose another slug`)
    }
}
```

In `emitRoutes`, make the first line `assertSlugNotAppOwned(opts.slug)`. Change `pruneOrphanRouteDirs`'s parameter `appOwnedEntries: Set<string>` to `appOwnedEntries: ReadonlySet<string>`.

In `scripts/generate.ts`: change the import to `import { APP_OWNED_ORG_ROUTE_DIRS, assertSlugNotAppOwned, emitPublicRoutes, emitRoutes, pruneOrphanRouteDirs } from './gen-routes'`; delete the line `const APP_OWNED_ORG_ROUTE_DIRS = new Set(['help', 'settings'])` and replace its comment's first line with `// Direct children of ROUTES_BASE the app owns are listed in gen-routes.ts.`; in `emitOrgRoutes`, add `assertSlugNotAppOwned(slug)` directly after `assertSafeSlug(slug)` (before the `rmSync`).

- [ ] **Step 4: Run the tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run scripts/__tests__/gen-routes.test.ts`
Expected: all pass (8 tests).

- [ ] **Step 5: Lint and typecheck the scripts**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome check scripts/gen-routes.ts scripts/generate.ts scripts/__tests__/gen-routes.test.ts && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg typecheck`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add scripts/gen-routes.ts scripts/generate.ts scripts/__tests__/gen-routes.test.ts
git commit -m "fix(app): generator keeps the reports area and refuses app route slugs"
```

---

### Task 4: The reports e2e stub package

**Files:**
- Create: `tests/scripts/scaffold-reports-stub.ts`
- Create: `tests/scripts/reports-stub/pb-migrations/2090000300_salesstub.js.txt`
- Create: `tests/scripts/reports-stub/server/register.go.txt`
- Create: `tests/scripts/reports-stub/screens/index.tsx.txt`
- Modify: `.github/workflows/ci.yml` (scaffold step)
- Modify: `tests/e2e/helpers.ts` (`skipWithoutReportsStub`, `reports` chrome heading)

**Interfaces:**
- Produces (fixture names the specs rely on): package dir `salesstub`, slug `salesstub`, nav label "Sales Stub"; collections `salesstub_teams` (`name`) and `salesstub_sales` (`owner`, `team`, `customer`, `amount`, `cost`, `currency`, `day`, `status` open|paid); sources `salesstub.sales` (label "Stub sales"; fields `customer` Customer, `amount` Amount, `cost` Cost, `currency` Currency, `day` Day, `status` Status; link `team` Team → `salesstub.teams`) and `salesstub.teams` (label "Stub teams"; field `name` Name; to-many link `sales` Sales → `salesstub.sales`); landing test ids `salesstub-landing`, `salesstub-team-name`, `salesstub-team-create`, `salesstub-team-saved`, `salesstub-sale-team` (SelectInput; items `salesstub-sale-team-<teamId>`), `salesstub-sale-customer`, `salesstub-sale-amount`, `salesstub-sale-cost`, `salesstub-sale-day`, `salesstub-sale-create`, `salesstub-sale-saved`; status buttons labelled "Open" and "Paid". `skipWithoutReportsStub(): boolean` in `tests/e2e/helpers.ts`.

- [ ] **Step 1: Write the migration template**

`tests/scripts/reports-stub/pb-migrations/2090000300_salesstub.js.txt`:

```js
/// <reference path="../../tinycld/server/pb_data/types.d.ts" />
// E2E fixture: the collections behind the salesstub report sources.
// Teams are shared by every signed-in user; a sale belongs to its owner alone,
// so a report over sales shows each reader only their own rows.
const SIGNED_IN = '@request.auth.id != ""'
const OWN = '@request.auth.id != "" && owner = @request.auth.id'

function stamps(prefix) {
    return [
        { id: `${prefix}_created`, name: 'created', type: 'autodate', onCreate: true, onUpdate: false },
        { id: `${prefix}_updated`, name: 'updated', type: 'autodate', onCreate: true, onUpdate: true },
    ]
}

migrate(
    app => {
        const users = app.findCollectionByNameOrId('users')
        const teams = new Collection({
            id: 'pbc_salesstub_teams',
            name: 'salesstub_teams',
            type: 'base',
            system: false,
            listRule: SIGNED_IN,
            viewRule: SIGNED_IN,
            createRule: SIGNED_IN,
            updateRule: SIGNED_IN,
            deleteRule: SIGNED_IN,
            fields: [
                { id: 'salesstub_teams_name', name: 'name', type: 'text', required: true, max: 200 },
                ...stamps('salesstub_teams'),
            ],
        })
        app.save(teams)
        const sales = new Collection({
            id: 'pbc_salesstub_sales',
            name: 'salesstub_sales',
            type: 'base',
            system: false,
            listRule: OWN,
            viewRule: OWN,
            createRule: OWN,
            updateRule: OWN,
            deleteRule: OWN,
            fields: [
                { id: 'salesstub_sales_owner', name: 'owner', type: 'relation', collectionId: users.id, required: true, maxSelect: 1, cascadeDelete: false },
                { id: 'salesstub_sales_team', name: 'team', type: 'relation', collectionId: teams.id, maxSelect: 1, cascadeDelete: false },
                { id: 'salesstub_sales_customer', name: 'customer', type: 'text', max: 200 },
                { id: 'salesstub_sales_amount', name: 'amount', type: 'number', onlyInt: true },
                { id: 'salesstub_sales_cost', name: 'cost', type: 'number', onlyInt: true },
                { id: 'salesstub_sales_currency', name: 'currency', type: 'text', max: 3 },
                { id: 'salesstub_sales_day', name: 'day', type: 'date' },
                { id: 'salesstub_sales_status', name: 'status', type: 'select', values: ['open', 'paid'], maxSelect: 1 },
                ...stamps('salesstub_sales'),
            ],
        })
        app.save(sales)
    },
    app => {
        app.delete(app.findCollectionByNameOrId('salesstub_sales'))
        app.delete(app.findCollectionByNameOrId('salesstub_teams'))
    }
)
```

- [ ] **Step 2: Write the Go sources template**

`tests/scripts/reports-stub/server/register.go.txt`:

```go
// Package stub is an E2E fixture: two report sources over its own collections,
// so the app shell's reports e2e has a to-one link, a to-many link, money, a
// date and a select with no real feature package installed. See
// tinycld/tests/scripts/scaffold-reports-stub.ts for why.
package stub

import (
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/reports"
)

// Register is the entry point the generated package_extensions.go calls.
func Register(_ core.App) error {
	reports.RegisterSource(reports.Source{
		Key: "salesstub.sales", Collection: "salesstub_sales", Label: "Stub sales",
		Fields: []reports.Field{
			{Key: "customer", Label: "Customer", Type: reports.Text, Aggregates: reports.CountOnly},
			{Key: "amount", Label: "Amount", Type: reports.Money, Aggregates: reports.SumAll, Currency: "currency"},
			{Key: "cost", Label: "Cost", Type: reports.Money, Aggregates: reports.SumAll, Currency: "currency"},
			{Key: "currency", Label: "Currency", Type: reports.Text},
			{Key: "day", Label: "Day", Type: reports.Date, Aggregates: reports.MinMax},
			{Key: "status", Label: "Status", Type: reports.Select, Options: []reports.Option{
				{Value: "open", Label: "Open"}, {Value: "paid", Label: "Paid"},
			}},
		},
		Links: []reports.Link{
			{Key: "team", Label: "Team", Target: "salesstub.teams", Cardinality: reports.ToOne, Field: "team"},
		},
	})
	reports.RegisterSource(reports.Source{
		Key: "salesstub.teams", Collection: "salesstub_teams", Label: "Stub teams",
		Fields: []reports.Field{{Key: "name", Label: "Name", Type: reports.Text, Aggregates: reports.MinMax}},
		Links: []reports.Link{
			{Key: "sales", Label: "Sales", Target: "salesstub.sales", Cardinality: reports.ToMany, BackField: "team"},
		},
	})
	return nil
}
```

- [ ] **Step 3: Write the landing screen template**

`tests/scripts/reports-stub/screens/index.tsx.txt`:

```tsx
import { useLiveQuery } from '@tanstack/react-db'
import { useAuth } from '@tinycld/core/lib/auth'
import { useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import {
    ButtonGroup,
    NumberInput,
    SelectInput,
    TextInput,
    useForm,
    z,
    zodResolver,
} from '@tinycld/core/ui/form'
import { newRecordId } from 'pbtsdb/core'
import { useState } from 'react'
import { ScrollView, Text, View } from 'react-native'

// E2E fixture screen: adds teams and sales through the UI, as the reports specs
// must set their data up.

const teamSchema = z.object({ name: z.string().trim().min(1).max(200) })
const saleSchema = z.object({
    team: z.string().min(1, 'Choose a team'),
    customer: z.string().trim().min(1).max(200),
    amount: z.number().int(),
    cost: z.number().int(),
    day: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, 'Enter a date as YYYY-MM-DD'),
    status: z.enum(['open', 'paid']),
})
type SaleValues = z.infer<typeof saleSchema>

const STATUS_OPTIONS = [
    { value: 'open', label: 'Open' },
    { value: 'paid', label: 'Paid' },
]

function SavedNote({ text, testID }: { text: string; testID: string }) {
    if (!text) return null
    return (
        <Text testID={testID} className="text-sm text-muted-foreground">
            {text}
        </Text>
    )
}

function TeamForm() {
    const [teams] = useStore('salesstub_teams')
    const [saved, setSaved] = useState('')
    const form = useForm({ resolver: zodResolver(teamSchema), mode: 'onChange', defaultValues: { name: '' } })
    const create = useMutation({
        mutationFn: function* (values: z.infer<typeof teamSchema>) {
            yield teams.insert({ id: newRecordId(), name: values.name })
        },
        onSuccess: (_data, values) => {
            setSaved(`Saved ${values.name}`)
            form.reset({ name: '' })
        },
    })
    const onCreate = form.handleSubmit(values => create.mutate(values))
    return (
        <View className="gap-2">
            <Text className="text-base font-semibold text-foreground">New team</Text>
            <TextInput name="name" control={form.control} label="Team name" testID="salesstub-team-name" />
            <Button onPress={onCreate} isDisabled={create.isPending} testID="salesstub-team-create">
                <ButtonText>Add team</ButtonText>
            </Button>
            <SavedNote text={saved} testID="salesstub-team-saved" />
        </View>
    )
}

function SaleForm() {
    const { user } = useAuth()
    const [teams, sales] = useStore('salesstub_teams', 'salesstub_sales')
    const [saved, setSaved] = useState('')
    const { data: teamRows } = useLiveQuery({
        queryKey: ['salesstub.teams'],
        query: q => q.from({ team: teams }).orderBy(({ team }) => team.name),
    })
    const teamOptions = (teamRows ?? []).map(team => ({ value: team.id, label: team.name }))
    const form = useForm<SaleValues>({
        resolver: zodResolver(saleSchema),
        mode: 'onChange',
        defaultValues: { team: '', customer: '', amount: 0, cost: 0, day: '', status: 'open' },
    })
    const create = useMutation({
        mutationFn: function* (values: SaleValues) {
            yield sales.insert({
                id: newRecordId(),
                owner: user.id,
                team: values.team,
                customer: values.customer,
                amount: values.amount,
                cost: values.cost,
                currency: 'USD',
                day: `${values.day} 00:00:00.000Z`,
                status: values.status,
            })
        },
        onSuccess: (_data, values) => {
            setSaved(`Saved ${values.customer}`)
            form.reset({ ...values, customer: '', amount: 0, cost: 0 })
        },
    })
    const onCreate = form.handleSubmit(values => create.mutate(values))
    return (
        <View className="gap-2">
            <Text className="text-base font-semibold text-foreground">New sale</Text>
            <SelectInput name="team" control={form.control} label="Team" options={teamOptions} testID="salesstub-sale-team" />
            <TextInput name="customer" control={form.control} label="Customer" testID="salesstub-sale-customer" />
            <NumberInput name="amount" control={form.control} label="Amount (cents)" testID="salesstub-sale-amount" />
            <NumberInput name="cost" control={form.control} label="Cost (cents)" testID="salesstub-sale-cost" />
            <TextInput name="day" control={form.control} label="Day" placeholder="YYYY-MM-DD" testID="salesstub-sale-day" />
            <ButtonGroup name="status" control={form.control} label="Status" horizontal options={STATUS_OPTIONS} />
            <Button onPress={onCreate} isDisabled={create.isPending} testID="salesstub-sale-create">
                <ButtonText>Add sale</ButtonText>
            </Button>
            <SavedNote text={saved} testID="salesstub-sale-saved" />
        </View>
    )
}

export default function ReportStubIndex() {
    return (
        <ScrollView className="flex-1 bg-background" contentContainerClassName="gap-6 p-4">
            <Text testID="salesstub-landing" className="text-lg font-semibold text-foreground">
                Sales stub
            </Text>
            <TeamForm />
            <SaleForm />
        </ScrollView>
    )
}
```

- [ ] **Step 4: Write the scaffolder**

`tests/scripts/scaffold-reports-stub.ts`:

```ts
#!/usr/bin/env tsx
/**
 * scaffold-reports-stub.ts — provisions the @tinycld/salesstub package into the
 * workspace that contains the app shell.
 *
 * Why a stub: the reports hub, view and builder read report sources, which only
 * packages register, and core must not name a package. App-shell CI installs no
 * feature package, so without a stub the reports e2e (tests/e2e/reports-*.spec.ts)
 * has nothing to report on. The stub registers two sources from Go (a to-one and
 * a to-many link, money with a currency field, a date and a select) over its own
 * collections, and its landing screen adds rows through the UI, as the specs must.
 *
 * Writes every file itself, as scaffold-forms-host-stub.ts does: no bootstrap
 * preset ships a migration, a Go module and collections together. The screen,
 * the migration and the Go source are kept as .txt templates in
 * tests/scripts/reports-stub/ and copied as they are. The Go module pins the Go and
 * PocketBase versions core's go.mod uses, as scaffold-search-stubs.ts explains.
 *
 * Invocation mirrors the other scaffolders:
 *   - CI: `pnpm exec tsx tinycld/tests/scripts/scaffold-reports-stub.ts` from the
 *     workspace root, after `pnpm install` and before the bundle build.
 *   - Local: run once before `tinycld-pkg test:e2e`.
 *
 * Idempotent: re-running re-emits the sources and installs only when the stub is
 * not yet a workspace member.
 *
 * Fixture names the specs assert on are listed in tests/e2e/reports-helpers.ts.
 */

import { spawnSync } from 'node:child_process'
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export const STUB_DIR = 'salesstub'
export const STUB_SLUG = 'salesstub'
export const STUB_PACKAGE = '@tinycld/salesstub'
const TEMPLATES_DIR = resolve(fileURLToPath(import.meta.url), '..', 'reports-stub')
const TEMPLATES: [string, string][] = [
    ['screens/index.tsx.txt', `tinycld/${STUB_SLUG}/screens/index.tsx`],
    ['pb-migrations/2090000300_salesstub.js.txt', 'pb-migrations/2090000300_salesstub.js'],
    ['server/register.go.txt', 'server/register.go'],
]
const SUBPROCESS_TIMEOUT_MS = 5 * 60_000

function workspaceRoot(): string {
    return resolve(fileURLToPath(import.meta.url), '..', '..', '..', '..')
}

function write(path: string, contents: string): void {
    mkdirSync(resolve(path, '..'), { recursive: true })
    writeFileSync(path, contents)
}

function goVersions(wsRoot: string): { go: string; pocketbase: string } {
    const modPath = join(wsRoot, 'tinycld', 'core', 'server', 'go.mod')
    const mod = readFileSync(modPath, 'utf8')
    const go = mod.match(/^go\s+(\S+)/m)?.[1]
    const pocketbase = mod.match(/github\.com\/pocketbase\/pocketbase\s+(v\S+)/)?.[1]
    if (!go || !pocketbase) throw new Error(`could not read go/pocketbase versions from ${modPath}`)
    return { go, pocketbase }
}

function writePackage(stubDir: string): void {
    const src = `./tinycld/${STUB_SLUG}`
    const pkg = {
        name: STUB_PACKAGE,
        version: '0.1.0',
        private: true,
        type: 'module',
        exports: {
            './package.json': './package.json',
            './manifest': './manifest.ts',
            './collections': `${src}/collections.ts`,
            './types': `${src}/types.ts`,
            './screens/*': `${src}/screens/*.tsx`,
        },
        peerDependencies: {},
    }
    write(join(stubDir, 'package.json'), `${JSON.stringify(pkg, null, 4)}\n`)
    write(
        join(stubDir, 'tsconfig.json'),
        `{
    "extends": "@tinycld/core/tsconfig.package-base.json",
    "compilerOptions": {
        "paths": {
            "~/tinycld/${STUB_SLUG}/*": ["./tinycld/${STUB_SLUG}/*"]
        }
    },
    "include": ["tinycld/**/*.ts", "tinycld/**/*.tsx"],
    "exclude": ["node_modules", "pb-migrations", "server"]
}
`
    )
}

function writeManifest(stubDir: string): void {
    write(
        join(stubDir, 'manifest.ts'),
        `const manifest = {
    name: 'Sales Stub',
    slug: '${STUB_SLUG}',
    version: '0.1.0',
    description: 'app E2E reports stub',
    routes: { directory: 'screens' },
    nav: { label: 'Sales Stub', icon: 'flask-conical', order: 997 },
    migrations: { directory: 'pb-migrations' },
    collections: { register: 'collections', types: 'types' },
    server: { package: 'server', module: 'tinycld.org/packages/${STUB_SLUG}' },
}

export default manifest
`
    )
}

function writeSources(stubDir: string): void {
    const dir = join(stubDir, 'tinycld', STUB_SLUG)
    write(
        join(dir, 'collections.ts'),
        `import type { Schema } from '@tinycld/core/types/pbSchema'
import type { createCollection } from 'pbtsdb/core'

const onDemand = { syncMode: 'on-demand', realtime: 'query' } as const

export function registerCollections(newCollection: ReturnType<typeof createCollection<Schema>>) {
    const salesstub_teams = newCollection('salesstub_teams', {
        omitOnInsert: ['created', 'updated'] as const,
        ...onDemand,
    })
    const salesstub_sales = newCollection('salesstub_sales', {
        omitOnInsert: ['created', 'updated'] as const,
        ...onDemand,
    })
    return { salesstub_teams, salesstub_sales }
}
`
    )
    write(
        join(dir, 'types.ts'),
        `import type { Schema } from '@tinycld/core/types/pbSchema'

export type ReportStubSchema = Pick<Schema, 'salesstub_teams' | 'salesstub_sales'>
`
    )
}

function writeGoModule(stubDir: string, versions: { go: string; pocketbase: string }): void {
    write(
        join(stubDir, 'server', 'go.mod'),
        `module tinycld.org/packages/${STUB_SLUG}

go ${versions.go}

require (
	github.com/pocketbase/pocketbase ${versions.pocketbase}
	tinycld.org/core v0.0.0
)
`
    )
}

function copyTemplates(stubDir: string): void {
    for (const [from, to] of TEMPLATES) write(join(stubDir, to), readFileSync(join(TEMPLATES_DIR, from), 'utf8'))
}

function ensureMember(wsRoot: string): boolean {
    const path = join(wsRoot, 'package.json')
    const pkg = JSON.parse(readFileSync(path, 'utf8'))
    const workspaces: string[] = Array.isArray(pkg.workspaces) ? pkg.workspaces : []
    const missing = !workspaces.includes(STUB_DIR)
    if (missing) {
        pkg.workspaces = [...workspaces, STUB_DIR]
        writeFileSync(path, `${JSON.stringify(pkg, null, 4)}\n`)
    }
    return ensurePnpmMember(join(wsRoot, 'pnpm-workspace.yaml')) || missing
}

function ensurePnpmMember(yamlPath: string): boolean {
    const lines = readFileSync(yamlPath, 'utf8').split('\n')
    const pkgIdx = lines.findIndex(l => /^packages:\s*$/.test(l))
    if (pkgIdx === -1) throw new Error(`no packages: block in ${yamlPath}`)
    let lastEntry = pkgIdx
    for (let i = pkgIdx + 1; i < lines.length; i++) {
        const line = lines[i] ?? ''
        if (line.trim() === `- ${STUB_DIR}`) return false
        if (/^\s+-\s+/.test(line)) lastEntry = i
        else if (line.trim() !== '' && !line.startsWith(' ')) break
    }
    const indent = (lines[lastEntry] ?? '').match(/^\s*/)?.[0] ?? '  '
    lines.splice(lastEntry + 1, 0, `${indent}- ${STUB_DIR}`)
    writeFileSync(yamlPath, lines.join('\n'))
    return true
}

function run(cmd: string, args: string[], cwd: string, what: string): void {
    const env = { ...process.env, COREPACK_ENABLE_DOWNLOAD_PROMPT: '0' }
    const r = spawnSync(cmd, args, { cwd, stdio: 'inherit', timeout: SUBPROCESS_TIMEOUT_MS, env })
    if (r.status !== 0) {
        const cause = r.error ? `: ${r.error.message}` : ''
        throw new Error(`${what} failed (status ${r.status})${cause}; the reports stub will not load`)
    }
}

function main(): void {
    const wsRoot = workspaceRoot()
    console.log(`[scaffold-reports-stub] workspace root: ${wsRoot}`)
    const stubDir = join(wsRoot, STUB_DIR)
    writePackage(stubDir)
    writeManifest(stubDir)
    writeSources(stubDir)
    writeGoModule(stubDir, goVersions(wsRoot))
    copyTemplates(stubDir)
    if (ensureMember(wsRoot)) {
        spawnSync('corepack', ['enable'], { cwd: wsRoot, stdio: 'inherit', timeout: SUBPROCESS_TIMEOUT_MS })
        run('pnpm', ['install', '--no-frozen-lockfile'], wsRoot, 'pnpm install')
    }
    // Links the stub's routes, migration, collections and Go module into the app.
    run('pnpm', ['run', 'packages:generate'], join(wsRoot, 'tinycld'), 'packages:generate')
    console.log(`[scaffold-reports-stub] done — ${STUB_DIR}/ ready at ${stubDir}`)
}

main()
```

- [ ] **Step 5: Add the CI step**

In `.github/workflows/ci.yml`, after the "Scaffold forms-host stub package" step, add:

```yaml
      # The reports hub, view and builder need report sources, which only
      # packages register, and app-shell CI installs none. Must precede the
      # bundle build so the stub's route, migration and Go sources are served.
      - name: Scaffold reports stub package
        working-directory: ws
        run: pnpm exec tsx tinycld/tests/scripts/scaffold-reports-stub.ts
```

- [ ] **Step 6: Add the e2e guard and the chrome heading**

In `tests/e2e/helpers.ts`, after `skipWithoutFormsStub`, add:

```ts
// The reports specs drive the salesstub package (tests/scripts/scaffold-reports-stub.ts),
// whose report sources and rows exist only there. Same rule as skipWithoutFormsStub:
// skip in a dev workspace without it, fail loudly on CI, where the scaffold step runs.
export function skipWithoutReportsStub(): boolean {
    const stubDir = path.resolve(import.meta.dirname, '..', '..', '..', 'salesstub')
    if (fs.existsSync(stubDir)) return false
    if (process.env.CI) {
        throw new Error(
            'salesstub is not scaffolded but CI is set — the reports stub scaffold step must run before e2e.'
        )
    }
    return true
}
```

and in `navigateToPackage` change the chrome map to

```ts
    const CHROME_HEADINGS: Record<string, string> = {
        settings: 'Settings',
        help: 'Help',
        reports: 'Reports',
    }
```

and its comment's line `// settings/help are app chrome, not packages:` to `// settings/help/reports are app chrome, not packages:`.

- [ ] **Step 7: Scaffold the stub in this workspace and check it**

Run: `cd /Users/nas/tmp/tinycld/books-purchases && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tsx tinycld/tests/scripts/scaffold-reports-stub.ts`
Expected: `[scaffold-reports-stub] done`; `salesstub/` exists at the workspace root. (The script runs the workspace install and the generator: this is the one place an executor runs them, as every stub scaffold does.)

Run: `cd /Users/nas/tmp/tinycld/books-purchases && grep -n salesstub tinycld/server/package_extensions.go && ls tinycld/server/pb_migrations | grep salesstub && grep -n "salesstub_sales" tinycld/core/types/pbSchema.ts | head -3`
Expected: the Go registration, the migration link and the generated row type are present.

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/server && go vet .`
Expected: clean (the stub's Go module builds with the server).

Run: `cd /Users/nas/tmp/tinycld/books-purchases/salesstub && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tsc --noEmit -p tsconfig.json && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome check .`
Expected: clean. (A Biome format complaint about the templates is fixed in the `.txt` template, then the scaffolder is run again.)

- [ ] **Step 8: Lint the scaffolder and helpers, then commit**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome check tests/scripts/scaffold-reports-stub.ts tests/e2e/helpers.ts`
Expected: clean.

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add tests/scripts/scaffold-reports-stub.ts tests/scripts/reports-stub .github/workflows/ci.yml tests/e2e/helpers.ts
git commit -m "test(app): reports stub package for the reports e2e"
```

---

### Task 4a: navigateToPackage matches the package's own href (retro check, approved 2026-11-01)

**Why:** `tests/e2e/helpers.ts` `navigateToPackage` finds the rail link with `a[href^="${appHref(pkg)}"]`, so a slug that is a prefix of another route (`reports` vs a package `reportsx`, or the reverse) clicks the wrong link.

**Files:** Modify `tests/e2e/helpers.ts` (the `railLink` locator in `navigateToPackage`).

- [ ] Change the locator to match the exact package root or a path under it: ``page.locator(`a[href="${appHref(pkg)}"], a[href^="${appHref(pkg)}/"]`).first()``. Keep the comment's reason (deep-linked rail hrefs such as `/a/calc/<id>`) and add that a bare prefix also matches other slugs.
- [ ] Proof: after Task 13 adds the Reports rail button, the `salesstub` specs and one existing spec of a deep-linking package's pattern still pass (`E2E_PORT=7210 E2E_PORT_2=7211 … tinycld-pkg test:e2e -- reports-create.spec.ts`); run biome and tsc on the file.
- [ ] Commit: `test(e2e): navigateToPackage matches the package's own href, not any prefix`.

### Task 5: Collections, result types, the spec schema and report access

**Files:**
- Modify: `core/lib/pocketbase.ts` (register `reports`, `report_shares`)
- Modify: `core/package.json` (export `./reports/*`)
- Create: `core/reports/lib/types.ts`
- Create: `core/reports/lib/spec.ts`
- Create: `core/reports/lib/access.ts`
- Test: `core/reports/lib/__tests__/spec.test.ts`, `core/reports/lib/__tests__/access.test.ts`

**Interfaces:**
- Consumes: the Go JSON of Tasks 1–2; `z` from `@tinycld/core/ui/form`.
- Produces (`types.ts`): `FIELD_TYPES`, `FieldType`, `AGGREGATES`, `Aggregate`, `RowKind`, `CellValue`, `PARAM_TYPES`, `ParamType`, `SelectOption {value,label}`, `ResultColumn`, `ResultRow`, `Notice`, `ResolvedParam`, `RunPage`, `RunResult`, `CatalogField`, `CatalogLink`, `CatalogSource`, `Problem {path, code, message}`.
- Produces (`spec.ts`): `CURRENT_SPEC_VERSION = 1`, `BUCKETS`, `Bucket`, `specHopSchema`, `SpecHop`, `SpecNode`, `specNodeSchema`, `paramDefSchema`, `ParamDef`, `specColumnSchema`, `SpecColumn`, `reportSpecSchema`, `ReportSpec`, `brokenSchema`, `BrokenMark`, `parseSpec(value: unknown): ReportSpec | null`, `parseBroken(value: unknown): BrokenMark | null`, `brokenProblems(mark: BrokenMark): Problem[]`.
- Produces (`access.ts`): `SHARE_ROLES` (`[{value:'viewer',label:'Viewer'},{value:'editor',label:'Editor'}]`), `ShareRole`, `ReportAccess {isOwner, canEdit, canShare, canPublish, canDelete, isShipped}`, `NO_ACCESS`, `reportAccess(input)`, `rolesByReport(shares)`.
- Produces: pbtsdb stores `reports` and `report_shares` (`useStore('reports', 'report_shares')`).

- [ ] **Step 1: Write the failing tests**

`core/reports/lib/__tests__/spec.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { brokenProblems, parseBroken, parseSpec } from '../spec'

const FEEDINGS = {
    version: 1,
    source: 'zoo.feedings',
    params: [{ id: 'period', type: 'dateRange', default: { preset: 'thisMonth' } }],
    columns: [
        { id: 'note', path: [{ field: 'note' }] },
        { id: 'species', path: [{ link: 'animal' }, { field: 'species' }], label: 'Species' },
        { id: 'cost', path: [{ field: 'cost' }], aggregate: 'sum' },
        { id: 'half', formula: { op: 'div', args: [{ col: 'cost' }, { value: 2 }], scale: 0 } },
    ],
    filter: {
        op: 'and',
        args: [
            { op: 'between', args: [{ path: [{ field: 'fed' }] }, { param: 'period' }] },
            { op: 'exists', link: [{ link: 'animal' }, { link: 'keepers' }] },
        ],
    },
    sort: [{ col: 'note', dir: 'desc' }],
    groups: [{ id: 'g', col: 'species', subtotals: true }],
}

describe('parseSpec', () => {
    it('reads a stored spec as it is', () => {
        expect(parseSpec(FEEDINGS)).toEqual(FEEDINGS)
    })

    it('reads null parts as absent', () => {
        expect(parseSpec({ ...FEEDINGS, layout: null, pivot: null, filter: null })).not.toBeNull()
    })

    it('reads a pivot and a layout with a computed row', () => {
        const spec = parseSpec({
            ...FEEDINGS,
            pivot: { path: [{ field: 'fed' }], bucket: 'month', cell: 'cost', maxColumns: 12 },
            layout: [
                { section: 'lions', title: 'Lions', key: 'lion' },
                { row: 'net', label: 'Net', formula: { op: 'sub', args: [{ subtotal: 'lions' }, { value: 0 }] } },
            ],
        })
        expect(spec?.pivot?.bucket).toBe('month')
        expect(spec?.layout?.[1]?.row).toBe('net')
    })

    it('refuses what is not a spec', () => {
        expect(parseSpec(null)).toBeNull()
        expect(parseSpec('a spec')).toBeNull()
        expect(parseSpec({ version: 1, source: 'zoo.feedings' })).toBeNull()
        expect(parseSpec({ ...FEEDINGS, sql: 'DELETE FROM zoo_feedings' })).toBeNull()
        expect(parseSpec({ ...FEEDINGS, columns: [{ id: 'a', path: [{ field: 'x', mode: 'sideways' }] }] })).toBeNull()
    })
})

describe('parseBroken', () => {
    it('lists each problem by spec path, in path order', () => {
        const mark = parseBroken({
            status: 422,
            problems: {
                source: { code: 'unknown_source', message: 'the source "zoo.gone" does not exist' },
                'columns.0.path.0.field': { code: 'unknown_field', message: 'no field "x"' },
            },
            more: 2,
        })
        expect(mark?.status).toBe(422)
        expect(mark?.more).toBe(2)
        expect(mark && brokenProblems(mark)).toEqual([
            { path: 'columns.0.path.0.field', code: 'unknown_field', message: 'no field "x"' },
            { path: 'source', code: 'unknown_source', message: 'the source "zoo.gone" does not exist' },
        ])
    })

    it('reads no mark as null', () => {
        expect(parseBroken(null)).toBeNull()
        expect(parseBroken(undefined)).toBeNull()
        expect(parseBroken({ status: 'bad' })).toBeNull()
    })
})
```

`core/reports/lib/__tests__/access.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { reportAccess, rolesByReport, type ShareRole } from '../access'

type Case = {
    name: string
    owner: string
    published: boolean
    shippedKey: string
    roles: ShareRole[]
    isAdmin: boolean
    want: { canEdit: boolean; canShare: boolean; canPublish: boolean; canDelete: boolean }
}

// The who-sees-what table of the sharing contracts, as the hub and view use it.
const CASES: Case[] = [
    { name: 'owner', owner: 'me', published: false, shippedKey: '', roles: [], isAdmin: false, want: { canEdit: true, canShare: true, canPublish: false, canDelete: true } },
    { name: 'owner who is an admin', owner: 'me', published: true, shippedKey: '', roles: [], isAdmin: true, want: { canEdit: true, canShare: true, canPublish: true, canDelete: true } },
    { name: 'editor', owner: 'ann', published: false, shippedKey: '', roles: ['editor'], isAdmin: false, want: { canEdit: true, canShare: true, canPublish: false, canDelete: false } },
    { name: 'editor through a group and a viewer share', owner: 'ann', published: false, shippedKey: '', roles: ['viewer', 'editor'], isAdmin: false, want: { canEdit: true, canShare: true, canPublish: false, canDelete: false } },
    { name: 'viewer', owner: 'ann', published: false, shippedKey: '', roles: ['viewer'], isAdmin: false, want: { canEdit: false, canShare: false, canPublish: false, canDelete: false } },
    { name: 'admin with no share on a published report', owner: 'ann', published: true, shippedKey: '', roles: [], isAdmin: true, want: { canEdit: false, canShare: false, canPublish: false, canDelete: true } },
    { name: 'admin editor of a published report', owner: 'ann', published: true, shippedKey: '', roles: ['editor'], isAdmin: true, want: { canEdit: true, canShare: true, canPublish: true, canDelete: true } },
    { name: 'member on a published report', owner: 'ann', published: true, shippedKey: '', roles: [], isAdmin: false, want: { canEdit: false, canShare: false, canPublish: false, canDelete: false } },
    { name: 'owner of a shipped report', owner: 'me', published: true, shippedKey: 'zoo.feed_costs', roles: [], isAdmin: true, want: { canEdit: true, canShare: true, canPublish: true, canDelete: false } },
]

describe('reportAccess', () => {
    for (const c of CASES) {
        it(c.name, () => {
            const access = reportAccess({
                owner: c.owner,
                published: c.published,
                shippedKey: c.shippedKey,
                userId: 'me',
                roles: c.roles,
                isAdmin: c.isAdmin,
            })
            expect({
                canEdit: access.canEdit,
                canShare: access.canShare,
                canPublish: access.canPublish,
                canDelete: access.canDelete,
            }).toEqual(c.want)
            expect(access.isOwner).toBe(c.owner === 'me')
            expect(access.isShipped).toBe(c.shippedKey !== '')
        })
    }
})

describe('rolesByReport', () => {
    it('groups a reader’s direct and derived shares by report', () => {
        const roles = rolesByReport([
            { report: 'r1', role: 'viewer' },
            { report: 'r1', role: 'editor' },
            { report: 'r2', role: 'viewer' },
        ])
        expect(roles.get('r1')).toEqual(['viewer', 'editor'])
        expect(roles.get('r2')).toEqual(['viewer'])
        expect(roles.get('r3')).toBeUndefined()
    })
})
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run reports/lib/__tests__/spec.test.ts reports/lib/__tests__/access.test.ts`
Expected: FAIL: `Failed to resolve import "../spec"` and `"../access"`.

- [ ] **Step 3: Write the types**

`core/reports/lib/types.ts`:

```ts
// The run endpoint's answer and the source catalog, as core/server/reports sends
// them (run.go Result, catalog.go CatalogSource). Money cells are integer cents;
// a percent cell is already a percent (12.5 means 12.5%).

export const FIELD_TYPES = [
    'text',
    'number',
    'money',
    'date',
    'datetime',
    'bool',
    'select',
    'relation',
    'percent',
] as const
export type FieldType = (typeof FIELD_TYPES)[number]

export const AGGREGATES = ['sum', 'count', 'countDistinct', 'min', 'max', 'avg'] as const
export type Aggregate = (typeof AGGREGATES)[number]

export type RowKind = 'detail' | 'group' | 'subtotal' | 'computed' | 'total'

export type CellValue = number | string | boolean | null

export const PARAM_TYPES = ['date', 'asOf', 'dateRange', 'choice', 'text', 'number'] as const
export type ParamType = (typeof PARAM_TYPES)[number]

export interface SelectOption {
    value: string
    label: string
}

export interface ResultColumn {
    id: string
    label: string
    type: FieldType
    /** The type of group, subtotal and total cells when it differs (a count of names). */
    totalType?: FieldType
    aggregate?: Aggregate
    format?: string
    options?: SelectOption[]
    /** A money column's fixed currency. */
    currency?: string
    /** The cell that holds each row's currency instead (`<id>@currency`). */
    currencyCell?: string
    mixedCurrency?: boolean
    pivot?: { cell: string; key: unknown }
}

export interface ResultRow {
    id: string
    kind: RowKind
    depth: number
    label: string
    /** The group row this row belongs to. */
    section?: string
    /** The row a nested child row hangs under. */
    parent?: string
    groupKey?: unknown[]
    expanded?: boolean
    canExpand?: boolean
    cells: Record<string, CellValue>
}

export interface Notice {
    code: string
    message: string
    columns?: string[]
    count?: number
    leftOut?: number
}

export interface ResolvedParam {
    id: string
    type: ParamType
    unset?: boolean
    value?: string | number
    list?: string[]
    from?: string
    to?: string
}

export interface RunPage {
    next?: string
    size: number
}

export interface RunResult {
    columns: ResultColumn[]
    rows: ResultRow[]
    notices: Notice[]
    topics: string[]
    page: RunPage | null
    params: ResolvedParam[]
}

export interface CatalogField {
    key: string
    label: string
    type: FieldType
    aggregates: Aggregate[]
    options?: SelectOption[]
    /** The key of the field that holds each row's currency. */
    currency?: string
}

export interface CatalogLink {
    key: string
    label: string
    target: string
    cardinality: 'toOne' | 'toMany'
}

export interface CatalogSource {
    key: string
    label: string
    fields: CatalogField[]
    links: CatalogLink[]
}

/** One refused part of a spec, keyed by its dotted spec path (`columns.0.id`). */
export interface Problem {
    path: string
    code: string
    message: string
}
```

- [ ] **Step 4: Write the spec schema**

`core/reports/lib/spec.ts`:

```ts
import { z } from '@tinycld/core/ui/form'
import { AGGREGATES, PARAM_TYPES, type Problem } from './types'

// The report spec as core/server/reports/spec.go reads it. A stored spec was
// decoded strictly and saved as the engine re-encodes it, so this schema is strict
// too. Null parts read as absent: a hand-written or shipped spec may say
// "layout": null.

export const CURRENT_SPEC_VERSION = 1
export const BUCKETS = ['day', 'month', 'quarter', 'year', 'fiscalYear'] as const
export type Bucket = (typeof BUCKETS)[number]

export const specHopSchema = z.strictObject({
    link: z.string().optional(),
    field: z.string().optional(),
    mode: z.enum(['aggregate', 'nest']).optional(),
    with: z.enum(['required', 'optional']).optional(),
})
export type SpecHop = z.infer<typeof specHopSchema>

export type SpecNode = {
    op?: string
    args?: SpecNode[]
    scale?: number
    col?: string
    path?: SpecHop[]
    param?: string
    value?: unknown
    subtotal?: string
    link?: SpecHop[]
    where?: SpecNode
}

// Input and output are one type, so a form schema built on this one types its values.
export const specNodeSchema: z.ZodType<SpecNode, SpecNode> = z.lazy(() =>
    z.strictObject({
        op: z.string().optional(),
        args: z.array(specNodeSchema).optional(),
        scale: z.number().int().optional(),
        col: z.string().optional(),
        path: z.array(specHopSchema).optional(),
        param: z.string().optional(),
        value: z.unknown().optional(),
        subtotal: z.string().optional(),
        link: z.array(specHopSchema).optional(),
        where: specNodeSchema.optional(),
    })
)

const optionSchema = z.strictObject({ value: z.string(), label: z.string() })

export const paramDefSchema = z.strictObject({
    id: z.string(),
    type: z.enum(PARAM_TYPES),
    label: z.string().optional(),
    default: z.unknown().optional(),
    options: z.array(optionSchema).optional(),
    multiple: z.boolean().optional(),
})
export type ParamDef = z.infer<typeof paramDefSchema>

export const specColumnSchema = z.strictObject({
    id: z.string(),
    label: z.string().optional(),
    path: z.array(specHopSchema).optional(),
    formula: specNodeSchema.optional(),
    aggregate: z.enum(AGGREGATES).optional(),
    scale: z.number().int().optional(),
    format: z.string().optional(),
})
export type SpecColumn = z.infer<typeof specColumnSchema>

const layoutItemSchema = z.strictObject({
    section: z.string().optional(),
    title: z.string().optional(),
    key: z.unknown().optional(),
    row: z.string().optional(),
    label: z.string().optional(),
    formula: specNodeSchema.optional(),
})

export const reportSpecSchema = z.strictObject({
    version: z.number().int().min(1),
    source: z.string().min(1),
    params: z.array(paramDefSchema).nullish(),
    columns: z.array(specColumnSchema),
    filter: specNodeSchema.nullish(),
    sort: z.array(z.strictObject({ col: z.string(), dir: z.enum(['asc', 'desc']).optional() })).nullish(),
    groups: z
        .array(
            z.strictObject({
                id: z.string(),
                col: z.string(),
                bucket: z.enum(BUCKETS).optional(),
                subtotals: z.boolean().optional(),
            })
        )
        .nullish(),
    pivot: z
        .strictObject({
            path: z.array(specHopSchema),
            bucket: z.enum(BUCKETS).optional(),
            cell: z.string(),
            maxColumns: z.number().int().optional(),
        })
        .nullish(),
    layout: z.array(layoutItemSchema).nullish(),
})
export type ReportSpec = z.infer<typeof reportSpecSchema>

export function parseSpec(value: unknown): ReportSpec | null {
    const parsed = reportSpecSchema.safeParse(value)
    return parsed.success ? parsed.data : null
}

// reports.broken: why a saved report cannot run (core/server/reports/broken.go).
export const brokenSchema = z.object({
    status: z.number(),
    problems: z.record(z.string(), z.object({ code: z.string(), message: z.string() })),
    more: z.number().optional(),
})
export type BrokenMark = z.infer<typeof brokenSchema>

export function parseBroken(value: unknown): BrokenMark | null {
    const parsed = brokenSchema.safeParse(value)
    return parsed.success ? parsed.data : null
}

export function brokenProblems(mark: BrokenMark): Problem[] {
    return Object.entries(mark.problems)
        .map(([path, problem]) => ({ path, code: problem.code, message: problem.message }))
        .sort((a, b) => a.path.localeCompare(b.path))
}
```

- [ ] **Step 5: Write report access**

`core/reports/lib/access.ts`:

```ts
// Who may do what with a saved report, as the sharing contracts' table says
// (docs/plans/2026-10-30-report-sharing.md). The server decides; this only
// chooses which actions to offer, so an action never appears that is refused.

export const SHARE_ROLES = [
    { value: 'viewer', label: 'Viewer' },
    { value: 'editor', label: 'Editor' },
] as const
export type ShareRole = (typeof SHARE_ROLES)[number]['value']

export interface ReportAccess {
    isOwner: boolean
    /** Owner or editor (direct or through a group): may change the report. */
    canEdit: boolean
    canShare: boolean
    /** An admin who may also edit (Ruling P4). */
    canPublish: boolean
    /** The owner, or an admin on a published report; never a shipped report. */
    canDelete: boolean
    isShipped: boolean
}

export const NO_ACCESS: ReportAccess = {
    isOwner: false,
    canEdit: false,
    canShare: false,
    canPublish: false,
    canDelete: false,
    isShipped: false,
}

export function reportAccess(input: {
    owner: string
    published: boolean
    shippedKey: string
    userId: string
    roles: readonly ShareRole[]
    isAdmin: boolean
}): ReportAccess {
    const isOwner = input.owner === input.userId
    const canEdit = isOwner || input.roles.includes('editor')
    const isShipped = input.shippedKey !== ''
    return {
        isOwner,
        canEdit,
        canShare: canEdit,
        canPublish: input.isAdmin && canEdit,
        canDelete: !isShipped && (isOwner || (input.isAdmin && input.published)),
        isShipped,
    }
}

/** A reader's share roles per report: one report may hold a direct and a derived row. */
export function rolesByReport(
    shares: readonly { report: string; role: ShareRole }[]
): Map<string, ShareRole[]> {
    const out = new Map<string, ShareRole[]>()
    for (const share of shares) out.set(share.report, [...(out.get(share.report) ?? []), share.role])
    return out
}
```

- [ ] **Step 6: Register the collections and the export**

In `core/lib/pocketbase.ts`, after `const passkeys = …`, add:

```ts
// Saved reports (core/server/reports). The server owns source, published_by,
// broken and the shipped fields and replaces any client value, so an insert
// never carries them. A share row is written by the owner or an editor; a
// derived row (user and group set) is the server's copy of a group grant.
const reports = newCollection('reports', {
    omitOnInsert: [
        'created',
        'updated',
        'source',
        'published_by',
        'broken',
        'shipped_key',
        'shipped_revision',
    ],
    ...onDemand,
    relations: { owner: users, published_by: users },
    ...indexing,
})

const report_shares = newCollection('report_shares', {
    omitOnInsert: ['created', 'updated'],
    ...onDemand,
    relations: { report: reports, user: users, group: groups },
    ...indexing,
})
```

and add `reports,` and `report_shares,` to `coreStores` after `passkeys,`.

In `core/package.json`, `exports`, add after the `./file-viewer/*` entry:

```json
        "./reports/*": [
            "./reports/*.ts",
            "./reports/*.tsx",
            "./reports/*/index.ts",
            "./reports/*/index.tsx"
        ],
```

- [ ] **Step 7: Run the tests and typecheck**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run reports/lib/__tests__/spec.test.ts reports/lib/__tests__/access.test.ts && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg typecheck`
Expected: tests pass; tsc clean (the `omitOnInsert` keys exist on `Reports`).

- [ ] **Step 8: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/lib/pocketbase.ts core/package.json core/reports/lib/types.ts core/reports/lib/spec.ts core/reports/lib/access.ts core/reports/lib/__tests__/spec.test.ts core/reports/lib/__tests__/access.test.ts
git commit -m "feat(core): report types, spec schema, access and the saved-report collections"
```

---

### Task 6: Money, dates and cell display

**Files:**
- Create: `core/reports/lib/money.ts`, `core/reports/lib/dates.ts`, `core/reports/lib/format.ts`
- Test: `core/reports/lib/__tests__/money.test.ts`, `core/reports/lib/__tests__/dates.test.ts`, `core/reports/lib/__tests__/format.test.ts`

**Interfaces:**
- Consumes: `ResultColumn`, `ResultRow`, `CellValue`, `FieldType`, `RowKind` (Task 5); `toDateString` (`@tinycld/core/lib/dates`); `log` (`@tinycld/core/lib/logger`).
- Produces (`money.ts`): `MAX_STORED_AMOUNT`, `roundHalfAwayFromZero(value)`, `formatMoney(cents, currency)`.
- Produces (`dates.ts`): `isIsoDay(text)`, `todayIso()`, `dayWords(day)`, `shortDay(day)`, `rangeWords(from, to)`, `rangeChipText(from, to)`.
- Produces (`format.ts`): `DisplayOptions {divideBy1000, withoutCents, negative: 'minus'|'parens', negativeRed, hidden: string[]}`, `NumberFormat` (= `Omit<DisplayOptions, 'hidden'>`), `DEFAULT_DISPLAY`, `REPORT_AMOUNT_ERROR`, `CellDisplay {text, align, isRed, isError}`, `formatReportAmount(cents, currency, format, {withSymbol})`, `formatReportPercent(value)`, `formatNumber(value)`, `cellType(column, row)`, `cellCurrency(column, row)`, `isRightAligned(column)`, `cellDisplay(column, row, value, format)`, `shownColumns(columns, hidden)`.

- [ ] **Step 1: Write the failing tests**

`core/reports/lib/__tests__/money.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { formatMoney, MAX_STORED_AMOUNT, roundHalfAwayFromZero } from '../money'

describe('formatMoney', () => {
    it('formats whole cents with the currency layout', () => {
        expect(formatMoney(123456, 'USD')).toBe('$1,234.56')
        expect(formatMoney(-5, 'USD')).toBe('-$0.05')
        expect(formatMoney(123456, 'EUR')).toBe('€1,234.56')
    })

    it('is exact at 2^53-1', () => {
        expect(MAX_STORED_AMOUNT).toBe(9007199254740991)
        expect(formatMoney(9007199254740991, 'USD')).toBe('$90,071,992,547,409.91')
    })

    it('never shows a negative zero', () => {
        expect(formatMoney(-0.4, 'USD')).toBe('$0.00')
    })

    it('falls back to plain digits for no or an unknown currency', () => {
        expect(formatMoney(123456, '')).toBe('1,234.56')
        expect(formatMoney(123456, 'NOT A CODE')).toBe('1,234.56 NOT A CODE')
    })

    it('shows a dash for a value that is not a number', () => {
        expect(formatMoney(Number.NaN, 'USD')).toBe('—')
    })
})

describe('roundHalfAwayFromZero', () => {
    it('rounds halves away from zero', () => {
        expect(roundHalfAwayFromZero(2.5)).toBe(3)
        expect(roundHalfAwayFromZero(-2.5)).toBe(-3)
        // 10.005 * 100 is 1000.4999999999999 in float64; a person reading 10.005 expects 1001.
        expect(roundHalfAwayFromZero(10.005 * 100)).toBe(1001)
    })
})
```

`core/reports/lib/__tests__/dates.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { dayWords, isIsoDay, rangeChipText, rangeWords, shortDay } from '../dates'

describe('isIsoDay', () => {
    it('accepts real days from 1900', () => {
        expect(isIsoDay('2026-02-28')).toBe(true)
        expect(isIsoDay('2024-02-29')).toBe(true)
        expect(isIsoDay('1900-01-01')).toBe(true)
    })

    it('refuses what is not a real day', () => {
        for (const text of ['2026-02-29', '2026-13-01', '2026-00-10', '1899-12-31', '2026-1-01', 'today', ''])
            expect(isIsoDay(text)).toBe(false)
    })
})

describe('day and range words', () => {
    it('writes a day out', () => {
        expect(dayWords('2026-10-05')).toBe('October 5, 2026')
        expect(shortDay('2026-10-05')).toBe('Oct 5, 2026')
    })

    it('drops the first year when both ends share it', () => {
        expect(rangeWords('2026-10-01', '2026-10-25')).toBe('October 1 – October 25, 2026')
        expect(rangeWords('2025-07-01', '2026-06-30')).toBe('July 1, 2025 – June 30, 2026')
        expect(rangeChipText('2026-01-01', '2026-10-25')).toBe('Jan 1 – Oct 25, 2026')
    })

    it('writes a one-day range as one day', () => {
        expect(rangeWords('2026-10-05', '2026-10-05')).toBe('October 5, 2026')
    })
})
```

`core/reports/lib/__tests__/format.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import {
    cellCurrency,
    cellDisplay,
    DEFAULT_DISPLAY,
    formatReportAmount,
    formatReportPercent,
    type NumberFormat,
    REPORT_AMOUNT_ERROR,
    shownColumns,
} from '../format'
import { MAX_STORED_AMOUNT } from '../money'
import type { ResultColumn, ResultRow } from '../types'

const PLAIN: NumberFormat = {
    divideBy1000: false,
    withoutCents: false,
    negative: 'minus',
    negativeRed: false,
}
const PARENS: NumberFormat = { ...PLAIN, negative: 'parens' }
const WITHOUT_CENTS: NumberFormat = { ...PLAIN, withoutCents: true }
const THOUSANDS: NumberFormat = { ...PLAIN, divideBy1000: true }
const THOUSANDS_WITHOUT_CENTS: NumberFormat = { ...PLAIN, divideBy1000: true, withoutCents: true }
const RED: NumberFormat = { ...PLAIN, negativeRed: true }

const plain = (cents: number, format: NumberFormat = PLAIN) =>
    formatReportAmount(cents, 'USD', format, { withSymbol: false })

const row = (kind: ResultRow['kind'], cells: ResultRow['cells'] = {}): ResultRow => ({
    id: `${kind}:1`,
    kind,
    depth: 0,
    label: 'x',
    cells,
})

const MONEY: ResultColumn = { id: 'cost', label: 'Cost', type: 'money', currencyCell: 'cost@currency' }
const FIXED: ResultColumn = { id: 'price', label: 'Price', type: 'money', currency: 'EUR' }

describe('formatReportAmount', () => {
    it('formats bare numbers, a symbol after the minus, parentheses', () => {
        expect(plain(1234567)).toBe('12,345.67')
        expect(plain(-1234567)).toBe('-12,345.67')
        expect(plain(-0.4)).toBe('0.00')
        expect(formatReportAmount(-1234567, 'USD', PLAIN, { withSymbol: true })).toBe('-$12,345.67')
        expect(plain(-1234567, PARENS)).toBe('(12,345.67)')
    })

    it('rounds once, half away from zero', () => {
        expect(plain(1234550, WITHOUT_CENTS)).toBe('12,346')
        expect(plain(-1234550, WITHOUT_CENTS)).toBe('-12,346')
        expect(plain(1234500, THOUSANDS)).toBe('12.35')
        expect(plain(1234550, THOUSANDS_WITHOUT_CENTS)).toBe('12')
    })

    it('is exact at the stored limit and refuses anything past it', () => {
        expect(plain(MAX_STORED_AMOUNT)).toBe('90,071,992,547,409.91')
        expect(plain(9007199254740949, WITHOUT_CENTS)).toBe('90,071,992,547,409')
        expect(plain(9007199254740499, THOUSANDS)).toBe('90,071,992,547.40')
        expect(plain(MAX_STORED_AMOUNT + 1)).toBe(REPORT_AMOUNT_ERROR)
        expect(plain(Number.POSITIVE_INFINITY)).toBe(REPORT_AMOUNT_ERROR)
    })

    // A float quotient misrounds next to a half near 2^53; integer-exact rounding does not.
    it('matches BigInt rounding at every half near the limit', () => {
        const reference = (cents: number, divisor: bigint) => {
            const big = BigInt(cents)
            const magnitude = big < 0n ? -big : big
            const rounded = magnitude / divisor + (2n * (magnitude % divisor) >= divisor ? 1n : 0n)
            return big < 0n ? -rounded : rounded
        }
        const group = (n: bigint) => n.toString().replace(/\B(?=(\d{3})+$)/g, ',')
        const mismatches: string[] = []
        for (let step = -4; step <= 4; step += 1) {
            for (const base of [MAX_STORED_AMOUNT - 50, MAX_STORED_AMOUNT - 450, MAX_STORED_AMOUNT - 1500]) {
                const cents = base + step
                const units = reference(cents, 100n)
                const expected = units < 0n ? `-${group(-units)}` : group(units)
                const actual = plain(cents, WITHOUT_CENTS)
                if (actual !== expected) mismatches.push(`${cents}: ${actual} != ${expected}`)
            }
        }
        expect(mismatches).toEqual([])
    })
})

describe('formatReportPercent', () => {
    it('shows one decimal and nothing for no figure', () => {
        expect(formatReportPercent(12.345)).toBe('12.3%')
        expect(formatReportPercent(-0.05)).toBe('-0.1%')
        expect(formatReportPercent(Number.NaN)).toBe('')
    })
})

describe('cellDisplay', () => {
    it('reads each row’s currency from its currency cell, or the fixed one', () => {
        const eur = row('detail', { cost: 500, 'cost@currency': 'EUR' })
        expect(cellCurrency(MONEY, eur)).toBe('EUR')
        expect(cellCurrency(FIXED, row('detail'))).toBe('EUR')
        expect(cellCurrency({ ...MONEY, currencyCell: undefined }, row('detail'))).toBe('')
    })

    it('shows a symbol on subtotal, computed and total rows only', () => {
        const usd = { 'cost@currency': 'USD' }
        expect(cellDisplay(MONEY, row('detail', usd), 1500, PLAIN).text).toBe('15.00')
        expect(cellDisplay(MONEY, row('group', usd), 1500, PLAIN).text).toBe('15.00')
        expect(cellDisplay(MONEY, row('subtotal', usd), 1500, PLAIN).text).toBe('$15.00')
        expect(cellDisplay(MONEY, row('computed', usd), 1500, PLAIN).text).toBe('$15.00')
        expect(cellDisplay(MONEY, row('total', usd), -1500, RED)).toEqual({
            text: '-$15.00',
            align: 'right',
            isRed: true,
            isError: false,
        })
    })

    it('marks an amount past the limit as an error cell', () => {
        expect(cellDisplay(MONEY, row('detail'), MAX_STORED_AMOUNT + 1, PLAIN)).toEqual({
            text: REPORT_AMOUNT_ERROR,
            align: 'right',
            isRed: false,
            isError: true,
        })
    })

    it('uses the total type on total rows (a count of yes/no values is a number)', () => {
        const active: ResultColumn = { id: 'active', label: 'Active', type: 'bool', totalType: 'number', aggregate: 'count' }
        expect(cellDisplay(active, row('detail'), true, PLAIN).text).toBe('Yes')
        expect(cellDisplay(active, row('detail'), false, PLAIN).text).toBe('No')
        expect(cellDisplay(active, row('total'), 1234, PLAIN)).toMatchObject({ text: '1,234', align: 'right' })
    })

    it('labels select values, writes dates, and formats percents', () => {
        const status: ResultColumn = {
            id: 'status',
            label: 'Status',
            type: 'select',
            options: [{ value: 'open', label: 'Open' }],
        }
        expect(cellDisplay(status, row('detail'), 'open', PLAIN).text).toBe('Open')
        expect(cellDisplay(status, row('detail'), 'gone', PLAIN).text).toBe('gone')
        const day: ResultColumn = { id: 'day', label: 'Day', type: 'date' }
        expect(cellDisplay(day, row('detail'), '2026-03-05 00:00:00.000Z', PLAIN)).toMatchObject({ text: 'Mar 5, 2026', align: 'left' })
        const share: ResultColumn = { id: 'share', label: 'Share', type: 'percent' }
        expect(cellDisplay(share, row('detail'), 25, PLAIN)).toMatchObject({ text: '25.0%', align: 'right' })
        const ratio: ResultColumn = { id: 'ratio', label: 'Ratio', type: 'number', format: 'percent' }
        expect(cellDisplay(ratio, row('detail'), 12.5, PLAIN).text).toBe('12.5%')
    })

    it('shows nothing for a blank cell', () => {
        expect(cellDisplay(MONEY, row('detail'), null, PLAIN).text).toBe('')
        expect(cellDisplay(MONEY, row('detail'), undefined, PLAIN).text).toBe('')
    })
})

describe('shownColumns', () => {
    const columns: ResultColumn[] = [
        { id: 'name', label: 'Name', type: 'text' },
        { id: 'cost', label: 'Cost', type: 'money' },
        { id: 'cost.p0', label: '2026-01', type: 'money', pivot: { cell: 'cost', key: '2026-01' } },
    ]

    it('hides the listed columns but never the label column', () => {
        expect(shownColumns(columns, ['cost', 'name']).map(c => c.id)).toEqual(['name', 'cost.p0'])
        expect(shownColumns(columns, DEFAULT_DISPLAY.hidden).map(c => c.id)).toEqual(['name', 'cost', 'cost.p0'])
    })
})
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run reports/lib/__tests__/money.test.ts reports/lib/__tests__/dates.test.ts reports/lib/__tests__/format.test.ts`
Expected: FAIL: the three modules do not resolve.

- [ ] **Step 3: Write money and dates**

`core/reports/lib/money.ts` (the Books shell's exact-cents formatter, moved):

```ts
import { log } from '@tinycld/core/lib/logger'

// Amounts are integer cents. PocketBase stores numbers as float64, exact only up
// to 2^53-1, so a larger amount cannot be shown truthfully.
export const MAX_STORED_AMOUNT = Number.MAX_SAFE_INTEGER

// The epsilon nudge makes 1000.4999999999999 (10.005 * 100) round as a person
// reading the decimal expects.
export function roundHalfAwayFromZero(value: number): number {
    return Math.sign(value) * Math.round(Math.abs(value) * (1 + Number.EPSILON))
}

const warned = new Set<string>()

function warnOnce(key: string, message: string, extra: Record<string, unknown>) {
    if (warned.has(key)) return
    warned.add(key)
    log.warn('reports.money', message, extra)
}

// cents / 100 as a float loses the last cent near 2^53, so whole cents are
// written from their digits; Intl supplies only the layout (symbol, sign,
// separators) through formatToParts of a sample value.
function formatCents(format: Intl.NumberFormat, cents: number): string {
    if (!Number.isSafeInteger(cents)) return format.format(cents / 100)
    const magnitude = Math.abs(cents)
    const whole = String(Math.floor(magnitude / 100)).replace(/\B(?=(\d{3})+$)/g, ',')
    const fraction = String(magnitude % 100).padStart(2, '0')
    return format
        .formatToParts(cents < 0 ? -1 : 1)
        .map(part => {
            if (part.type === 'integer') return whole
            if (part.type === 'fraction') return fraction
            return part.value
        })
        .join('')
}

export function formatMoney(cents: number, currency: string): string {
    if (!Number.isFinite(cents)) {
        warnOnce('non-finite', 'non-finite amount', { cents })
        return '—'
    }
    // A value that rounds to zero would otherwise read "-$0.00".
    const amount = Math.abs(cents) < 0.5 ? 0 : cents
    const digits = { minimumFractionDigits: 2, maximumFractionDigits: 2 }
    if (currency !== '') {
        try {
            const currencyFormat = new Intl.NumberFormat('en-US', { style: 'currency', currency, ...digits })
            return formatCents(currencyFormat, amount)
        } catch (err) {
            warnOnce(`currency:${currency}`, 'invalid currency code', { currency, err })
        }
    }
    const plain = formatCents(new Intl.NumberFormat('en-US', digits), amount)
    return currency === '' ? plain : `${plain} ${currency}`
}
```

`core/reports/lib/dates.ts`:

```ts
import { toDateString } from '@tinycld/core/lib/dates'

// Report days are YYYY-MM-DD. Month names are local, so the words do not depend
// on the runtime's locale data (Hermes ships little of it).

const FIRST_YEAR = 1900
const MONTH_NAMES = [
    'January',
    'February',
    'March',
    'April',
    'May',
    'June',
    'July',
    'August',
    'September',
    'October',
    'November',
    'December',
]
const SHORT_MONTH_NAMES = MONTH_NAMES.map(name => name.slice(0, 3))

function daysInMonth(year: number, month: number): number {
    return new Date(Date.UTC(year, month, 0)).getUTCDate()
}

/** The engine's day rule: YYYY-MM-DD, a real day, year 1900 or later. */
export function isIsoDay(text: string): boolean {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(text)) return false
    const year = Number(text.slice(0, 4))
    const month = Number(text.slice(5, 7))
    const day = Number(text.slice(8, 10))
    if (year < FIRST_YEAR || month < 1 || month > 12 || day < 1) return false
    return day <= daysInMonth(year, month)
}

/** The reader's own day, which period presets resolve against. */
export function todayIso(): string {
    return toDateString(new Date())
}

function monthDay(day: string, names: readonly string[]): string {
    return `${names[Number(day.slice(5, 7)) - 1]} ${Number(day.slice(8, 10))}`
}

export function dayWords(day: string): string {
    return `${monthDay(day, MONTH_NAMES)}, ${day.slice(0, 4)}`
}

export function shortDay(day: string): string {
    return `${monthDay(day, SHORT_MONTH_NAMES)}, ${day.slice(0, 4)}`
}

// One day for a one-day range; the first year is dropped when both ends share it.
function rangeText(
    from: string,
    to: string,
    withYear: (day: string) => string,
    withoutYear: (day: string) => string
): string {
    if (from === to) return withYear(to)
    const start = from.slice(0, 4) === to.slice(0, 4) ? withoutYear(from) : withYear(from)
    return `${start} – ${withYear(to)}`
}

export function rangeWords(from: string, to: string): string {
    return rangeText(from, to, dayWords, day => monthDay(day, MONTH_NAMES))
}

export function rangeChipText(from: string, to: string): string {
    return rangeText(from, to, shortDay, day => monthDay(day, SHORT_MONTH_NAMES))
}
```

- [ ] **Step 4: Write cell display**

`core/reports/lib/format.ts`:

```ts
import { isIsoDay, shortDay } from './dates'
import { formatMoney, MAX_STORED_AMOUNT, roundHalfAwayFromZero } from './money'
import type { CellValue, FieldType, ResultColumn, ResultRow, RowKind } from './types'

// The Customize options of a run. They stay in the client (the server accepts and
// ignores options.display): formatting never changes a figure.
export interface DisplayOptions {
    divideBy1000: boolean
    withoutCents: boolean
    negative: 'minus' | 'parens'
    negativeRed: boolean
    /** Ids of columns the reader hid; the first column is never hidden. */
    hidden: string[]
}
export type NumberFormat = Omit<DisplayOptions, 'hidden'>

export const DEFAULT_DISPLAY: DisplayOptions = {
    divideBy1000: false,
    withoutCents: false,
    negative: 'minus',
    negativeRed: false,
    hidden: [],
}

export const REPORT_AMOUNT_ERROR = 'Too large'

export type CellDisplay = {
    text: string
    align: 'left' | 'right'
    isRed: boolean
    isError: boolean
}

// Integer cents divided by `divisor`, half away from zero. A float division near
// 2^53 can land on the wrong side of a half, so the remainder is split off with
// `%`, which is exact for integers.
function roundedQuotient(cents: number, divisor: number): number {
    const magnitude = Math.abs(cents)
    const remainder = magnitude % divisor
    const quotient = (magnitude - remainder) / divisor + (2 * remainder >= divisor ? 1 : 0)
    return cents < 0 ? -quotient : quotient
}

// The one rounding of an amount, to whole cents of the display unit. Null when the
// amount cannot be stored, so it cannot be shown either.
function displayUnits(cents: number, format: NumberFormat): number | null {
    if (!Number.isFinite(cents) || Math.abs(cents) > MAX_STORED_AMOUNT) return null
    const centsPerUnit = format.withoutCents ? 100 : 1
    const divisor = (format.divideBy1000 ? 1000 : 1) * centsPerUnit
    const rounded = Number.isInteger(cents)
        ? roundedQuotient(cents, divisor)
        : roundHalfAwayFromZero(cents / divisor)
    return rounded * centsPerUnit
}

function groupDigits(magnitude: number): string {
    return String(magnitude).replace(/\B(?=(\d{3})+$)/g, ',')
}

function plainAmount(magnitude: number): string {
    return `${groupDigits(Math.floor(magnitude / 100))}.${String(magnitude % 100).padStart(2, '0')}`
}

function unitsText(units: number, currency: string, format: NumberFormat, withSymbol: boolean): string {
    const magnitude = Math.abs(units)
    const amount = withSymbol ? formatMoney(magnitude, currency) : plainAmount(magnitude)
    const body = format.withoutCents ? amount.replace(/\.00(?=\D*$)/, '') : amount
    if (units >= 0) return body
    return format.negative === 'parens' ? `(${body})` : `-${body}`
}

export function formatReportAmount(
    cents: number,
    currency: string,
    format: NumberFormat,
    options: { withSymbol: boolean }
): string {
    const units = displayUnits(cents, format)
    if (units === null) return REPORT_AMOUNT_ERROR
    return unitsText(units, currency, format, options.withSymbol)
}

/** `value` is already a percent: 12.345 shows as 12.3%. A share of nothing shows nothing. */
export function formatReportPercent(value: number): string {
    if (!Number.isFinite(value)) return ''
    return `${(roundHalfAwayFromZero(value * 10) / 10).toFixed(1)}%`
}

export function formatNumber(value: number): string {
    if (Number.isSafeInteger(value)) return `${value < 0 ? '-' : ''}${groupDigits(Math.abs(value))}`
    return value.toLocaleString('en-US', { maximumFractionDigits: 6 })
}

const SYMBOL_ROWS: readonly RowKind[] = ['subtotal', 'computed', 'total']

/** Group, subtotal, computed and total cells take the column's total type when it has one. */
export function cellType(column: ResultColumn, row: ResultRow): FieldType {
    return row.kind !== 'detail' && column.totalType ? column.totalType : column.type
}

export function cellCurrency(column: ResultColumn, row: ResultRow): string {
    if (column.currency) return column.currency
    const cell = column.currencyCell ? row.cells[column.currencyCell] : null
    return typeof cell === 'string' ? cell : ''
}

const RIGHT_TYPES: readonly FieldType[] = ['money', 'number', 'percent']

export function isRightAligned(column: ResultColumn): boolean {
    return RIGHT_TYPES.includes(column.type) || RIGHT_TYPES.includes(column.totalType ?? column.type)
}

function plainDisplay(text: string, align: CellDisplay['align']): CellDisplay {
    return { text, align, isRed: false, isError: false }
}

function moneyDisplay(column: ResultColumn, row: ResultRow, cents: number, format: NumberFormat): CellDisplay {
    const units = displayUnits(cents, format)
    if (units === null) return { text: REPORT_AMOUNT_ERROR, align: 'right', isRed: false, isError: true }
    return {
        text: unitsText(units, cellCurrency(column, row), format, SYMBOL_ROWS.includes(row.kind)),
        align: 'right',
        isRed: format.negativeRed && units < 0,
        isError: false,
    }
}

function dayText(value: string): string {
    const day = value.slice(0, 10)
    return isIsoDay(day) ? shortDay(day) : value
}

export function cellDisplay(
    column: ResultColumn,
    row: ResultRow,
    value: CellValue | undefined,
    format: NumberFormat
): CellDisplay {
    const type = cellType(column, row)
    const align = RIGHT_TYPES.includes(type) ? 'right' : 'left'
    if (value === null || value === undefined) return plainDisplay('', align)
    if (type === 'money' && typeof value === 'number') return moneyDisplay(column, row, value, format)
    if (typeof value === 'number' && (type === 'percent' || column.format === 'percent')) {
        return plainDisplay(formatReportPercent(value), 'right')
    }
    if (typeof value === 'number') return plainDisplay(formatNumber(value), align)
    if (typeof value === 'boolean') return plainDisplay(value ? 'Yes' : 'No', align)
    if ((type === 'date' || type === 'datetime') && typeof value === 'string') {
        return plainDisplay(dayText(value), align)
    }
    if (type === 'select') {
        return plainDisplay(column.options?.find(o => o.value === value)?.label ?? value, align)
    }
    return plainDisplay(value, align)
}

export function shownColumns(columns: readonly ResultColumn[], hidden: readonly string[]): ResultColumn[] {
    return columns.filter((column, index) => index === 0 || !hidden.includes(column.id))
}
```

- [ ] **Step 5: Run the tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run reports/lib/__tests__/money.test.ts reports/lib/__tests__/dates.test.ts reports/lib/__tests__/format.test.ts`
Expected: all pass.

- [ ] **Step 6: Lint, typecheck, commit**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome check core/reports && cd core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg typecheck`
Expected: clean.

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/reports/lib/money.ts core/reports/lib/dates.ts core/reports/lib/format.ts core/reports/lib/__tests__/money.test.ts core/reports/lib/__tests__/dates.test.ts core/reports/lib/__tests__/format.test.ts
git commit -m "feat(core): report money, dates and cell display (moved from the Books shell)"
```

---

### Task 7: Rows, folding and pages

**Files:**
- Create: `core/reports/lib/rows.ts`
- Test: `core/reports/lib/__tests__/rows.test.ts`

**Interfaces:**
- Consumes: `ResultRow`, `RunResult`, `CellValue`, `Notice` (Task 5).
- Produces: `VisibleRow {row, cells, isToggle, isOpen}`, `visibleRows(rows)`, `Expansion {all: boolean; keys: readonly unknown[][]}`, `NO_EXPANSION`, `EXPAND_ALL`, `toggleGroup(expansion, row, rows): Expansion`, `foldAllLabel(rows): 'Collapse all' | 'Expand all' | null`, `foldAllTarget(rows): Expansion`, `expansionOptions(expansion): { expand?: unknown[][]; expandAll?: boolean }`, `hasRows(rows): boolean`, `mergePages(pages): RunResult`.

- [ ] **Step 1: Write the failing tests**

`core/reports/lib/__tests__/rows.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import {
    EXPAND_ALL,
    expansionOptions,
    foldAllLabel,
    foldAllTarget,
    hasRows,
    mergePages,
    NO_EXPANSION,
    toggleGroup,
    visibleRows,
} from '../rows'
import type { ResultRow, RunResult } from '../types'

const group = (id: string, key: unknown[], extra: Partial<ResultRow> = {}): ResultRow => ({
    id,
    kind: 'group',
    depth: key.length - 1,
    label: id,
    groupKey: key,
    canExpand: true,
    cells: { cost: 100 },
    ...extra,
})
const detail = (id: string, section?: string): ResultRow => ({ id, kind: 'detail', depth: 1, label: id, section, cells: { cost: 50 } })
const subtotal = (id: string, section: string): ResultRow => ({ id, kind: 'subtotal', depth: 0, label: `Total ${id}`, section, cells: { cost: 100 } })
const total: ResultRow = { id: 'total', kind: 'total', depth: 0, label: 'Total', cells: { cost: 300 } }

describe('visibleRows', () => {
    it('blanks a group row that its own subtotal row follows', () => {
        const rows = [group('g:a', ['a'], { expanded: true }), detail('d:1', 'g:a'), subtotal('s:a', 'g:a'), group('g:b', ['b']), total]
        const shown = visibleRows(rows)
        expect(shown[0].cells).toEqual({})
        expect(shown[0]).toMatchObject({ isToggle: true, isOpen: true })
        expect(shown[3].cells).toEqual({ cost: 100 })
        expect(shown[3]).toMatchObject({ isToggle: true, isOpen: false })
        expect(shown[4].isToggle).toBe(false)
    })

    it('does not offer a toggle on a group that cannot open', () => {
        expect(visibleRows([group('g:a', ['a'], { canExpand: false })])[0].isToggle).toBe(false)
    })
})

describe('toggleGroup', () => {
    const outer = group('g:a', ['a'], { expanded: true })
    const inner = group('g:a.x', ['a', 'x'], { section: 'g:a', expanded: true })
    const other = group('g:b', ['b'], { expanded: true })
    const rows = [outer, inner, detail('d:1', 'g:a.x'), other, total]

    it('opens a closed group by adding its key', () => {
        const closed = group('g:c', ['c'])
        expect(toggleGroup(NO_EXPANSION, closed, [closed])).toEqual({ all: false, keys: [['c']] })
    })

    it('closes a group and every group under it', () => {
        const next = toggleGroup({ all: false, keys: [['a'], ['a', 'x'], ['b']] }, outer, rows)
        expect(next).toEqual({ all: false, keys: [['b']] })
    })

    it('keeps every other open group when closing one under Expand all', () => {
        expect(toggleGroup(EXPAND_ALL, inner, rows)).toEqual({ all: false, keys: [['a'], ['b']] })
    })

    it('adds a key once', () => {
        const closed = group('g:c', ['c'])
        expect(toggleGroup({ all: false, keys: [['c']] }, closed, [closed])).toEqual({ all: false, keys: [['c']] })
    })
})

describe('fold all', () => {
    it('collapses when any group is open, expands when none is', () => {
        expect(foldAllLabel([group('g:a', ['a'], { expanded: true }), group('g:b', ['b'])])).toBe('Collapse all')
        expect(foldAllTarget([group('g:a', ['a'], { expanded: true })])).toEqual(NO_EXPANSION)
        expect(foldAllLabel([group('g:a', ['a'])])).toBe('Expand all')
        expect(foldAllTarget([group('g:a', ['a'])])).toEqual(EXPAND_ALL)
    })

    it('has no button without a group that opens', () => {
        expect(foldAllLabel([detail('d:1'), total])).toBeNull()
        expect(foldAllLabel([group('g:a', ['a'], { canExpand: false })])).toBeNull()
    })

    it('turns an expansion into run options', () => {
        expect(expansionOptions(NO_EXPANSION)).toEqual({})
        expect(expansionOptions(EXPAND_ALL)).toEqual({ expandAll: true })
        expect(expansionOptions({ all: false, keys: [['a']] })).toEqual({ expand: [['a']] })
    })
})

describe('hasRows', () => {
    it('counts detail and group rows only', () => {
        expect(hasRows([total])).toBe(false)
        expect(hasRows([{ ...total, id: 'c:net', kind: 'computed' }, total])).toBe(false)
        expect(hasRows([detail('d:1'), total])).toBe(true)
        expect(hasRows([group('g:a', ['a']), total])).toBe(true)
    })
})

describe('mergePages', () => {
    const page = (ids: string[], totalCost: number, notices: RunResult['notices'] = []): RunResult => ({
        columns: [{ id: 'cost', label: 'Cost', type: 'money' }],
        rows: [...ids.map(id => detail(id)), { ...total, cells: { cost: totalCost } }],
        notices,
        topics: ['collection/zoo_feedings'],
        page: { size: ids.length, next: 'next' },
        params: [],
    })

    it('keeps every page’s detail rows and the newest total', () => {
        const merged = mergePages([page(['d:1', 'd:2'], 300), page(['d:3'], 350)])
        expect(merged.rows.map(r => r.id)).toEqual(['d:1', 'd:2', 'd:3', 'total'])
        expect(merged.rows[3].cells.cost).toBe(350)
    })

    it('keeps one notice of each code', () => {
        const cap = { code: 'rowCap', message: 'The report shows its first 5000 rows.' }
        expect(mergePages([page(['d:1'], 1, [cap]), page(['d:2'], 2, [cap])]).notices).toEqual([cap])
    })
})
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run reports/lib/__tests__/rows.test.ts`
Expected: FAIL: `../rows` does not resolve.

- [ ] **Step 3: Implement**

`core/reports/lib/rows.ts`:

```ts
import type { CellValue, Notice, ResultRow, RunResult } from './types'

// The rows of a run as the table draws them. Groups load lazily on the server
// (options.expand), so a closed group's children are simply not in the result:
// folding is a new run, never a client-side hide.

export type VisibleRow = {
    row: ResultRow
    cells: Record<string, CellValue>
    /** A group row that can open or close. */
    isToggle: boolean
    isOpen: boolean
}

const NO_CELLS: Record<string, CellValue> = {}

// A group row that its own subtotal row follows shows no figures, so a figure is
// never shown twice (the Books shell's rule). A closed group with no subtotal row
// keeps its figures.
export function visibleRows(rows: readonly ResultRow[]): VisibleRow[] {
    const withSubtotal = new Set(
        rows.filter(row => row.kind === 'subtotal' && row.section).map(row => row.section)
    )
    return rows.map(row => {
        const isGroup = row.kind === 'group'
        return {
            row,
            cells: isGroup && withSubtotal.has(row.id) ? NO_CELLS : row.cells,
            isToggle: isGroup && row.canExpand === true,
            isOpen: isGroup && row.expanded === true,
        }
    })
}

/** Which groups a run opens: every one, or those whose group keys are listed. */
export type Expansion = { all: boolean; keys: readonly unknown[][] }

export const NO_EXPANSION: Expansion = { all: false, keys: [] }
export const EXPAND_ALL: Expansion = { all: true, keys: [] }

const keyText = (key: readonly unknown[] | undefined) => JSON.stringify(key ?? [])

function openGroupKeys(rows: readonly ResultRow[]): unknown[][] {
    return rows.filter(row => row.kind === 'group' && row.expanded).map(row => row.groupKey ?? [])
}

// The row and every group row whose chain of sections reaches it. Rows come parent
// first, so one pass finds them all.
function groupAndDescendants(row: ResultRow, rows: readonly ResultRow[]): Set<string> {
    const ids = new Set([row.id])
    for (const r of rows) {
        if (r.kind === 'group' && r.section !== undefined && ids.has(r.section)) ids.add(r.id)
    }
    return ids
}

export function toggleGroup(expansion: Expansion, row: ResultRow, rows: readonly ResultRow[]): Expansion {
    // Under Expand all, the open groups are the ones the result shows open.
    const current = expansion.all ? openGroupKeys(rows) : [...expansion.keys]
    const key = row.groupKey ?? []
    if (!row.expanded) {
        const text = keyText(key)
        return { all: false, keys: current.some(k => keyText(k) === text) ? current : [...current, key] }
    }
    const closing = groupAndDescendants(row, rows)
    const closedKeys = new Set(rows.filter(r => closing.has(r.id)).map(r => keyText(r.groupKey)))
    return { all: false, keys: current.filter(k => !closedKeys.has(keyText(k))) }
}

export function foldAllLabel(rows: readonly ResultRow[]): 'Collapse all' | 'Expand all' | null {
    const toggles = rows.filter(row => row.kind === 'group' && row.canExpand)
    if (toggles.length === 0) return null
    return toggles.some(row => row.expanded) ? 'Collapse all' : 'Expand all'
}

export function foldAllTarget(rows: readonly ResultRow[]): Expansion {
    return foldAllLabel(rows) === 'Collapse all' ? NO_EXPANSION : EXPAND_ALL
}

export function expansionOptions(expansion: Expansion): { expand?: unknown[][]; expandAll?: boolean } {
    if (expansion.all) return { expandAll: true }
    return expansion.keys.length > 0 ? { expand: [...expansion.keys] } : {}
}

/** A result with no detail row and no group row is empty: totals alone do not count. */
export function hasRows(rows: readonly ResultRow[]): boolean {
    return rows.some(row => row.kind === 'detail' || row.kind === 'group')
}

function uniqueNotices(notices: readonly Notice[]): Notice[] {
    const seen = new Set<string>()
    return notices.filter(notice => {
        if (seen.has(notice.code)) return false
        seen.add(notice.code)
        return true
    })
}

// Every detail page ends in the total of every row (core/server/reports paging), so
// the merged rows keep each page's detail rows and the newest page's total rows.
export function mergePages(pages: readonly RunResult[]): RunResult {
    const last = pages[pages.length - 1]
    if (!last) throw new Error('mergePages: no page')
    return {
        ...last,
        rows: [
            ...pages.flatMap(page => page.rows.filter(row => row.kind !== 'total')),
            ...last.rows.filter(row => row.kind === 'total'),
        ],
        notices: uniqueNotices(pages.flatMap(page => page.notices)),
    }
}
```

- [ ] **Step 4: Run the tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run reports/lib/__tests__/rows.test.ts`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/reports/lib/rows.ts core/reports/lib/__tests__/rows.test.ts
git commit -m "feat(core): report rows, lazy folding and page merge"
```

---

### Task 8: Params, chips and the view URL

**Files:**
- Create: `core/reports/lib/params.ts`, `core/reports/lib/view-url.ts`
- Test: `core/reports/lib/__tests__/params.test.ts`, `core/reports/lib/__tests__/view-url.test.ts`

**Interfaces:**
- Consumes: `ParamDef` (Task 5), `ResolvedParam` (Task 5), `isIsoDay`, `dayWords`, `shortDay`, `rangeWords`, `rangeChipText` (Task 6), `DisplayOptions`, `DEFAULT_DISPLAY` (Task 6), `appHref` (`@tinycld/core/lib/org-routes`).
- Produces (`params.ts`): `RANGE_PRESETS`, `AS_OF_PRESETS`, `Preset`, `PRESET_LABELS`, `CUSTOM_LABEL`, `MAX_TEXT_PARAM`, `ParamValue`, `ParamValues`, `isDateParam(def)`, `presetsFor(def)`, `rangeValue(from, to)`, `valueFromJson(def, raw)`, `valueToJson(def, value)`, `defaultValueOf(def)`, `isOptional(def)`, `effectiveValue(def, values)`, `postedParams(defs, values)`, `ControlItem`, `controlItems(defs, values)`, `valueText(def, value, resolved)`, `chipText(def, value, resolved)`, `firstDateParam(defs)`, `periodWords(def, resolved)`, `emptyMessage(def, resolved)`, `EmptyAction`, `emptyAction(defs, values)`, `withoutOptional(defs, values)`.
- Produces (`view-url.ts`): `PARAM_KEY_PREFIX`, `EDIT_KEY`, `SearchParams`, `ViewOptions {params, display}`, `paramKey(id)`, `encodeParamValue(def, value)`, `decodeParamValue(def, raw)`, `parseViewOptions(defs, search)`, `serializeViewOptions(defs, options)`, `reportLink(origin, reportId, defs, options)`.

- [ ] **Step 1: Write the failing tests**

`core/reports/lib/__tests__/params.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import {
    chipText,
    controlItems,
    defaultValueOf,
    emptyAction,
    emptyMessage,
    isOptional,
    type ParamValues,
    periodWords,
    postedParams,
    valueFromJson,
    valueToJson,
} from '../params'
import type { ParamDef } from '../spec'

const PERIOD: ParamDef = { id: 'period', type: 'dateRange', label: 'Period', default: { preset: 'thisMonth' } }
const AS_OF: ParamDef = { id: 'asOf', type: 'asOf', default: { preset: 'endOfLastMonth' } }
const STATUS: ParamDef = {
    id: 'status',
    type: 'choice',
    label: 'Status',
    options: [
        { value: 'open', label: 'Open' },
        { value: 'paid', label: 'Paid' },
    ],
}
const KINDS: ParamDef = { ...STATUS, id: 'kinds', label: 'Kinds', multiple: true }
const NAME: ParamDef = { id: 'name', type: 'text', label: 'Name' }
const MIN: ParamDef = { id: 'min', type: 'number', label: 'At least', default: 100 }

describe('param values from JSON', () => {
    it('reads presets, ranges, days, options, text and numbers', () => {
        expect(valueFromJson(PERIOD, { preset: 'thisMonth' })).toEqual({ kind: 'preset', preset: 'thisMonth' })
        expect(valueFromJson(PERIOD, { from: '2026-01-01', to: '2026-03-31' })).toEqual({ kind: 'range', from: '2026-01-01', to: '2026-03-31' })
        expect(valueFromJson(AS_OF, '2026-06-30')).toEqual({ kind: 'day', day: '2026-06-30' })
        expect(valueFromJson(STATUS, 'paid')).toEqual({ kind: 'choice', values: ['paid'] })
        expect(valueFromJson(KINDS, ['paid', 'open', 'paid'])).toEqual({ kind: 'choice', values: ['open', 'paid'] })
        expect(valueFromJson(NAME, 'Acme')).toEqual({ kind: 'text', text: 'Acme' })
        expect(valueFromJson(MIN, 5)).toEqual({ kind: 'number', number: 5 })
    })

    it('refuses JSON that does not fit the param', () => {
        expect(valueFromJson(PERIOD, { preset: 'endOfLastMonth' })).toBeNull()
        expect(valueFromJson(AS_OF, { preset: 'thisMonth' })).toBeNull()
        expect(valueFromJson(PERIOD, { from: '2026-03-31', to: '2026-01-01' })).toBeNull()
        expect(valueFromJson(STATUS, 'gone')).toBeNull()
        expect(valueFromJson(KINDS, [])).toBeNull()
        expect(valueFromJson(NAME, 'x'.repeat(501))).toBeNull()
        expect(valueFromJson(MIN, Number.NaN)).toBeNull()
    })

    it('writes the JSON the run endpoint reads', () => {
        expect(valueToJson(PERIOD, { kind: 'preset', preset: 'lastMonth' })).toEqual({ preset: 'lastMonth' })
        expect(valueToJson(PERIOD, { kind: 'range', from: '2026-01-01', to: '2026-01-31' })).toEqual({ from: '2026-01-01', to: '2026-01-31' })
        expect(valueToJson(STATUS, { kind: 'choice', values: ['open'] })).toBe('open')
        expect(valueToJson(KINDS, { kind: 'choice', values: ['open'] })).toEqual(['open'])
    })
})

describe('defaults and optional params', () => {
    it('reads a default and calls a param without one optional', () => {
        expect(defaultValueOf(PERIOD)).toEqual({ kind: 'preset', preset: 'thisMonth' })
        expect(isOptional(PERIOD)).toBe(false)
        expect(isOptional(STATUS)).toBe(true)
    })

    it('posts only the values the reader set', () => {
        const values: ParamValues = { status: { kind: 'choice', values: ['open'] } }
        expect(postedParams([PERIOD, STATUS, NAME], values)).toEqual({ status: 'open' })
    })
})

describe('controlItems', () => {
    it('puts the first date param first, then params with defaults, then set filters, then Add filter', () => {
        const values: ParamValues = { name: { kind: 'text', text: 'Acme' } }
        const items = controlItems([STATUS, MIN, NAME, PERIOD], values)
        expect(items.map(item => (item.kind === 'param' ? `${item.def.id}:${item.isRemovable}` : 'add'))).toEqual([
            'period:false',
            'min:false',
            'name:true',
            'add',
        ])
        const add = items[3]
        expect(add.kind === 'add-filter' && add.defs.map(d => d.id)).toEqual(['status'])
    })

    it('offers no Add filter when every optional param is set', () => {
        const values: ParamValues = { status: { kind: 'choice', values: ['paid'] } }
        expect(controlItems([STATUS], values).map(i => i.kind)).toEqual(['param'])
    })
})

describe('chip text', () => {
    it('shows a preset with the days the server resolved', () => {
        const resolved = { id: 'period', type: 'dateRange' as const, from: '2026-10-01', to: '2026-10-31' }
        expect(chipText(PERIOD, { kind: 'preset', preset: 'thisMonth' }, resolved)).toBe('Period: This month · Oct 1 – Oct 31, 2026')
        expect(chipText(PERIOD, { kind: 'preset', preset: 'thisMonth' }, undefined)).toBe('Period: This month')
    })

    it('names choices, text and numbers', () => {
        expect(chipText(KINDS, { kind: 'choice', values: ['open', 'paid'] }, undefined)).toBe('Kinds: Open, Paid')
        expect(chipText(NAME, { kind: 'text', text: 'Acme' }, undefined)).toBe('Name: “Acme”')
        expect(chipText(AS_OF, { kind: 'day', day: '2026-06-30' }, undefined)).toBe('Jun 30, 2026')
    })
})

describe('empty state', () => {
    const range = { id: 'period', type: 'dateRange' as const, from: '2026-10-01', to: '2026-10-31' }

    it('names the period', () => {
        expect(periodWords(PERIOD, range)).toBe('October 1 – October 31, 2026')
        expect(emptyMessage(PERIOD, range)).toBe('No rows for October 1 – October 31, 2026')
        expect(emptyMessage(AS_OF, { id: 'asOf', type: 'asOf', value: '2026-09-30' })).toBe('No rows as of September 30, 2026')
        expect(emptyMessage(undefined, undefined)).toBe('No rows')
    })

    it('offers Clear filters first, then the wider period', () => {
        expect(emptyAction([PERIOD, STATUS], { status: { kind: 'choice', values: ['open'] } })).toEqual({ kind: 'clear-filters', label: 'Clear filters' })
        expect(emptyAction([PERIOD], {})).toEqual({ kind: 'preset', paramId: 'period', preset: 'thisFiscalYear', label: 'Show this fiscal year' })
        expect(emptyAction([PERIOD], { period: { kind: 'preset', preset: 'thisFiscalYear' } })).toBeNull()
        expect(emptyAction([AS_OF], {})).toEqual({ kind: 'preset', paramId: 'asOf', preset: 'endOfLastFiscalYear', label: 'Show end of last fiscal year' })
        expect(emptyAction([NAME], {})).toBeNull()
    })
})
```

`core/reports/lib/__tests__/view-url.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { DEFAULT_DISPLAY } from '../format'
import type { ParamDef } from '../spec'
import { parseViewOptions, reportLink, serializeViewOptions, type ViewOptions } from '../view-url'

const PERIOD: ParamDef = { id: 'period', type: 'dateRange', default: { preset: 'thisMonth' } }
const KINDS: ParamDef = {
    id: 'kinds',
    type: 'choice',
    multiple: true,
    options: [
        { value: 'a,b', label: 'A or B' },
        { value: 'c', label: 'C' },
    ],
}
const NAME: ParamDef = { id: 'name', type: 'text' }
const MIN: ParamDef = { id: 'min', type: 'number' }
const DEFS = [PERIOD, KINDS, NAME, MIN]

describe('the view URL', () => {
    it('round-trips params and display options', () => {
        const options: ViewOptions = {
            params: {
                period: { kind: 'range', from: '2026-01-01', to: '2026-03-31' },
                kinds: { kind: 'choice', values: ['a,b', 'c'] },
                name: { kind: 'text', text: 'Acme & Co' },
                min: { kind: 'number', number: 2.5 },
            },
            display: { divideBy1000: true, withoutCents: false, negative: 'parens', negativeRed: true, hidden: ['cost', 'cost.p0'] },
        }
        const search = serializeViewOptions(DEFS, options)
        expect(search).toMatchObject({ 'p-period': '2026-01-01..2026-03-31', div1000: '1', negative: 'parens', red: '1', hide: 'cost,cost.p0' })
        expect(parseViewOptions(DEFS, search)).toEqual(options)
    })

    it('writes nothing for defaults, so a clean link stays clean', () => {
        const search = serializeViewOptions(DEFS, { params: { period: { kind: 'preset', preset: 'thisMonth' } }, display: DEFAULT_DISPLAY })
        expect(Object.values(search).every(value => value === undefined)).toBe(true)
        expect(Object.keys(search)).toEqual(expect.arrayContaining(['p-period', 'p-kinds', 'div1000', 'hide']))
    })

    it('falls back to the default for a value the report no longer offers', () => {
        const parsed = parseViewOptions(DEFS, {
            'p-period': 'endOfLastMonth',
            'p-kinds': '["gone"]',
            'p-min': 'many',
            negative: 'sideways',
            div1000: 'yes',
            hide: 'cost,<script>',
        })
        expect(parsed.params).toEqual({})
        expect(parsed.display).toEqual({ ...DEFAULT_DISPLAY, hidden: ['cost'] })
    })

    it('reads a preset by name and refuses a backwards range', () => {
        expect(parseViewOptions(DEFS, { 'p-period': 'lastQuarter' }).params.period).toEqual({ kind: 'preset', preset: 'lastQuarter' })
        expect(parseViewOptions(DEFS, { 'p-period': '2026-03-31..2026-01-01' }).params).toEqual({})
    })

    it('builds a full link the reader can copy', () => {
        const link = reportLink('https://acme.example', 'r1', DEFS, {
            params: { name: { kind: 'text', text: 'Acme' } },
            display: DEFAULT_DISPLAY,
        })
        expect(link).toBe('https://acme.example/a/reports/r1?p-name=Acme')
    })
})
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run reports/lib/__tests__/params.test.ts reports/lib/__tests__/view-url.test.ts`
Expected: FAIL: the modules do not resolve.

- [ ] **Step 3: Implement params**

`core/reports/lib/params.ts`:

```ts
import { dayWords, isIsoDay, rangeChipText, rangeWords, shortDay } from './dates'
import type { ParamDef } from './spec'
import type { ResolvedParam } from './types'

// A report's params as the shell edits them: the period chip and the filter chips.
// Presets carry the engine's names (core/server/reports/params.go) and the Books
// shell's labels. The server resolves a preset against the reader's day and the
// source's fiscal year; the client only names it.

export const RANGE_PRESETS = [
    'today',
    'thisMonth',
    'thisMonthToDate',
    'thisQuarter',
    'thisQuarterToDate',
    'thisFiscalYear',
    'thisFiscalYearToDate',
    'lastMonth',
    'lastQuarter',
    'lastFiscalYear',
    'last12Months',
] as const
export const AS_OF_PRESETS = ['today', 'endOfLastMonth', 'endOfLastQuarter', 'endOfLastFiscalYear'] as const
const DAY_PRESETS = ['today'] as const
export type Preset = (typeof RANGE_PRESETS)[number] | (typeof AS_OF_PRESETS)[number]

export const PRESET_LABELS: Record<Preset, string> = {
    today: 'Today',
    thisMonth: 'This month',
    thisMonthToDate: 'This month to date',
    thisQuarter: 'This quarter',
    thisQuarterToDate: 'This quarter to date',
    thisFiscalYear: 'This fiscal year',
    thisFiscalYearToDate: 'This fiscal year to date',
    lastMonth: 'Last month',
    lastQuarter: 'Last quarter',
    lastFiscalYear: 'Last fiscal year',
    last12Months: 'Last 12 months',
    endOfLastMonth: 'End of last month',
    endOfLastQuarter: 'End of last quarter',
    endOfLastFiscalYear: 'End of last fiscal year',
}
export const CUSTOM_LABEL = 'Custom'
export const MAX_TEXT_PARAM = 500

export type ParamValue =
    | { kind: 'preset'; preset: Preset }
    | { kind: 'day'; day: string }
    | { kind: 'range'; from: string; to: string }
    | { kind: 'choice'; values: string[] }
    | { kind: 'text'; text: string }
    | { kind: 'number'; number: number }

/** The values the reader set; a param that is not here takes its default, or is unset. */
export type ParamValues = Readonly<Record<string, ParamValue>>

const isRecord = (value: unknown): value is Record<string, unknown> =>
    typeof value === 'object' && value !== null && !Array.isArray(value)

export function isDateParam(def: Pick<ParamDef, 'type'>): boolean {
    return def.type === 'date' || def.type === 'asOf' || def.type === 'dateRange'
}

export function presetsFor(def: Pick<ParamDef, 'type'>): readonly Preset[] {
    if (def.type === 'dateRange') return RANGE_PRESETS
    if (def.type === 'asOf') return AS_OF_PRESETS
    if (def.type === 'date') return DAY_PRESETS
    return []
}

function presetValue(def: ParamDef, raw: unknown): ParamValue | null {
    if (!isRecord(raw) || Object.keys(raw).length !== 1) return null
    const preset = presetsFor(def).find(p => p === raw.preset)
    return preset ? { kind: 'preset', preset } : null
}

export function rangeValue(from: string, to: string): ParamValue | null {
    return isIsoDay(from) && isIsoDay(to) && from <= to ? { kind: 'range', from, to } : null
}

function choiceValue(def: ParamDef, raw: unknown): ParamValue | null {
    if (!Array.isArray(raw) || raw.length === 0) return null
    const offered = (def.options ?? []).map(option => option.value)
    if (!raw.every(item => typeof item === 'string' && offered.includes(item))) return null
    const values = offered.filter(value => raw.includes(value))
    if (!def.multiple && values.length !== 1) return null
    return { kind: 'choice', values }
}

/** A value from its JSON form (a spec default): null when the JSON does not fit the param. */
export function valueFromJson(def: ParamDef, raw: unknown): ParamValue | null {
    switch (def.type) {
        case 'date':
        case 'asOf':
            if (typeof raw === 'string') return isIsoDay(raw) ? { kind: 'day', day: raw } : null
            return presetValue(def, raw)
        case 'dateRange':
            if (isRecord(raw) && typeof raw.from === 'string' && typeof raw.to === 'string' && Object.keys(raw).length === 2) {
                return rangeValue(raw.from, raw.to)
            }
            return presetValue(def, raw)
        case 'choice':
            return choiceValue(def, def.multiple ? raw : [raw])
        case 'text':
            return typeof raw === 'string' && raw.length <= MAX_TEXT_PARAM ? { kind: 'text', text: raw } : null
        case 'number':
            return typeof raw === 'number' && Number.isFinite(raw) ? { kind: 'number', number: raw } : null
    }
}

/** The JSON the run endpoint reads for a value (params.go parseParam). */
export function valueToJson(def: ParamDef, value: ParamValue): unknown {
    switch (value.kind) {
        case 'preset':
            return { preset: value.preset }
        case 'day':
            return value.day
        case 'range':
            return { from: value.from, to: value.to }
        case 'choice':
            return def.multiple ? value.values : value.values[0]
        case 'text':
            return value.text
        case 'number':
            return value.number
    }
}

export function defaultValueOf(def: ParamDef): ParamValue | null {
    if (def.default === undefined || def.default === null) return null
    return valueFromJson(def, def.default)
}

/** A param with no default is a filter the reader adds; unset, it filters nothing. */
export function isOptional(def: ParamDef): boolean {
    return defaultValueOf(def) === null
}

export function effectiveValue(def: ParamDef, values: ParamValues): ParamValue | null {
    return values[def.id] ?? defaultValueOf(def)
}

/** Only the values the reader set: an absent param takes its default on the server. */
export function postedParams(defs: readonly ParamDef[], values: ParamValues): Record<string, unknown> {
    const out: Record<string, unknown> = {}
    for (const def of defs) {
        const value = values[def.id]
        if (value) out[def.id] = valueToJson(def, value)
    }
    return out
}

export function firstDateParam(defs: readonly ParamDef[]): ParamDef | undefined {
    return defs.find(isDateParam)
}

export type ControlItem =
    | { kind: 'param'; def: ParamDef; value: ParamValue | null; isRemovable: boolean }
    | { kind: 'add-filter'; defs: ParamDef[] }

// The control row: the first date param first, then the other params in spec order.
// A param with a default is always a chip; an optional one is a chip only once set,
// and is offered under Add filter until then.
export function controlItems(defs: readonly ParamDef[], values: ParamValues): ControlItem[] {
    const period = firstDateParam(defs)
    const ordered = period ? [period, ...defs.filter(def => def !== period)] : [...defs]
    const items: ControlItem[] = []
    for (const def of ordered) {
        const optional = isOptional(def)
        const set = values[def.id]
        if (!optional) items.push({ kind: 'param', def, value: effectiveValue(def, values), isRemovable: false })
        else if (set) items.push({ kind: 'param', def, value: set, isRemovable: true })
    }
    const addable = ordered.filter(def => isOptional(def) && !values[def.id])
    if (addable.length > 0) items.push({ kind: 'add-filter', defs: addable })
    return items
}

function resolvedDates(resolved: ResolvedParam | undefined): string | null {
    if (resolved?.from && resolved.to) return rangeChipText(resolved.from, resolved.to)
    if (typeof resolved?.value === 'string' && isIsoDay(resolved.value)) return shortDay(resolved.value)
    return null
}

function choiceLabels(def: ParamDef, values: readonly string[]): string {
    const labels = values.map(value => def.options?.find(option => option.value === value)?.label ?? value)
    return labels.length <= 2 ? labels.join(', ') : `${labels.length} selected`
}

export function valueText(def: ParamDef, value: ParamValue | null, resolved: ResolvedParam | undefined): string {
    if (!value) return 'Any'
    switch (value.kind) {
        case 'preset': {
            const dates = resolvedDates(resolved)
            return dates ? `${PRESET_LABELS[value.preset]} · ${dates}` : PRESET_LABELS[value.preset]
        }
        case 'day':
            return shortDay(value.day)
        case 'range':
            return rangeChipText(value.from, value.to)
        case 'choice':
            return choiceLabels(def, value.values)
        case 'text':
            return `“${value.text}”`
        case 'number':
            return String(value.number)
    }
}

export function chipText(def: ParamDef, value: ParamValue | null, resolved: ResolvedParam | undefined): string {
    const text = valueText(def, value, resolved)
    return def.label ? `${def.label}: ${text}` : text
}

/** The period in words for the document header: the days the server ran. */
export function periodWords(def: ParamDef | undefined, resolved: ResolvedParam | undefined): string | null {
    if (!def || !resolved || resolved.unset) return null
    if (def.type === 'dateRange' && resolved.from && resolved.to) return rangeWords(resolved.from, resolved.to)
    if (typeof resolved.value !== 'string' || !isIsoDay(resolved.value)) return null
    return def.type === 'asOf' ? `As of ${dayWords(resolved.value)}` : dayWords(resolved.value)
}

export function emptyMessage(def: ParamDef | undefined, resolved: ResolvedParam | undefined): string {
    const words = periodWords(def, resolved)
    if (!def || !words) return 'No rows'
    if (def.type === 'dateRange') return `No rows for ${words}`
    if (def.type === 'asOf') return `No rows as of ${words.replace(/^As of /, '')}`
    return `No rows on ${words}`
}

export type EmptyAction =
    | { kind: 'clear-filters'; label: string }
    | { kind: 'preset'; paramId: string; preset: Preset; label: string }

export function emptyAction(defs: readonly ParamDef[], values: ParamValues): EmptyAction | null {
    if (defs.some(def => isOptional(def) && values[def.id])) return { kind: 'clear-filters', label: 'Clear filters' }
    const period = firstDateParam(defs)
    if (!period) return null
    const target = period.type === 'dateRange' ? 'thisFiscalYear' : period.type === 'asOf' ? 'endOfLastFiscalYear' : null
    if (!target) return null
    const current = effectiveValue(period, values)
    if (current?.kind === 'preset' && current.preset === target) return null
    const label = target === 'thisFiscalYear' ? 'Show this fiscal year' : 'Show end of last fiscal year'
    return { kind: 'preset', paramId: period.id, preset: target, label }
}

/** The values with every optional param removed: Clear filters. */
export function withoutOptional(defs: readonly ParamDef[], values: ParamValues): Record<string, ParamValue> {
    const out: Record<string, ParamValue> = {}
    for (const def of defs) {
        const value = values[def.id]
        if (value && !isOptional(def)) out[def.id] = value
    }
    return out
}
```

- [ ] **Step 4: Implement the view URL**

`core/reports/lib/view-url.ts`:

```ts
import { appHref } from '@tinycld/core/lib/org-routes'
import { isIsoDay } from './dates'
import { DEFAULT_DISPLAY, type DisplayOptions } from './format'
import { defaultValueOf, isDateParam, type ParamValue, presetsFor, rangeValue, valueFromJson } from './params'
import type { ParamDef } from './spec'

// A run's params and Customize options live in the route's search params, so a
// copied link reopens the report as the reader left it. Each value is checked on
// its own: a bad one falls back to its default and no other. Defaults are left
// out, so an untouched report has a clean URL.

export const PARAM_KEY_PREFIX = 'p-'
export const EDIT_KEY = 'edit'
const FLAG_ON = '1'
const FLAG_OFF = '0'
const RANGE_SEPARATOR = '..'
const LIST_SEPARATOR = ','
const COLUMN_ID = /^[A-Za-z][A-Za-z0-9_.]*$/

export type SearchParams = Record<string, string | string[] | undefined>

export interface ViewOptions {
    params: Record<string, ParamValue>
    display: DisplayOptions
}

export function paramKey(id: string): string {
    return `${PARAM_KEY_PREFIX}${id}`
}

function first(value: string | string[] | undefined): string | undefined {
    return Array.isArray(value) ? value[0] : value
}

export function encodeParamValue(def: ParamDef, value: ParamValue): string {
    switch (value.kind) {
        case 'preset':
            return value.preset
        case 'day':
            return value.day
        case 'range':
            return `${value.from}${RANGE_SEPARATOR}${value.to}`
        case 'choice':
            // An option value may hold a comma, so a list is written as JSON.
            return def.multiple ? JSON.stringify(value.values) : (value.values[0] ?? '')
        case 'text':
            return value.text
        case 'number':
            return String(value.number)
    }
}

function jsonList(raw: string): unknown {
    try {
        return JSON.parse(raw)
    } catch {
        return null
    }
}

export function decodeParamValue(def: ParamDef, raw: string): ParamValue | null {
    if (isDateParam(def)) {
        const preset = presetsFor(def).find(p => p === raw)
        if (preset) return { kind: 'preset', preset }
        if (def.type === 'dateRange') {
            const [from, to, extra] = raw.split(RANGE_SEPARATOR)
            return from && to && extra === undefined ? rangeValue(from, to) : null
        }
        return isIsoDay(raw) ? { kind: 'day', day: raw } : null
    }
    if (def.type === 'choice') return valueFromJson(def, def.multiple ? jsonList(raw) : raw)
    if (def.type === 'number') return raw.trim() === '' ? null : valueFromJson(def, Number(raw))
    return valueFromJson(def, raw)
}

function parseFlag(value: string | undefined, fallback: boolean): boolean {
    if (value === FLAG_ON) return true
    if (value === FLAG_OFF) return false
    return fallback
}

function parseHidden(raw: string | undefined): string[] {
    if (!raw) return []
    return [...new Set(raw.split(LIST_SEPARATOR).filter(id => COLUMN_ID.test(id)))]
}

export function parseViewOptions(defs: readonly ParamDef[], search: SearchParams): ViewOptions {
    const params: Record<string, ParamValue> = {}
    for (const def of defs) {
        const raw = first(search[paramKey(def.id)])
        const value = raw === undefined ? null : decodeParamValue(def, raw)
        if (value) params[def.id] = value
    }
    return {
        params,
        display: {
            divideBy1000: parseFlag(first(search.div1000), DEFAULT_DISPLAY.divideBy1000),
            withoutCents: parseFlag(first(search.nocents), DEFAULT_DISPLAY.withoutCents),
            negative: first(search.negative) === 'parens' ? 'parens' : 'minus',
            negativeRed: parseFlag(first(search.red), DEFAULT_DISPLAY.negativeRed),
            hidden: parseHidden(first(search.hide)),
        },
    }
}

function flag(on: boolean, fallback: boolean): string | undefined {
    if (on === fallback) return undefined
    return on ? FLAG_ON : FLAG_OFF
}

// Every key is present; a default is undefined so router.setParams removes it.
export function serializeViewOptions(
    defs: readonly ParamDef[],
    options: ViewOptions
): Record<string, string | undefined> {
    const out: Record<string, string | undefined> = {}
    for (const def of defs) {
        const value = options.params[def.id]
        const fallback = defaultValueOf(def)
        const text = value ? encodeParamValue(def, value) : undefined
        const isDefault = fallback !== null && text === encodeParamValue(def, fallback)
        out[paramKey(def.id)] = isDefault ? undefined : text
    }
    const { display } = options
    out.div1000 = flag(display.divideBy1000, DEFAULT_DISPLAY.divideBy1000)
    out.nocents = flag(display.withoutCents, DEFAULT_DISPLAY.withoutCents)
    out.negative = display.negative === 'parens' ? 'parens' : undefined
    out.red = flag(display.negativeRed, DEFAULT_DISPLAY.negativeRed)
    out.hide = display.hidden.length > 0 ? display.hidden.join(LIST_SEPARATOR) : undefined
    return out
}

/** The full link to a report as the reader sees it; `origin` is the server's address. */
export function reportLink(
    origin: string,
    reportId: string,
    defs: readonly ParamDef[],
    options: ViewOptions
): string {
    const query = new URLSearchParams()
    for (const [key, value] of Object.entries(serializeViewOptions(defs, options))) {
        if (value !== undefined) query.set(key, value)
    }
    const text = query.toString()
    return `${origin}${appHref(`reports/${reportId}`)}${text ? `?${text}` : ''}`
}
```

- [ ] **Step 5: Run the tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run reports/lib/__tests__/params.test.ts reports/lib/__tests__/view-url.test.ts`
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/reports/lib/params.ts core/reports/lib/view-url.ts core/reports/lib/__tests__/params.test.ts core/reports/lib/__tests__/view-url.test.ts
git commit -m "feat(core): report params, chips and the view URL"
```

---

### Task 9: Run client, re-run scheduler and the reports stores

**Files:**
- Create: `core/reports/lib/run-client.ts`, `core/reports/lib/rerun-scheduler.ts`
- Create: `core/reports/stores/report-ui-store.ts`, `core/reports/stores/report-recent-store.ts`
- Test: `core/reports/lib/__tests__/run-client.test.ts`, `core/reports/lib/__tests__/rerun-scheduler.test.ts`, `core/reports/stores/__tests__/report-recent-store.test.ts`

**Interfaces:**
- Consumes: `ReportSpec` (Task 5), `RunResult`, `CatalogSource`, `Problem` (Task 5), `Expansion`, `NO_EXPANSION` (Task 7), `pb` (`@tinycld/core/lib/pocketbase`), `isRecord` (`@tinycld/core/lib/errors`), `create`, `persist`, `asyncStorage` (`@tinycld/core/lib/store`).
- Produces (`run-client.ts`): `RUN_PATH`, `SOURCES_PATH`, `RunOptions {expand?, expandAll?, cursor?, limit?, today}`, `RunBody` (`{reportId, params, options}` | `{spec, params, options}`), `RunRefusal` (`{kind:'invalid'|'missing', message, problems}` | `{kind:'no-access'}` | `{kind:'timeout'}`), `RunOutcome` (`{kind:'ok', result}` | `RunRefusal`), `problemsOf(data)`, `refusalOf(error)`, `isTransientRunError(error)`, `runReport(body, signal?)`, `fetchCatalog(signal?)`.
- Produces (`rerun-scheduler.ts`): `RERUN_DELAY_MS = 250`, `RerunScheduler {signal(), dispose()}`, `createRerunScheduler({delayMs, run, onError})`.
- Produces (`report-ui-store.ts`): `ReportSurface` (`{kind:'param', paramId}` | `{kind:'add-filter'}`), `NEW_REPORT_KEY = 'new'`, `useReportUIStore` with `expansion: Record<string, Expansion>`, `surface`, `isCustomizeOpen`, `shareReportId: string | null`, `setExpansion(key, expansion)`, `openSurface(surface)`, `closeSurface()`, `closeSurfaceOf(kind)`, `openCustomize()`, `closeCustomize()`, `openShare(reportId)`, `closeShare()`, `startVisit(key)`.
- Produces (`report-recent-store.ts`): `RECENT_CAP = 200`, `recentKey(userId, reportId)`, `withRun(lastRun, key, at, cap?)`, `useReportRecentStore` with `lastRun: Record<string, string>` and `markRun(userId, reportId, at)`.

- [ ] **Step 1: Write the failing tests**

`core/reports/lib/__tests__/run-client.test.ts`:

```ts
import { ClientResponseError } from 'pocketbase'
import { describe, expect, it } from 'vitest'
import { isTransientRunError, refusalOf } from '../run-client'

const refused = (status: number, body: Record<string, unknown> = {}) =>
    new ClientResponseError({ status, response: { status, ...body } })

describe('refusalOf', () => {
    it('reads a 422 as missing parts with each problem by spec path', () => {
        const error = refused(422, {
            message: 'The report names something that does not exist.',
            data: {
                source: { code: 'unknown_source', message: 'the source "zoo.gone" does not exist' },
                'columns.1.path.0.link': { code: 'unknown_link', message: 'no link "keeper"' },
            },
        })
        expect(refusalOf(error)).toEqual({
            kind: 'missing',
            message: 'The report names something that does not exist.',
            problems: [
                { path: 'columns.1.path.0.link', code: 'unknown_link', message: 'no link "keeper"' },
                { path: 'source', code: 'unknown_source', message: 'the source "zoo.gone" does not exist' },
            ],
        })
    })

    it('reads 400, 403 and 408', () => {
        expect(refusalOf(refused(400, { message: 'The report is not valid.', data: {} }))).toEqual({
            kind: 'invalid',
            message: 'The report is not valid.',
            problems: [],
        })
        expect(refusalOf(refused(403, { data: { reportId: { code: 'no_access', message: 'You cannot view this report.' } } }))).toEqual({ kind: 'no-access' })
        expect(refusalOf(refused(408))).toEqual({ kind: 'timeout' })
    })

    it('leaves every other failure to be thrown', () => {
        expect(refusalOf(refused(500))).toBeNull()
        expect(refusalOf(refused(0))).toBeNull()
        expect(refusalOf(new Error('boom'))).toBeNull()
    })
})

describe('isTransientRunError', () => {
    it('retries an unreachable or failing server only', () => {
        expect(isTransientRunError(refused(0))).toBe(true)
        expect(isTransientRunError(refused(503))).toBe(true)
        expect(isTransientRunError(refused(500))).toBe(true)
        expect(isTransientRunError(refused(404))).toBe(false)
        expect(isTransientRunError(new Error('boom'))).toBe(false)
    })
})
```

`core/reports/lib/__tests__/rerun-scheduler.test.ts`:

```ts
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createRerunScheduler } from '../rerun-scheduler'

// A run that stays in flight until the test settles it.
function heldRuns() {
    const settles: (() => void)[] = []
    const run = vi.fn(
        () =>
            new Promise<void>(resolve => {
                settles.push(resolve)
            })
    )
    // The run's promise, its catch and its finally each take a microtask.
    const settle = async () => {
        settles.shift()?.()
        for (let i = 0; i < 5; i += 1) await Promise.resolve()
    }
    return { run, settle }
}

describe('createRerunScheduler', () => {
    beforeEach(() => vi.useFakeTimers())
    afterEach(() => vi.useRealTimers())

    it('runs once after a burst goes quiet', () => {
        const { run } = heldRuns()
        const scheduler = createRerunScheduler({ delayMs: 250, run, onError: vi.fn() })
        scheduler.signal()
        vi.advanceTimersByTime(200)
        scheduler.signal()
        vi.advanceTimersByTime(200)
        expect(run).not.toHaveBeenCalled()
        vi.advanceTimersByTime(50)
        expect(run).toHaveBeenCalledTimes(1)
    })

    it('queues exactly one follow-up for messages that arrive during a run', async () => {
        const { run, settle } = heldRuns()
        const scheduler = createRerunScheduler({ delayMs: 250, run, onError: vi.fn() })
        scheduler.signal()
        vi.advanceTimersByTime(250)
        scheduler.signal()
        scheduler.signal()
        scheduler.signal()
        vi.advanceTimersByTime(1000)
        expect(run).toHaveBeenCalledTimes(1)
        await settle()
        vi.advanceTimersByTime(250)
        expect(run).toHaveBeenCalledTimes(2)
        await settle()
        vi.advanceTimersByTime(1000)
        expect(run).toHaveBeenCalledTimes(2)
    })

    it('runs nothing after it is disposed', () => {
        const { run } = heldRuns()
        const scheduler = createRerunScheduler({ delayMs: 250, run, onError: vi.fn() })
        scheduler.signal()
        scheduler.dispose()
        scheduler.signal()
        vi.advanceTimersByTime(1000)
        expect(run).not.toHaveBeenCalled()
    })

    it('reports a failed run and keeps working', async () => {
        const onError = vi.fn()
        const run = vi.fn().mockRejectedValueOnce(new Error('boom')).mockResolvedValue(undefined)
        const scheduler = createRerunScheduler({ delayMs: 250, run, onError })
        scheduler.signal()
        await vi.advanceTimersByTimeAsync(250)
        for (let i = 0; i < 5; i += 1) await Promise.resolve()
        expect(onError).toHaveBeenCalledWith(expect.objectContaining({ message: 'boom' }))
        scheduler.signal()
        await vi.advanceTimersByTimeAsync(250)
        expect(run).toHaveBeenCalledTimes(2)
    })
})
```

`core/reports/stores/__tests__/report-recent-store.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { recentKey, withRun } from '../report-recent-store'

describe('withRun', () => {
    it('records a report’s last run for one reader', () => {
        expect(withRun({}, recentKey('u1', 'r1'), '2026-10-31T10:00:00.000Z')).toEqual({
            'u1:r1': '2026-10-31T10:00:00.000Z',
        })
    })

    it('keeps the newest runs when the list is full', () => {
        const lastRun = {
            'u1:a': '2026-10-01T00:00:00.000Z',
            'u1:b': '2026-10-03T00:00:00.000Z',
            'u1:c': '2026-10-02T00:00:00.000Z',
        }
        expect(withRun(lastRun, 'u1:d', '2026-10-04T00:00:00.000Z', 3)).toEqual({
            'u1:b': '2026-10-03T00:00:00.000Z',
            'u1:c': '2026-10-02T00:00:00.000Z',
            'u1:d': '2026-10-04T00:00:00.000Z',
        })
    })
})
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run reports/lib/__tests__/run-client.test.ts reports/lib/__tests__/rerun-scheduler.test.ts reports/stores/__tests__/report-recent-store.test.ts`
Expected: FAIL: the modules do not resolve.

- [ ] **Step 3: Implement the run client**

`core/reports/lib/run-client.ts`:

```ts
import { isRecord } from '@tinycld/core/lib/errors'
import { pb } from '@tinycld/core/lib/pocketbase'
import { ClientResponseError } from 'pocketbase'
import type { ReportSpec } from './spec'
import type { CatalogSource, Problem, RunResult } from './types'

// The report endpoints (core/server/reports/endpoint.go, catalog.go). A run is a
// read sent by POST, because a draft spec does not fit in a URL. The refusals a
// person can cause (400, 403, 408, 422) come back as data, so the global query
// error handler does not report them; every other failure is thrown.

export const RUN_PATH = '/api/reports/run'
export const SOURCES_PATH = '/api/reports/sources'

export interface RunOptions {
    expand?: unknown[][]
    expandAll?: boolean
    cursor?: string
    limit?: number
    /** The reader's day, which presets resolve against. */
    today: string
}

export type RunBody =
    | { reportId: string; params: Record<string, unknown>; options: RunOptions }
    | { spec: ReportSpec; params: Record<string, unknown>; options: RunOptions }

export type RunRefusal =
    | { kind: 'invalid'; message: string; problems: Problem[] }
    | { kind: 'missing'; message: string; problems: Problem[] }
    | { kind: 'no-access' }
    | { kind: 'timeout' }

export type RunOutcome = { kind: 'ok'; result: RunResult } | RunRefusal

export function problemsOf(data: unknown): Problem[] {
    if (!isRecord(data)) return []
    const out: Problem[] = []
    for (const [path, problem] of Object.entries(data)) {
        if (isRecord(problem) && typeof problem.code === 'string' && typeof problem.message === 'string') {
            out.push({ path, code: problem.code, message: problem.message })
        }
    }
    return out.sort((a, b) => a.path.localeCompare(b.path))
}

export function refusalOf(error: unknown): RunRefusal | null {
    if (!(error instanceof ClientResponseError)) return null
    const body = isRecord(error.response) ? error.response : {}
    const message = typeof body.message === 'string' ? body.message : ''
    switch (error.status) {
        case 400:
            return { kind: 'invalid', message, problems: problemsOf(body.data) }
        case 422:
            return { kind: 'missing', message, problems: problemsOf(body.data) }
        case 403:
            return { kind: 'no-access' }
        case 408:
            return { kind: 'timeout' }
    }
    return null
}

/** The server could not be reached, or failed (5xx, including the read-only 503). */
export function isTransientRunError(error: unknown): boolean {
    return error instanceof ClientResponseError && (error.status === 0 || error.status >= 500)
}

export async function runReport(body: RunBody, signal?: AbortSignal): Promise<RunOutcome> {
    try {
        // requestKey null: two reports may run at once (a frozen tab, the builder's
        // preview); the SDK would otherwise cancel one for the other.
        const result = await pb.send<RunResult>(RUN_PATH, { method: 'POST', body, signal, requestKey: null })
        return { kind: 'ok', result }
    } catch (error) {
        const refusal = refusalOf(error)
        if (refusal) return refusal
        throw error
    }
}

export async function fetchCatalog(signal?: AbortSignal): Promise<CatalogSource[]> {
    const res = await pb.send<{ sources: CatalogSource[] }>(SOURCES_PATH, { method: 'GET', signal, requestKey: null })
    return res.sources
}
```

- [ ] **Step 4: Implement the scheduler**

`core/reports/lib/rerun-scheduler.ts`:

```ts
// A report open on screen runs again when a collection it reads changes. Change
// signals come in bursts (a posted entry writes several rows), so the scheduler
// waits for quiet; a signal that arrives while a run is in flight queues one more
// run after it, so the figures shown are never older than the last change.

export const RERUN_DELAY_MS = 250

export interface RerunScheduler {
    signal(): void
    dispose(): void
}

export function createRerunScheduler(input: {
    delayMs: number
    run: () => Promise<void>
    onError: (error: unknown) => void
}): RerunScheduler {
    let timer: ReturnType<typeof setTimeout> | null = null
    let running = false
    let queued = false
    let disposed = false

    const arm = () => {
        if (timer) clearTimeout(timer)
        timer = setTimeout(start, input.delayMs)
    }

    function start() {
        timer = null
        running = true
        input
            .run()
            .catch(input.onError)
            .finally(() => {
                running = false
                if (queued && !disposed) {
                    queued = false
                    arm()
                }
            })
    }

    return {
        signal() {
            if (disposed) return
            if (running) {
                queued = true
                return
            }
            arm()
        },
        dispose() {
            disposed = true
            if (timer) clearTimeout(timer)
            timer = null
        },
    }
}
```

- [ ] **Step 5: Implement the stores**

`core/reports/stores/report-ui-store.ts`:

```ts
import { create } from '@tinycld/core/lib/store'
import { type Expansion, NO_EXPANSION } from '../lib/rows'

// Shared UI state of the reports screens. One chip popover or sheet is open at a
// time; open groups last for the session per report (not persisted, not in the
// URL); the share dialog can be opened from the hub and from a report.

export type ReportSurface = { kind: 'param'; paramId: string } | { kind: 'add-filter' }

/** The expansion key of the builder's draft before it is saved. */
export const NEW_REPORT_KEY = 'new'

interface ReportUIState {
    expansion: Record<string, Expansion>
    surface: ReportSurface | null
    isCustomizeOpen: boolean
    shareReportId: string | null
    setExpansion: (reportKey: string, expansion: Expansion) => void
    openSurface: (surface: ReportSurface) => void
    closeSurface: () => void
    // Closes the surface only while it is still of this kind: a menu closes itself
    // after an item opened the next surface, and that close must not take the new one.
    closeSurfaceOf: (kind: ReportSurface['kind']) => void
    openCustomize: () => void
    closeCustomize: () => void
    openShare: (reportId: string) => void
    closeShare: () => void
    startVisit: (reportKey: string) => void
}

export const useReportUIStore = create<ReportUIState>()(set => ({
    expansion: {},
    surface: null,
    isCustomizeOpen: false,
    shareReportId: null,
    setExpansion: (reportKey, expansion) =>
        set(state => ({ expansion: { ...state.expansion, [reportKey]: expansion } })),
    openSurface: surface => set({ surface }),
    closeSurface: () => set({ surface: null }),
    closeSurfaceOf: kind => set(state => (state.surface?.kind === kind ? { surface: null } : {})),
    openCustomize: () => set({ isCustomizeOpen: true }),
    closeCustomize: () => set({ isCustomizeOpen: false }),
    openShare: reportId => set({ shareReportId: reportId }),
    closeShare: () => set({ shareReportId: null }),
    startVisit: reportKey =>
        set(state => ({
            expansion: { ...state.expansion, [reportKey]: NO_EXPANSION },
            surface: null,
            isCustomizeOpen: false,
        })),
}))
```

`core/reports/stores/report-recent-store.ts`:

```ts
import { asyncStorage, create, persist } from '@tinycld/core/lib/store'

// When this device last ran each saved report, per reader (the hub's "Last run").
// The run endpoint writes nothing, so the record stays on the device (R12).

export const RECENT_CAP = 200

export function recentKey(userId: string, reportId: string): string {
    return `${userId}:${reportId}`
}

/** The map with one run recorded; past `cap` entries the oldest runs are dropped. */
export function withRun(
    lastRun: Readonly<Record<string, string>>,
    key: string,
    at: string,
    cap = RECENT_CAP
): Record<string, string> {
    const next = { ...lastRun, [key]: at }
    const keys = Object.keys(next)
    if (keys.length <= cap) return next
    const newest = new Set(keys.sort((a, b) => next[b].localeCompare(next[a])).slice(0, cap))
    return Object.fromEntries(Object.entries(next).filter(([k]) => newest.has(k)))
}

interface RecentState {
    lastRun: Record<string, string>
    markRun: (userId: string, reportId: string, at: string) => void
}

export const useReportRecentStore = create<RecentState>()(
    persist(
        set => ({
            lastRun: {},
            markRun: (userId, reportId, at) =>
                set(state => ({ lastRun: withRun(state.lastRun, recentKey(userId, reportId), at) })),
        }),
        { name: 'tinycld_report_recent', storage: asyncStorage, partialize: state => ({ lastRun: state.lastRun }) }
    )
)
```

- [ ] **Step 6: Run the tests, lint, typecheck**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run reports/lib/__tests__/run-client.test.ts reports/lib/__tests__/rerun-scheduler.test.ts reports/stores/__tests__/report-recent-store.test.ts && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg typecheck && cd .. && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome check core/reports`
Expected: all pass, clean.

- [ ] **Step 7: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/reports/lib/run-client.ts core/reports/lib/rerun-scheduler.ts core/reports/stores core/reports/lib/__tests__/run-client.test.ts core/reports/lib/__tests__/rerun-scheduler.test.ts
git commit -m "feat(core): report run client, re-run scheduler and stores"
```

---

### Task 10: Data hooks: catalog, live topics, runs and the saved report

**Files:**
- Create: `core/reports/hooks/useReportCatalog.ts`, `core/reports/hooks/useLiveTopics.ts`, `core/reports/hooks/useReportRun.ts`, `core/reports/hooks/useReportRecord.ts`

These hooks are covered by the feature e2e (Tasks 13–18); their rules live in the tested `lib/` modules. This task ends in a typecheck.

**Interfaces:**
- Consumes: `fetchCatalog`, `runReport`, `isTransientRunError`, `RunBody`, `RunRefusal` (Task 9); `createRerunScheduler`, `RERUN_DELAY_MS` (Task 9); `mergePages` (Task 7); `useReportRecentStore` (Task 9); `parseSpec`, `parseBroken` (Task 5); `reportAccess`, `NO_ACCESS`, `ShareRole` (Task 5); pbtsdb `subscribeTopic`, `RealtimeSubscribeAbortedError`; core `useStore`, `useMyLiveQuery`, `useCurrentRole`, `useAuth`, `recoverLiveQueries`, `log`.
- Produces:
  - `useReportCatalog(): { sources: CatalogSource[]; byKey: ReadonlyMap<string, CatalogSource>; isReady: boolean; isError: boolean; retry(): void }`
  - `useLiveTopics(topics: readonly string[], rerun: () => Promise<void>): void`
  - `type RunState = 'idle' | 'loading' | 'error' | 'invalid' | 'missing' | 'no-access' | 'timeout' | 'ready'`; `interface ReportRun { state; result: RunResult | undefined; problems: Problem[]; isUpdating: boolean; hasMore: boolean; isLoadingMore: boolean; loadMore(): void; retry(): void }`; `useReportRun(input: { body: RunBody | null; userId: string }): ReportRun`; `RUN_PAGE_SIZE = 200`.
  - `type RecordState = 'loading' | 'error' | 'missing' | 'ready'`; `useReportRecord(reportId: string): { record; spec: ReportSpec | null; broken: BrokenMark | null; access: ReportAccess; state: RecordState; retry(): void }`; `type SavedReport = NonNullable<ReturnType<typeof useReportRecord>['record']>`.

- [ ] **Step 1: Write the catalog hook**

`core/reports/hooks/useReportCatalog.ts`:

```ts
import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { fetchCatalog, isTransientRunError } from '../lib/run-client'
import type { CatalogSource } from '../lib/types'

export const CATALOG_QUERY_KEY = ['reports', 'sources'] as const
const NO_SOURCES: CatalogSource[] = []

// The registry changes only when the server restarts with other packages, so the
// catalog is kept for minutes, not fetched per screen.
export function useReportCatalog() {
    const query = useQuery({
        queryKey: CATALOG_QUERY_KEY,
        queryFn: ({ signal }) => fetchCatalog(signal),
        staleTime: 5 * 60_000,
        retry: (count, error) => count < 2 && isTransientRunError(error),
    })
    const sources = query.data ?? NO_SOURCES
    // One map per catalog answer, so a memo keyed on it (the builder's schema) holds.
    const byKey: ReadonlyMap<string, CatalogSource> = useMemo(
        () => new Map(sources.map(source => [source.key, source])),
        [sources]
    )
    return {
        sources,
        byKey,
        isReady: query.isSuccess,
        isError: query.isError,
        retry: () => void query.refetch(),
    }
}
```

- [ ] **Step 2: Write the live topics hook**

`core/reports/hooks/useLiveTopics.ts`:

```ts
import { log } from '@tinycld/core/lib/logger'
import { pb } from '@tinycld/core/lib/pocketbase'
import { RealtimeSubscribeAbortedError, subscribeTopic } from 'pbtsdb'
import { useEffect, useEffectEvent } from 'react'
import { createRerunScheduler, RERUN_DELAY_MS } from '../lib/rerun-scheduler'

// While a report is on screen it listens to the change-signal topic of every
// collection its result read, and runs again after each burst of changes. A
// subscription is an external system, which is what an effect is for; the topics
// joined into one string are its only dependency.
export function useLiveTopics(topics: readonly string[], rerun: () => Promise<void>) {
    const key = topics.join('\n')
    const onRerun = useEffectEvent(rerun)
    useEffect(() => {
        if (!key) return
        const controller = new AbortController()
        const scheduler = createRerunScheduler({
            delayMs: RERUN_DELAY_MS,
            run: () => onRerun(),
            onError: error => log.error('reports.live', error),
        })
        const subscriptions = key.split('\n').map(topic =>
            subscribeTopic(pb, topic, () => scheduler.signal(), { signal: controller.signal }).catch(
                (error: unknown) => {
                    if (!(error instanceof RealtimeSubscribeAbortedError)) {
                        log.error('reports.live', error, { topic })
                    }
                    return null
                }
            )
        )
        return () => {
            controller.abort()
            scheduler.dispose()
            for (const pending of subscriptions) {
                void pending
                    .then(unsubscribe => unsubscribe?.())
                    .catch((error: unknown) => log.warn('reports.live', 'unsubscribe failed', { error }))
            }
        }
    }, [key])
}
```

- [ ] **Step 3: Write the run hook**

`core/reports/hooks/useReportRun.ts`:

```ts
import { keepPreviousData, useInfiniteQuery } from '@tanstack/react-query'
import { mergePages } from '../lib/rows'
import { isTransientRunError, type RunBody, type RunOutcome, type RunRefusal, runReport } from '../lib/run-client'
import type { Problem, RunResult } from '../lib/types'
import { useReportRecentStore } from '../stores/report-recent-store'
import { useLiveTopics } from './useLiveTopics'

export const RUN_PAGE_SIZE = 200
const NO_TOPICS: string[] = []
const NO_PROBLEMS: Problem[] = []

export type RunState = 'idle' | 'loading' | 'error' | 'invalid' | 'missing' | 'no-access' | 'timeout' | 'ready'

export interface ReportRun {
    state: RunState
    result: RunResult | undefined
    problems: Problem[]
    /** A changed run is loading behind the figures shown (R29). */
    isUpdating: boolean
    hasMore: boolean
    isLoadingMore: boolean
    loadMore(): void
    retry(): void
}

function withPage(body: RunBody, cursor: string): RunBody {
    return { ...body, options: { ...body.options, limit: RUN_PAGE_SIZE, cursor: cursor || undefined } }
}

function firstRefusal(pages: readonly RunOutcome[]): RunRefusal | undefined {
    for (const page of pages) if (page.kind !== 'ok') return page
    return undefined
}

function runState(hasBody: boolean, isPending: boolean, isError: boolean, refusal: RunRefusal | undefined): RunState {
    if (!hasBody) return 'idle'
    if (refusal) return refusal.kind
    if (isError) return 'error'
    return isPending ? 'loading' : 'ready'
}

// Runs a report through POST /api/reports/run (R9). A report without groups pages
// by keyset; a grouped one answers in one page. The result's topics keep it live.
export function useReportRun({ body, userId }: { body: RunBody | null; userId: string }): ReportRun {
    const query = useInfiniteQuery({
        queryKey: ['reports', 'run', body],
        enabled: body !== null,
        initialPageParam: '',
        queryFn: async ({ pageParam, signal }) => {
            if (!body) throw new Error('reports.run: a run needs a body')
            const outcome = await runReport(withPage(body, pageParam), signal)
            if (outcome.kind === 'ok' && pageParam === '' && 'reportId' in body) {
                useReportRecentStore.getState().markRun(userId, body.reportId, new Date().toISOString())
            }
            return outcome
        },
        getNextPageParam: last => (last.kind === 'ok' ? last.result.page?.next || undefined : undefined),
        placeholderData: keepPreviousData,
        retry: (count, error) => count < 2 && isTransientRunError(error),
        refetchOnWindowFocus: false,
    })
    const pages = query.data?.pages ?? []
    const refusal = firstRefusal(pages)
    const results = pages.flatMap(page => (page.kind === 'ok' ? [page.result] : []))
    const result = results.length > 0 ? mergePages(results) : undefined
    useLiveTopics(result?.topics ?? NO_TOPICS, () => query.refetch().then(() => undefined))
    return {
        state: runState(body !== null, query.isPending, query.isError, refusal),
        result: refusal ? undefined : result,
        problems: refusal && 'problems' in refusal ? refusal.problems : NO_PROBLEMS,
        isUpdating: query.isPlaceholderData,
        hasMore: query.hasNextPage,
        isLoadingMore: query.isFetchingNextPage,
        loadMore: () => {
            if (query.hasNextPage && !query.isFetchingNextPage) void query.fetchNextPage()
        },
        retry: () => void query.refetch(),
    }
}
```

- [ ] **Step 4: Write the saved report hook**

`core/reports/hooks/useReportRecord.ts`:

```ts
import { and, eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useAuth } from '@tinycld/core/lib/auth'
import { log } from '@tinycld/core/lib/logger'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { recoverLiveQueries } from '@tinycld/core/lib/recover-live-queries'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'
import { NO_ACCESS, reportAccess } from '../lib/access'
import { parseBroken, parseSpec } from '../lib/spec'

export type RecordState = 'loading' | 'error' | 'missing' | 'ready'

function recordState(input: { isError: boolean; isLoading: boolean; hasRecord: boolean }): RecordState {
    if (input.isError) return 'error'
    if (input.isLoading) return 'loading'
    return input.hasRecord ? 'ready' : 'missing'
}

// One saved report, live, with what the reader may do with it. The record leaves
// the store when the reader loses access (the server sends a delete to the
// subscription), so a removed share shows at once.
export function useReportRecord(reportId: string) {
    const { user } = useAuth()
    const [reportsCollection, sharesCollection] = useStore('reports', 'report_shares')
    const role = useCurrentRole()
    const recordQuery = useLiveQuery({
        queryKey: ['reports.record', reportId],
        query: q =>
            q
                .from({ report: reportsCollection })
                .where(({ report }) => eq(report.id, reportId))
                .findOne(),
    })
    const sharesQuery = useMyLiveQuery(
        (q, { userId }) =>
            q
                .from({ share: sharesCollection })
                .where(({ share }) => and(eq(share.report, reportId), eq(share.user, userId))),
        { queryKey: ['reports.my-shares', reportId] }
    )
    const record = recordQuery.data
    const roles = (sharesQuery.data ?? []).map(share => share.role)
    const access = record
        ? reportAccess({
              owner: record.owner,
              published: record.published,
              shippedKey: record.shipped_key,
              userId: user.id,
              roles,
              isAdmin: role.isAdmin,
          })
        : NO_ACCESS
    const state = recordState({
        isError: recordQuery.isError || sharesQuery.isError,
        isLoading: !recordQuery.isReady || !sharesQuery.isReady || !role.isReady,
        hasRecord: record !== undefined,
    })
    const retry = () => {
        void recoverLiveQueries({
            queries: [recordQuery.collection, sharesQuery.collection],
            sources: [reportsCollection, sharesCollection],
            onError: error => log.error('reports.record', error),
        })
    }
    return {
        record,
        spec: record ? parseSpec(record.spec) : null,
        broken: record ? parseBroken(record.broken) : null,
        access,
        state,
        retry,
    }
}

export type SavedReport = NonNullable<ReturnType<typeof useReportRecord>['record']>
```

- [ ] **Step 5: Typecheck and lint**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome format --write core/reports/hooks && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome check core/reports && cd core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg typecheck`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/reports/hooks/useReportCatalog.ts core/reports/hooks/useLiveTopics.ts core/reports/hooks/useReportRun.ts core/reports/hooks/useReportRecord.ts
git commit -m "feat(core): report data hooks: catalog, live runs and the saved report"
```

---

### Task 11: Builder draft: columns, formulas, filters, conversion and errors

**Files:**
- Create: `core/reports/lib/builder/columns.ts`, `core/reports/lib/builder/formula.ts`, `core/reports/lib/builder/filter.ts`, `core/reports/lib/builder/draft.ts`, `core/reports/lib/builder/errors.ts`
- Test: `core/reports/lib/builder/__tests__/catalog-fixture.ts`, `columns.test.ts`, `formula.test.ts`, `filter.test.ts`, `draft.test.ts`, `errors.test.ts` (same dir)

The builder edits a draft shaped for its forms; these pure functions turn a spec into a draft and back. A part the builder cannot edit (a formula with `if`, a filter node of an unknown shape) is held as a `fixed` node and written back unchanged (R15).

**Interfaces:**
- Consumes: `SpecHop`, `SpecNode`, `ParamDef`, `ReportSpec`, `specHopSchema`, `specNodeSchema`, `paramDefSchema`, `BUCKETS`, `CURRENT_SPEC_VERSION` (Task 5); `AGGREGATES`, `CatalogField`, `CatalogSource`, `FieldType`, `Aggregate`, `Problem` (Task 5); `isIsoDay` (Task 6); `ComputeExpr`, `computeExprSchema` (`@tinycld/core/lib/forms/expression`); `FormulaOperands` (`@tinycld/core/lib/forms/builder`); `extractValidationErrors` (`@tinycld/core/lib/errors`).
- Produces (`columns.ts`): `MAX_HOPS = 4`, `MAX_COLUMNS = 100`, `MAX_LABEL = 200`, `DEFAULT_PIVOT_COLUMNS = 50`, `MAX_PIVOT_COLUMNS = 200`, `Catalog` (= `ReadonlyMap<string, CatalogSource>`), `catalogOf(sources)`, `AGGREGATE_LABELS`, `PathInfo {field, source, viaToMany, mode, labels}`, `describePath(catalog, baseKey, path)`, `pathLabel(info)`, `sourceAtLinks(catalog, baseKey, links)`, `aggregatesFor(field, viaToMany)`, `defaultAggregate(field, viaToMany)`, `aggregateResultType(aggregate, fieldType)`, `avgScaleFor(fieldType)`, `nextColumnId(taken, hint)`, `columnHint(path)`.
- Produces (`formula.ts`): `FormulaLeaf` (`'col' | 'subtotal'`), `RATIO_SCALE = 4`, `toComputeExpr(node, leaf)`, `fromComputeExpr(expr, leaf, isAmount)`, `formulaOperands(choices, selfKey)`.
- Produces (`filter.ts`): `CONDITION_OPS`, `ConditionOp`, `ConditionDraft`, `RelatedDraft`, `GroupDraft`, `FixedDraft`, `FilterItem`, `filterGroupSchema`, `emptyGroup()`, `emptyCondition(path)`, `opsFor(type)`, `opLabel(op, type)`, `needsValue(op)`, `centsFromText(text)`, `textFromCents(cents)`, `askParamFor(field, op, id)`, `filterToNode(group, catalog, baseKey)`, `nodeToFilter(node, catalog, baseKey)`, `paramsInNode(node, into)`, `paramsInGroup(group, into)`, `conditionProblems(group, catalog, baseKey)`.
- Produces (`draft.ts`): `formulaDraftSchema`, `FormulaDraft`, `columnDraftSchema`, `ColumnDraft`, `LayoutItemDraft`, `builderSchema(catalog)`, `BuilderDraft`, `draftIssues(draft, catalog)`, `emptyDraft(source)`, `draftFromSpec(spec, meta, catalog)`, `specFromDraft(draft, catalog)`, `columnIsAmount(column, draft, catalog)`, `columnChoices(draft, catalog)`.
- Produces (`errors.ts`): `BuilderSection` (`'details' | 'columns' | 'filter' | 'group' | 'pivot' | 'layout'`), `sectionOf(specPath)`, `describeProblem(problem)`, `problemsFromError(error)`, `problemsFor(section, problems)`.

- [ ] **Step 1: Write the test catalog**

`core/reports/lib/builder/__tests__/catalog-fixture.ts`:

```ts
import { catalogOf } from '../columns'

// The salesstub sources (tests/scripts/reports-stub/server/register.go.txt), as
// the catalog endpoint answers for them.
export const SALES_KEY = 'salesstub.sales'
export const TEAMS_KEY = 'salesstub.teams'

export const CATALOG = catalogOf([
    {
        key: SALES_KEY,
        label: 'Stub sales',
        fields: [
            { key: 'id', label: 'ID', type: 'relation', aggregates: ['count', 'countDistinct'] },
            { key: 'customer', label: 'Customer', type: 'text', aggregates: ['count', 'countDistinct'] },
            { key: 'amount', label: 'Amount', type: 'money', aggregates: ['sum', 'avg', 'min', 'max', 'count', 'countDistinct'], currency: 'currency' },
            { key: 'cost', label: 'Cost', type: 'money', aggregates: ['sum', 'avg', 'min', 'max', 'count', 'countDistinct'], currency: 'currency' },
            { key: 'currency', label: 'Currency', type: 'text', aggregates: ['count', 'countDistinct'] },
            { key: 'day', label: 'Day', type: 'date', aggregates: ['min', 'max', 'count', 'countDistinct'] },
            {
                key: 'status',
                label: 'Status',
                type: 'select',
                aggregates: ['count', 'countDistinct'],
                options: [
                    { value: 'open', label: 'Open' },
                    { value: 'paid', label: 'Paid' },
                ],
            },
        ],
        links: [{ key: 'team', label: 'Team', target: TEAMS_KEY, cardinality: 'toOne' }],
    },
    {
        key: TEAMS_KEY,
        label: 'Stub teams',
        fields: [
            { key: 'id', label: 'ID', type: 'relation', aggregates: ['count', 'countDistinct'] },
            { key: 'name', label: 'Name', type: 'text', aggregates: ['min', 'max', 'count', 'countDistinct'] },
        ],
        links: [{ key: 'sales', label: 'Sales', target: SALES_KEY, cardinality: 'toMany' }],
    },
])
```

- [ ] **Step 2: Write the failing tests**

`core/reports/lib/builder/__tests__/columns.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import {
    aggregatesFor,
    columnHint,
    defaultAggregate,
    describePath,
    nextColumnId,
    pathLabel,
    sourceAtLinks,
} from '../columns'
import { CATALOG, SALES_KEY, TEAMS_KEY } from './catalog-fixture'

describe('describePath', () => {
    it('follows a to-one link to a field', () => {
        const info = describePath(CATALOG, SALES_KEY, [{ link: 'team' }, { field: 'name' }])
        expect(info && pathLabel(info)).toBe('Team › Name')
        expect(info).toMatchObject({ viaToMany: false, mode: undefined })
    })

    it('marks a path through a to-many link and keeps its mode', () => {
        const info = describePath(CATALOG, TEAMS_KEY, [{ link: 'sales', mode: 'aggregate' }, { field: 'amount' }])
        expect(info).toMatchObject({ viaToMany: true, mode: 'aggregate' })
        expect(info?.field.key).toBe('amount')
    })

    it('reads an unknown link or field as nothing', () => {
        expect(describePath(CATALOG, SALES_KEY, [{ link: 'vendor' }, { field: 'name' }])).toBeNull()
        expect(describePath(CATALOG, SALES_KEY, [{ field: 'gone' }])).toBeNull()
        expect(describePath(CATALOG, 'zoo.gone', [{ field: 'name' }])).toBeNull()
    })

    it('finds the source a list of links reaches', () => {
        expect(sourceAtLinks(CATALOG, SALES_KEY, [{ link: 'team' }, { link: 'sales' }])?.key).toBe(SALES_KEY)
        expect(sourceAtLinks(CATALOG, SALES_KEY, [{ link: 'nope' }])).toBeNull()
    })
})

describe('aggregates', () => {
    const sales = CATALOG.get(SALES_KEY)
    const amount = sales?.fields.find(f => f.key === 'amount')
    const day = sales?.fields.find(f => f.key === 'day')

    it('offers a field its own aggregates on its own source', () => {
        expect(amount && aggregatesFor(amount, false)).toEqual(['sum', 'avg', 'min', 'max', 'count', 'countDistinct'])
        expect(amount && defaultAggregate(amount, false)).toBe('')
    })

    it('refuses what a to-many total cannot add up', () => {
        // An amount with a per-row currency cannot be totalled per parent, so only a count.
        expect(amount && aggregatesFor(amount, true)).toEqual(['count'])
        expect(amount && defaultAggregate(amount, true)).toBe('count')
        expect(day && aggregatesFor(day, true)).toEqual(['min', 'max', 'count'])
    })
})

describe('nextColumnId', () => {
    it('makes an id from a hint and keeps it unique', () => {
        expect(nextColumnId([], 'customer')).toBe('customer')
        expect(nextColumnId(['customer'], 'customer')).toBe('customer2')
        expect(nextColumnId(['customer', 'customer2'], 'customer')).toBe('customer3')
        expect(nextColumnId([], '1 st/name')).toBe('stname')
        expect(nextColumnId([], '42')).toBe('col')
    })

    it('hints an id from a path', () => {
        expect(columnHint([{ link: 'team' }, { field: 'name' }])).toBe('teamName')
        expect(columnHint([{ field: 'amount' }])).toBe('amount')
    })
})
```

`core/reports/lib/builder/__tests__/formula.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import type { SpecNode } from '../../spec'
import { formulaOperands, fromComputeExpr, toComputeExpr } from '../formula'

const amounts = new Set(['amount', 'cost'])
const isAmount = (key: string) => amounts.has(key)

describe('the forms formula editor and report formulas', () => {
    it('round-trips arithmetic over columns', () => {
        const node: SpecNode = { op: 'sub', args: [{ col: 'amount' }, { col: 'cost' }] }
        const expr = toComputeExpr(node, 'col')
        expect(expr).toEqual({ sub: [{ var: 'amount' }, { var: 'cost' }] })
        expect(expr === null ? null : fromComputeExpr(expr, 'col', isAmount)).toEqual(node)
    })

    it('reads a division with its scale as Round of a Divide, and back', () => {
        const node: SpecNode = { op: 'div', args: [{ col: 'profit' }, { col: 'amount' }], scale: 2 }
        const expr = toComputeExpr(node, 'col')
        expect(expr).toEqual({ round: [{ div: [{ var: 'profit' }, { var: 'amount' }] }, 2] })
        expect(expr === null ? null : fromComputeExpr(expr, 'col', isAmount)).toEqual(node)
    })

    it('gives a bare Divide scale 0 for an amount over a number, else 4', () => {
        expect(fromComputeExpr({ div: [{ var: 'amount' }, 2] }, 'col', isAmount)).toEqual({
            op: 'div',
            args: [{ col: 'amount' }, { value: 2 }],
            scale: 0,
        })
        expect(fromComputeExpr({ div: [{ var: 'cost' }, { var: 'amount' }] }, 'col', isAmount)).toMatchObject({ scale: 4 })
    })

    it('writes Round of anything else as round with a scale', () => {
        expect(fromComputeExpr({ round: [{ mul: [{ var: 'amount' }, 1.1] }, 0] }, 'col', isAmount)).toEqual({
            op: 'round',
            args: [{ op: 'mul', args: [{ col: 'amount' }, { value: 1.1 }] }],
            scale: 0,
        })
    })

    it('reads layout rows over subtotals', () => {
        const node: SpecNode = { op: 'sub', args: [{ subtotal: 'income' }, { subtotal: 'expense' }] }
        const expr = toComputeExpr(node, 'subtotal')
        expect(expr).toEqual({ sub: [{ var: 'income' }, { var: 'expense' }] })
        expect(expr === null ? null : fromComputeExpr(expr, 'subtotal', () => true)).toEqual(node)
    })

    it('leaves to the fixed view what the editor cannot show', () => {
        expect(toComputeExpr({ op: 'if', args: [{ col: 'a' }, { value: 1 }, { value: 2 }] }, 'col')).toBeNull()
        expect(toComputeExpr({ op: 'sum', args: [{ col: 'amount' }] }, 'col')).toBeNull()
        expect(toComputeExpr({ op: 'max', args: [{ col: 'amount' }] }, 'col')).toBeNull()
        expect(toComputeExpr({ op: 'add', args: [{ col: 'a' }, { param: 'n' }] }, 'col')).toBeNull()
        expect(toComputeExpr({ col: 'amount' }, 'subtotal')).toBeNull()
    })

    it('offers the other numeric columns to a column formula', () => {
        const operands = formulaOperands(
            [
                { key: 'customer', label: 'Customer', isNumeric: false },
                { key: 'amount', label: 'Amount', isNumeric: true },
                { key: 'profit', label: 'Profit', isNumeric: true },
            ],
            'profit'
        )
        expect(operands).toEqual({ values: [{ key: 'amount', label: 'Amount' }], tables: [] })
    })
})
```

`core/reports/lib/builder/__tests__/filter.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import type { SpecNode } from '../../spec'
import {
    askParamFor,
    centsFromText,
    conditionProblems,
    emptyCondition,
    filterToNode,
    type GroupDraft,
    nodeToFilter,
    opsFor,
    paramsInGroup,
    textFromCents,
} from '../filter'
import { CATALOG, SALES_KEY, TEAMS_KEY } from './catalog-fixture'

const roundTrip = (node: SpecNode, base = SALES_KEY) => filterToNode(nodeToFilter(node, CATALOG, base), CATALOG, base)

describe('conditions', () => {
    it('round-trips each shape the builder writes', () => {
        const day = { op: 'addDays', args: [{ path: [{ field: 'day' }] }, { value: 0 }] }
        const shapes: SpecNode[] = [
            { op: 'contains', args: [{ path: [{ field: 'customer' }] }, { value: 'acme' }] },
            { op: 'gt', args: [{ path: [{ field: 'amount' }] }, { value: 50000 }] },
            { op: 'eq', args: [day, { value: '2026-01-05' }] },
            { op: 'lte', args: [day, { param: 'asOf' }] },
            { op: 'between', args: [day, { value: '2026-01-01' }, { value: '2026-01-31' }] },
            { op: 'between', args: [{ path: [{ field: 'day' }] }, { param: 'period' }] },
            { op: 'between', args: [{ path: [{ field: 'amount' }] }, { value: 100 }, { value: 200 }] },
            { op: 'in', args: [{ path: [{ field: 'status' }] }, { value: ['open', 'paid'] }] },
            { op: 'isEmpty', args: [{ path: [{ link: 'team' }, { field: 'name' }] }] },
            { op: 'not', args: [{ op: 'isEmpty', args: [{ path: [{ field: 'customer' }] }] }] },
        ]
        for (const node of shapes) expect(roundTrip(node)).toEqual(node)
    })

    it('reads a money value in cents and writes it back exactly', () => {
        const group = nodeToFilter({ op: 'gte', args: [{ path: [{ field: 'amount' }] }, { value: 123456 }] }, CATALOG, SALES_KEY)
        const condition = group.items[0]
        expect(condition?.kind === 'condition' && condition.value).toBe('1234.56')
        expect(centsFromText('1234.5')).toBe(123450)
        expect(centsFromText('-0.07')).toBe(-7)
        expect(centsFromText('12.345')).toBeNull()
        expect(textFromCents(-7)).toBe('-0.07')
    })

    it('keeps a date comparison of another shape as a fixed part', () => {
        const raw: SpecNode = { op: 'lte', args: [{ path: [{ field: 'day' }] }, { param: 'asOf' }] }
        const group = nodeToFilter(raw, CATALOG, SALES_KEY)
        expect(group.items[0]?.kind).toBe('fixed')
        expect(filterToNode(group, CATALOG, SALES_KEY)).toEqual(raw)
    })
})

describe('groups and related rows', () => {
    it('round-trips all, any and none', () => {
        const a = { op: 'eq', args: [{ path: [{ field: 'status' }] }, { value: 'open' }] }
        const b = { op: 'contains', args: [{ path: [{ field: 'customer' }] }, { value: 'x' }] }
        expect(roundTrip({ op: 'and', args: [a, b] })).toEqual({ op: 'and', args: [a, b] })
        expect(roundTrip({ op: 'or', args: [a, b] })).toEqual({ op: 'or', args: [a, b] })
        expect(roundTrip({ op: 'not', args: [{ op: 'or', args: [a, b] }] })).toEqual({ op: 'not', args: [{ op: 'or', args: [a, b] }] })
        expect(roundTrip({ op: 'and', args: [a, { op: 'or', args: [a, b] }] })).toEqual({ op: 'and', args: [a, { op: 'or', args: [a, b] }] })
    })

    it('round-trips has and has no related rows with their own conditions', () => {
        const node: SpecNode = {
            op: 'exists',
            link: [{ link: 'sales' }],
            where: { op: 'gt', args: [{ path: [{ field: 'amount' }] }, { value: 50000 }] },
        }
        expect(roundTrip(node, TEAMS_KEY)).toEqual(node)
        expect(roundTrip({ op: 'notExists', link: [{ link: 'sales' }] }, TEAMS_KEY)).toEqual({ op: 'notExists', link: [{ link: 'sales' }] })
    })

    it('writes no filter for an empty group, and keeps an unknown node as it is', () => {
        expect(filterToNode({ kind: 'group', match: 'all', items: [] }, CATALOG, SALES_KEY)).toBeUndefined()
        const odd: SpecNode = { op: 'startsWith', args: [{ op: 'upper', args: [{ path: [{ field: 'customer' }] }] }, { value: 'A' }] }
        expect(roundTrip(odd)).toEqual(odd)
    })
})

describe('asking when run', () => {
    const sales = CATALOG.get(SALES_KEY)
    const field = (key: string) => {
        const found = sales?.fields.find(f => f.key === key)
        if (!found) throw new Error(key)
        return found
    }

    it('makes a param that fits the field and operator', () => {
        expect(askParamFor(field('day'), 'between', 'period')).toEqual({ id: 'period', type: 'dateRange', label: 'Day' })
        expect(askParamFor(field('day'), 'lte', 'asOf')).toEqual({ id: 'asOf', type: 'asOf', label: 'Day' })
        expect(askParamFor(field('customer'), 'contains', 'who')).toEqual({ id: 'who', type: 'text', label: 'Customer' })
        expect(askParamFor(field('status'), 'in', 's')).toEqual({
            id: 's',
            type: 'choice',
            label: 'Status',
            multiple: true,
            options: [
                { value: 'open', label: 'Open' },
                { value: 'paid', label: 'Paid' },
            ],
        })
        expect(askParamFor(field('amount'), 'gt', 'a')).toBeNull()
        expect(askParamFor(field('customer'), 'isEmpty', 'e')).toBeNull()
    })

    it('lists the params a filter asks for', () => {
        const group = nodeToFilter(
            { op: 'and', args: [
                { op: 'between', args: [{ path: [{ field: 'day' }] }, { param: 'period' }] },
                { op: 'startsWith', args: [{ op: 'upper', args: [{ param: 'raw' }] }, { value: 'A' }] },
            ] },
            CATALOG,
            SALES_KEY
        )
        expect([...paramsInGroup(group, new Set())].sort()).toEqual(['period', 'raw'])
    })
})

describe('client checks', () => {
    it('names a condition with a missing or wrong value', () => {
        const group: GroupDraft = {
            kind: 'group',
            match: 'all',
            items: [
                { ...emptyCondition([{ field: 'amount' }]), op: 'gt', value: 'lots' },
                { ...emptyCondition([{ field: 'day' }]), op: 'eq', value: '2026-02-30' },
                { ...emptyCondition([{ field: 'customer' }]), op: 'contains', value: '' },
                { ...emptyCondition([{ field: 'customer' }]), op: 'isEmpty' },
            ],
        }
        expect(conditionProblems(group, CATALOG, SALES_KEY)).toEqual([
            'Amount: enter an amount such as 12.50',
            'Day: enter a date as YYYY-MM-DD',
            'Customer: enter a value',
        ])
    })

    it('offers operators by field type', () => {
        expect(opsFor('bool')).toEqual(['isTrue', 'isFalse'])
        expect(opsFor('select')).toEqual(['eq', 'ne', 'in', 'isEmpty', 'notEmpty'])
        expect(opsFor('date')).toContain('between')
    })
})
```

`core/reports/lib/builder/__tests__/draft.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import type { ReportSpec } from '../../spec'
import { builderSchema, draftFromSpec, emptyDraft, specFromDraft } from '../draft'
import { CATALOG, SALES_KEY, TEAMS_KEY } from './catalog-fixture'

const RICH: ReportSpec = {
    version: 1,
    source: SALES_KEY,
    params: [{ id: 'period', type: 'dateRange', label: 'Day', default: { preset: 'thisMonth' } }],
    columns: [
        { id: 'status', label: 'Status', path: [{ field: 'status' }] },
        { id: 'team', label: 'Team › Name', path: [{ link: 'team' }, { field: 'name' }] },
        { id: 'amount', label: 'Amount', path: [{ field: 'amount' }], aggregate: 'sum' },
        { id: 'cost', label: 'Cost', path: [{ field: 'cost' }], aggregate: 'sum' },
        { id: 'profit', label: 'Profit', formula: { op: 'sub', args: [{ col: 'amount' }, { col: 'cost' }] }, aggregate: 'sum' },
        { id: 'avg', label: 'Average', path: [{ field: 'amount' }], aggregate: 'avg', scale: 0 },
    ],
    filter: { op: 'between', args: [{ path: [{ field: 'day' }] }, { param: 'period' }] },
    sort: [{ col: 'amount', dir: 'desc' }],
    groups: [{ id: 'byStatus', col: 'status', subtotals: true }],
    pivot: { path: [{ field: 'day' }], bucket: 'month', cell: 'amount', maxColumns: 12 },
    layout: [
        { section: 'open', title: 'Open', key: 'open' },
        { section: 'paid', title: 'Paid', key: 'paid' },
        { row: 'gap', label: 'Open less paid', formula: { op: 'sub', args: [{ subtotal: 'open' }, { subtotal: 'paid' }] } },
    ],
}

describe('draftFromSpec and specFromDraft', () => {
    it('round-trip a spec the builder can edit in full', () => {
        const draft = draftFromSpec(RICH, { title: 'Sales', description: '' }, CATALOG)
        expect(specFromDraft(draft, CATALOG)).toEqual(RICH)
    })

    it('keeps parts the builder cannot edit unchanged', () => {
        const shipped: ReportSpec = {
            version: 1,
            source: SALES_KEY,
            columns: [
                { id: 'customer', label: 'Customer', path: [{ field: 'customer' }] },
                {
                    id: 'flag',
                    label: 'Big',
                    formula: { op: 'if', args: [{ op: 'gt', args: [{ col: 'amount' }, { value: 100 }] }, { value: 1 }, { value: 0 }] },
                },
                { id: 'amount', label: 'Amount', path: [{ field: 'amount' }] },
            ],
            filter: { op: 'case', args: [{ value: true }, { value: true }] },
        }
        const draft = draftFromSpec(shipped, { title: 'Shipped', description: '' }, CATALOG)
        expect(draft.columns[1]).toMatchObject({ kind: 'formula', formula: { kind: 'fixed' } })
        const renamed = { ...draft, title: 'Renamed' }
        expect(specFromDraft(renamed, CATALOG)).toEqual(shipped)
    })

    it('drops a param no filter or formula asks for', () => {
        const draft = draftFromSpec(RICH, { title: 'Sales', description: '' }, CATALOG)
        const unasked = { ...draft, filter: { kind: 'group' as const, match: 'all' as const, items: [] } }
        expect(specFromDraft(unasked, CATALOG).params).toBeUndefined()
    })
})

describe('emptyDraft', () => {
    it('starts a new report with the source’s first field', () => {
        const source = CATALOG.get(TEAMS_KEY)
        if (!source) throw new Error('fixture')
        const draft = emptyDraft(source)
        expect(draft.columns).toEqual([
            { kind: 'path', id: 'name', label: 'Name', path: [{ field: 'name' }], aggregate: '', scale: null, format: '' },
        ])
        expect(specFromDraft({ ...draft, title: 'Teams' }, CATALOG)).toEqual({
            version: 1,
            source: TEAMS_KEY,
            columns: [{ id: 'name', label: 'Name', path: [{ field: 'name' }] }],
        })
    })
})

describe('builderSchema', () => {
    const schema = builderSchema(CATALOG)
    const base = draftFromSpec(RICH, { title: 'Sales', description: '' }, CATALOG)
    const messages = (value: unknown) => {
        const parsed = schema.safeParse(value)
        return parsed.success ? [] : parsed.error.issues.map(issue => `${issue.path.join('.')}: ${issue.message}`)
    }

    it('accepts a valid draft', () => {
        expect(messages(base)).toEqual([])
    })

    it('asks how to total a column across a to-many link', () => {
        const source = CATALOG.get(TEAMS_KEY)
        if (!source) throw new Error('fixture')
        const draft = {
            ...emptyDraft(source),
            title: 'Teams',
            columns: [{ kind: 'path', id: 'n', label: 'Sales', path: [{ link: 'sales', mode: 'aggregate' }, { field: 'id' }], aggregate: '', scale: null, format: '' }],
        }
        expect(messages(draft)).toEqual(['columns.0.aggregate: Choose how to total it'])
    })

    it('refuses a pivot or a layout without a group, and two columns with one id', () => {
        const noGroups = { ...base, groups: [], sort: [] }
        expect(messages(noGroups)).toEqual(['pivot.on: A pivot needs a group', 'layout: A layout needs a group'])
        const twice = { ...base, columns: [...base.columns, { ...base.columns[0] }] }
        expect(messages(twice)).toContain('columns.6.id: Two columns have this id')
    })

    it('needs a title', () => {
        expect(messages({ ...base, title: ' ' })).toEqual(['title: Enter a title'])
    })
})
```

`core/reports/lib/builder/__tests__/errors.test.ts`:

```ts
import { ClientResponseError } from 'pocketbase'
import { describe, expect, it } from 'vitest'
import { describeProblem, problemsFor, problemsFromError, sectionOf } from '../errors'

describe('builder errors', () => {
    it('puts each spec path in its builder section', () => {
        expect(sectionOf('columns.2.path.1.link')).toBe('columns')
        expect(sectionOf('filter.args.0')).toBe('filter')
        expect(sectionOf('params.0.default')).toBe('filter')
        expect(sectionOf('groups.1.bucket')).toBe('group')
        expect(sectionOf('sort.0.col')).toBe('group')
        expect(sectionOf('pivot.cell')).toBe('pivot')
        expect(sectionOf('layout.3.formula')).toBe('layout')
        expect(sectionOf('source')).toBe('details')
        expect(sectionOf('title')).toBe('details')
    })

    it('names the part a problem is about', () => {
        expect(describeProblem({ path: 'columns.2.path.1.link', code: 'unknown_link', message: 'no link "x"' })).toBe('Column 3: no link "x"')
        expect(describeProblem({ path: 'groups.0.col', code: 'invalid', message: 'pick one' })).toBe('Group 1: pick one')
        expect(describeProblem({ path: 'source', code: 'unknown_source', message: 'gone' })).toBe('gone')
    })

    it('reads a save refusal and filters it by section', () => {
        const error = new ClientResponseError({
            status: 400,
            response: {
                message: 'The report is not valid.',
                data: { 'columns.0.id': { code: 'invalid', message: 'bad id' }, title: { code: 'validation_required', message: 'Cannot be blank.' } },
            },
        })
        const problems = problemsFromError(error)
        expect(problemsFor('columns', problems)).toEqual([{ path: 'columns.0.id', code: '', message: 'bad id' }])
        expect(problemsFor('details', problems)).toEqual([{ path: 'title', code: '', message: 'Cannot be blank.' }])
    })
})
```

- [ ] **Step 3: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run reports/lib/builder`
Expected: FAIL: the builder modules do not resolve.

- [ ] **Step 4: Write the columns module**

`core/reports/lib/builder/columns.ts`:

```ts
import type { SpecHop } from '../spec'
import type { Aggregate, CatalogField, CatalogSource, FieldType } from '../types'

// Columns as the builder offers them: a path of link hops ending in a field, as
// the catalog describes it. The engine's limits are repeated here only to stop a
// person early; the server is the judge.

export const MAX_HOPS = 4
export const MAX_COLUMNS = 100
export const MAX_LABEL = 200
export const DEFAULT_PIVOT_COLUMNS = 50
export const MAX_PIVOT_COLUMNS = 200

export type Catalog = ReadonlyMap<string, CatalogSource>

export function catalogOf(sources: readonly CatalogSource[]): Catalog {
    return new Map(sources.map(source => [source.key, source]))
}

export const AGGREGATE_LABELS: Record<Aggregate, string> = {
    sum: 'Sum',
    count: 'Count',
    countDistinct: 'Count distinct',
    min: 'Smallest',
    max: 'Largest',
    avg: 'Average',
}

export interface PathInfo {
    field: CatalogField
    source: CatalogSource
    /** The path follows a to-many link. */
    viaToMany: boolean
    /** The to-many hop's mode: a total per row, or child rows. */
    mode: 'aggregate' | 'nest' | undefined
    labels: string[]
}

export function describePath(catalog: Catalog, baseKey: string, path: readonly SpecHop[]): PathInfo | null {
    let source = catalog.get(baseKey)
    const labels: string[] = []
    let viaToMany = false
    let mode: PathInfo['mode']
    for (const hop of path) {
        if (!source) return null
        if (hop.link) {
            const link = source.links.find(l => l.key === hop.link)
            if (!link) return null
            labels.push(link.label)
            if (link.cardinality === 'toMany') {
                viaToMany = true
                mode = hop.mode
            }
            source = catalog.get(link.target)
            continue
        }
        const field = source.fields.find(f => f.key === hop.field)
        return field ? { field, source, viaToMany, mode, labels: [...labels, field.label] } : null
    }
    return null
}

export function pathLabel(info: PathInfo): string {
    return info.labels.join(' › ')
}

/** The source a list of link hops reaches (an exists filter's target), or null. */
export function sourceAtLinks(catalog: Catalog, baseKey: string, links: readonly SpecHop[]): CatalogSource | null {
    let source = catalog.get(baseKey) ?? null
    for (const hop of links) {
        const link = source?.links.find(l => l.key === hop.link)
        source = link ? (catalog.get(link.target) ?? null) : null
    }
    return source
}

// A total across a to-many link is computed per parent row. A distinct count
// cannot be, and an amount whose currency differs by row cannot be totalled
// apart by currency (the engine's Decision 8 refuses both).
export function aggregatesFor(field: CatalogField, viaToMany: boolean): Aggregate[] {
    if (!viaToMany) return [...field.aggregates]
    const offered = field.aggregates.filter(a => a !== 'countDistinct')
    return field.type === 'money' && field.currency ? offered.filter(a => a === 'count') : offered
}

export function defaultAggregate(field: CatalogField, viaToMany: boolean): Aggregate | '' {
    if (!viaToMany) return ''
    const offered = aggregatesFor(field, true)
    if ((field.type === 'money' || field.type === 'number') && offered.includes('sum')) return 'sum'
    return offered.includes('count') ? 'count' : (offered[0] ?? '')
}

export function aggregateResultType(aggregate: Aggregate | '', fieldType: FieldType): FieldType {
    return aggregate === 'count' || aggregate === 'countDistinct' ? 'number' : fieldType
}

/** An average needs a scale: whole cents for amounts, two places for numbers. */
export function avgScaleFor(fieldType: FieldType): number {
    return fieldType === 'money' ? 0 : 2
}

export function nextColumnId(taken: readonly string[], hint: string): string {
    const stem = hint.replace(/[^A-Za-z0-9_]/g, '').replace(/^[^A-Za-z]+/, '').slice(0, 60) || 'col'
    if (!taken.includes(stem)) return stem
    for (let n = 2; ; n += 1) {
        const id = `${stem}${n}`
        if (!taken.includes(id)) return id
    }
}

/** A column id hint from a path: its keys run together, `team` + `name` is `teamName`. */
export function columnHint(path: readonly SpecHop[]): string {
    return path
        .map(hop => hop.link ?? hop.field ?? '')
        .map((key, i) => (i === 0 ? key : key.charAt(0).toUpperCase() + key.slice(1)))
        .join('')
}
```

- [ ] **Step 5: Write the formula module**

`core/reports/lib/builder/formula.ts`:

```ts
import type { FormulaOperands } from '@tinycld/core/lib/forms/builder'
import type { ComputeExpr } from '@tinycld/core/lib/forms/expression'
import type { SpecNode } from '../spec'

// The builder edits formulas with the forms engine's FormulaEditor (spec §4), whose
// tree is ComputeExpr. A report formula is the engine's { op, args } tree. These
// two functions translate the arithmetic both share; any other operator is not
// editable here and stays a fixed part (R15). A report division always carries a
// scale: Round of a Divide is a division at that scale, and a bare Divide takes
// whole cents for an amount over a number, else RATIO_SCALE places.

export type FormulaLeaf = 'col' | 'subtotal'
export const RATIO_SCALE = 4
const MAX_PLACES = 6
const ARITHMETIC = new Set(['add', 'sub', 'mul', 'div', 'round', 'min', 'max'])

function onlyKeys(node: SpecNode, keys: readonly string[]): boolean {
    return Object.entries(node).every(([key, value]) => value === undefined || keys.includes(key))
}

function hasScale(node: SpecNode): node is SpecNode & { scale: number } {
    return node.scale !== undefined && node.scale >= 0 && node.scale <= MAX_PLACES
}

export function toComputeExpr(node: SpecNode, leaf: FormulaLeaf): ComputeExpr | null {
    if (typeof node.value === 'number' && onlyKeys(node, ['value'])) return node.value
    if (leaf === 'col' && node.col && onlyKeys(node, ['col'])) return { var: node.col }
    if (leaf === 'subtotal' && node.subtotal && onlyKeys(node, ['subtotal'])) return { var: node.subtotal }
    if (!node.op || !ARITHMETIC.has(node.op) || !onlyKeys(node, ['op', 'args', 'scale'])) return null
    const args: ComputeExpr[] = []
    for (const arg of node.args ?? []) {
        const expr = toComputeExpr(arg, leaf)
        if (expr === null) return null
        args.push(expr)
    }
    const [a, b] = args
    switch (node.op) {
        case 'add':
            return args.length >= 2 ? { add: args } : null
        case 'mul':
            return args.length >= 2 ? { mul: args } : null
        // One argument makes min and max aggregates in a report formula.
        case 'min':
            return args.length >= 2 ? { min: args } : null
        case 'max':
            return args.length >= 2 ? { max: args } : null
        case 'sub':
            return args.length === 2 && a !== undefined && b !== undefined ? { sub: [a, b] } : null
        case 'div':
            return args.length === 2 && a !== undefined && b !== undefined && hasScale(node)
                ? { round: [{ div: [a, b] }, node.scale] }
                : null
        case 'round':
            return args.length === 1 && a !== undefined && hasScale(node) ? { round: [a, node.scale] } : null
    }
    return null
}

function isAmountExpr(expr: ComputeExpr, isAmount: (key: string) => boolean): boolean {
    const amount = (e: ComputeExpr) => isAmountExpr(e, isAmount)
    if (typeof expr === 'number') return false
    if ('var' in expr) return isAmount(expr.var)
    if ('add' in expr) return expr.add.some(amount)
    if ('sub' in expr) return expr.sub.some(amount)
    if ('min' in expr) return expr.min.some(amount)
    if ('max' in expr) return expr.max.some(amount)
    if ('mul' in expr) return expr.mul.filter(amount).length === 1
    if ('div' in expr) return amount(expr.div[0]) && !amount(expr.div[1])
    if ('round' in expr) return amount(expr.round[0])
    return false
}

export function fromComputeExpr(
    expr: ComputeExpr,
    leaf: FormulaLeaf,
    isAmount: (key: string) => boolean
): SpecNode {
    const walk = (e: ComputeExpr): SpecNode => {
        if (typeof e === 'number') return { value: e }
        if ('var' in e) return leaf === 'col' ? { col: e.var } : { subtotal: e.var }
        if ('add' in e) return { op: 'add', args: e.add.map(walk) }
        if ('sub' in e) return { op: 'sub', args: e.sub.map(walk) }
        if ('mul' in e) return { op: 'mul', args: e.mul.map(walk) }
        if ('div' in e) {
            const [top, bottom] = e.div
            const scale = isAmountExpr(top, isAmount) && !isAmountExpr(bottom, isAmount) ? 0 : RATIO_SCALE
            return { op: 'div', args: e.div.map(walk), scale }
        }
        if ('round' in e) {
            const [inner, places] = e.round
            if (typeof inner === 'object' && 'div' in inner) return { op: 'div', args: inner.div.map(walk), scale: places }
            return { op: 'round', args: [walk(inner)], scale: places }
        }
        if ('min' in e) return e.min.length === 1 && e.min[0] !== undefined ? walk(e.min[0]) : { op: 'min', args: e.min.map(walk) }
        if ('max' in e) return e.max.length === 1 && e.max[0] !== undefined ? walk(e.max[0]) : { op: 'max', args: e.max.map(walk) }
        // Sum of a column and Number of rows read form tables; a report offers none.
        return { value: 0 }
    }
    return walk(expr)
}

/** What a formula may read: the other numeric choices, and no table. */
export function formulaOperands(
    choices: readonly { key: string; label: string; isNumeric: boolean }[],
    selfKey: string
): FormulaOperands {
    return {
        values: choices
            .filter(choice => choice.key !== selfKey && choice.isNumeric)
            .map(choice => ({ key: choice.key, label: choice.label })),
        tables: [],
    }
}
```

- [ ] **Step 6: Write the filter module**

`core/reports/lib/builder/filter.ts`:

```ts
import { z } from '@tinycld/core/ui/form'
import { isIsoDay } from '../dates'
import { type ParamDef, type SpecHop, type SpecNode, specHopSchema, specNodeSchema } from '../spec'
import type { CatalogField, FieldType } from '../types'
import { type Catalog, describePath, sourceAtLinks } from './columns'

// The builder's filter: groups of conditions joined by All of / Any of / None of,
// "has / has no related" rows over a link, and fixed parts it cannot edit. Only
// shapes this module writes are read back as editable, so a hand-written or
// shipped filter is never rewritten by a save (R15). A date is compared by its
// day: addDays(field, 0) is the day of a stored date-time (R31).

export const CONDITION_OPS = [
    'eq',
    'ne',
    'lt',
    'lte',
    'gt',
    'gte',
    'between',
    'in',
    'contains',
    'startsWith',
    'isEmpty',
    'notEmpty',
    'isTrue',
    'isFalse',
] as const
export type ConditionOp = (typeof CONDITION_OPS)[number]

export type ConditionDraft = {
    kind: 'condition'
    path: SpecHop[]
    op: ConditionOp
    /** The value, or the low end of between. Money is typed in units (12.50). */
    value: string
    /** The high end of between. */
    high: string
    /** The options of "is any of". */
    values: string[]
    /** A param id when the value is asked when the report runs; else ''. */
    param: string
}
export type FixedDraft = { kind: 'fixed'; node: SpecNode }
export type GroupDraft = { kind: 'group'; match: 'all' | 'any' | 'none'; items: FilterItem[] }
export type RelatedDraft = { kind: 'related'; has: boolean; link: SpecHop[]; where: GroupDraft }
export type FilterItem = ConditionDraft | RelatedDraft | GroupDraft | FixedDraft

const conditionSchema = z.object({
    kind: z.literal('condition'),
    path: z.array(specHopSchema).min(1),
    op: z.enum(CONDITION_OPS),
    value: z.string(),
    high: z.string(),
    values: z.array(z.string()),
    param: z.string(),
})
const fixedSchema = z.object({ kind: z.literal('fixed'), node: specNodeSchema })

export const filterGroupSchema: z.ZodType<GroupDraft, GroupDraft> = z.lazy(() =>
    z.object({
        kind: z.literal('group'),
        match: z.enum(['all', 'any', 'none']),
        items: z.array(filterItemSchema),
    })
)
const relatedSchema: z.ZodType<RelatedDraft, RelatedDraft> = z.lazy(() =>
    z.object({ kind: z.literal('related'), has: z.boolean(), link: z.array(specHopSchema).min(1), where: filterGroupSchema })
)
const filterItemSchema: z.ZodType<FilterItem, FilterItem> = z.lazy(() =>
    z.union([conditionSchema, relatedSchema, filterGroupSchema, fixedSchema])
)

export function emptyGroup(): GroupDraft {
    return { kind: 'group', match: 'all', items: [] }
}

export function emptyCondition(path: SpecHop[]): ConditionDraft {
    return { kind: 'condition', path, op: 'eq', value: '', high: '', values: [], param: '' }
}

const OPS_BY_TYPE: Record<FieldType, ConditionOp[]> = {
    text: ['eq', 'ne', 'contains', 'startsWith', 'isEmpty', 'notEmpty'],
    relation: ['eq', 'ne', 'isEmpty', 'notEmpty'],
    number: ['eq', 'ne', 'lt', 'lte', 'gt', 'gte', 'between', 'isEmpty', 'notEmpty'],
    money: ['eq', 'ne', 'lt', 'lte', 'gt', 'gte', 'between', 'isEmpty', 'notEmpty'],
    percent: ['eq', 'ne', 'lt', 'lte', 'gt', 'gte', 'between'],
    date: ['eq', 'lt', 'lte', 'gt', 'gte', 'between', 'isEmpty', 'notEmpty'],
    datetime: ['eq', 'lt', 'lte', 'gt', 'gte', 'between', 'isEmpty', 'notEmpty'],
    bool: ['isTrue', 'isFalse'],
    select: ['eq', 'ne', 'in', 'isEmpty', 'notEmpty'],
}

export function opsFor(type: FieldType): ConditionOp[] {
    return OPS_BY_TYPE[type]
}

const isDayType = (type: FieldType) => type === 'date' || type === 'datetime'
const isAmountType = (type: FieldType) => type === 'money'

export function opLabel(op: ConditionOp, type: FieldType): string {
    if (isDayType(type)) {
        const day: Partial<Record<ConditionOp, string>> = { eq: 'is on', lt: 'is before', lte: 'is on or before', gt: 'is after', gte: 'is on or after' }
        if (day[op]) return day[op]
    }
    const labels: Record<ConditionOp, string> = {
        eq: type === 'number' || type === 'money' || type === 'percent' ? '=' : 'is',
        ne: type === 'number' || type === 'money' || type === 'percent' ? '≠' : 'is not',
        lt: '<',
        lte: '≤',
        gt: '>',
        gte: '≥',
        between: 'is between',
        in: 'is any of',
        contains: 'contains',
        startsWith: 'starts with',
        isEmpty: 'is empty',
        notEmpty: 'is not empty',
        isTrue: 'is yes',
        isFalse: 'is no',
    }
    return labels[op]
}

export function needsValue(op: ConditionOp): boolean {
    return !['isEmpty', 'notEmpty', 'isTrue', 'isFalse'].includes(op)
}

export function centsFromText(text: string): number | null {
    const match = /^(-?)(\d+)(?:\.(\d{1,2}))?$/.exec(text.trim())
    if (!match) return null
    const cents = Number(match[2]) * 100 + Number((match[3] ?? '').padEnd(2, '0'))
    return match[1] ? -cents : cents
}

export function textFromCents(cents: number): string {
    const magnitude = Math.abs(cents)
    const text = `${Math.floor(magnitude / 100)}.${String(magnitude % 100).padStart(2, '0')}`
    return cents < 0 ? `-${text}` : text
}

/** The param an asked value becomes, or null when the field cannot be asked (R16). */
export function askParamFor(field: CatalogField, op: ConditionOp, id: string): ParamDef | null {
    const label = field.label
    if (!needsValue(op)) return null
    if (isDayType(field.type)) {
        if (op === 'between') return { id, type: 'dateRange', label }
        if (op === 'lte') return { id, type: 'asOf', label }
        return op === 'in' ? null : { id, type: 'date', label }
    }
    if (field.type === 'number') return op === 'between' ? null : { id, type: 'number', label }
    if (field.type === 'text' || field.type === 'relation') return { id, type: 'text', label }
    if (field.type === 'select') {
        const options = field.options ?? []
        if (op === 'in') return { id, type: 'choice', label, multiple: true, options }
        return { id, type: 'choice', label, options }
    }
    return null
}

function fieldType(catalog: Catalog, baseKey: string, path: readonly SpecHop[]): FieldType | null {
    return describePath(catalog, baseKey, path)?.field.type ?? null
}

function literal(type: FieldType, text: string): unknown {
    if (isAmountType(type)) return centsFromText(text) ?? 0
    if (type === 'number' || type === 'percent') return Number(text)
    return text
}

function conditionNode(c: ConditionDraft, type: FieldType): SpecNode {
    const target: SpecNode = { path: c.path }
    const subject: SpecNode = isDayType(type) ? { op: 'addDays', args: [target, { value: 0 }] } : target
    const leaf = (text: string): SpecNode => (c.param ? { param: c.param } : { value: literal(type, text) })
    switch (c.op) {
        case 'isEmpty':
            return { op: 'isEmpty', args: [target] }
        case 'notEmpty':
            return { op: 'not', args: [{ op: 'isEmpty', args: [target] }] }
        case 'isTrue':
            return { op: 'eq', args: [target, { value: true }] }
        case 'isFalse':
            return { op: 'eq', args: [target, { value: false }] }
        case 'between':
            // A date range param reads the stored value's day itself (params.go).
            if (c.param) return { op: 'between', args: [target, { param: c.param }] }
            return { op: 'between', args: [subject, leaf(c.value), leaf(c.high)] }
        case 'in':
            return { op: 'in', args: [target, c.param ? { param: c.param } : { value: c.values }] }
        default:
            return { op: c.op, args: [subject, leaf(c.value)] }
    }
}

function groupNode(group: GroupDraft, catalog: Catalog, baseKey: string): SpecNode | undefined {
    const nodes = group.items
        .map(item => itemNode(item, catalog, baseKey))
        .filter((node): node is SpecNode => node !== undefined)
    if (nodes.length === 0) return undefined
    const joined = nodes.length === 1 && nodes[0] ? nodes[0] : { op: group.match === 'all' ? 'and' : 'or', args: nodes }
    return group.match === 'none' ? { op: 'not', args: [joined] } : joined
}

function itemNode(item: FilterItem, catalog: Catalog, baseKey: string): SpecNode | undefined {
    switch (item.kind) {
        case 'fixed':
            return item.node
        case 'group':
            return groupNode(item, catalog, baseKey)
        case 'related': {
            const target = sourceAtLinks(catalog, baseKey, item.link)
            const where = target ? groupNode(item.where, catalog, target.key) : undefined
            const node: SpecNode = { op: item.has ? 'exists' : 'notExists', link: item.link }
            return where ? { ...node, where } : node
        }
        case 'condition': {
            const type = fieldType(catalog, baseKey, item.path)
            return type ? conditionNode(item, type) : undefined
        }
    }
}

export function filterToNode(group: GroupDraft, catalog: Catalog, baseKey: string): SpecNode | undefined {
    return groupNode(group, catalog, baseKey)
}

function onlyKeys(node: SpecNode, keys: readonly string[]): boolean {
    return Object.entries(node).every(([key, value]) => value === undefined || keys.includes(key))
}

function plainPath(node: SpecNode | undefined): SpecHop[] | null {
    return node?.path && onlyKeys(node, ['path']) ? node.path : null
}

function dayPath(node: SpecNode | undefined): SpecHop[] | null {
    if (node?.op !== 'addDays' || !onlyKeys(node, ['op', 'args'])) return null
    const [subject, zero] = node.args ?? []
    if (node.args?.length !== 2 || !zero || zero.value !== 0 || !onlyKeys(zero, ['value'])) return null
    return plainPath(subject)
}

type Leaf = { param: string } | { text: string }

function leafOf(node: SpecNode | undefined, type: FieldType): Leaf | null {
    if (!node) return null
    if (node.param && onlyKeys(node, ['param'])) return { param: node.param }
    if (!onlyKeys(node, ['value'])) return null
    const v = node.value
    if (isAmountType(type)) return typeof v === 'number' && Number.isSafeInteger(v) ? { text: textFromCents(v) } : null
    if (type === 'number' || type === 'percent') return typeof v === 'number' ? { text: String(v) } : null
    return typeof v === 'string' ? { text: v } : null
}

function condition(path: SpecHop[], op: ConditionOp, parts: Partial<ConditionDraft> = {}): ConditionDraft {
    return { ...emptyCondition(path), op, ...parts }
}

function fromLeaf(leaf: Leaf): Partial<ConditionDraft> {
    return 'param' in leaf ? { param: leaf.param } : { value: leaf.text }
}

// What a condition node compares: the field's path, as written plainly or as the
// day of a date (addDays(path, 0)), and the field's type.
type Subject = { raw: SpecHop[] | null; day: SpecHop[] | null; path: SpecHop[]; type: FieldType }

function paramLeaf(node: SpecNode | undefined): string | null {
    return node?.param && onlyKeys(node, ['param']) ? node.param : null
}

function inCondition(s: Subject, args: SpecNode[]): ConditionDraft | null {
    if (!s.raw || args.length !== 2) return null
    const param = paramLeaf(args[1])
    if (param) return condition(s.raw, 'in', { param })
    const list = args[1] && onlyKeys(args[1], ['value']) ? args[1].value : null
    if (!Array.isArray(list) || !list.every(v => typeof v === 'string')) return null
    return condition(s.raw, 'in', { values: list })
}

function betweenCondition(s: Subject, args: SpecNode[]): ConditionDraft | null {
    const wantsDay = isDayType(s.type)
    const param = paramLeaf(args[1])
    if (args.length === 2 && s.raw && param) return wantsDay ? condition(s.raw, 'between', { param }) : null
    if (args.length !== 3 || (wantsDay ? !s.day : !s.raw)) return null
    const low = leafOf(args[1], s.type)
    const high = leafOf(args[2], s.type)
    if (!low || !high || 'param' in low || 'param' in high) return null
    return condition(s.path, 'between', { value: low.text, high: high.text })
}

function boolCondition(s: Subject, args: SpecNode[]): ConditionDraft | null {
    const second = args[1]
    if (!s.raw || args.length !== 2 || s.type !== 'bool' || !second || !onlyKeys(second, ['value'])) return null
    return typeof second.value === 'boolean' ? condition(s.raw, second.value ? 'isTrue' : 'isFalse') : null
}

const COMPARE_OPS: readonly ConditionOp[] = ['eq', 'ne', 'lt', 'lte', 'gt', 'gte', 'contains', 'startsWith']

function compareCondition(s: Subject, op: string, args: SpecNode[]): ConditionDraft | null {
    const known = COMPARE_OPS.find(o => o === op)
    if (!known || args.length !== 2 || (isDayType(s.type) ? !s.day : !s.raw)) return null
    const leaf = leafOf(args[1], s.type)
    return leaf ? condition(s.path, known, fromLeaf(leaf)) : null
}

function subjectOf(node: SpecNode | undefined, catalog: Catalog, baseKey: string): Subject | null {
    const raw = plainPath(node)
    const day = dayPath(node)
    const path = raw ?? day
    const type = path ? fieldType(catalog, baseKey, path) : null
    return path && type ? { raw, day, path, type } : null
}

function conditionFrom(node: SpecNode, catalog: Catalog, baseKey: string): ConditionDraft | null {
    if (!node.op || !onlyKeys(node, ['op', 'args'])) return null
    const args = node.args ?? []
    const s = subjectOf(args[0], catalog, baseKey)
    if (!s) return null
    if (node.op === 'isEmpty') return s.raw && args.length === 1 ? condition(s.raw, 'isEmpty') : null
    if (node.op === 'in') return inCondition(s, args)
    if (node.op === 'between') return betweenCondition(s, args)
    const yesNo = node.op === 'eq' ? boolCondition(s, args) : null
    return yesNo ?? compareCondition(s, node.op, args)
}

function itemFrom(node: SpecNode, catalog: Catalog, baseKey: string): FilterItem {
    if ((node.op === 'and' || node.op === 'or') && onlyKeys(node, ['op', 'args']) && (node.args?.length ?? 0) >= 2) {
        return { kind: 'group', match: node.op === 'and' ? 'all' : 'any', items: (node.args ?? []).map(a => itemFrom(a, catalog, baseKey)) }
    }
    if (node.op === 'not' && onlyKeys(node, ['op', 'args']) && node.args?.length === 1 && node.args[0]) {
        const inner = node.args[0]
        const empty = inner.op === 'isEmpty' ? conditionFrom(inner, catalog, baseKey) : null
        if (empty) return { ...empty, op: 'notEmpty' }
        const items = inner.op === 'or' && onlyKeys(inner, ['op', 'args']) && (inner.args?.length ?? 0) >= 2 ? (inner.args ?? []) : [inner]
        return { kind: 'group', match: 'none', items: items.map(a => itemFrom(a, catalog, baseKey)) }
    }
    if ((node.op === 'exists' || node.op === 'notExists') && node.link && onlyKeys(node, ['op', 'link', 'where'])) {
        const target = sourceAtLinks(catalog, baseKey, node.link)
        if (target) {
            return { kind: 'related', has: node.op === 'exists', link: node.link, where: nodeToFilter(node.where, catalog, target.key) }
        }
    }
    return conditionFrom(node, catalog, baseKey) ?? { kind: 'fixed', node }
}

export function nodeToFilter(node: SpecNode | null | undefined, catalog: Catalog, baseKey: string): GroupDraft {
    if (!node) return emptyGroup()
    const item = itemFrom(node, catalog, baseKey)
    return item.kind === 'group' ? item : { kind: 'group', match: 'all', items: [item] }
}

/** Every param id a node reads, fixed parts included. */
export function paramsInNode(node: SpecNode | undefined, into: Set<string>): Set<string> {
    if (!node) return into
    if (node.param) into.add(node.param)
    for (const arg of node.args ?? []) paramsInNode(arg, into)
    paramsInNode(node.where, into)
    return into
}

export function paramsInGroup(group: GroupDraft, into: Set<string>): Set<string> {
    for (const item of group.items) {
        if (item.kind === 'condition' && item.param) into.add(item.param)
        if (item.kind === 'fixed') paramsInNode(item.node, into)
        if (item.kind === 'group') paramsInGroup(item, into)
        if (item.kind === 'related') paramsInGroup(item.where, into)
    }
    return into
}

function valueProblem(c: ConditionDraft, field: CatalogField): string | null {
    if (!needsValue(c.op) || c.param) return null
    if (c.op === 'in') return c.values.length > 0 ? null : 'choose at least one'
    const texts = c.op === 'between' ? [c.value, c.high] : [c.value]
    for (const text of texts) {
        if (text.trim() === '') return 'enter a value'
        if (isAmountType(field.type) && centsFromText(text) === null) return 'enter an amount such as 12.50'
        if ((field.type === 'number' || field.type === 'percent') && !Number.isFinite(Number(text))) return 'enter a number'
        if (isDayType(field.type) && !isIsoDay(text)) return 'enter a date as YYYY-MM-DD'
    }
    return null
}

/** What a person must fix before the filter can run, one line per condition. */
export function conditionProblems(group: GroupDraft, catalog: Catalog, baseKey: string): string[] {
    const out: string[] = []
    for (const item of group.items) {
        if (item.kind === 'group') out.push(...conditionProblems(item, catalog, baseKey))
        if (item.kind === 'related') {
            const target = sourceAtLinks(catalog, baseKey, item.link)
            if (target) out.push(...conditionProblems(item.where, catalog, target.key))
            else out.push('A related-rows condition names a link that is no longer offered')
        }
        if (item.kind !== 'condition') continue
        const info = describePath(catalog, baseKey, item.path)
        if (!info) {
            out.push('A condition names a field that is no longer offered')
            continue
        }
        const problem = valueProblem(item, info.field)
        if (problem) out.push(`${info.field.label}: ${problem}`)
    }
    return out
}
```

- [ ] **Step 7: Write the draft module**

`core/reports/lib/builder/draft.ts`:

```ts
import { type ComputeExpr, computeExprSchema } from '@tinycld/core/lib/forms/expression'
import { z } from '@tinycld/core/ui/form'
import {
    BUCKETS,
    CURRENT_SPEC_VERSION,
    type ParamDef,
    paramDefSchema,
    type ReportSpec,
    type SpecColumn,
    specHopSchema,
    type SpecNode,
    specNodeSchema,
} from '../spec'
import { AGGREGATES, type CatalogSource, type FieldType } from '../types'
import {
    aggregateResultType,
    aggregatesFor,
    avgScaleFor,
    type Catalog,
    DEFAULT_PIVOT_COLUMNS,
    describePath,
    MAX_COLUMNS,
    MAX_HOPS,
    MAX_LABEL,
    MAX_PIVOT_COLUMNS,
    nextColumnId,
    pathLabel,
} from './columns'
import { conditionProblems, emptyGroup, filterGroupSchema, filterToNode, nodeToFilter, paramsInGroup, paramsInNode } from './filter'
import { fromComputeExpr, toComputeExpr } from './formula'

// The builder's form value. specFromDraft and draftFromSpec turn it into the
// engine's spec and back; the form's zod schema (builderSchema) checks what a
// person can fix before a run, and the server checks the rest.

const ID_RE = /^[A-Za-z][A-Za-z0-9_]{0,63}$/
const idSchema = z.string().regex(ID_RE, 'Use letters, digits and _, starting with a letter')
const labelSchema = z.string().trim().min(1, 'Enter a label').max(MAX_LABEL)
const AGGREGATE_CHOICES = ['', ...AGGREGATES] as const
const BUCKET_CHOICES = ['', ...BUCKETS] as const
// computeExprSchema reads unknown input; the form needs one type in and out.
const exprSchema = z.custom<ComputeExpr>(value => computeExprSchema.safeParse(value).success, 'This formula is not valid')

export const formulaDraftSchema = z.union([
    z.object({ kind: z.literal('editable'), expr: exprSchema }),
    z.object({ kind: z.literal('fixed'), node: specNodeSchema }),
])
export type FormulaDraft = z.infer<typeof formulaDraftSchema>

const columnFields = {
    id: idSchema,
    label: labelSchema,
    aggregate: z.enum(AGGREGATE_CHOICES),
    /** An average's decimal places; null takes the field type's default. */
    scale: z.number().int().min(0).max(6).nullable(),
    /** The client's display format, kept as given ('percent' is the one the builder offers). */
    format: z.string().max(50),
}
export const columnDraftSchema = z.union([
    z.object({ kind: z.literal('path'), ...columnFields, path: z.array(specHopSchema).min(1).max(MAX_HOPS + 1) }),
    z.object({ kind: z.literal('formula'), ...columnFields, formula: formulaDraftSchema }),
])
export type ColumnDraft = z.infer<typeof columnDraftSchema>

const layoutItemSchema = z.union([
    z.object({ kind: z.literal('section'), id: idSchema, title: labelSchema, key: z.string().min(1, 'Choose the group this section shows') }),
    z.object({ kind: z.literal('row'), id: idSchema, label: labelSchema, formula: formulaDraftSchema }),
])
export type LayoutItemDraft = z.infer<typeof layoutItemSchema>

const draftShape = z.object({
    title: z.string().trim().min(1, 'Enter a title').max(200),
    description: z.string().max(2000),
    source: z.string().min(1),
    params: z.array(paramDefSchema).max(20),
    columns: z.array(columnDraftSchema).min(1, 'Add a column').max(MAX_COLUMNS),
    filter: filterGroupSchema,
    groups: z
        .array(z.object({ id: idSchema, col: z.string().min(1, 'Choose a column'), bucket: z.enum(BUCKET_CHOICES), subtotals: z.boolean() }))
        .max(6),
    sort: z.array(z.object({ col: z.string().min(1, 'Choose a column'), dir: z.enum(['asc', 'desc']) })).max(10),
    pivot: z.object({
        on: z.boolean(),
        path: z.array(specHopSchema),
        bucket: z.enum(BUCKET_CHOICES),
        cell: z.string(),
        maxColumns: z.number().int().min(1).max(MAX_PIVOT_COLUMNS),
    }),
    layout: z.array(layoutItemSchema).max(200),
})
export type BuilderDraft = z.infer<typeof draftShape>

type Issue = { path: (string | number)[]; message: string }

export function draftIssues(draft: BuilderDraft, catalog: Catalog): Issue[] {
    const issues: Issue[] = []
    const ids = draft.columns.map(column => column.id)
    draft.columns.forEach((column, i) => {
        if (ids.indexOf(column.id) !== i) issues.push({ path: ['columns', i, 'id'], message: 'Two columns have this id' })
        if (column.kind !== 'path') return
        const info = describePath(catalog, draft.source, column.path)
        if (!info) {
            issues.push({ path: ['columns', i, 'label'], message: 'This field is no longer offered' })
            return
        }
        if (info.mode === 'aggregate' && column.aggregate === '') {
            issues.push({ path: ['columns', i, 'aggregate'], message: 'Choose how to total it' })
        } else if (column.aggregate !== '' && !aggregatesFor(info.field, info.viaToMany).includes(column.aggregate)) {
            issues.push({ path: ['columns', i, 'aggregate'], message: 'This total is not offered for this field' })
        }
    })
    draft.groups.forEach((group, i) => {
        if (group.col !== '' && !ids.includes(group.col)) issues.push({ path: ['groups', i, 'col'], message: 'Choose a column' })
    })
    draft.sort.forEach((key, i) => {
        if (key.col !== '' && !ids.includes(key.col)) issues.push({ path: ['sort', i, 'col'], message: 'Choose a column' })
    })
    if (draft.pivot.on) {
        if (draft.groups.length === 0) issues.push({ path: ['pivot', 'on'], message: 'A pivot needs a group' })
        if (!describePath(catalog, draft.source, draft.pivot.path)) issues.push({ path: ['pivot', 'path'], message: 'Choose a field' })
        const cell = draft.columns.find(column => column.id === draft.pivot.cell)
        if (!cell || cell.aggregate === '') issues.push({ path: ['pivot', 'cell'], message: 'Choose a column with a total' })
    }
    if (draft.layout.length > 0 && draft.groups.length === 0) issues.push({ path: ['layout'], message: 'A layout needs a group' })
    const layoutIds = draft.layout.map(item => item.id)
    draft.layout.forEach((item, i) => {
        if (layoutIds.indexOf(item.id) !== i) issues.push({ path: ['layout', i, 'id'], message: 'Two layout items have this id' })
    })
    const [firstProblem] = conditionProblems(draft.filter, catalog, draft.source)
    if (firstProblem) issues.push({ path: ['filter'], message: firstProblem })
    return issues
}

export function builderSchema(catalog: Catalog) {
    return draftShape.superRefine((draft, ctx) => {
        for (const issue of draftIssues(draft, catalog)) ctx.addIssue({ code: 'custom', path: issue.path, message: issue.message })
    })
}

export function emptyDraft(source: CatalogSource): BuilderDraft {
    const first = source.fields.find(field => field.key !== 'id') ?? source.fields[0]
    const columns: ColumnDraft[] = first
        ? [{ kind: 'path', id: nextColumnId([], first.key), label: first.label, path: [{ field: first.key }], aggregate: '', scale: null, format: '' }]
        : []
    return {
        title: '',
        description: '',
        source: source.key,
        params: [],
        columns,
        filter: emptyGroup(),
        groups: [],
        sort: [],
        pivot: { on: false, path: [], bucket: '', cell: '', maxColumns: DEFAULT_PIVOT_COLUMNS },
        layout: [],
    }
}

function formulaDraft(node: SpecNode, leaf: 'col' | 'subtotal'): FormulaDraft {
    const expr = toComputeExpr(node, leaf)
    return expr === null ? { kind: 'fixed', node } : { kind: 'editable', expr }
}

function columnDraft(column: SpecColumn, catalog: Catalog, baseKey: string): ColumnDraft {
    const common = {
        id: column.id,
        aggregate: column.aggregate ?? '',
        scale: column.scale ?? null,
        format: column.format ?? '',
    } as const
    if (column.formula) {
        return { kind: 'formula', ...common, label: column.label ?? column.id, formula: formulaDraft(column.formula, 'col') }
    }
    const path = column.path ?? []
    const info = describePath(catalog, baseKey, path)
    return { kind: 'path', ...common, label: column.label ?? (info ? pathLabel(info) : column.id), path }
}

export function draftFromSpec(
    spec: ReportSpec,
    meta: { title: string; description: string },
    catalog: Catalog
): BuilderDraft {
    return {
        title: meta.title,
        description: meta.description,
        source: spec.source,
        params: spec.params ?? [],
        columns: spec.columns.map(column => columnDraft(column, catalog, spec.source)),
        filter: nodeToFilter(spec.filter, catalog, spec.source),
        groups: (spec.groups ?? []).map(g => ({ id: g.id, col: g.col, bucket: g.bucket ?? '', subtotals: g.subtotals === true })),
        sort: (spec.sort ?? []).map(s => ({ col: s.col, dir: s.dir ?? 'asc' })),
        pivot: spec.pivot
            ? { on: true, path: spec.pivot.path, bucket: spec.pivot.bucket ?? '', cell: spec.pivot.cell, maxColumns: spec.pivot.maxColumns ?? DEFAULT_PIVOT_COLUMNS }
            : { on: false, path: [], bucket: '', cell: '', maxColumns: DEFAULT_PIVOT_COLUMNS },
        layout: (spec.layout ?? []).map((item): LayoutItemDraft =>
            item.row
                ? { kind: 'row', id: item.row, label: item.label ?? item.row, formula: formulaDraft(item.formula ?? { value: 0 }, 'subtotal') }
                : { kind: 'section', id: item.section ?? '', title: item.title ?? '', key: String(item.key ?? '') }
        ),
    }
}

type ColumnsOf = Pick<BuilderDraft, 'source' | 'columns'>

/** The type of a column's detail cells, for formula operands, pivot cells and layout keys. */
function columnType(column: ColumnDraft, draft: ColumnsOf, catalog: Catalog): FieldType {
    if (column.kind === 'formula') return 'number'
    const info = describePath(catalog, draft.source, column.path)
    return info ? aggregateResultType(column.aggregate, info.field.type) : 'text'
}

export function columnIsAmount(column: ColumnDraft, draft: ColumnsOf, catalog: Catalog): boolean {
    return columnType(column, draft, catalog) === 'money'
}

/** The columns a formula, a group or a pivot cell can name. */
export function columnChoices(draft: ColumnsOf, catalog: Catalog) {
    return draft.columns.map(column => {
        const type = columnType(column, draft, catalog)
        return {
            key: column.id,
            label: column.label,
            type,
            isNumeric: type === 'number' || type === 'money' || type === 'percent',
            hasAggregate: column.aggregate !== '',
            isDate: type === 'date' || type === 'datetime',
            isNested: column.kind === 'path' && describePath(catalog, draft.source, column.path)?.mode === 'nest',
            options: column.kind === 'path' ? (describePath(catalog, draft.source, column.path)?.field.options ?? []) : [],
        }
    })
}

function formulaNode(formula: FormulaDraft, leaf: 'col' | 'subtotal', isAmount: (key: string) => boolean): SpecNode {
    return formula.kind === 'fixed' ? formula.node : fromComputeExpr(formula.expr, leaf, isAmount)
}

function sectionKey(text: string, type: FieldType): unknown {
    if ((type === 'number' || type === 'money') && text.trim() !== '' && Number.isFinite(Number(text))) return Number(text)
    if (type === 'bool') return text === 'true'
    return text
}

export function specFromDraft(draft: BuilderDraft, catalog: Catalog): ReportSpec {
    const amounts = new Set(draft.columns.filter(c => columnIsAmount(c, draft, catalog)).map(c => c.id))
    const isAmount = (key: string) => amounts.has(key)
    const columns: SpecColumn[] = draft.columns.map(column => {
        const out: SpecColumn = { id: column.id, label: column.label }
        if (column.kind === 'path') out.path = column.path
        else out.formula = formulaNode(column.formula, 'col', isAmount)
        if (column.aggregate !== '') out.aggregate = column.aggregate
        if (column.aggregate === 'avg') {
            const info = column.kind === 'path' ? describePath(catalog, draft.source, column.path) : null
            out.scale = column.scale ?? avgScaleFor(info?.field.type ?? 'number')
        }
        if (column.format !== '') out.format = column.format
        return out
    })
    const filter = filterToNode(draft.filter, catalog, draft.source)
    const asked = paramsInGroup(draft.filter, new Set<string>())
    for (const column of draft.columns) if (column.kind === 'formula' && column.formula.kind === 'fixed') paramsInNode(column.formula.node, asked)
    const params: ParamDef[] = draft.params.filter(param => asked.has(param.id))
    const firstGroup = draft.columns.find(column => column.id === draft.groups[0]?.col)
    const keyType = firstGroup ? columnType(firstGroup, draft, catalog) : 'text'
    const spec: ReportSpec = { version: CURRENT_SPEC_VERSION, source: draft.source, columns }
    if (params.length > 0) spec.params = params
    if (filter) spec.filter = filter
    if (draft.sort.length > 0) spec.sort = draft.sort.map(s => (s.dir === 'desc' ? { col: s.col, dir: 'desc' } : { col: s.col }))
    if (draft.groups.length > 0) {
        spec.groups = draft.groups.map(g => ({
            id: g.id,
            col: g.col,
            ...(g.bucket !== '' ? { bucket: g.bucket } : {}),
            ...(g.subtotals ? { subtotals: true } : {}),
        }))
    }
    if (draft.pivot.on) {
        spec.pivot = {
            path: draft.pivot.path,
            ...(draft.pivot.bucket !== '' ? { bucket: draft.pivot.bucket } : {}),
            cell: draft.pivot.cell,
            maxColumns: draft.pivot.maxColumns,
        }
    }
    if (draft.layout.length > 0) {
        spec.layout = draft.layout.map(item =>
            item.kind === 'row'
                ? { row: item.id, label: item.label, formula: formulaNode(item.formula, 'subtotal', () => true) }
                : { section: item.id, title: item.title, key: sectionKey(item.key, keyType) }
        )
    }
    return spec
}
```

- [ ] **Step 8: Write the errors module**

`core/reports/lib/builder/errors.ts`:

```ts
import { extractValidationErrors } from '@tinycld/core/lib/errors'
import type { Problem } from '../types'

// The server answers a refused save or draft run with problems keyed by spec path
// (`columns.0.id`). The builder shows each under the section it is about, naming
// the part ("Column 3: …") (R17).

export type BuilderSection = 'details' | 'columns' | 'filter' | 'group' | 'pivot' | 'layout'

export function sectionOf(specPath: string): BuilderSection {
    const head = specPath.split('.')[0]
    if (head === 'columns') return 'columns'
    if (head === 'filter' || head === 'params') return 'filter'
    if (head === 'groups' || head === 'sort') return 'group'
    if (head === 'pivot') return 'pivot'
    if (head === 'layout') return 'layout'
    return 'details'
}

const PART_NAMES: Record<string, string> = {
    columns: 'Column',
    groups: 'Group',
    sort: 'Sort',
    layout: 'Layout item',
    params: 'Asked value',
}

export function describeProblem(problem: Problem): string {
    const [head, index] = problem.path.split('.')
    const name = head ? PART_NAMES[head] : undefined
    if (name && index !== undefined && /^\d+$/.test(index)) return `${name} ${Number(index) + 1}: ${problem.message}`
    if (head === 'filter') return `Filter: ${problem.message}`
    if (head === 'pivot') return `Pivot: ${problem.message}`
    return problem.message
}

export function problemsFromError(error: unknown): Problem[] {
    const fields = extractValidationErrors(error) ?? {}
    return Object.entries(fields)
        .map(([path, message]) => ({ path, code: '', message }))
        .sort((a, b) => a.path.localeCompare(b.path))
}

export function problemsFor(section: BuilderSection, problems: readonly Problem[]): Problem[] {
    return problems.filter(problem => sectionOf(problem.path) === section)
}
```

- [ ] **Step 9: Run the tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run reports/lib/builder`
Expected: all pass.

- [ ] **Step 10: Format, lint, typecheck, commit**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome format --write core/reports/lib/builder && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome check core/reports && cd core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg typecheck`
Expected: clean.

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/reports/lib/builder
git commit -m "feat(core): report builder draft, formulas, filters and errors"
```

---

### Task 12: The report table, moved

**Files:**
- Create: `core/reports/components/report-table-layout.ts`, `core/reports/components/use-row-window.ts`, `core/reports/components/ReportRow.tsx`, `core/reports/components/ReportTable.tsx`
- Test: `core/reports/components/__tests__/report-table-layout.test.ts`, `core/reports/components/__tests__/use-row-window.test.ts` (pure functions only)

The Books shell's table (`books/tinycld/books/components/reports/ReportTable.tsx`, `ReportRow.tsx`, `report-table-layout.ts`, `hooks/useRowWindow.ts`, `lib/near-end.ts`), moved to core and fed by the run result: group rows replace section rows and fold through the server (R4, R5); computed rows draw like subtotals; nested child rows indent by their depth; money cells take each row's currency; a paged report loads more near the end. Drawn on screen in Task 13 and covered by its e2e.

**Interfaces:**
- Consumes: `VisibleRow` (Task 7), `ResultColumn`, `ResultRow`, `RowKind` (Task 5), `cellDisplay`, `isRightAligned`, `NumberFormat`, `CellDisplay` (Task 6), `Breakpoint`, `useWindowHeight` (`@tinycld/core/components/workspace/useBreakpoint`), `useThemeColor`.
- Produces: `ROW_HEIGHT = 44`, `AMOUNT_WIDTH = 128`, `LABEL_MIN_WIDTH = 220`, `PHONE_LABEL_WIDTH = 168`, `PHONE_AMOUNT_WIDTH = 108`, `INDENT = 16`, `ReportTableLayout`, `reportTableLayout({breakpoint, amountCount})`, `columnAlign(column, index)`; `ROW_OVERSCAN`, `NEAR_END_PX`, `rowWindowBounds(...)`, `isNearEnd(metrics)`, `useRowWindow({total, rowHeight, viewHeight, onNearEnd?})`; `ReportTable` props `{ columns: ResultColumn[]; rows: VisibleRow[]; format: NumberFormat; breakpoint: Breakpoint; onToggleGroup(row: ResultRow): void; onNearEnd(): void; isLoadingMore: boolean }`.

- [ ] **Step 1: Write the failing tests**

`core/reports/components/__tests__/report-table-layout.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import type { ResultColumn } from '../../lib/types'
import { AMOUNT_WIDTH, columnAlign, LABEL_MIN_WIDTH, PHONE_AMOUNT_WIDTH, PHONE_LABEL_WIDTH, reportTableLayout } from '../report-table-layout'

describe('reportTableLayout', () => {
    it('gives the label the free space on wide screens and scrolls the amounts sideways', () => {
        expect(reportTableLayout({ breakpoint: 'desktop', amountCount: 3 })).toEqual({
            labelStyle: { flex: 1, minWidth: LABEL_MIN_WIDTH },
            amountWidth: AMOUNT_WIDTH,
            scrollsSideways: true,
            amountContentMinWidth: 3 * AMOUNT_WIDTH,
        })
    })

    it('fits one or two narrower amount columns on a phone, and scrolls three or more', () => {
        expect(reportTableLayout({ breakpoint: 'mobile', amountCount: 2 })).toMatchObject({ amountWidth: PHONE_AMOUNT_WIDTH, scrollsSideways: false, labelStyle: { flex: 1 } })
        expect(reportTableLayout({ breakpoint: 'mobile', amountCount: 3 })).toMatchObject({ amountWidth: AMOUNT_WIDTH, scrollsSideways: true, labelStyle: { width: PHONE_LABEL_WIDTH } })
    })
})

describe('columnAlign', () => {
    const money: ResultColumn = { id: 'cost', label: 'Cost', type: 'money' }
    const text: ResultColumn = { id: 'note', label: 'Note', type: 'text' }

    it('keeps the label column left and numbers right', () => {
        expect(columnAlign(money, 0)).toBe('left')
        expect(columnAlign(money, 1)).toBe('right')
        expect(columnAlign(text, 1)).toBe('left')
        expect(columnAlign({ ...text, totalType: 'number' }, 2)).toBe('right')
    })
})
```

`core/reports/components/__tests__/use-row-window.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { isNearEnd, NEAR_END_PX, ROW_OVERSCAN, rowWindowBounds } from '../use-row-window'

describe('rowWindowBounds', () => {
    it('mounts the rows in view plus an overscan on each side', () => {
        const bounds = rowWindowBounds({ firstVisible: 20, total: 100, rowHeight: 44, viewHeight: 440 })
        expect(bounds).toEqual({
            first: 20 - ROW_OVERSCAN,
            last: 20 - ROW_OVERSCAN + 10 + ROW_OVERSCAN * 2,
            topSpace: (20 - ROW_OVERSCAN) * 44,
            bottomSpace: (100 - (20 - ROW_OVERSCAN + 10 + ROW_OVERSCAN * 2)) * 44,
        })
    })

    it('never goes empty when the list shrinks under a stale scroll position', () => {
        const bounds = rowWindowBounds({ firstVisible: 500, total: 8, rowHeight: 44, viewHeight: 440 })
        expect(bounds.first).toBe(0)
        expect(bounds.last).toBe(8)
    })
})

describe('isNearEnd', () => {
    it('is true within the near-end distance of the content’s end', () => {
        const metrics = (y: number) => ({ contentOffset: { y }, layoutMeasurement: { height: 400 }, contentSize: { height: 2000 } })
        expect(isNearEnd(metrics(2000 - 400 - NEAR_END_PX))).toBe(true)
        expect(isNearEnd(metrics(2000 - 400 - NEAR_END_PX - 1))).toBe(false)
    })
})
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run reports/components/__tests__`
Expected: FAIL: modules do not resolve.

- [ ] **Step 3: Write the layout and the row window**

`core/reports/components/report-table-layout.ts`:

```ts
import type { Breakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import { isRightAligned } from '../lib/format'
import type { ResultColumn } from '../lib/types'

export const ROW_HEIGHT = 44
export const AMOUNT_WIDTH = 128
export const LABEL_MIN_WIDTH = 220
export const PHONE_LABEL_WIDTH = 168
export const PHONE_AMOUNT_WIDTH = 108
export const INDENT = 16

// A phone fits this many amount columns whole beside the label; more scroll sideways.
const PHONE_FIT_COLUMNS = 2

export type ReportTableLayout = {
    labelStyle: { flex?: number; minWidth?: number; width?: number }
    amountWidth: number
    scrollsSideways: boolean
    amountContentMinWidth: number
}

function layoutFor(
    labelStyle: ReportTableLayout['labelStyle'],
    amountWidth: number,
    scrollsSideways: boolean,
    amountCount: number
): ReportTableLayout {
    return { labelStyle, amountWidth, scrollsSideways, amountContentMinWidth: amountCount * amountWidth }
}

/** One rule for a heading and its cells. The first column holds the row labels, so it is left. */
export function columnAlign(column: ResultColumn, index: number): 'left' | 'right' {
    if (index === 0) return 'left'
    return isRightAligned(column) ? 'right' : 'left'
}

export function reportTableLayout(input: { breakpoint: Breakpoint; amountCount: number }): ReportTableLayout {
    const { breakpoint, amountCount } = input
    if (breakpoint !== 'mobile') {
        return layoutFor({ flex: 1, minWidth: LABEL_MIN_WIDTH }, AMOUNT_WIDTH, true, amountCount)
    }
    if (amountCount <= PHONE_FIT_COLUMNS) return layoutFor({ flex: 1 }, PHONE_AMOUNT_WIDTH, false, amountCount)
    return layoutFor({ width: PHONE_LABEL_WIDTH }, AMOUNT_WIDTH, true, amountCount)
}
```

`core/reports/components/use-row-window.ts`:

```ts
import { useState } from 'react'

// Mounts only the rows near the scroll position; two spacers stand in for the rest,
// so the scroll bar still spans the whole report. Near the end it asks for the next
// page. Moved from the Books shell (hooks/useRowWindow.ts, lib/near-end.ts).

export const ROW_OVERSCAN = 6
/** About ten rows before the end, so the next page is there before the reader is. */
export const NEAR_END_PX = 600

export type ScrollMetrics = {
    contentOffset: { y: number }
    layoutMeasurement: { height: number }
    contentSize: { height: number }
}

export function isNearEnd({ contentOffset, layoutMeasurement, contentSize }: ScrollMetrics): boolean {
    return contentOffset.y + layoutMeasurement.height >= contentSize.height - NEAR_END_PX
}

export function rowWindowBounds(input: { firstVisible: number; total: number; rowHeight: number; viewHeight: number }) {
    const { firstVisible, total, rowHeight, viewHeight } = input
    const visibleCount = Math.ceil(viewHeight / rowHeight)
    // The upper clamp keeps the window from going empty when the list shrinks under a stale scroll position.
    const first = Math.min(Math.max(0, firstVisible - ROW_OVERSCAN), Math.max(0, total - visibleCount))
    const last = Math.min(total, first + visibleCount + ROW_OVERSCAN * 2)
    return { first, last, topSpace: first * rowHeight, bottomSpace: (total - last) * rowHeight }
}

export function useRowWindow(input: { total: number; rowHeight: number; viewHeight: number; onNearEnd?: () => void }) {
    // The row index, not the pixel offset, so scrolling within one row causes no render.
    const [firstVisible, setFirstVisible] = useState(0)
    return {
        ...rowWindowBounds({ firstVisible, total: input.total, rowHeight: input.rowHeight, viewHeight: input.viewHeight }),
        onScroll: (event: { nativeEvent: ScrollMetrics }) => {
            setFirstVisible(Math.floor(event.nativeEvent.contentOffset.y / input.rowHeight))
            if (input.onNearEnd && isNearEnd(event.nativeEvent)) input.onNearEnd()
        },
    }
}
```

- [ ] **Step 4: Write the row and the table**

`core/reports/components/ReportRow.tsx`:

```tsx
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ChevronDown, ChevronRight } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { type CellDisplay, cellDisplay, type NumberFormat } from '../lib/format'
import type { VisibleRow } from '../lib/rows'
import type { ResultColumn, ResultRow, RowKind } from '../lib/types'
import { columnAlign, INDENT, type ReportTableLayout, ROW_HEIGHT } from './report-table-layout'

// The rule above a row; the label and amount stacks draw it alike, so it runs the full width.
const ROW_RULE: Record<RowKind, string> = {
    group: '',
    detail: '',
    subtotal: 'border-t border-border',
    computed: 'border-t border-border',
    total: 'border-t-2 border-foreground',
}

const ROW_WEIGHT: Record<RowKind, string> = {
    group: 'font-semibold',
    detail: '',
    subtotal: 'font-semibold',
    computed: 'font-semibold',
    total: 'font-semibold',
}

const CARET_SIZE = 14

function labelRowStyle(row: ResultRow) {
    return { height: ROW_HEIGHT, paddingLeft: INDENT + row.depth * INDENT }
}

function GroupCaret({ isOpen }: { isOpen: boolean }) {
    const color = useThemeColor('muted-foreground')
    const Icon = isOpen ? ChevronDown : ChevronRight
    return <Icon size={CARET_SIZE} color={color} />
}

function LabelText({ row }: { row: ResultRow }) {
    return (
        <Text numberOfLines={1} className={`min-w-0 flex-1 text-sm text-foreground ${ROW_WEIGHT[row.kind]}`}>
            {row.label}
        </Text>
    )
}

interface ReportRowLabelProps {
    item: VisibleRow
    onToggleGroup(row: ResultRow): void
}

// A role-button Pressable takes a click, Enter and Space on web and a tap on native.
function GroupToggleLabel({ item, onToggleGroup }: ReportRowLabelProps) {
    const { row } = item
    return (
        <Pressable
            testID={`report-row-${row.id}`}
            accessibilityRole="button"
            accessibilityLabel={row.label}
            aria-expanded={item.isOpen}
            onPress={() => onToggleGroup(row)}
            className="flex-row items-center gap-1 pr-3"
            style={labelRowStyle(row)}
        >
            <GroupCaret isOpen={item.isOpen} />
            <LabelText row={row} />
        </Pressable>
    )
}

export function ReportRowLabel({ item, onToggleGroup }: ReportRowLabelProps) {
    if (item.isToggle) return <GroupToggleLabel item={item} onToggleGroup={onToggleGroup} />
    const { row } = item
    return (
        <View testID={`report-row-${row.id}`} className={`flex-row items-center pr-3 ${ROW_RULE[row.kind]}`} style={labelRowStyle(row)}>
            <LabelText row={row} />
        </View>
    )
}

function cellTextClass(display: CellDisplay, align: 'left' | 'right', kind: RowKind): string {
    const alignClass = align === 'right' ? 'text-right tabular-nums' : 'text-left'
    const tone = display.isRed || display.isError ? 'text-danger' : 'text-foreground'
    return `text-sm ${alignClass} ${tone} ${ROW_WEIGHT[kind]}`
}

interface AmountCellProps {
    item: VisibleRow
    column: ResultColumn
    index: number
    width: number
    format: NumberFormat
}

function AmountCell({ item, column, index, width, format }: AmountCellProps) {
    const { row } = item
    const display = cellDisplay(column, row, item.cells[column.id], format)
    const align = columnAlign(column, index)
    return (
        <View testID={`report-cell-${column.id}-${row.id}`} className={`justify-center px-3 ${ROW_RULE[row.kind]}`} style={{ width, height: ROW_HEIGHT }}>
            <Text numberOfLines={1} className={cellTextClass(display, align, row.kind)}>
                {display.text}
            </Text>
        </View>
    )
}

interface ReportRowAmountsProps {
    item: VisibleRow
    columns: ResultColumn[]
    layout: ReportTableLayout
    format: NumberFormat
    onToggleGroup(row: ResultRow): void
}

// The label side is the row's one accessible toggle. This side takes the same press but
// is not a button and has no tab stop, so a closed group's figures stay readable.
export function ReportRowAmounts({ item, columns, layout, format, onToggleGroup }: ReportRowAmountsProps) {
    const cells = columns.map((column, i) => (
        <AmountCell key={column.id} item={item} column={column} index={i + 1} width={layout.amountWidth} format={format} />
    ))
    if (!item.isToggle) return <View className="flex-row">{cells}</View>
    return (
        <Pressable onPress={() => onToggleGroup(item.row)} accessible={false} tabIndex={-1} className="flex-row">
            {cells}
        </Pressable>
    )
}
```

`core/reports/components/ReportTable.tsx`:

```tsx
import { type Breakpoint, useWindowHeight } from '@tinycld/core/components/workspace/useBreakpoint'
import { type RefObject, useRef } from 'react'
import {
    ActivityIndicator,
    type NativeScrollEvent,
    type NativeSyntheticEvent,
    ScrollView,
    type ScrollViewInstance,
    Text,
    View,
} from 'react-native'
import type { NumberFormat } from '../lib/format'
import type { VisibleRow } from '../lib/rows'
import type { ResultColumn, ResultRow } from '../lib/types'
import { ReportRowAmounts, ReportRowLabel } from './ReportRow'
import { columnAlign, type ReportTableLayout, ROW_HEIGHT, reportTableLayout } from './report-table-layout'
import { useRowWindow } from './use-row-window'

// Sized by its content, then shrunk (and scrolled) when the label column needs the room.
const AMOUNT_SCROLLER_STYLE = { flexGrow: 0, flexShrink: 1 }

function amountContentStyle(layout: ReportTableLayout) {
    return { width: '100%', minWidth: layout.amountContentMinWidth } as const
}

function HeadingCell({ column, index, width }: { column: ResultColumn; index: number; width?: number }) {
    const align = columnAlign(column, index) === 'right' ? 'text-right' : 'text-left'
    return (
        <View testID={`report-heading-${column.id}`} className="justify-center px-3" style={{ width, height: ROW_HEIGHT }}>
            <Text numberOfLines={1} className={`text-xs font-semibold uppercase text-muted-foreground ${align}`}>
                {column.label}
            </Text>
        </View>
    )
}

interface HeadingRowProps {
    labelColumn: ResultColumn
    amountColumns: ResultColumn[]
    layout: ReportTableLayout
    scrollRef: RefObject<ScrollViewInstance | null>
}

// Outside the rows' vertical scroller, so it stays visible; its amount labels sit in a
// scroller the body drives, so they move sideways in step with the amount columns.
function HeadingRow({ labelColumn, amountColumns, layout, scrollRef }: HeadingRowProps) {
    const amounts = amountColumns.map((column, i) => (
        <HeadingCell key={column.id} column={column} index={i + 1} width={layout.amountWidth} />
    ))
    return (
        <View testID="report-heading-row" className="flex-row border-b border-border bg-background web:overflow-y-hidden web:[scrollbar-gutter:stable]">
            <View className="pl-1" style={layout.labelStyle}>
                <HeadingCell column={labelColumn} index={0} />
            </View>
            <ScrollView
                ref={scrollRef}
                horizontal
                scrollEnabled={false}
                showsHorizontalScrollIndicator={false}
                style={AMOUNT_SCROLLER_STYLE}
                contentContainerStyle={amountContentStyle(layout)}
            >
                {amounts}
            </ScrollView>
        </View>
    )
}

function LoadingMore({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View testID="report-loading-more" className="items-center py-3">
            <ActivityIndicator />
        </View>
    )
}

export interface ReportTableProps {
    columns: ResultColumn[]
    rows: VisibleRow[]
    format: NumberFormat
    breakpoint: Breakpoint
    onToggleGroup(row: ResultRow): void
    onNearEnd(): void
    isLoadingMore: boolean
}

// Two stacks in one vertical scroller: the label column stays put while the amount
// columns scroll sideways beside it, and both move together vertically.
export function ReportTable({ columns, rows, format, breakpoint, onToggleGroup, onNearEnd, isLoadingMore }: ReportTableProps) {
    const [labelColumn, ...amountColumns] = columns
    const layout = reportTableLayout({ breakpoint, amountCount: amountColumns.length })
    const headingRef = useRef<ScrollViewInstance>(null)
    const rowWindow = useRowWindow({ total: rows.length, rowHeight: ROW_HEIGHT, viewHeight: useWindowHeight(), onNearEnd })
    if (!labelColumn) return null
    const shown = rows.slice(rowWindow.first, rowWindow.last)
    const labels = shown.map(item => <ReportRowLabel key={item.row.id} item={item} onToggleGroup={onToggleGroup} />)
    const amounts = shown.map(item => (
        <ReportRowAmounts key={item.row.id} item={item} columns={amountColumns} layout={layout} format={format} onToggleGroup={onToggleGroup} />
    ))
    const topSpacer = { height: rowWindow.topSpace }
    const bottomSpacer = { height: rowWindow.bottomSpace }
    const onAmountsScroll = (event: NativeSyntheticEvent<NativeScrollEvent>) =>
        headingRef.current?.scrollTo({ x: event.nativeEvent.contentOffset.x, animated: false })
    return (
        <View testID="report-table" className="min-h-0 flex-1">
            <HeadingRow labelColumn={labelColumn} amountColumns={amountColumns} layout={layout} scrollRef={headingRef} />
            <ScrollView className="flex-1 web:[scrollbar-gutter:stable]" onScroll={rowWindow.onScroll} scrollEventThrottle={16}>
                <View className="flex-row">
                    <View style={layout.labelStyle}>
                        <View style={topSpacer} />
                        {labels}
                        <View style={bottomSpacer} />
                    </View>
                    <ScrollView
                        horizontal
                        scrollEnabled={layout.scrollsSideways}
                        style={AMOUNT_SCROLLER_STYLE}
                        contentContainerStyle={amountContentStyle(layout)}
                        onScroll={onAmountsScroll}
                        scrollEventThrottle={16}
                    >
                        <View className="flex-1">
                            <View style={topSpacer} />
                            {amounts}
                            <View style={bottomSpacer} />
                        </View>
                    </ScrollView>
                </View>
                <LoadingMore isVisible={isLoadingMore} />
            </ScrollView>
        </View>
    )
}
```

- [ ] **Step 5: Run the tests, format, lint, typecheck**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run reports/components/__tests__ && cd .. && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome format --write core/reports/components && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome check core/reports && cd core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg typecheck`
Expected: tests pass; clean.

- [ ] **Step 6: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/reports/components
git commit -m "feat(core): the report table, moved from the Books shell"
```

---

### Task 13: Create a report and open it (area, hub, builder columns, report view)

The first vertical slice: the Reports area in the app's navigation, the hub with its three tabs, New report, the builder panel with Details and Columns (paths through to-one and to-many links), Save, the report view with its states and the table, and Edit report.

**Files:**
- Create: `tests/e2e/reports-helpers.ts`, `tests/e2e/reports-create.spec.ts`
- Create: `app/a/(app)/reports/_layout.tsx`, `app/a/(app)/reports/index.tsx`, `app/a/(app)/reports/new.tsx`, `app/a/(app)/reports/[id].tsx`
- Modify: `app/a/(app)/_layout.tsx` (`ActivePkgSync`: reports is chrome), `core/components/workspace/PackageTabs.tsx`, `core/components/workspace/PackageRail.tsx`, `core/components/workspace/MoreDrawer.tsx`
- Create: `core/reports/hooks/useBackToReports.ts`, `core/reports/hooks/useReportsHub.ts`, `core/reports/hooks/useReportView.ts`, `core/reports/hooks/useReportBuilder.ts`
- Create: `core/reports/components/ReportHeader.tsx`, `core/reports/components/ReportDocumentHeader.tsx`, `core/reports/components/ReportStates.tsx`, `core/reports/components/ReportView.tsx`
- Create: `core/reports/components/hub/ReportsHub.tsx`, `core/reports/components/hub/HubList.tsx`, `core/reports/components/hub/NewReportDialog.tsx`
- Create: `core/reports/components/builder/ReportBuilderPanel.tsx`, `BuilderSection.tsx`, `DetailsSection.tsx`, `ColumnsSection.tsx`, `ColumnPicker.tsx` (same dir)
- Create: `core/reports/screens/ReportsHubScreen.tsx`, `core/reports/screens/ReportScreen.tsx`, `core/reports/screens/NewReportScreen.tsx`

**Interfaces:**
- Consumes: Tasks 5–12 (all `lib/`, stores, data hooks, `ReportTable`); core `Dialog`, `Drawer*`, `Sheet`, `Popover`, `ConfirmDialog`, `PromptDialog`, `ResponsiveToolbar`, `Button`, `ServerActionButton`, `TextInput`, `TextAreaInput`, `SelectInput`, `useLeaveGuard` (`@tinycld/core/components/forms-builder/use-leave-guard`), `useDebouncedValue`, `useOrgInfo`, `useOrgHref`, `useRecord`, `formatTimeAgo`.
- Produces:
  - `useBackToReports(): () => void`.
  - `useReportsHub()` → `{ tab: HubTab; setTab(tab); rows: HubRow[]; state: 'loading'|'error'|'ready'; retry(); sources: CatalogSource[]; sourceLabel(key): string; packageSlug: string | null; packageLabel: string; onShowAll(); onOpen(report); onNew(sourceKey) }`; `HUB_TABS`; `HubTab`; `HubRow { report: Reports; access: ReportAccess }`.
  - `useReportView(input: ReportViewInput)` → `ReportViewModel` (fields used later: `options`, `controls`, `resolved`, `surface`, `onOpenSurface`, `onCloseSurface`, `onCloseSurfaceOf`, `onApplyParam`, `onRemoveParam`, `isCustomizeOpen`, `onOpenCustomize`, `onCloseCustomize`, `onDisplayChange`, `onCopyLink`, `notices`, `isUpdating`); `ViewSource`; `ViewState`.
  - `useReportBuilder(input: BuilderInput)` → `ReportBuilder` (`mode`, `form`, `catalog`, `base`, `previewSpec`, `title`, `isDirty`, `isSaving`, `isSaved`, `saveProblems`, `onSave()`, `onSaveAs(title)`, `onClose()`, `isConfirmingDiscard`, `onKeepEditing()`, `onDiscard()`, `setColumns(items)`, `setGroups(items)`, `setSort(items)`, `setLayout(items)`, `setFilter(group)`, `setParams(defs)`, `setPivotPath(path)`).
  - `ReportView({ model, side? })`, `ReportBody({ model })`, `ReportBuilderPanel({ builder, preview })`, `BuilderSection`, `ColumnPicker({ catalog, baseKey, onPick, label, testID, toOneOnly? })`.
  - Test ids listed in "For review" (R21).

- [ ] **Step 1: Write the e2e helpers**

`tests/e2e/reports-helpers.ts`:

```ts
import { expect, type Locator, type Page } from '@playwright/test'
import { navigateToPackage } from './helpers'

// Helpers for the reports specs. Fixture names come from the salesstub package
// (tests/scripts/scaffold-reports-stub.ts): sources salesstub.sales ("Stub sales":
// customer, amount, cost, currency, day, status; link team) and salesstub.teams
// ("Stub teams": name; to-many link sales).
export const SALES_SOURCE = 'salesstub.sales'
export const TEAMS_SOURCE = 'salesstub.teams'

let counter = 0

/** A name no other spec, worker or run uses. */
export function uniqueName(prefix: string): string {
    counter += 1
    return `${prefix} ${process.env.TEST_WORKER_INDEX ?? '0'}${Date.now().toString(36)}${counter}`
}

export async function openReportStub(page: Page) {
    await navigateToPackage(page, 'salesstub', { waitFor: page.getByTestId('salesstub-landing') })
}

export async function addTeam(page: Page, name: string) {
    await page.getByTestId('salesstub-team-name').fill(name)
    await page.getByTestId('salesstub-team-create').click()
    await expect(page.getByTestId('salesstub-team-saved')).toHaveText(`Saved ${name}`)
}

export interface StubSale {
    team: string
    customer: string
    /** Cents. */
    amount: number
    cost: number
    day: string
    status?: 'open' | 'paid'
}

export async function addSale(page: Page, sale: StubSale) {
    await page.getByTestId('salesstub-sale-team').click()
    await page.getByRole('menuitem', { name: sale.team, exact: true }).click()
    await page.getByTestId('salesstub-sale-customer').fill(sale.customer)
    await page.getByTestId('salesstub-sale-amount').fill(String(sale.amount))
    await page.getByTestId('salesstub-sale-cost').fill(String(sale.cost))
    await page.getByTestId('salesstub-sale-day').fill(sale.day)
    await page.getByRole('radio', { name: sale.status === 'paid' ? 'Paid' : 'Open', exact: true }).click()
    await page.getByTestId('salesstub-sale-create').click()
    await expect(page.getByTestId('salesstub-sale-saved')).toHaveText(`Saved ${sale.customer}`)
}

export async function openReportsHub(page: Page) {
    await navigateToPackage(page, 'reports', { waitFor: page.getByTestId('reports-hub') })
}

export function hubRow(page: Page, title: string): Locator {
    return page.locator('[data-testid^="reports-hub-row-"]').filter({ hasText: title })
}

/** New report → pick a source → the builder opens on a draft. */
export async function startReport(page: Page, sourceKey: string) {
    await page.getByTestId('reports-hub-new').click()
    await page.getByTestId(`reports-new-source-${sourceKey}`).click()
    await expect(page.getByTestId('report-builder')).toBeVisible()
}

/** Adds a column through the picker: link keys, then a field key; `mode` for a to-many hop. */
export async function addColumn(page: Page, keys: string[], mode?: 'total' | 'nest') {
    await page.getByTestId('report-builder-add-column').click()
    const picker = page.getByTestId('report-builder-picker')
    for (const key of keys.slice(0, -1)) {
        await picker.getByTestId(`report-builder-pick-link-${key}`).click()
        if (mode && (await picker.getByTestId(`report-builder-pick-${mode}`).isVisible())) {
            await picker.getByTestId(`report-builder-pick-${mode}`).click()
        }
    }
    await picker.getByTestId(`report-builder-pick-field-${keys[keys.length - 1]}`).click()
    await expect(picker).toBeHidden()
}

export async function chooseOption(page: Page, selectTestId: string, value: string) {
    await page.getByTestId(selectTestId).click()
    await page.getByTestId(`${selectTestId}-${value}`).click()
}

/** Saves a new report and waits for its own view (the builder closes, the URL changes). */
export async function saveNewReport(page: Page, title: string) {
    await page.getByTestId('report-builder-title').fill(title)
    await page.getByTestId('report-builder-save').click()
    await expect(page.getByTestId('report-builder')).toBeHidden()
    await expect(page).toHaveURL(/\/a\/reports\/[a-z0-9]{15}(\?|$)/)
    await expect(page.getByTestId('report-title')).toHaveText(title)
}

/** A toolbar action, folded into "More actions" when the row is narrow. */
export async function pressToolbar(page: Page, testId: string, label: string) {
    const button = page.getByTestId(testId)
    if (await button.isVisible()) await button.click()
    else {
        await page.getByTestId('toolbar-more-button').click()
        await page.getByRole('menuitem', { name: label }).click()
    }
}

/** The label cell of the row whose label is `label` (a group row's is its toggle). */
export function rowOf(page: Page, label: string): Locator {
    return page
        .getByTestId('report-table')
        .locator('[data-testid^="report-row-"]')
        .filter({ has: page.getByText(label, { exact: true }) })
        .first()
}

/** The cell of `columnId` (not the first column) in the row whose label is `label`. */
export async function cellOf(page: Page, columnId: string, label: string): Promise<Locator> {
    const labelCell = rowOf(page, label)
    await expect(labelCell).toBeVisible()
    const testId = await labelCell.getAttribute('data-testid')
    if (!testId) throw new Error(`no row labelled ${label}`)
    return page.getByTestId(`report-cell-${columnId}-${testId.replace('report-row-', '')}`)
}
```

- [ ] **Step 2: Write the failing spec**

`tests/e2e/reports-create.spec.ts`:

```ts
import { expect, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub } from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    cellOf,
    chooseOption,
    hubRow,
    openReportStub,
    openReportsHub,
    pressToolbar,
    SALES_SOURCE,
    saveNewReport,
    startReport,
    uniqueName,
} from './reports-helpers'

/**
 * Creating a report with the builder and opening it, against the salesstub
 * package (tests/scripts/scaffold-reports-stub.ts). Each test signs in a fresh
 * member made through the invite flow: salesstub_sales lets each user read only
 * their own rows, so the report shows exactly the sales this test added.
 */
test.describe('creating a report', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a new report shows its columns through a to-one link, then opens from the hub', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptcreate')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, { team, customer: 'Acme', amount: 150000, cost: 90000, day: '2026-01-05' })
            await addSale(p, { team, customer: 'Bolt', amount: 50000, cost: 10000, day: '2026-02-10', status: 'paid' })

            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            // A new report starts with the source's first field.
            await expect(p.getByTestId('report-builder-column-0-label')).toHaveValue('Customer')
            await addColumn(p, ['team', 'name'])
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-2-aggregate', 'sum')
            // The preview behind the panel runs the draft.
            await expect(p.getByTestId('report-heading-teamName')).toHaveText('Team › Name')
            await expect(await cellOf(p, 'amount', 'Acme')).toHaveText('1,500.00')

            const title = uniqueName('Sales by customer')
            await saveNewReport(p, title)
            await expect(await cellOf(p, 'teamName', 'Bolt')).toHaveText(team)
            await expect(p.getByTestId('report-cell-amount-total')).toHaveText('$2,000.00')
            await expect(p.getByTestId('report-document-title')).toHaveText(title)

            await p.getByTestId('report-back').click()
            await expect(p.getByTestId('reports-hub')).toBeVisible()
            const row = hubRow(p, title)
            await expect(row).toContainText('Stub sales')
            await expect(row).toContainText('just now')
            await row.click()
            await expect(p.getByTestId('report-title')).toHaveText(title)
        } finally {
            await close()
        }
    })

    test('Edit report changes a saved report, and closing with changes asks first', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptedit')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, { team, customer: 'Cobalt', amount: 1000, cost: 500, day: '2026-03-01' })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            const title = uniqueName('Edit me')
            await saveNewReport(p, title)

            await pressToolbar(p, 'report-edit-open', 'Edit report')
            await expect(p.getByTestId('report-builder-title')).toHaveValue(title)
            await p.getByTestId('report-builder-column-0-label').fill('Client')
            await p.getByTestId('report-builder-close').click()
            await expect(p.getByText('Discard changes?', { exact: true })).toBeVisible()
            await p.getByText('Keep editing', { exact: true }).click()
            await expect(p.getByTestId('report-builder')).toBeVisible()
            await p.getByTestId('report-builder-save').click()
            await expect(p.getByTestId('report-builder')).toBeHidden()
            await expect(p.getByTestId('report-heading-customer')).toHaveText('Client')
        } finally {
            await close()
        }
    })

    test('on a phone the hub shows cards and the builder is a full sheet with a preview', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptphone')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, { team, customer: 'Delta', amount: 2500, cost: 1000, day: '2026-04-01' })
            await openReportsHub(p)
            await p.setViewportSize({ width: 390, height: 844 })
            await startReport(p, SALES_SOURCE)
            await expect(p.getByTestId('report-builder-mode-design')).toBeVisible()
            await p.getByTestId('report-builder-mode-preview').click()
            await expect(p.getByTestId('report-table').getByText('Delta', { exact: true })).toBeVisible()
            await p.getByTestId('report-builder-mode-design').click()
            const title = uniqueName('Phone report')
            await saveNewReport(p, title)
            await p.getByTestId('report-back').click()
            await expect(hubRow(p, title)).toContainText('Stub sales')
        } finally {
            await close()
        }
    })
})
```

- [ ] **Step 3: Run the spec to see it fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-create.spec.ts`
Expected: FAIL: `navigateToPackage(page, 'reports')` finds no rail link `a[href^="/a/reports"]` (there is no Reports area yet). (The webServer builds the bundle first; that takes minutes.)

- [ ] **Step 4: Wire the app-owned area**

`app/a/(app)/reports/_layout.tsx`:

```tsx
import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { Slot } from 'expo-router'

export default function ReportsLayout() {
    return (
        <>
            <DocumentTitle pkg="Reports" />
            <Slot />
        </>
    )
}
```

`app/a/(app)/reports/index.tsx`:

```tsx
export { ReportsHubScreen as default } from '@tinycld/core/reports/screens/ReportsHubScreen'
```

`app/a/(app)/reports/new.tsx`:

```tsx
export { NewReportScreen as default } from '@tinycld/core/reports/screens/NewReportScreen'
```

`app/a/(app)/reports/[id].tsx`:

```tsx
import { ReportScreen } from '@tinycld/core/reports/screens/ReportScreen'
import { useLocalSearchParams } from 'expo-router'

export default function ReportRoute() {
    const { id } = useLocalSearchParams<{ id: string }>()
    return <ReportScreen reportId={id} />
}
```

In `app/a/(app)/_layout.tsx`: change `const NON_RESTORABLE_SLUGS = new Set(['settings', 'help'])` to `new Set(['settings', 'help', 'reports'])`, and in `ActivePkgSync` change

```ts
        const nextSlug = slug === 'settings' || slug === 'help' ? null : slug
```

to

```ts
        const nextSlug = slug !== null && NON_RESTORABLE_SLUGS.has(slug) ? null : slug
```

and its comment `// settings/help are chrome,` to `// settings/help/reports are chrome,`.

In `core/components/workspace/PackageTabs.tsx`: `const APP_ROUTES = ['index', 'help', 'reports', 'settings']`.

In `core/components/workspace/PackageRail.tsx`: add `BarChart3` to the lucide import and, before the Help link, add:

```tsx
                <Link href={orgHref('reports')} asChild>
                    <Pressable
                        testID="nav-reports"
                        className="w-11 h-11 rounded-xl justify-center items-center"
                        accessibilityLabel="Reports"
                    >
                        <BarChart3 size={22} color={railText} />
                    </Pressable>
                </Link>
```

In `core/components/workspace/MoreDrawer.tsx`: add `BarChart3` to the lucide import and, before the Settings row, add:

```tsx
                {/* Reports is an app area with no tab of its own on a phone. */}
                <Pressable
                    testID="more-reports"
                    className="flex-row items-center gap-3.5 px-4 py-3.5 rounded-lg"
                    onPress={() => handleNav(() => router.push(orgHref('reports')))}
                >
                    <BarChart3 size={20} color={textColor} />
                    <Text className="text-base font-medium" style={{ color: textColor }}>
                        Reports
                    </Text>
                </Pressable>
```

- [ ] **Step 5: Write the shared view parts**

`core/reports/hooks/useBackToReports.ts`:

```ts
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { router } from 'expo-router'

// A report is opened from the hub, which is still in the stack. Going back keeps
// one copy of the hub; a deep link with nothing to go back to lands on it.
export function useBackToReports() {
    const orgHref = useOrgHref()
    return () => {
        if (router.canGoBack()) router.back()
        else router.replace(orgHref('reports'))
    }
}
```

`core/reports/components/ReportHeader.tsx`:

```tsx
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ArrowLeft } from 'lucide-react-native'
import type { ReactNode } from 'react'
import { Pressable, Text, View } from 'react-native'

export const REPORT_TAP_TARGET = 44

function ReportBackLink({ onBack }: { onBack: () => void }) {
    const muted = useThemeColor('muted-foreground')
    return (
        <Pressable testID="report-back" accessibilityRole="link" onPress={onBack} className="flex-row items-center gap-1 self-start" style={{ minHeight: REPORT_TAP_TARGET }}>
            <ArrowLeft size={16} color={muted} />
            <Text className="text-sm text-muted-foreground">Reports</Text>
        </Pressable>
    )
}

export function ReportHeader({ title, onBack, toolbar }: { title: string; onBack: () => void; toolbar: ReactNode }) {
    return (
        <View className="gap-1 px-4 pt-2">
            <ReportBackLink onBack={onBack} />
            <View className="flex-row items-center justify-between gap-2">
                <Text testID="report-title" numberOfLines={1} className="min-w-0 shrink text-2xl font-semibold text-foreground">
                    {title}
                </Text>
                <View className="min-w-0 flex-1 items-end">{toolbar}</View>
            </View>
        </View>
    )
}
```

`core/reports/components/ReportDocumentHeader.tsx`:

```tsx
import { Text, View } from 'react-native'

function Line({ text, testID, className }: { text: string | null; testID: string; className: string }) {
    if (!text) return null
    return (
        <Text testID={testID} className={className}>
            {text}
        </Text>
    )
}

interface ReportDocumentHeaderProps {
    orgName: string
    title: string
    periodWords: string | null
    isThousands: boolean
}

// The finished-document strip above the table (the Books shell's spark 1).
export function ReportDocumentHeader({ orgName, title, periodWords, isThousands }: ReportDocumentHeaderProps) {
    return (
        <View testID="report-document-header" className="gap-0.5 border-y border-border bg-card px-4 py-3">
            <Line text={orgName.trim() || null} testID="report-business-name" className="text-xs text-muted-foreground" />
            <Text testID="report-document-title" className="text-lg font-semibold text-foreground">
                {title}
            </Text>
            <Line text={periodWords} testID="report-document-period" className="text-sm text-muted-foreground" />
            <Line text={isThousands ? 'Amounts in thousands' : null} testID="report-thousands-note" className="text-sm text-muted-foreground" />
        </View>
    )
}
```

`core/reports/components/ReportStates.tsx`:

```tsx
import { LoadingState } from '@tinycld/core/components/LoadingState'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { Text, View } from 'react-native'
import { describeProblem } from '../lib/builder/errors'
import type { EmptyAction } from '../lib/params'
import type { Problem } from '../lib/types'
import { REPORT_TAP_TARGET } from './ReportHeader'

export function LoadingBody({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View testID="report-loading" className="flex-1">
            <LoadingState />
        </View>
    )
}

interface MessageBodyProps {
    isVisible: boolean
    testID: string
    message: string
    action?: { label: string; testID: string; onPress: () => void } | null
}

function ActionButton({ action }: { action: MessageBodyProps['action'] }) {
    if (!action) return null
    return (
        <Button testID={action.testID} variant="outline" style={{ minHeight: REPORT_TAP_TARGET }} onPress={action.onPress}>
            <ButtonText>{action.label}</ButtonText>
        </Button>
    )
}

/** A state with a message and at most one action, drawn in place of the table. */
export function MessageBody({ isVisible, testID, message, action }: MessageBodyProps) {
    if (!isVisible) return null
    return (
        <View testID={testID} accessibilityRole="alert" className="items-center gap-3 p-6">
            <Text className="text-center text-base font-medium text-foreground">{message}</Text>
            <ActionButton action={action} />
        </View>
    )
}

interface ProblemsBodyProps {
    isVisible: boolean
    testID: string
    message: string
    problems: Problem[]
    action: MessageBodyProps['action']
}

// A report that cannot run names each missing or invalid part (spec "Errors").
export function ProblemsBody({ isVisible, testID, message, problems, action }: ProblemsBodyProps) {
    if (!isVisible) return null
    const lines = problems.map((problem, i) => (
        <Text key={problem.path} testID={`${testID}-problem-${i}`} className="text-sm text-muted-foreground">
            {describeProblem(problem)}
        </Text>
    ))
    return (
        <View testID={testID} accessibilityRole="alert" className="items-center gap-2 p-6">
            <Text className="text-center text-base font-medium text-foreground">{message}</Text>
            {lines}
            <ActionButton action={action} />
        </View>
    )
}

interface EmptyBodyProps {
    isVisible: boolean
    message: string
    action: EmptyAction | null
    onAction: () => void
}

export function EmptyBody({ isVisible, message, action, onAction }: EmptyBodyProps) {
    const button = action ? { label: action.label, testID: 'report-empty-action', onPress: onAction } : null
    return <MessageBody isVisible={isVisible} testID="report-empty" message={message} action={button} />
}
```

- [ ] **Step 6: Write the view model**

`core/reports/hooks/useReportView.ts`:

```ts
import { useBreakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import { useAuth } from '@tinycld/core/lib/auth'
import { log } from '@tinycld/core/lib/logger'
import { getResolvedAddress } from '@tinycld/core/lib/server-address'
import { useToastStore } from '@tinycld/core/lib/stores/toast-store'
import { useOrgInfo } from '@tinycld/core/lib/use-org-info'
import * as Clipboard from 'expo-clipboard'
import { router, useFocusEffect, useIsFocused, useLocalSearchParams } from 'expo-router'
import { useCallback, useEffectEvent } from 'react'
import type { ReportAccess } from '../lib/access'
import { todayIso } from '../lib/dates'
import { type DisplayOptions, DEFAULT_DISPLAY, shownColumns } from '../lib/format'
import {
    controlItems,
    emptyAction,
    emptyMessage,
    firstDateParam,
    type ParamValue,
    periodWords,
    postedParams,
    withoutOptional,
} from '../lib/params'
import { type Expansion, expansionOptions, foldAllLabel, foldAllTarget, hasRows, NO_EXPANSION, toggleGroup, visibleRows } from '../lib/rows'
import type { RunBody } from '../lib/run-client'
import { type BrokenMark, brokenProblems, type ParamDef, type ReportSpec } from '../lib/spec'
import type { Notice, ResolvedParam, ResultColumn, ResultRow } from '../lib/types'
import { parseViewOptions, reportLink, serializeViewOptions, type ViewOptions } from '../lib/view-url'
import { useReportUIStore } from '../stores/report-ui-store'
import { type ReportRun, useReportRun } from './useReportRun'

export type ViewSource = { kind: 'saved'; reportId: string } | { kind: 'draft'; spec: ReportSpec }
export type ViewState =
    | 'loading'
    | 'incomplete'
    | 'error'
    | 'timeout'
    | 'invalid'
    | 'missing'
    | 'no-access'
    | 'empty'
    | 'ready'

const NO_COLUMNS: ResultColumn[] = []
const NO_NOTICES: Notice[] = []
const NO_ROWS: ResultRow[] = []

function viewState(input: { source: ViewSource | null; broken: BrokenMark | null; isEditing: boolean; run: ReportRun }): ViewState {
    if (input.broken && !input.isEditing) return 'missing'
    if (!input.source) return input.isEditing ? 'incomplete' : 'loading'
    const { run } = input
    if (run.state === 'idle' || run.state === 'loading') return 'loading'
    if (run.state !== 'ready') return run.state
    return run.result && hasRows(run.result.rows) ? 'ready' : 'empty'
}

function runBody(source: ViewSource | null, defs: readonly ParamDef[], options: ViewOptions, expansion: Expansion): RunBody | null {
    if (!source) return null
    const common = {
        params: postedParams(defs, options.params),
        options: { today: todayIso(), ...expansionOptions(expansion) },
    }
    return source.kind === 'saved' ? { reportId: source.reportId, ...common } : { spec: source.spec, ...common }
}

export interface ReportViewInput {
    /** Keys the open groups: the report id, or NEW_REPORT_KEY for an unsaved draft. */
    reportKey: string
    title: string
    defs: readonly ParamDef[]
    source: ViewSource | null
    access: ReportAccess
    broken: BrokenMark | null
    isEditing: boolean
    onBack(): void
    onEdit?(): void
}

export function useReportView(input: ReportViewInput) {
    const { reportKey, defs, source, broken, isEditing } = input
    const { user } = useAuth()
    const search = useLocalSearchParams()
    const options = parseViewOptions(defs, search)
    const expansion = useReportUIStore(s => s.expansion[reportKey]) ?? NO_EXPANSION
    const setExpansion = useReportUIStore(s => s.setExpansion)
    const body = broken && !isEditing ? null : runBody(source, defs, options, expansion)
    const run = useReportRun({ body, userId: user.id })
    const breakpoint = useBreakpoint()
    const { org } = useOrgInfo()
    // A pushed screen keeps the one under it mounted; only the focused one draws surfaces.
    const isFocused = useIsFocused()
    const storedSurface = useReportUIStore(s => s.surface)
    const openSurface = useReportUIStore(s => s.openSurface)
    const closeSurface = useReportUIStore(s => s.closeSurface)
    const closeSurfaceOf = useReportUIStore(s => s.closeSurfaceOf)
    const isCustomizeOpen = useReportUIStore(s => s.isCustomizeOpen) && isFocused
    const openCustomize = useReportUIStore(s => s.openCustomize)
    const closeCustomize = useReportUIStore(s => s.closeCustomize)
    const rows = run.result?.rows ?? NO_ROWS
    const resolved: Record<string, ResolvedParam> = Object.fromEntries((run.result?.params ?? []).map(p => [p.id, p]))
    const period = firstDateParam(defs)
    const action = emptyAction(defs, options.params)

    // setParams replaces the history entry, so a change adds none; defaults are left out.
    const write = (next: ViewOptions) => router.setParams(serializeViewOptions(defs, next))
    // A preset or a picked value closes its popover; a typed custom date keeps it open,
    // so a date error stays in view.
    const onApplyParam = (id: string, value: ParamValue, keepOpen = false) => {
        write({ ...options, params: { ...options.params, [id]: value } })
        if (!keepOpen) closeSurface()
    }
    const onRemoveParam = (id: string) => {
        write({ ...options, params: Object.fromEntries(Object.entries(options.params).filter(([key]) => key !== id)) })
        closeSurface()
    }
    const onEmptyAction = () => {
        if (action?.kind === 'clear-filters') write({ ...options, params: withoutOptional(defs, options.params) })
        if (action?.kind === 'preset') onApplyParam(action.paramId, { kind: 'preset', preset: action.preset })
    }
    const onCopyLink =
        source?.kind === 'saved'
            ? () => {
                  const link = reportLink(getResolvedAddress() ?? '', source.reportId, defs, options)
                  Clipboard.setStringAsync(link)
                      .then(() => useToastStore.getState().addToast({ title: 'Link copied', variant: 'success', duration: 3000 }))
                      .catch((error: unknown) => {
                          log.warn('reports.copy-link', 'could not copy the link', { error })
                          useToastStore.getState().addToast({ title: 'Could not copy the link', variant: 'error', duration: 5000 })
                      })
              }
            : null

    // A surface left open would come back with this screen, or show over the next one.
    const closeOnBlur = useEffectEvent(() => {
        closeSurface()
        closeCustomize()
    })
    useFocusEffect(useCallback(() => () => closeOnBlur(), []))

    return {
        title: input.title,
        access: input.access,
        isEditing,
        onBack: input.onBack,
        onEdit: input.onEdit ?? null,
        options,
        display: options.display,
        state: viewState({ source, broken, isEditing, run }),
        problems: broken && !isEditing ? brokenProblems(broken) : run.problems,
        run,
        rows: visibleRows(rows),
        columns: shownColumns(run.result?.columns ?? NO_COLUMNS, options.display.hidden),
        allColumns: run.result?.columns ?? NO_COLUMNS,
        notices: run.result?.notices ?? NO_NOTICES,
        isUpdating: run.isUpdating,
        breakpoint,
        orgName: org?.name ?? '',
        periodWords: periodWords(period, period ? resolved[period.id] : undefined),
        emptyMessage: emptyMessage(period, period ? resolved[period.id] : undefined),
        emptyAction: action,
        onEmptyAction,
        defs,
        controls: controlItems(defs, options.params),
        resolved,
        foldAllLabel: foldAllLabel(rows),
        onFoldAll: () => setExpansion(reportKey, foldAllTarget(rows)),
        onToggleGroup: (row: ResultRow) => setExpansion(reportKey, toggleGroup(expansion, row, rows)),
        surface: isFocused ? storedSurface : null,
        onOpenSurface: openSurface,
        onCloseSurface: closeSurface,
        onCloseSurfaceOf: closeSurfaceOf,
        onApplyParam,
        onRemoveParam,
        isCustomizeOpen,
        onOpenCustomize: openCustomize,
        onCloseCustomize: closeCustomize,
        onDisplayChange: (display: DisplayOptions) => write({ ...options, display }),
        onResetDisplay: () => write({ params: {}, display: DEFAULT_DISPLAY }),
        onCopyLink,
    }
}

export type ReportViewModel = ReturnType<typeof useReportView>
```

- [ ] **Step 7: Write the report view**

`core/reports/components/ReportView.tsx`:

```tsx
import { ResponsiveToolbar, type ToolbarItem } from '@tinycld/core/components/ResponsiveToolbar'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import type { ReactNode } from 'react'
import { View } from 'react-native'
import type { ReportViewModel } from '../hooks/useReportView'
import { ReportDocumentHeader } from './ReportDocumentHeader'
import { REPORT_TAP_TARGET, ReportHeader } from './ReportHeader'
import { EmptyBody, LoadingBody, MessageBody, ProblemsBody } from './ReportStates'
import { ReportTable } from './ReportTable'

function toolbarButton(key: string, label: string, testID: string, onPress: () => void): ToolbarItem {
    return {
        type: 'custom',
        key,
        overflow: { label, onPress },
        element: (
            <Button testID={testID} variant="outline" style={{ minHeight: REPORT_TAP_TARGET }} onPress={onPress}>
                <ButtonText>{label}</ButtonText>
            </Button>
        ),
    }
}

// Every action the reader may take on this report, in the order the toolbar shows them;
// ResponsiveToolbar folds what does not fit into "⋯".
function toolbarItems(model: ReportViewModel): ToolbarItem[] {
    const items: ToolbarItem[] = []
    if (model.foldAllLabel) items.push(toolbarButton('fold', model.foldAllLabel, 'report-fold-all', model.onFoldAll))
    if (model.onEdit && model.access.canEdit && !model.isEditing) {
        items.push(toolbarButton('edit', 'Edit report', 'report-edit-open', model.onEdit))
    }
    return items
}

export function ReportBody({ model }: { model: ReportViewModel }) {
    const { state } = model
    const retry = { label: 'Retry', testID: 'report-retry', onPress: model.run.retry }
    const edit = model.onEdit && model.access.canEdit ? { label: 'Edit report', testID: 'report-missing-edit', onPress: model.onEdit } : null
    return (
        <>
            <LoadingBody isVisible={state === 'loading'} />
            <MessageBody isVisible={state === 'error'} testID="report-error" message="Could not load this report" action={retry} />
            <MessageBody isVisible={state === 'timeout'} testID="report-timeout" message="This report took too long to run" action={retry} />
            <MessageBody isVisible={state === 'no-access'} testID="report-no-access" message="You no longer have access to this report" />
            <MessageBody isVisible={state === 'incomplete'} testID="report-incomplete" message="Finish the report's design to see it here" />
            <ProblemsBody isVisible={state === 'missing'} testID="report-missing" message="This report names something that no longer exists" problems={model.problems} action={edit} />
            <ProblemsBody isVisible={state === 'invalid'} testID="report-invalid" message="This report cannot run" problems={model.problems} action={edit} />
            <EmptyBody isVisible={state === 'empty'} message={model.emptyMessage} action={model.emptyAction} onAction={model.onEmptyAction} />
            <TableBody model={model} />
        </>
    )
}

function TableBody({ model }: { model: ReportViewModel }) {
    if (model.state !== 'ready') return null
    return (
        <ReportTable
            columns={model.columns}
            rows={model.rows}
            format={model.display}
            breakpoint={model.breakpoint}
            onToggleGroup={model.onToggleGroup}
            onNearEnd={model.run.loadMore}
            isLoadingMore={model.run.isLoadingMore}
        />
    )
}

export function ReportView({ model, side }: { model: ReportViewModel; side?: ReactNode }) {
    const toolbar = <ResponsiveToolbar items={toolbarItems(model)} />
    return (
        <View testID="report-screen" className="flex-1 flex-row bg-background">
            <View className="min-w-0 flex-1">
                <ReportHeader title={model.title} onBack={model.onBack} toolbar={toolbar} />
                <ReportDocumentHeader orgName={model.orgName} title={model.title} periodWords={model.periodWords} isThousands={model.display.divideBy1000} />
                <ReportBody model={model} />
            </View>
            {side}
        </View>
    )
}
```

- [ ] **Step 8: Write the builder hook**

`core/reports/hooks/useReportBuilder.ts`:

```ts
import { useAuth } from '@tinycld/core/lib/auth'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useDebouncedValue } from '@tinycld/core/lib/use-debounced-value'
import { useForm, useWatch, zodResolver } from '@tinycld/core/ui/form'
import { router } from 'expo-router'
import { newRecordId } from 'pbtsdb'
import { useMemo, useState } from 'react'
import type { Catalog } from '../lib/builder/columns'
import { type BuilderDraft, builderSchema, draftFromSpec, emptyDraft, specFromDraft } from '../lib/builder/draft'
import { problemsFromError } from '../lib/builder/errors'
import { parseSpec, type ReportSpec } from '../lib/spec'
import type { CatalogSource } from '../lib/types'
import type { SavedReport } from './useReportRecord'

const PREVIEW_DELAY_MS = 400
const MAX_TITLE = 200

export function copyTitle(title: string): string {
    return `Copy of ${title}`.slice(0, MAX_TITLE)
}

export interface BuilderInput {
    mode: 'new' | 'edit'
    catalog: Catalog
    base: CatalogSource
    record: SavedReport | null
    spec: ReportSpec | null
}

type SaveInput = { draft: BuilderDraft; asCopy: boolean; title: string }
const LIST_CHANGE = { shouldDirty: true, shouldValidate: true } as const

// The builder's form and its save. The preview runs the draft as it would be
// saved, 400 ms after the last valid change; a title is not needed to preview.
export function useReportBuilder(input: BuilderInput) {
    const { mode, catalog, base, record, spec } = input
    const { user } = useAuth()
    const orgHref = useOrgHref()
    const [reportsCollection] = useStore('reports')
    const schema = useMemo(() => builderSchema(catalog), [catalog])
    const defaults =
        record && spec ? draftFromSpec(spec, { title: record.title, description: record.description }, catalog) : emptyDraft(base)
    const form = useForm<BuilderDraft>({ resolver: zodResolver(schema), mode: 'onChange', defaultValues: defaults })
    const values = useWatch({ control: form.control })
    const parsed = schema.safeParse({ ...values, title: 'Preview' })
    const specText = parsed.success ? JSON.stringify(specFromDraft(parsed.data, catalog)) : ''
    const debounced = useDebouncedValue(specText, PREVIEW_DELAY_MS)
    const previewSpec = useMemo(() => (debounced ? parseSpec(JSON.parse(debounced)) : null), [debounced])
    // The Discard confirm of an edit: local UI state.
    const [isConfirmingDiscard, setConfirmingDiscard] = useState(false)

    const exit = () => {
        if (mode === 'edit') router.setParams({ edit: undefined })
        else if (router.canGoBack()) router.back()
        else router.replace(orgHref('reports'))
    }

    const save = useMutation({
        mutationFn: function* ({ draft, asCopy, title }: SaveInput) {
            const nextSpec = specFromDraft(draft, catalog)
            if (mode === 'edit' && record && !asCopy) {
                yield reportsCollection.update(record.id, row => {
                    row.title = draft.title
                    row.description = draft.description
                    row.spec = nextSpec
                })
                return record.id
            }
            const id = newRecordId()
            yield reportsCollection.insert({ id, owner: user.id, title, description: draft.description, spec: nextSpec, published: false })
            return id
        },
        onSuccess: (id, { asCopy }) => {
            form.reset(form.getValues())
            if (mode === 'edit' && !asCopy) router.setParams({ edit: undefined })
            else router.replace(orgHref('reports/[id]', { id }))
        },
        onError: (error: unknown) => {
            const problems = problemsFromError(error)
            for (const problem of problems) {
                if (problem.path === 'title' || problem.path === 'description') {
                    form.setError(problem.path, { type: 'server', message: problem.message })
                }
            }
            // Spec problems show in their sections (saveProblems); anything else is a toast.
            if (problems.length === 0) {
                handleMutationErrorsWithForm({ setError: form.setError, getValues: form.getValues, operation: 'reports.save' })(error)
            }
        },
    })

    return {
        mode,
        form,
        catalog,
        base,
        previewSpec,
        title: values.title ?? '',
        isDirty: form.formState.isDirty,
        isSaving: save.isPending,
        isSaved: save.isSuccess,
        saveProblems: save.error ? problemsFromError(save.error) : [],
        onSave: () => {
            void form.handleSubmit(draft => save.mutate({ draft, asCopy: false, title: draft.title }))()
        },
        onSaveAs: (title: string) => {
            void form.handleSubmit(draft => save.mutate({ draft, asCopy: true, title }))()
        },
        onClose: () => {
            if (mode === 'edit' && form.formState.isDirty) setConfirmingDiscard(true)
            else exit()
        },
        isConfirmingDiscard,
        onKeepEditing: () => setConfirmingDiscard(false),
        onDiscard: () => {
            setConfirmingDiscard(false)
            form.reset(defaults)
            exit()
        },
        // Each replaces a whole list as one change, so a move or a removal is one edit.
        setColumns: (items: BuilderDraft['columns']) => form.setValue('columns', items, LIST_CHANGE),
        setGroups: (items: BuilderDraft['groups']) => form.setValue('groups', items, LIST_CHANGE),
        setSort: (items: BuilderDraft['sort']) => form.setValue('sort', items, LIST_CHANGE),
        setLayout: (items: BuilderDraft['layout']) => form.setValue('layout', items, LIST_CHANGE),
        setFilter: (filter: BuilderDraft['filter']) => form.setValue('filter', filter, LIST_CHANGE),
        setParams: (params: BuilderDraft['params']) => form.setValue('params', params, LIST_CHANGE),
        setPivotPath: (path: BuilderDraft['pivot']['path']) => form.setValue('pivot.path', path, LIST_CHANGE),
    }
}

export type ReportBuilder = ReturnType<typeof useReportBuilder>
```

- [ ] **Step 9: Write the builder panel, sections, Details, Columns and the picker**

`core/reports/components/builder/BuilderSection.tsx`:

```tsx
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ChevronDown, ChevronRight } from 'lucide-react-native'
import { type ReactNode, useState } from 'react'
import { Pressable, Text, View } from 'react-native'
import { type BuilderSection as Section, describeProblem, problemsFor } from '../../lib/builder/errors'
import type { Problem } from '../../lib/types'
import { REPORT_TAP_TARGET } from '../ReportHeader'

function SectionProblems({ section, problems }: { section: Section; problems: Problem[] }) {
    const mine = problemsFor(section, problems)
    if (mine.length === 0) return null
    const lines = mine.map(problem => (
        <Text key={problem.path} className="text-xs text-danger">
            {describeProblem(problem)}
        </Text>
    ))
    return (
        <View testID={`report-builder-problems-${section}`} accessibilityRole="alert" className="gap-1 pb-2">
            {lines}
        </View>
    )
}

function SectionBody({ isVisible, children }: { isVisible: boolean; children: ReactNode }) {
    if (!isVisible) return null
    return <View className="gap-3 pb-3">{children}</View>
}

interface BuilderSectionProps {
    title: string
    section: Section
    problems: Problem[]
    isOpenAtStart?: boolean
    children: ReactNode
}

// A folding part of the builder; its server problems stay in view while it is folded.
// The fold state is the section's own: nothing else reads it.
export function BuilderSection({ title, section, problems, isOpenAtStart = false, children }: BuilderSectionProps) {
    const [isOpen, setIsOpen] = useState(isOpenAtStart)
    const color = useThemeColor('muted-foreground')
    const Caret = isOpen ? ChevronDown : ChevronRight
    return (
        <View className="border-b border-border">
            <Pressable
                testID={`report-builder-section-${section}`}
                accessibilityRole="button"
                aria-expanded={isOpen}
                onPress={() => setIsOpen(open => !open)}
                className="flex-row items-center gap-2"
                style={{ minHeight: REPORT_TAP_TARGET }}
            >
                <Caret size={16} color={color} />
                <Text className="text-sm font-semibold text-foreground">{title}</Text>
            </Pressable>
            <SectionProblems section={section} problems={problems} />
            <SectionBody isVisible={isOpen}>{children}</SectionBody>
        </View>
    )
}
```

`core/reports/components/builder/DetailsSection.tsx`:

```tsx
import { TextAreaInput, TextInput } from '@tinycld/core/ui/form'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import type { Problem } from '../../lib/types'
import { BuilderSection } from './BuilderSection'

export function DetailsSection({ builder, problems }: { builder: ReportBuilder; problems: Problem[] }) {
    const { control } = builder.form
    return (
        <BuilderSection title="Details" section="details" problems={problems} isOpenAtStart={builder.mode === 'new'}>
            <TextInput name="title" control={control} label="Title" testID="report-builder-title" maxLength={200} />
            <TextAreaInput name="description" control={control} label="Description" testID="report-builder-description" />
        </BuilderSection>
    )
}
```

`core/reports/components/builder/ColumnPicker.tsx`:

```tsx
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { Popover } from '@tinycld/core/ui/popover'
import { ArrowLeft, ChevronRight } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { type Catalog, MAX_HOPS, sourceAtLinks } from '../../lib/builder/columns'
import type { SpecHop } from '../../lib/spec'
import type { CatalogLink } from '../../lib/types'
import { REPORT_TAP_TARGET } from '../ReportHeader'

const PICKER_WIDTH = 340
const LIST_MAX_HEIGHT = 380

function PickRow({ label, hint, testID, onPress }: { label: string; hint?: string; testID: string; onPress: () => void }) {
    const muted = useThemeColor('muted-foreground')
    return (
        <Pressable testID={testID} accessibilityRole="button" onPress={onPress} className="flex-row items-center gap-2 rounded-md px-2" style={{ minHeight: REPORT_TAP_TARGET }}>
            <Text numberOfLines={1} className="min-w-0 flex-1 text-sm text-foreground">
                {label}
            </Text>
            <Text className="text-xs text-muted-foreground">{hint ?? ''}</Text>
            <ChevronRight size={14} color={muted} />
        </Pressable>
    )
}

function BackRow({ isVisible, onPress }: { isVisible: boolean; onPress: () => void }) {
    const muted = useThemeColor('muted-foreground')
    if (!isVisible) return null
    return (
        <Pressable testID="report-builder-pick-back" accessibilityRole="button" onPress={onPress} className="flex-row items-center gap-1 px-2" style={{ minHeight: REPORT_TAP_TARGET }}>
            <ArrowLeft size={14} color={muted} />
            <Text className="text-sm text-muted-foreground">Back</Text>
        </Pressable>
    )
}

// A to-many link reaches many rows: they are totalled onto each row, or shown as child rows.
function ManyChoice({ link, onTotal, onNest }: { link: CatalogLink | null; onTotal: () => void; onNest: () => void }) {
    if (!link) return null
    return (
        <View className="gap-2 p-2">
            <Text className="text-sm text-foreground">{`Each row has many ${link.label}.`}</Text>
            <Button testID="report-builder-pick-total" variant="outline" onPress={onTotal}>
                <ButtonText>Add as total</ButtonText>
            </Button>
            <Button testID="report-builder-pick-nest" variant="outline" onPress={onNest}>
                <ButtonText>Show as child rows</ButtonText>
            </Button>
        </View>
    )
}

interface OpenPickerProps {
    catalog: Catalog
    baseKey: string
    toOneOnly: boolean
    onPick(path: SpecHop[]): void
}

// The links walked so far and a to-many link waiting for its choice are this open
// picker's own state; a new open starts at the source.
function OpenPicker({ catalog, baseKey, toOneOnly, onPick }: OpenPickerProps) {
    const [hops, setHops] = useState<SpecHop[]>([])
    const [pending, setPending] = useState<CatalogLink | null>(null)
    const source = sourceAtLinks(catalog, baseKey, hops)
    const canFollow = hops.length < MAX_HOPS
    const back = () => {
        if (pending) setPending(null)
        else setHops(hops.slice(0, -1))
    }
    const follow = (link: CatalogLink) => {
        if (link.cardinality === 'toMany') setPending(link)
        else setHops([...hops, { link: link.key }])
    }
    const choose = (mode: 'aggregate' | 'nest') => {
        if (!pending) return
        setHops([...hops, { link: pending.key, mode }])
        setPending(null)
    }
    const fields = (pending ? [] : (source?.fields ?? [])).map(field => (
        <PickRow key={field.key} label={field.label} testID={`report-builder-pick-field-${field.key}`} onPress={() => onPick([...hops, { field: field.key }])} />
    ))
    const offered = pending || !canFollow ? [] : (source?.links ?? []).filter(l => !toOneOnly || l.cardinality === 'toOne')
    const links = offered.map(link => (
        <PickRow key={link.key} label={link.label} hint={link.cardinality === 'toMany' ? 'many' : 'one'} testID={`report-builder-pick-link-${link.key}`} onPress={() => follow(link)} />
    ))
    return (
        <View className="gap-1">
            <BackRow isVisible={hops.length > 0 || pending !== null} onPress={back} />
            <Text className="px-2 text-xs font-semibold uppercase text-muted-foreground">{source?.label ?? ''}</Text>
            <ManyChoice link={pending} onTotal={() => choose('aggregate')} onNest={() => choose('nest')} />
            <ScrollView style={{ maxHeight: LIST_MAX_HEIGHT }} nestedScrollEnabled>
                {fields}
                {links}
            </ScrollView>
        </View>
    )
}

function PickerBody({ isVisible, ...props }: OpenPickerProps & { isVisible: boolean }) {
    if (!isVisible) return null
    return <OpenPicker {...props} />
}

export interface ColumnPickerProps {
    catalog: Catalog
    baseKey: string
    onPick(path: SpecHop[]): void
    label: string
    testID: string
    /** For a filter field or a pivot: one value per row, so no to-many link. */
    toOneOnly?: boolean
}

// A tree of the source's fields and links (spec §4): a to-one link opens in place, a
// to-many link offers "Add as total" or "Show as child rows". The open flag is the
// picker's own UI state.
export function ColumnPicker({ catalog, baseKey, onPick, label, testID, toOneOnly = false }: ColumnPickerProps) {
    const [isOpen, setIsOpen] = useState(false)
    const trigger = (
        <Button testID={testID} variant="outline" size="sm">
            <ButtonText>{label}</ButtonText>
        </Button>
    )
    const pick = (path: SpecHop[]) => {
        setIsOpen(false)
        onPick(path)
    }
    return (
        <Popover trigger={trigger} isOpen={isOpen} onOpenChange={setIsOpen} title={label} sheetSide="bottom" width={PICKER_WIDTH} testID="report-builder-picker">
            <PickerBody isVisible={isOpen} catalog={catalog} baseKey={baseKey} toOneOnly={toOneOnly} onPick={pick} />
        </Popover>
    )
}
```

`core/reports/components/builder/ColumnsSection.tsx`:

```tsx
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { SelectInput, TextInput, useWatch } from '@tinycld/core/ui/form'
import { ArrowDown, ArrowUp, Trash2 } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import {
    AGGREGATE_LABELS,
    aggregatesFor,
    columnHint,
    defaultAggregate,
    describePath,
    nextColumnId,
    pathLabel,
} from '../../lib/builder/columns'
import type { ColumnDraft } from '../../lib/builder/draft'
import type { SpecHop } from '../../lib/spec'
import type { Aggregate, Problem } from '../../lib/types'
import { REPORT_TAP_TARGET } from '../ReportHeader'
import { BuilderSection } from './BuilderSection'
import { ColumnPicker } from './ColumnPicker'

const FORMAT_OPTIONS = [
    { value: '', label: 'Number' },
    { value: 'percent', label: 'Percent' },
]
const FORMULA_AGGREGATES: Aggregate[] = ['sum', 'min', 'max', 'avg']

export function moved<T>(items: readonly T[], from: number, to: number): T[] {
    const next = [...items]
    const [item] = next.splice(from, 1)
    if (item !== undefined) next.splice(to, 0, item)
    return next
}

function IconButton({ label, testID, isVisible, icon: Icon, onPress }: { label: string; testID: string; isVisible: boolean; icon: typeof Trash2; onPress: () => void }) {
    const color = useThemeColor('muted-foreground')
    if (!isVisible) return null
    return (
        <Pressable testID={testID} accessibilityRole="button" accessibilityLabel={label} onPress={onPress} className="items-center justify-center" style={{ width: REPORT_TAP_TARGET, height: REPORT_TAP_TARGET }}>
            <Icon size={16} color={color} />
        </Pressable>
    )
}

function aggregateOptions(builder: ReportBuilder, column: ColumnDraft) {
    const offered = column.kind === 'formula' ? FORMULA_AGGREGATES : (() => {
        const info = describePath(builder.catalog, builder.base.key, column.path)
        return info ? aggregatesFor(info.field, info.viaToMany) : []
    })()
    return [{ value: '', label: 'None' }, ...offered.map(a => ({ value: a, label: AGGREGATE_LABELS[a] }))]
}

function columnSummary(builder: ReportBuilder, column: ColumnDraft): string {
    if (column.kind === 'formula') return 'Formula'
    const info = describePath(builder.catalog, builder.base.key, column.path)
    if (!info) return 'A field that is no longer offered'
    if (info.mode === 'nest') return `${pathLabel(info)} · child rows`
    return info.mode === 'aggregate' ? `${pathLabel(info)} · total` : pathLabel(info)
}

function isNumberColumn(builder: ReportBuilder, column: ColumnDraft): boolean {
    if (column.kind === 'formula') return true
    return describePath(builder.catalog, builder.base.key, column.path)?.field.type === 'number'
}

interface ColumnRowProps {
    builder: ReportBuilder
    columns: ColumnDraft[]
    index: number
}

function ColumnRow({ builder, columns, index }: ColumnRowProps) {
    const column = columns[index]
    if (!column) return null
    const { control } = builder.form
    const testID = `report-builder-column-${index}`
    const last = columns.length - 1
    return (
        <View testID={testID} className="gap-1 rounded-lg border border-border p-2">
            <View className="flex-row items-center">
                <Text numberOfLines={1} className="min-w-0 flex-1 text-xs text-muted-foreground">
                    {columnSummary(builder, column)}
                </Text>
                <IconButton label={`Move ${column.label} up`} testID={`${testID}-up`} isVisible={index > 0} icon={ArrowUp} onPress={() => builder.setColumns(moved(columns, index, index - 1))} />
                <IconButton label={`Move ${column.label} down`} testID={`${testID}-down`} isVisible={index < last} icon={ArrowDown} onPress={() => builder.setColumns(moved(columns, index, index + 1))} />
                <IconButton label={`Remove ${column.label}`} testID={`${testID}-remove`} isVisible={columns.length > 1} icon={Trash2} onPress={() => builder.setColumns(columns.filter((_, i) => i !== index))} />
            </View>
            <TextInput name={`columns.${index}.label`} control={control} label="Label" testID={`${testID}-label`} maxLength={200} />
            <SelectInput name={`columns.${index}.aggregate`} control={control} label="Total" options={aggregateOptions(builder, column)} testID={`${testID}-aggregate`} />
            <FormatField builder={builder} column={column} index={index} />
            <ColumnFormula builder={builder} columns={columns} index={index} />
        </View>
    )
}

function FormatField({ builder, column, index }: { builder: ReportBuilder; column: ColumnDraft; index: number }) {
    if (!isNumberColumn(builder, column)) return null
    return <SelectInput name={`columns.${index}.format`} control={builder.form.control} label="Show as" options={FORMAT_OPTIONS} testID={`report-builder-column-${index}-format`} />
}

// Task 16 draws a formula column's editor here.
function ColumnFormula(_props: ColumnRowProps) {
    return null
}

export function ColumnsSection({ builder, problems }: { builder: ReportBuilder; problems: Problem[] }) {
    const columns = useWatch({ control: builder.form.control, name: 'columns' })
    const onPick = (path: SpecHop[]) => {
        const info = describePath(builder.catalog, builder.base.key, path)
        if (!info) return
        const column: ColumnDraft = {
            kind: 'path',
            id: nextColumnId(columns.map(c => c.id), columnHint(path)),
            label: pathLabel(info),
            path,
            aggregate: info.mode === 'aggregate' ? defaultAggregate(info.field, true) : '',
            scale: null,
            format: '',
        }
        builder.setColumns([...columns, column])
    }
    const rows = columns.map((column, index) => (
        <ColumnRow key={`${index}:${column.id}`} builder={builder} columns={columns} index={index} />
    ))
    return (
        <BuilderSection title="Columns" section="columns" problems={problems} isOpenAtStart>
            {rows}
            <View className="flex-row flex-wrap gap-2">
                <ColumnPicker catalog={builder.catalog} baseKey={builder.base.key} onPick={onPick} label="Add column" testID="report-builder-add-column" />
            </View>
        </BuilderSection>
    )
}
```

`core/reports/components/builder/ReportBuilderPanel.tsx`:

```tsx
import { useLeaveGuard } from '@tinycld/core/components/forms-builder/use-leave-guard'
import { type Breakpoint, useBreakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import { ConfirmDialog } from '@tinycld/core/ui/ConfirmDialog'
import { Drawer, DrawerBackdrop, DrawerBody, DrawerContent, DrawerFooter, DrawerHeader } from '@tinycld/core/ui/drawer'
import { PromptDialog } from '@tinycld/core/ui/PromptDialog'
import { Sheet } from '@tinycld/core/ui/sheet'
import { X } from 'lucide-react-native'
import { type ReactNode, useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { copyTitle, type ReportBuilder } from '../../hooks/useReportBuilder'
import type { ReportViewModel } from '../../hooks/useReportView'
import type { Problem } from '../../lib/types'
import { REPORT_TAP_TARGET } from '../ReportHeader'
import { ReportBody } from '../ReportView'
import { ColumnsSection } from './ColumnsSection'
import { DetailsSection } from './DetailsSection'

const DISCARD = { title: 'Discard changes?', message: 'Your changes to this report are not saved.' }

function CloseButton({ onPress }: { onPress: () => void }) {
    const color = useThemeColor('foreground')
    return (
        <Pressable testID="report-builder-close" accessibilityRole="button" accessibilityLabel="Close" onPress={onPress} className="items-center justify-center" style={{ width: REPORT_TAP_TARGET, height: REPORT_TAP_TARGET }}>
            <X size={18} color={color} />
        </Pressable>
    )
}

function PanelTitle({ builder }: { builder: ReportBuilder }) {
    return <Text className="text-lg font-semibold text-foreground">{builder.mode === 'new' ? 'New report' : 'Edit report'}</Text>
}

// The sections, in the order a report is designed. Later tasks add Filter, Group and
// sort, Pivot and Layout here.
function Sections({ builder, problems }: { builder: ReportBuilder; problems: Problem[] }) {
    return (
        <View>
            <DetailsSection builder={builder} problems={problems} />
            <ColumnsSection builder={builder} problems={problems} />
        </View>
    )
}

function SaveAsButton({ isVisible, onPress }: { isVisible: boolean; onPress: () => void }) {
    if (!isVisible) return null
    return (
        <Button testID="report-builder-save-as" variant="outline" onPress={onPress}>
            <ButtonText>Save as…</ButtonText>
        </Button>
    )
}

function Footer({ builder, onSaveAs }: { builder: ReportBuilder; onSaveAs: () => void }) {
    return (
        <View className="flex-row justify-end gap-2 border-t border-border p-3">
            <SaveAsButton isVisible={builder.mode === 'edit'} onPress={onSaveAs} />
            <ServerActionButton testID="report-builder-save" onPress={builder.onSave} isDisabled={builder.isSaving}>
                <ButtonText>Save</ButtonText>
            </ServerActionButton>
        </View>
    )
}

interface SurfaceProps {
    builder: ReportBuilder
    preview: ReportViewModel
    problems: Problem[]
    onSaveAs: () => void
}

function DrawerSurface({ builder, problems, onSaveAs, breakpoint }: SurfaceProps & { breakpoint: Breakpoint }) {
    const isInline = breakpoint === 'desktop'
    return (
        <Drawer isOpen onClose={builder.onClose} presentation={isInline ? 'inline' : 'overlay'} anchor="right" size="md">
            <DrawerBackdrop />
            <DrawerContent testID="report-builder" role={isInline ? 'complementary' : 'dialog'} accessibilityLabel="Report builder" className="w-[440px]">
                <DrawerHeader className="border-b border-border">
                    <PanelTitle builder={builder} />
                    <CloseButton onPress={builder.onClose} />
                </DrawerHeader>
                <DrawerBody style={{ flexShrink: 1 }}>
                    <Sections builder={builder} problems={problems} />
                </DrawerBody>
                <DrawerFooter>
                    <Footer builder={builder} onSaveAs={onSaveAs} />
                </DrawerFooter>
            </DrawerContent>
        </Drawer>
    )
}

const MODES = [
    { value: 'design', label: 'Design' },
    { value: 'preview', label: 'Preview' },
] as const
type Mode = (typeof MODES)[number]['value']

function ModeSwitch({ mode, onChange }: { mode: Mode; onChange: (mode: Mode) => void }) {
    const pills = MODES.map(item => (
        <Pressable
            key={item.value}
            testID={`report-builder-mode-${item.value}`}
            accessibilityRole="radio"
            aria-checked={item.value === mode}
            onPress={() => onChange(item.value)}
            className={`flex-1 items-center justify-center rounded-full border px-3 ${item.value === mode ? 'border-primary bg-primary' : 'border-border'}`}
            style={{ minHeight: REPORT_TAP_TARGET }}
        >
            <Text className={`text-sm font-medium ${item.value === mode ? 'text-primary-foreground' : 'text-foreground'}`}>{item.label}</Text>
        </Pressable>
    ))
    return (
        <View accessibilityRole="radiogroup" className="flex-row gap-2 px-3 pb-2">
            {pills}
        </View>
    )
}

function ModeBody({ mode, design, preview }: { mode: Mode; design: ReactNode; preview: ReactNode }) {
    return <View className="min-h-0 flex-1">{mode === 'design' ? design : preview}</View>
}

// A phone cannot show the design and the report together, so a full-height sheet
// switches between them (R14, Q6). The switch is this sheet's own UI state.
function SheetSurface({ builder, preview, problems, onSaveAs }: SurfaceProps) {
    const [mode, setMode] = useState<Mode>('design')
    const design = (
        <ScrollView contentContainerClassName="px-3">
            <Sections builder={builder} problems={problems} />
        </ScrollView>
    )
    return (
        <Sheet isOpen onClose={builder.onClose} maxHeightPercent={1} hideHandle testID="report-builder">
            <View className="min-h-0 flex-1">
                <View className="flex-row items-center justify-between px-3">
                    <PanelTitle builder={builder} />
                    <CloseButton onPress={builder.onClose} />
                </View>
                <ModeSwitch mode={mode} onChange={setMode} />
                <ModeBody mode={mode} design={design} preview={<ReportBody model={preview} />} />
                <Footer builder={builder} onSaveAs={onSaveAs} />
            </View>
        </Sheet>
    )
}

function BuilderSurface(props: SurfaceProps) {
    const breakpoint = useBreakpoint()
    if (breakpoint === 'mobile') return <SheetSurface {...props} />
    return <DrawerSurface {...props} breakpoint={breakpoint} />
}

export function ReportBuilderPanel({ builder, preview }: { builder: ReportBuilder; preview: ReportViewModel }) {
    // A dirty builder asks before its route is left; a save in flight or done does not.
    const leave = useLeaveGuard(builder.isDirty && !builder.isSaving && !builder.isSaved)
    // The Save as prompt is a modal toggle: local UI state.
    const [isSavingAs, setSavingAs] = useState(false)
    const problems = [...builder.saveProblems, ...preview.problems]
    return (
        <>
            <BuilderSurface builder={builder} preview={preview} problems={problems} onSaveAs={() => setSavingAs(true)} />
            <ConfirmDialog isOpen={builder.isConfirmingDiscard} onClose={builder.onKeepEditing} onConfirm={builder.onDiscard} title={DISCARD.title} message={DISCARD.message} confirmLabel="Discard" cancelLabel="Keep editing" isDestructive requiresServer={false} />
            <ConfirmDialog isOpen={leave.isConfirming} onClose={leave.cancel} onConfirm={leave.discard} title={DISCARD.title} message={DISCARD.message} confirmLabel="Discard" cancelLabel="Keep editing" isDestructive requiresServer={false} />
            <PromptDialog
                isOpen={isSavingAs}
                onClose={() => setSavingAs(false)}
                onSubmit={title => {
                    setSavingAs(false)
                    builder.onSaveAs(title)
                }}
                title="Save as"
                placeholder="Report title"
                confirmLabel="Save copy"
                defaultValue={copyTitle(builder.title)}
                maxLength={200}
                required
            />
        </>
    )
}
```

- [ ] **Step 10: Write the hub**

`core/reports/hooks/useReportsHub.ts`:

```ts
import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useAuth } from '@tinycld/core/lib/auth'
import { log } from '@tinycld/core/lib/logger'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { usePackages } from '@tinycld/core/lib/packages/use-packages'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { recoverLiveQueries } from '@tinycld/core/lib/recover-live-queries'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'
import type { Reports } from '@tinycld/core/types/pbSchema'
import { router, useLocalSearchParams } from 'expo-router'
import { materialize } from 'pbtsdb'
import { type ReportAccess, reportAccess, rolesByReport, type ShareRole } from '../lib/access'
import { useReportUIStore } from '../stores/report-ui-store'
import { useReportCatalog } from './useReportCatalog'

export const HUB_TABS = [
    { id: 'mine', label: 'Mine' },
    { id: 'shared', label: 'Shared with me' },
    { id: 'published', label: 'Published' },
] as const
export type HubTab = (typeof HUB_TABS)[number]['id']

export interface HubRow {
    report: Reports
    access: ReportAccess
}

const PACKAGE_SLUG = /^[a-z][a-z0-9-]*$/

function tabOf(value: unknown): HubTab {
    return HUB_TABS.find(tab => tab.id === value)?.id ?? 'mine'
}

function hubState(query: { isReady: boolean; isError: boolean }): 'loading' | 'error' | 'ready' {
    if (query.isError) return 'error'
    return query.isReady ? 'ready' : 'loading'
}

// The hub's three lists. Only the open tab's query runs; each is a pbtsdb live query,
// so a report shared, published or deleted elsewhere appears or leaves at once.
export function useReportsHub() {
    const search = useLocalSearchParams<{ tab?: string; package?: string }>()
    const tab = tabOf(search.tab)
    const packageSlug = typeof search.package === 'string' && PACKAGE_SLUG.test(search.package) ? search.package : null
    const { user } = useAuth()
    const role = useCurrentRole()
    const orgHref = useOrgHref()
    const catalog = useReportCatalog()
    const packages = usePackages()
    const startVisit = useReportUIStore(s => s.startVisit)
    const [reportsCollection, sharesCollection] = useStore('reports', 'report_shares')

    const mine = useMyLiveQuery(
        (q, { userId }) =>
            tab === 'mine'
                ? q
                      .from({ report: reportsCollection })
                      .where(({ report }) => eq(report.owner, userId))
                      .orderBy(({ report }) => report.updated, 'desc')
                : null,
        { queryKey: ['reports.hub.mine', tab] }
    )
    const shared = useMyLiveQuery(
        (q, { userId }) =>
            tab === 'shared'
                ? q
                      .from({ share: sharesCollection })
                      .where(({ share }) => eq(share.user, userId))
                      .select(({ share }) => ({
                          role: share.role,
                          report: materialize(
                              q
                                  .from({ report: reportsCollection })
                                  .where(({ report }) => eq(report.id, share.report))
                                  .findOne()
                          ),
                      }))
                : null,
        { queryKey: ['reports.hub.shared', tab] }
    )
    const published = useLiveQuery({
        queryKey: ['reports.hub.published', tab],
        query: q =>
            tab === 'published'
                ? q
                      .from({ report: reportsCollection })
                      .where(({ report }) => eq(report.published, true))
                      .orderBy(({ report }) => report.title)
                : null,
    })
    const myShares = useMyLiveQuery(
        (q, { userId }) => (tab === 'published' ? q.from({ share: sharesCollection }).where(({ share }) => eq(share.user, userId)) : null),
        { queryKey: ['reports.hub.my-shares', tab] }
    )

    const accessOf = (report: Reports, roles: readonly ShareRole[]) =>
        reportAccess({ owner: report.owner, published: report.published, shippedKey: report.shipped_key, userId: user.id, roles, isAdmin: role.isAdmin })

    const sharedRows = (shared.data ?? []).flatMap(row => (row.report ? [{ report: row.report, role: row.role }] : []))
    // A report can reach a reader through a direct and a group share: one row each.
    const sharedRoles = rolesByReport(sharedRows.map(row => ({ report: row.report.id, role: row.role })))
    const sharedReports = [...new Map(sharedRows.map(row => [row.report.id, row.report])).values()]
    const publishedRoles = rolesByReport((myShares.data ?? []).map(share => ({ report: share.report, role: share.role })))

    const all: HubRow[] =
        tab === 'mine'
            ? (mine.data ?? []).map(report => ({ report, access: accessOf(report, []) }))
            : tab === 'shared'
              ? sharedReports.map(report => ({ report, access: accessOf(report, sharedRoles.get(report.id) ?? []) }))
              : (published.data ?? []).map(report => ({ report, access: accessOf(report, publishedRoles.get(report.id) ?? []) }))
    const prefix = packageSlug ? `${packageSlug}.` : null
    const rows = prefix ? all.filter(row => row.report.source.startsWith(prefix)) : all
    const active = tab === 'mine' ? mine : tab === 'shared' ? shared : published

    return {
        tab,
        setTab: (next: HubTab) => router.setParams({ tab: next === 'mine' ? undefined : next }),
        rows,
        state: hubState(active),
        retry: () => {
            void recoverLiveQueries({
                queries: [mine.collection, shared.collection, published.collection, myShares.collection],
                sources: [reportsCollection, sharesCollection],
                onError: error => log.error('reports.hub', error),
            })
        },
        sources: prefix ? catalog.sources.filter(source => source.key.startsWith(prefix)) : catalog.sources,
        isCatalogReady: catalog.isReady,
        sourceLabel: (key: string) => catalog.byKey.get(key)?.label ?? key,
        packageSlug,
        packageLabel: packages.find(p => p.slug === packageSlug)?.name ?? packageSlug ?? '',
        onShowAll: () => router.setParams({ package: undefined }),
        onOpen: (report: Reports) => {
            startVisit(report.id)
            router.push(orgHref('reports/[id]', { id: report.id }))
        },
        onNew: (sourceKey: string) => router.push(orgHref('reports/new', { source: sourceKey })),
    }
}

export type ReportsHubModel = ReturnType<typeof useReportsHub>
```

`core/reports/components/hub/NewReportDialog.tsx`:

```tsx
import { Dialog } from '@tinycld/core/ui/dialog'
import { Pressable, Text, View } from 'react-native'
import type { CatalogSource } from '../../lib/types'
import { REPORT_TAP_TARGET } from '../ReportHeader'

function NoSources({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text testID="reports-new-none" className="text-sm text-muted-foreground">
            No report sources are installed.
        </Text>
    )
}

interface NewReportDialogProps {
    isOpen: boolean
    sources: CatalogSource[]
    onPick(sourceKey: string): void
    onClose(): void
}

export function NewReportDialog({ isOpen, sources, onPick, onClose }: NewReportDialogProps) {
    const rows = sources.map(source => (
        <Pressable key={source.key} testID={`reports-new-source-${source.key}`} accessibilityRole="button" onPress={() => onPick(source.key)} className="justify-center rounded-md px-2" style={{ minHeight: REPORT_TAP_TARGET }}>
            <Text className="text-sm font-medium text-foreground">{source.label}</Text>
        </Pressable>
    ))
    return (
        <Dialog isOpen={isOpen} onClose={onClose} title="Choose what to report on" testID="reports-new-dialog">
            <Dialog.Body>
                <View className="gap-1">
                    {rows}
                    <NoSources isVisible={sources.length === 0} />
                </View>
            </Dialog.Body>
        </Dialog>
    )
}
```

`core/reports/components/hub/HubList.tsx`:

```tsx
import { useAuth } from '@tinycld/core/lib/auth'
import { formatTimeAgo } from '@tinycld/core/lib/format-utils'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useRecord } from '@tinycld/core/lib/use-record'
import { Pressable, Text, View } from 'react-native'
import type { HubRow, ReportsHubModel } from '../../hooks/useReportsHub'
import { parseBroken } from '../../lib/spec'
import { recentKey, useReportRecentStore } from '../../stores/report-recent-store'

const ROW_MIN_HEIGHT = 44

function OwnerName({ userId }: { userId: string }) {
    const [users] = useStore('users')
    const { record } = useRecord(users, userId)
    return <Text numberOfLines={1} className="text-sm text-muted-foreground">{record?.name ?? ''}</Text>
}

function BrokenBadge({ isVisible, reportId }: { isVisible: boolean; reportId: string }) {
    if (!isVisible) return null
    return (
        <View testID={`reports-hub-broken-${reportId}`} className="self-start rounded-full bg-danger-soft px-2">
            <Text className="text-xs font-medium text-danger">Broken</Text>
        </View>
    )
}

function useLastRun(reportId: string): string {
    const { user } = useAuth()
    const at = useReportRecentStore(s => s.lastRun[recentKey(user.id, reportId)])
    return at ? formatTimeAgo(at) : 'Never'
}

interface RowProps {
    row: HubRow
    hub: ReportsHubModel
    isPhone: boolean
    showOwner: boolean
    menu: React.ReactNode
}

function TitleBlock({ row }: { row: HubRow }) {
    const { report } = row
    return (
        <View className="min-w-0 flex-1 gap-0.5">
            <Text testID="reports-hub-row-title" numberOfLines={1} className="text-sm font-semibold text-foreground">
                {report.title}
            </Text>
            <Text numberOfLines={1} className="text-xs text-muted-foreground">{report.description}</Text>
            <BrokenBadge isVisible={parseBroken(report.broken) !== null} reportId={report.id} />
        </View>
    )
}

function WideRow({ row, hub, showOwner, menu }: RowProps) {
    const lastRun = useLastRun(row.report.id)
    return (
        <Pressable testID={`reports-hub-row-${row.report.id}`} accessibilityRole="link" onPress={() => hub.onOpen(row.report)} className="flex-row items-center gap-3 border-b border-border px-4 py-2" style={{ minHeight: ROW_MIN_HEIGHT }}>
            <TitleBlock row={row} />
            <Text numberOfLines={1} className="w-40 text-sm text-muted-foreground">{hub.sourceLabel(row.report.source)}</Text>
            <OwnerCell isVisible={showOwner} userId={row.report.owner} />
            <Text numberOfLines={1} className="w-28 text-sm text-muted-foreground">{lastRun}</Text>
            {menu}
        </Pressable>
    )
}

function OwnerCell({ isVisible, userId }: { isVisible: boolean; userId: string }) {
    if (!isVisible) return null
    return (
        <View className="w-36">
            <OwnerName userId={userId} />
        </View>
    )
}

function PhoneCard({ row, hub, menu }: RowProps) {
    const lastRun = useLastRun(row.report.id)
    return (
        <Pressable testID={`reports-hub-row-${row.report.id}`} accessibilityRole="link" onPress={() => hub.onOpen(row.report)} className="flex-row items-start gap-2 rounded-lg border border-border bg-card p-3">
            <View className="min-w-0 flex-1 gap-1">
                <TitleBlock row={row} />
                <View className="flex-row gap-1">
                    <Text className="text-xs text-muted-foreground">{`${hub.sourceLabel(row.report.source)} ·`}</Text>
                    <OwnerName userId={row.report.owner} />
                </View>
                <Text className="text-xs text-muted-foreground">{lastRun}</Text>
            </View>
            {menu}
        </Pressable>
    )
}

function HeadingRow({ isVisible, showOwner }: { isVisible: boolean; showOwner: boolean }) {
    if (!isVisible) return null
    const owner = showOwner ? <Text className="w-36 text-xs font-semibold uppercase text-muted-foreground">Owner</Text> : null
    return (
        <View className="flex-row gap-3 border-b border-border px-4 py-2">
            <Text className="flex-1 text-xs font-semibold uppercase text-muted-foreground">Title</Text>
            <Text className="w-40 text-xs font-semibold uppercase text-muted-foreground">Source</Text>
            {owner}
            <Text className="w-28 text-xs font-semibold uppercase text-muted-foreground">Last run</Text>
            <View style={{ width: ROW_MIN_HEIGHT }} />
        </View>
    )
}

interface HubListProps {
    hub: ReportsHubModel
    isPhone: boolean
    showOwner: boolean
    menuFor(row: HubRow): React.ReactNode
}

export function HubList({ hub, isPhone, showOwner, menuFor }: HubListProps) {
    const Row = isPhone ? PhoneCard : WideRow
    const items = hub.rows.map(row => (
        <Row key={row.report.id} row={row} hub={hub} isPhone={isPhone} showOwner={showOwner} menu={menuFor(row)} />
    ))
    const frame = isPhone ? 'gap-2' : 'overflow-hidden rounded-lg border border-border bg-card'
    return (
        <View testID="reports-hub-list" className={frame}>
            <HeadingRow isVisible={!isPhone} showOwner={showOwner} />
            {items}
        </View>
    )
}
```

`core/reports/components/hub/ReportsHub.tsx`:

```tsx
import { LoadingState } from '@tinycld/core/components/LoadingState'
import { useBreakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { HUB_TABS, type HubRow, type HubTab, type ReportsHubModel, useReportsHub } from '../../hooks/useReportsHub'
import { REPORT_TAP_TARGET } from '../ReportHeader'
import { MessageBody } from '../ReportStates'
import { HubList } from './HubList'
import { NewReportDialog } from './NewReportDialog'

const EMPTY: Record<HubTab, string> = {
    mine: 'You have no reports yet. Use New report to make one.',
    shared: 'No one has shared a report with you.',
    published: 'No reports are published.',
}

function Tabs({ tab, onChange }: { tab: HubTab; onChange: (tab: HubTab) => void }) {
    const pills = HUB_TABS.map(item => (
        <Pressable
            key={item.id}
            testID={`reports-hub-tab-${item.id}`}
            accessibilityRole="tab"
            aria-selected={item.id === tab}
            onPress={() => onChange(item.id)}
            className={`justify-center rounded-full border px-3 ${item.id === tab ? 'border-primary bg-primary' : 'border-border'}`}
            style={{ minHeight: REPORT_TAP_TARGET }}
        >
            <Text className={`text-sm font-medium ${item.id === tab ? 'text-primary-foreground' : 'text-foreground'}`}>{item.label}</Text>
        </Pressable>
    ))
    return (
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerClassName="gap-2" accessibilityRole="tablist">
            {pills}
        </ScrollView>
    )
}

function Loading({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View testID="reports-hub-loading">
            <LoadingState />
        </View>
    )
}

interface ReportsHubViewProps {
    hub: ReportsHubModel
    menuFor(row: HubRow): React.ReactNode
    banner: React.ReactNode
}

export function ReportsHubView({ hub, menuFor, banner }: ReportsHubViewProps) {
    const breakpoint = useBreakpoint()
    // The New report dialog is a modal toggle: local UI state.
    const [isChoosing, setChoosing] = useState(false)
    const isEmpty = hub.state === 'ready' && hub.rows.length === 0
    return (
        <ScrollView testID="reports-hub" className="flex-1 bg-background" contentContainerClassName="w-full gap-4 p-4">
            <View className="flex-row items-center justify-between gap-2">
                <Text className="text-2xl font-semibold text-foreground">Reports</Text>
                <Button testID="reports-hub-new" onPress={() => setChoosing(true)} isDisabled={!hub.isCatalogReady}>
                    <ButtonText>New report</ButtonText>
                </Button>
            </View>
            {banner}
            <Tabs tab={hub.tab} onChange={hub.setTab} />
            <Loading isVisible={hub.state === 'loading'} />
            <MessageBody isVisible={hub.state === 'error'} testID="reports-hub-error" message="Could not load reports" action={{ label: 'Retry', testID: 'reports-hub-retry', onPress: hub.retry }} />
            <MessageBody isVisible={isEmpty} testID="reports-hub-empty" message={EMPTY[hub.tab]} />
            <HubList hub={hub} isPhone={breakpoint === 'mobile'} showOwner={breakpoint === 'desktop'} menuFor={menuFor} />
            <NewReportDialog
                isOpen={isChoosing}
                sources={hub.sources}
                onClose={() => setChoosing(false)}
                onPick={key => {
                    setChoosing(false)
                    hub.onNew(key)
                }}
            />
        </ScrollView>
    )
}

export function ReportsHub() {
    const hub = useReportsHub()
    // Task 18 adds each row's menu and the package banner.
    return <ReportsHubView hub={hub} menuFor={() => null} banner={null} />
}
```

- [ ] **Step 11: Write the screens**

`core/reports/screens/ReportsHubScreen.tsx`:

```tsx
import { ReportsHub } from '../components/hub/ReportsHub'

export function ReportsHubScreen() {
    return <ReportsHub />
}
```

`core/reports/screens/ReportScreen.tsx`:

```tsx
import { router } from 'expo-router'
import { useLocalSearchParams } from 'expo-router'
import { View } from 'react-native'
import { ReportBuilderPanel } from '../components/builder/ReportBuilderPanel'
import { ReportHeader } from '../components/ReportHeader'
import { LoadingBody, MessageBody } from '../components/ReportStates'
import { ReportView } from '../components/ReportView'
import { useBackToReports } from '../hooks/useBackToReports'
import { useReportBuilder } from '../hooks/useReportBuilder'
import { useReportCatalog } from '../hooks/useReportCatalog'
import { useReportRecord } from '../hooks/useReportRecord'
import { useReportView } from '../hooks/useReportView'
import type { Catalog } from '../lib/builder/columns'
import type { BrokenMark } from '../lib/spec'
import type { CatalogSource } from '../lib/types'

type Record = ReturnType<typeof useReportRecord>

// A stored spec this app cannot read (a newer spec version) cannot run here.
const UNREADABLE: BrokenMark = {
    status: 400,
    problems: { spec: { code: 'unreadable', message: 'This report was saved by a newer version of the app.' } },
}

interface FrameProps {
    title: string
    state: 'loading' | 'error' | 'no-access' | 'unknown-source'
    onBack: () => void
    onRetry?: () => void
}

/** The report screen before its record or catalog is ready, or when it cannot be shown. */
export function ReportFrame({ title, state, onBack, onRetry }: FrameProps) {
    const retry = onRetry ? { label: 'Retry', testID: 'report-retry', onPress: onRetry } : null
    return (
        <View testID="report-screen" className="flex-1 bg-background">
            <ReportHeader title={title} onBack={onBack} toolbar={null} />
            <LoadingBody isVisible={state === 'loading'} />
            <MessageBody isVisible={state === 'error'} testID="report-error" message="Could not load this report" action={retry} />
            <MessageBody isVisible={state === 'no-access'} testID="report-no-access" message="You no longer have access to this report" />
            <MessageBody isVisible={state === 'unknown-source'} testID="report-unknown-source" message="This kind of report is not installed" />
        </View>
    )
}

function SavedReport({ record, onBack }: { record: Record; onBack: () => void }) {
    const report = record.record
    const model = useReportView({
        reportKey: report?.id ?? '',
        title: report?.title ?? '',
        defs: record.spec?.params ?? [],
        source: report && record.spec ? { kind: 'saved', reportId: report.id } : null,
        access: record.access,
        broken: record.broken ?? (record.spec ? null : UNREADABLE),
        isEditing: false,
        onBack,
        onEdit: () => router.setParams({ edit: '1' }),
    })
    return <ReportView model={model} />
}

function EditingBody({ record, onBack }: { record: Record; onBack: () => void }) {
    const catalog = useReportCatalog()
    const report = record.record
    const spec = record.spec
    const base = spec ? catalog.byKey.get(spec.source) : undefined
    if (!catalog.isReady) return <ReportFrame title={report?.title ?? ''} state={catalog.isError ? 'error' : 'loading'} onBack={onBack} onRetry={catalog.retry} />
    // A report whose source is gone cannot be designed; its view names what is missing.
    if (!report || !spec || !base) return <SavedReport record={record} onBack={onBack} />
    return <EditingReport record={record} catalog={catalog.byKey} base={base} onBack={onBack} />
}

interface EditingReportProps {
    record: Record
    catalog: Catalog
    base: CatalogSource
    onBack: () => void
}

function EditingReport({ record, catalog, base, onBack }: EditingReportProps) {
    const builder = useReportBuilder({ mode: 'edit', catalog, base, record: record.record ?? null, spec: record.spec })
    const model = useReportView({
        reportKey: record.record?.id ?? '',
        title: builder.title || (record.record?.title ?? ''),
        defs: builder.previewSpec?.params ?? [],
        source: builder.previewSpec ? { kind: 'draft', spec: builder.previewSpec } : null,
        access: record.access,
        broken: null,
        isEditing: true,
        onBack,
    })
    return <ReportView model={model} side={<ReportBuilderPanel builder={builder} preview={model} />} />
}

export function ReportScreen({ reportId }: { reportId: string }) {
    const record = useReportRecord(reportId)
    const { edit } = useLocalSearchParams<{ edit?: string }>()
    const onBack = useBackToReports()
    if (record.state === 'loading') return <ReportFrame title="" state="loading" onBack={onBack} />
    if (record.state === 'error') return <ReportFrame title="" state="error" onBack={onBack} onRetry={record.retry} />
    if (record.state === 'missing') return <ReportFrame title="" state="no-access" onBack={onBack} />
    if (edit === '1' && record.access.canEdit) return <EditingBody record={record} onBack={onBack} />
    return <SavedReport record={record} onBack={onBack} />
}
```

`core/reports/screens/NewReportScreen.tsx`:

```tsx
import { useLocalSearchParams } from 'expo-router'
import { ReportBuilderPanel } from '../components/builder/ReportBuilderPanel'
import { ReportView } from '../components/ReportView'
import { useBackToReports } from '../hooks/useBackToReports'
import { useReportBuilder } from '../hooks/useReportBuilder'
import { useReportCatalog } from '../hooks/useReportCatalog'
import { useReportView } from '../hooks/useReportView'
import type { ReportAccess } from '../lib/access'
import type { Catalog } from '../lib/builder/columns'
import type { CatalogSource } from '../lib/types'
import { NEW_REPORT_KEY } from '../stores/report-ui-store'
import { ReportFrame } from './ReportScreen'

// A draft the reader is making: theirs to design and save, not yet shareable.
const DRAFT_ACCESS: ReportAccess = {
    isOwner: true,
    canEdit: true,
    canShare: false,
    canPublish: false,
    canDelete: false,
    isShipped: false,
}

function NewReport({ catalog, base, onBack }: { catalog: Catalog; base: CatalogSource; onBack: () => void }) {
    const builder = useReportBuilder({ mode: 'new', catalog, base, record: null, spec: null })
    const model = useReportView({
        reportKey: NEW_REPORT_KEY,
        title: builder.title || 'New report',
        defs: builder.previewSpec?.params ?? [],
        source: builder.previewSpec ? { kind: 'draft', spec: builder.previewSpec } : null,
        access: DRAFT_ACCESS,
        broken: null,
        isEditing: true,
        onBack,
    })
    return <ReportView model={model} side={<ReportBuilderPanel builder={builder} preview={model} />} />
}

export function NewReportScreen() {
    const { source } = useLocalSearchParams<{ source?: string }>()
    const catalog = useReportCatalog()
    const onBack = useBackToReports()
    const base = catalog.byKey.get(source ?? '')
    if (!catalog.isReady) return <ReportFrame title="New report" state={catalog.isError ? 'error' : 'loading'} onBack={onBack} onRetry={catalog.retry} />
    if (!base) return <ReportFrame title="New report" state="unknown-source" onBack={onBack} />
    return <NewReport catalog={catalog.byKey} base={base} onBack={onBack} />
}
```

`catalog.byKey` is a `ReadonlyMap<string, CatalogSource>`, which is a `Catalog`, and keeps one identity while the catalog does (Task 10), so `builderSchema` is built once.

- [ ] **Step 12: Format, lint, typecheck, unit tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome format --write core/reports app/a/\(app\)/reports app/a/\(app\)/_layout.tsx core/components/workspace tests/e2e/reports-helpers.ts tests/e2e/reports-create.spec.ts && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Expected: Biome, tsc and vitest pass. Fix any type error at its source (for example a pbtsdb row type that does not reach `Reports`: derive `HubRow['report']` from the `mine` query's row type instead).

- [ ] **Step 13: Run the spec**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-create.spec.ts`
Expected: 3 passed.

- [ ] **Step 14: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add app/a/\(app\)/reports app/a/\(app\)/_layout.tsx core/components/workspace/PackageTabs.tsx core/components/workspace/PackageRail.tsx core/components/workspace/MoreDrawer.tsx core/reports tests/e2e/reports-helpers.ts tests/e2e/reports-create.spec.ts
git commit -m "feat(core): reports area, hub, report view and builder columns"
```

---

### Task 14: Builder filter: conditions, all/any/none, related rows, asked values

**Files:**
- Modify: `core/reports/lib/builder/filter.ts` (tree edits: `FilterPath`, `updateFilterItem`, `removeFilterItem`, `addFilterItem`, `setMatchAt`)
- Test: `core/reports/lib/builder/__tests__/filter-tree.test.ts`
- Create: `core/reports/components/builder/FilterSection.tsx`
- Modify: `core/reports/components/builder/ReportBuilderPanel.tsx` (`Sections`: add `FilterSection`)
- Create: `tests/e2e/reports-filter.spec.ts`

**Interfaces:**
- Consumes: Task 11 filter functions (`opsFor`, `opLabel`, `needsValue`, `askParamFor`, `emptyCondition`, `emptyGroup`), `describePath`, `pathLabel`, `sourceAtLinks`, `nextColumnId` (Task 11); `ReportBuilder.setFilter`, `setParams` (Task 13); `ColumnPicker`, `BuilderSection` (Task 13); core `PickMenu`, `PlainInput`, `DateField`, `Switch`.
- Produces: `FilterPath` (= `readonly number[]`), `updateFilterItem(root, path, fn)`, `removeFilterItem(root, path)`, `addFilterItem(root, path, item)`, `setMatchAt(root, path, match)`; test ids `report-builder-filter[-<path>]`, `…-match` (items `…-match-all|any|none`), `…-add-condition`, `…-add-related` (items `…-add-related-<link>`), `…-add-group`, a condition's `…-field`, `…-op` (items `…-op-<op>`), `…-value`, `…-high`, `…-value-<option>` (choice), `…-ask`, `…-default` (items `…-default-<preset>|none`), `…-remove`, a related item's `…-has` (items `…-has-yes|no`).

- [ ] **Step 1: Write the failing e2e spec**

`tests/e2e/reports-filter.spec.ts`:

```ts
import { expect, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub } from './helpers'
import { addSale, addTeam, openReportStub, openReportsHub, SALES_SOURCE, startReport, TEAMS_SOURCE, uniqueName } from './reports-helpers'

/**
 * The builder's filter against the salesstub package. Teams are shared by every
 * user, so the first test scopes its teams by a name stem; a fresh invited member
 * owns the sales, so related-row totals count only this test's sales.
 */
test.describe('filtering a report', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('has and has no related rows filter teams by their sales', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptfilter')
        try {
            const stem = uniqueName('F')
            const big = `${stem} big`
            const small = `${stem} small`
            await openReportStub(p)
            await addTeam(p, big)
            await addTeam(p, small)
            await addSale(p, { team: big, customer: 'Large', amount: 60000, cost: 1000, day: '2026-01-05' })
            await addSale(p, { team: small, customer: 'Little', amount: 1000, cost: 100, day: '2026-01-06' })

            await openReportsHub(p)
            await startReport(p, TEAMS_SOURCE)
            await p.getByTestId('report-builder-section-filter').click()
            // Name starts with this test's stem.
            await p.getByTestId('report-builder-filter-add-condition').click()
            await p.getByTestId('report-builder-picker').getByTestId('report-builder-pick-field-name').click()
            await p.getByTestId('report-builder-filter-0-op').click()
            await p.getByTestId('report-builder-filter-0-op-startsWith').click()
            await p.getByTestId('report-builder-filter-0-value').fill(stem)
            // Has related sales where the amount is over 500.00.
            await p.getByTestId('report-builder-filter-add-related').click()
            await p.getByTestId('report-builder-filter-add-related-sales').click()
            await p.getByTestId('report-builder-filter-1-add-condition').click()
            await p.getByTestId('report-builder-picker').getByTestId('report-builder-pick-field-amount').click()
            await p.getByTestId('report-builder-filter-1-0-op').click()
            await p.getByTestId('report-builder-filter-1-0-op-gt').click()
            await p.getByTestId('report-builder-filter-1-0-value').fill('500')

            const table = p.getByTestId('report-table')
            await expect(table.getByText(big, { exact: true })).toBeVisible()
            await expect(table.getByText(small, { exact: true })).toHaveCount(0)

            await p.getByTestId('report-builder-filter-1-has').click()
            await p.getByTestId('report-builder-filter-1-has-no').click()
            await expect(table.getByText(small, { exact: true })).toBeVisible()
            await expect(table.getByText(big, { exact: true })).toHaveCount(0)
        } finally {
            await close()
        }
    })

    test('any of: a date on or after a day, or a status', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptany')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, { team, customer: 'January open', amount: 100, cost: 0, day: '2026-01-10' })
            await addSale(p, { team, customer: 'March open', amount: 200, cost: 0, day: '2026-03-05' })
            await addSale(p, { team, customer: 'January paid', amount: 300, cost: 0, day: '2026-01-20', status: 'paid' })

            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await p.getByTestId('report-builder-section-filter').click()
            await p.getByTestId('report-builder-filter-match').click()
            await p.getByTestId('report-builder-filter-match-any').click()
            await p.getByTestId('report-builder-filter-add-condition').click()
            await p.getByTestId('report-builder-picker').getByTestId('report-builder-pick-field-day').click()
            await p.getByTestId('report-builder-filter-0-op').click()
            await p.getByTestId('report-builder-filter-0-op-gte').click()
            await p.getByTestId('report-builder-filter-0-value').fill('2026-03-01')
            await p.getByTestId('report-builder-filter-add-condition').click()
            await p.getByTestId('report-builder-picker').getByTestId('report-builder-pick-field-status').click()
            await p.getByTestId('report-builder-filter-1-value').click()
            await p.getByTestId('report-builder-filter-1-value-paid').click()

            const table = p.getByTestId('report-table')
            await expect(table.getByText('March open', { exact: true })).toBeVisible()
            await expect(table.getByText('January paid', { exact: true })).toBeVisible()
            await expect(table.getByText('January open', { exact: true })).toHaveCount(0)
        } finally {
            await close()
        }
    })
})
```

- [ ] **Step 2: Write the failing unit test for the tree edits**

`core/reports/lib/builder/__tests__/filter-tree.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { addFilterItem, emptyCondition, emptyGroup, type GroupDraft, removeFilterItem, setMatchAt, updateFilterItem } from '../filter'

const condition = (field: string) => emptyCondition([{ field }])

const tree: GroupDraft = {
    kind: 'group',
    match: 'all',
    items: [
        condition('customer'),
        { kind: 'related', has: true, link: [{ link: 'sales' }], where: { kind: 'group', match: 'all', items: [condition('amount')] } },
        { kind: 'group', match: 'any', items: [condition('status')] },
    ],
}

describe('filter tree edits', () => {
    it('adds to the root, a related item’s conditions and a nested group', () => {
        expect(addFilterItem(emptyGroup(), [], condition('a')).items).toHaveLength(1)
        const related = addFilterItem(tree, [1], condition('cost')).items[1]
        expect(related?.kind === 'related' && related.where.items.map(i => (i.kind === 'condition' ? i.path[0]?.field : ''))).toEqual(['amount', 'cost'])
        const group = addFilterItem(tree, [2], condition('day')).items[2]
        expect(group?.kind === 'group' && group.items).toHaveLength(2)
    })

    it('updates an item by its path, inside a related item too', () => {
        const next = updateFilterItem(tree, [1, 0], item => (item.kind === 'condition' ? { ...item, op: 'gt', value: '5' } : item))
        const related = next.items[1]
        expect(related?.kind === 'related' && related.where.items[0]).toMatchObject({ op: 'gt', value: '5' })
        expect(tree.items[1]).not.toBe(related)
    })

    it('removes an item and never the root', () => {
        expect(removeFilterItem(tree, [0]).items).toHaveLength(2)
        expect(removeFilterItem(tree, [2, 0]).items[2]).toMatchObject({ items: [] })
        expect(removeFilterItem(tree, [])).toBe(tree)
    })

    it('sets how a group or a related item’s conditions are joined', () => {
        expect(setMatchAt(tree, [], 'none').match).toBe('none')
        const related = setMatchAt(tree, [1], 'any').items[1]
        expect(related?.kind === 'related' && related.where.match).toBe('any')
    })
})
```

- [ ] **Step 3: Run both to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run reports/lib/builder/__tests__/filter-tree.test.ts`
Expected: FAIL: `addFilterItem` is not exported.

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-filter.spec.ts`
Expected: FAIL: `report-builder-section-filter` is not found.

- [ ] **Step 4: Add the tree edits**

Append to `core/reports/lib/builder/filter.ts`:

```ts
/** An item's place in the tree: indexes from the root; a related item's children are its conditions. */
export type FilterPath = readonly number[]

function childItems(item: FilterItem): FilterItem[] | null {
    if (item.kind === 'group') return item.items
    if (item.kind === 'related') return item.where.items
    return null
}

function withChildItems(item: FilterItem, items: FilterItem[]): FilterItem {
    if (item.kind === 'group') return { ...item, items }
    if (item.kind === 'related') return { ...item, where: { ...item.where, items } }
    return item
}

// Replaces the item at `path` with fn's answer; null removes it.
function mapAt(item: FilterItem, path: FilterPath, fn: (item: FilterItem) => FilterItem | null): FilterItem | null {
    if (path.length === 0) return fn(item)
    const [head, ...rest] = path
    const items = childItems(item)
    const child = head === undefined ? undefined : items?.[head]
    if (!items || child === undefined) return item
    const next = mapAt(child, rest, fn)
    return withChildItems(item, next === null ? items.filter((_, i) => i !== head) : items.map((c, i) => (i === head ? next : c)))
}

function asGroup(item: FilterItem | null, fallback: GroupDraft): GroupDraft {
    return item?.kind === 'group' ? item : fallback
}

export function updateFilterItem(root: GroupDraft, path: FilterPath, fn: (item: FilterItem) => FilterItem): GroupDraft {
    return asGroup(mapAt(root, path, fn), root)
}

export function removeFilterItem(root: GroupDraft, path: FilterPath): GroupDraft {
    return path.length === 0 ? root : asGroup(mapAt(root, path, () => null), root)
}

/** Adds an item to the group, or the related item's conditions, at `path`. */
export function addFilterItem(root: GroupDraft, path: FilterPath, added: FilterItem): GroupDraft {
    return asGroup(mapAt(root, path, item => withChildItems(item, [...(childItems(item) ?? []), added])), root)
}

export function setMatchAt(root: GroupDraft, path: FilterPath, match: GroupDraft['match']): GroupDraft {
    return asGroup(
        mapAt(root, path, item => {
            if (item.kind === 'group') return { ...item, match }
            if (item.kind === 'related') return { ...item, where: { ...item.where, match } }
            return item
        }),
        root
    )
}
```

- [ ] **Step 5: Write the filter section**

`core/reports/components/builder/FilterSection.tsx`:

```tsx
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { useWatch } from '@tinycld/core/ui/form'
import { DateField } from '@tinycld/core/ui/forms/fields/DateField'
import { type PickItem, PickMenu } from '@tinycld/core/ui/pick-menu'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { Switch } from '@tinycld/core/ui/switch'
import { Trash2 } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import { type Catalog, describePath, nextColumnId, type PathInfo, pathLabel, sourceAtLinks } from '../../lib/builder/columns'
import {
    addFilterItem,
    askParamFor,
    type ConditionDraft,
    emptyCondition,
    emptyGroup,
    type FilterItem,
    type FilterPath,
    type GroupDraft,
    needsValue,
    opLabel,
    opsFor,
    type RelatedDraft,
    removeFilterItem,
    setMatchAt,
    updateFilterItem,
} from '../../lib/builder/filter'
import { isDateParam, PRESET_LABELS, presetsFor } from '../../lib/params'
import type { ParamDef, SpecHop } from '../../lib/spec'
import type { Problem } from '../../lib/types'
import { REPORT_TAP_TARGET } from '../ReportHeader'
import { BuilderSection } from './BuilderSection'
import { ColumnPicker } from './ColumnPicker'

const MATCHES = [
    { value: 'all', label: 'All of' },
    { value: 'any', label: 'Any of' },
    { value: 'none', label: 'None of' },
] as const

function filterTestId(path: FilterPath): string {
    return path.length === 0 ? 'report-builder-filter' : `report-builder-filter-${path.join('-')}`
}

// What every part of the filter editor needs: the tree, the asked params, and how
// to change them. Every change replaces the whole filter in the form (one edit).
interface FilterContext {
    builder: ReportBuilder
    catalog: Catalog
    root: GroupDraft
    params: ParamDef[]
    change(next: GroupDraft): void
}

function RemoveButton({ label, testID, onPress }: { label: string; testID: string; onPress: () => void }) {
    const color = useThemeColor('muted-foreground')
    return (
        <Pressable testID={testID} accessibilityRole="button" accessibilityLabel={label} onPress={onPress} className="items-center justify-center" style={{ width: REPORT_TAP_TARGET, height: REPORT_TAP_TARGET }}>
            <Trash2 size={16} color={color} />
        </Pressable>
    )
}

function TextValue({ value, testID, label, isNumeric, onChange }: { value: string; testID: string; label: string; isNumeric: boolean; onChange: (text: string) => void }) {
    return (
        <PlainInput
            value={value}
            onChangeText={onChange}
            accessibilityLabel={label}
            keyboardType={isNumeric ? 'decimal-pad' : 'default'}
            testID={testID}
            className="min-w-24 flex-1 rounded-md border border-border px-2 py-2 text-sm text-foreground"
        />
    )
}

interface ValueProps {
    ctx: FilterContext
    item: ConditionDraft
    path: FilterPath
    info: PathInfo
}

function setCondition(ctx: FilterContext, path: FilterPath, patch: Partial<ConditionDraft>) {
    ctx.change(updateFilterItem(ctx.root, path, item => (item.kind === 'condition' ? { ...item, ...patch } : item)))
}

function ChoiceValues({ ctx, item, path, info }: ValueProps) {
    const id = filterTestId(path)
    const options = info.field.options ?? []
    const toggle = (value: string) => {
        const values = item.values.includes(value) ? item.values.filter(v => v !== value) : [...item.values, value]
        setCondition(ctx, path, { values })
    }
    const chips = options.map(option => {
        const isOn = item.values.includes(option.value)
        return (
            <Pressable key={option.value} testID={`${id}-value-${option.value}`} accessibilityRole="checkbox" aria-checked={isOn} onPress={() => toggle(option.value)} className={`justify-center rounded-full border px-3 ${isOn ? 'border-primary bg-primary' : 'border-border'}`} style={{ minHeight: REPORT_TAP_TARGET }}>
                <Text className={`text-sm ${isOn ? 'text-primary-foreground' : 'text-foreground'}`}>{option.label}</Text>
            </Pressable>
        )
    })
    return <View className="flex-row flex-wrap gap-1.5">{chips}</View>
}

function ChoiceValue({ ctx, item, path, info }: ValueProps) {
    const options = info.field.options ?? []
    const items: PickItem[] = options.map(option => ({
        id: option.value,
        label: option.label,
        isSelected: option.value === item.value,
        onSelect: () => setCondition(ctx, path, { value: option.value }),
    }))
    const label = options.find(o => o.value === item.value)?.label ?? 'Choose'
    return <PickMenu label={label} purpose={`${info.field.label} value`} items={items} testID={`${filterTestId(path)}-value`} size="touch" />
}

function OneValue({ ctx, item, path, info, part }: ValueProps & { part: 'value' | 'high' }) {
    const testID = `${filterTestId(path)}-${part}`
    const label = part === 'high' ? `${info.field.label}, to` : `${info.field.label} value`
    const onChange = (text: string | null) => setCondition(ctx, path, { [part]: text ?? '' })
    const type = info.field.type
    if (type === 'date' || type === 'datetime') {
        return <DateField value={item[part] || null} onChange={onChange} withTime={false} accessibilityLabel={label} testID={testID} />
    }
    return <TextValue value={item[part]} testID={testID} label={label} isNumeric={type === 'number' || type === 'money' || type === 'percent'} onChange={onChange} />
}

function ConditionValue(props: ValueProps) {
    const { item, info } = props
    if (!needsValue(item.op) || item.param) return null
    if (info.field.type === 'select' && item.op === 'in') return <ChoiceValues {...props} />
    if (info.field.type === 'select') return <ChoiceValue {...props} />
    const high = item.op === 'between' ? <OneValue {...props} part="high" /> : null
    return (
        <View className="flex-row flex-wrap items-center gap-1.5">
            <OneValue {...props} part="value" />
            {high}
        </View>
    )
}

function DefaultPreset({ ctx, item, path }: ValueProps) {
    const param = ctx.params.find(p => p.id === item.param)
    if (!param || !isDateParam(param)) return null
    const current = typeof param.default === 'object' && param.default !== null && 'preset' in param.default ? String(param.default.preset) : ''
    const setDefault = (preset: string) =>
        ctx.builder.setParams(ctx.params.map(p => (p.id === param.id ? { ...p, default: preset ? { preset } : undefined } : p)))
    const items: PickItem[] = [
        { id: 'none', label: 'No default', isSelected: current === '', onSelect: () => setDefault('') },
        ...presetsFor(param).map(preset => ({ id: preset, label: PRESET_LABELS[preset], isSelected: preset === current, onSelect: () => setDefault(preset) })),
    ]
    const label = current ? `Default: ${items.find(i => i.id === current)?.label ?? current}` : 'No default'
    return <PickMenu label={label} purpose="Default period" items={items} testID={`${filterTestId(path)}-default`} />
}

// "Ask when run" turns the value into a param the reader sets from a chip (R16).
function AskToggle(props: ValueProps) {
    const { ctx, item, path, info } = props
    if (!askParamFor(info.field, item.op, 'probe')) return null
    const isAsking = item.param !== ''
    const onChange = (on: boolean) => {
        if (!on) {
            ctx.builder.setParams(ctx.params.filter(p => p.id !== item.param))
            setCondition(ctx, path, { param: '' })
            return
        }
        const hint = item.path[item.path.length - 1]?.field ?? 'value'
        const def = askParamFor(info.field, item.op, nextColumnId(ctx.params.map(p => p.id), hint))
        if (!def) return
        ctx.builder.setParams([...ctx.params, def])
        setCondition(ctx, path, { param: def.id })
    }
    return (
        <View className="flex-row flex-wrap items-center gap-2">
            <Switch value={isAsking} onValueChange={onChange} accessibilityLabel="Ask when the report runs" testID={`${filterTestId(path)}-ask`} />
            <Text className="text-sm text-foreground">Ask when the report runs</Text>
            <DefaultPreset {...props} />
        </View>
    )
}

function ConditionRow({ ctx, item, path, baseKey }: { ctx: FilterContext; item: ConditionDraft; path: FilterPath; baseKey: string }) {
    const id = filterTestId(path)
    const info = describePath(ctx.catalog, baseKey, item.path)
    const remove = <RemoveButton label="Remove condition" testID={`${id}-remove`} onPress={() => ctx.change(removeFilterItem(ctx.root, path))} />
    if (!info) {
        return (
            <View testID={id} className="flex-row items-center gap-2 rounded-md border border-border p-2">
                <Text className="flex-1 text-sm text-muted-foreground">A condition names a field that is no longer offered.</Text>
                {remove}
            </View>
        )
    }
    const type = info.field.type
    // A new operator starts without an asked value: the param's kind follows the operator.
    const opItems: PickItem[] = opsFor(type).map(op => ({
        id: op,
        label: opLabel(op, type),
        isSelected: op === item.op,
        onSelect: () => {
            if (item.param) ctx.builder.setParams(ctx.params.filter(p => p.id !== item.param))
            setCondition(ctx, path, { op, param: '' })
        },
    }))
    const onField = (next: SpecHop[]) => {
        const nextType = describePath(ctx.catalog, baseKey, next)?.field.type ?? 'text'
        ctx.change(updateFilterItem(ctx.root, path, () => ({ ...emptyCondition(next), op: opsFor(nextType)[0] ?? 'eq' })))
    }
    return (
        <View testID={id} className="gap-1.5 rounded-md border border-border p-2">
            <View className="flex-row flex-wrap items-center gap-1.5">
                <ColumnPicker catalog={ctx.catalog} baseKey={baseKey} onPick={onField} label={pathLabel(info)} testID={`${id}-field`} toOneOnly />
                <PickMenu label={opLabel(item.op, type)} purpose="Condition" items={opItems} testID={`${id}-op`} />
                {remove}
            </View>
            <ConditionValue ctx={ctx} item={item} path={path} info={info} />
            <AskToggle ctx={ctx} item={item} path={path} info={info} />
        </View>
    )
}

function RelatedRow({ ctx, item, path, baseKey }: { ctx: FilterContext; item: RelatedDraft; path: FilterPath; baseKey: string }) {
    const id = filterTestId(path)
    const target = sourceAtLinks(ctx.catalog, baseKey, item.link)
    const linkLabel = ctx.catalog.get(baseKey)?.links.find(l => l.key === item.link[0]?.link)?.label ?? 'rows'
    const hasItems: PickItem[] = [
        { id: 'yes', label: 'Has', isSelected: item.has, onSelect: () => ctx.change(updateFilterItem(ctx.root, path, i => (i.kind === 'related' ? { ...i, has: true } : i))) },
        { id: 'no', label: 'Has no', isSelected: !item.has, onSelect: () => ctx.change(updateFilterItem(ctx.root, path, i => (i.kind === 'related' ? { ...i, has: false } : i))) },
    ]
    return (
        <View testID={id} className="gap-1.5 rounded-md border border-border p-2">
            <View className="flex-row flex-wrap items-center gap-1.5">
                <PickMenu label={item.has ? 'Has' : 'Has no'} purpose="Related rows" items={hasItems} testID={`${id}-has`} />
                <Text className="text-sm text-foreground">{`related ${linkLabel}`}</Text>
                <RemoveButton label="Remove related rows" testID={`${id}-remove`} onPress={() => ctx.change(removeFilterItem(ctx.root, path))} />
            </View>
            <GroupBody ctx={ctx} group={item.where} path={path} baseKey={target?.key ?? baseKey} />
        </View>
    )
}

function FixedRow({ ctx, path }: { ctx: FilterContext; path: FilterPath }) {
    const id = filterTestId(path)
    return (
        <View testID={id} className="flex-row items-center gap-2 rounded-md border border-border p-2">
            <Text className="flex-1 text-sm text-muted-foreground">A condition the builder cannot edit. It is kept as it is.</Text>
            <RemoveButton label="Remove condition" testID={`${id}-remove`} onPress={() => ctx.change(removeFilterItem(ctx.root, path))} />
        </View>
    )
}

function ItemEditor({ ctx, item, path, baseKey }: { ctx: FilterContext; item: FilterItem; path: FilterPath; baseKey: string }) {
    if (item.kind === 'condition') return <ConditionRow ctx={ctx} item={item} path={path} baseKey={baseKey} />
    if (item.kind === 'related') return <RelatedRow ctx={ctx} item={item} path={path} baseKey={baseKey} />
    if (item.kind === 'fixed') return <FixedRow ctx={ctx} path={path} />
    return (
        <View className="gap-1.5 rounded-md border border-border p-2">
            <View className="flex-row justify-end">
                <RemoveButton label="Remove group" testID={`${filterTestId(path)}-remove`} onPress={() => ctx.change(removeFilterItem(ctx.root, path))} />
            </View>
            <GroupBody ctx={ctx} group={item} path={path} baseKey={baseKey} />
        </View>
    )
}

function GroupBody({ ctx, group, path, baseKey }: { ctx: FilterContext; group: GroupDraft; path: FilterPath; baseKey: string }) {
    const id = filterTestId(path)
    const matchItems: PickItem[] = MATCHES.map(m => ({
        id: m.value,
        label: m.label,
        isSelected: m.value === group.match,
        onSelect: () => ctx.change(setMatchAt(ctx.root, path, m.value)),
    }))
    const manyLinks = (ctx.catalog.get(baseKey)?.links ?? []).filter(l => l.cardinality === 'toMany')
    const relatedItems: PickItem[] = manyLinks.map(link => ({
        id: link.key,
        label: link.label,
        onSelect: () => ctx.change(addFilterItem(ctx.root, path, { kind: 'related', has: true, link: [{ link: link.key }], where: emptyGroup() })),
    }))
    const onCondition = (next: SpecHop[]) => {
        const type = describePath(ctx.catalog, baseKey, next)?.field.type ?? 'text'
        ctx.change(addFilterItem(ctx.root, path, { ...emptyCondition(next), op: opsFor(type)[0] ?? 'eq' }))
    }
    const items = group.items.map((item, i) => (
        <ItemEditor key={`${i}:${item.kind}`} ctx={ctx} item={item} path={[...path, i]} baseKey={baseKey} />
    ))
    const related = relatedItems.length > 0 ? <PickMenu label="Add related rows" purpose="Related rows" items={relatedItems} testID={`${id}-add-related`} /> : null
    return (
        <View testID={`${id}-group`} className="gap-1.5">
            <PickMenu label={MATCHES.find(m => m.value === group.match)?.label ?? 'All of'} purpose="Join conditions" items={matchItems} testID={`${id}-match`} />
            {items}
            <View className="flex-row flex-wrap gap-1.5">
                <ColumnPicker catalog={ctx.catalog} baseKey={baseKey} onPick={onCondition} label="Add condition" testID={`${id}-add-condition`} toOneOnly />
                {related}
                <Button testID={`${id}-add-group`} variant="outline" size="sm" onPress={() => ctx.change(addFilterItem(ctx.root, path, { ...emptyGroup(), match: 'any' }))}>
                    <ButtonText>Add group</ButtonText>
                </Button>
            </View>
        </View>
    )
}

function FilterError({ message }: { message: string | undefined }) {
    if (!message) return null
    return (
        <Text testID="report-builder-filter-error" role="alert" className="text-xs text-danger">
            {message}
        </Text>
    )
}

export function FilterSection({ builder, problems }: { builder: ReportBuilder; problems: Problem[] }) {
    const { control } = builder.form
    const root = useWatch({ control, name: 'filter' })
    const params = useWatch({ control, name: 'params' })
    const error = builder.form.getFieldState('filter', builder.form.formState).error?.message
    const ctx: FilterContext = { builder, catalog: builder.catalog, root, params, change: builder.setFilter }
    return (
        <BuilderSection title="Filter" section="filter" problems={problems}>
            <FilterError message={error} />
            <GroupBody ctx={ctx} group={root} path={[]} baseKey={builder.base.key} />
        </BuilderSection>
    )
}
```

In `ReportBuilderPanel.tsx`, `Sections`: import `FilterSection` from `'./FilterSection'` and add `<FilterSection builder={builder} problems={problems} />` after `<ColumnsSection … />`.

- [ ] **Step 6: Run the unit test, format, lint, typecheck**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run reports/lib/builder && cd .. && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome format --write core/reports tests/e2e/reports-filter.spec.ts && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Expected: all pass.

- [ ] **Step 7: Run the e2e spec**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-filter.spec.ts`
Expected: 2 passed.

- [ ] **Step 8: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/reports tests/e2e/reports-filter.spec.ts
git commit -m "feat(core): report builder filter with related rows and asked values"
```

---

### Task 15: Builder group and sort, and pivot; folding on the view

**Files:**
- Create: `core/reports/components/builder/GroupSortSection.tsx`, `core/reports/components/builder/PivotSection.tsx`
- Modify: `core/reports/components/builder/ReportBuilderPanel.tsx` (`Sections`: add both after `FilterSection`)
- Create: `tests/e2e/reports-group-pivot.spec.ts`

**Interfaces:**
- Consumes: `columnChoices` (Task 11, with `isDate`, `isNested`, `hasAggregate`), `nextColumnId`, `describePath`, `pathLabel`, `BUCKETS` (Task 5), `moved` (`ColumnsSection.tsx`, Task 13), `ReportBuilder.setGroups`, `setSort`, `setPivotPath` (Task 13), core `SelectInput`, `Toggle`, `ButtonGroup`, `NumberInput`.
- Produces: test ids `report-builder-group-add`, `report-builder-group-<i>` with `-col` (items `-col-<columnId>`), `-bucket` (items `-bucket-<bucket>`), `-subtotals`, `-up`, `-down`, `-remove`; `report-builder-sort-add`, `report-builder-sort-<i>` with `-col`, `-remove`; `report-builder-pivot-on`, `report-builder-pivot-field`, `report-builder-pivot-bucket`, `report-builder-pivot-cell` (items `-<columnId>`), `report-builder-pivot-max`.

- [ ] **Step 1: Write the failing spec**

`tests/e2e/reports-group-pivot.spec.ts`:

```ts
import { expect, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub } from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    cellOf,
    chooseOption,
    openReportStub,
    openReportsHub,
    rowOf,
    SALES_SOURCE,
    saveNewReport,
    startReport,
    uniqueName,
} from './reports-helpers'

/**
 * Group levels with subtotals, folding on the report view, and a pivot, against
 * the salesstub package. A fresh invited member owns every sale the report reads.
 */
async function salesByStatus(page: import('@playwright/test').Page, label: string) {
    const { inviteePage: p, close } = await createInvitedUser(page, label)
    const team = uniqueName('Team')
    await openReportStub(p)
    await addTeam(p, team)
    await addSale(p, { team, customer: 'Alpha', amount: 1000, cost: 0, day: '2026-01-10' })
    await addSale(p, { team, customer: 'Beta', amount: 2000, cost: 0, day: '2026-02-10' })
    await addSale(p, { team, customer: 'Gamma', amount: 5000, cost: 0, day: '2026-01-20', status: 'paid' })
    await openReportsHub(p)
    await startReport(p, SALES_SOURCE)
    await addColumn(p, ['status'])
    await addColumn(p, ['amount'])
    await chooseOption(p, 'report-builder-column-2-aggregate', 'sum')
    await p.getByTestId('report-builder-section-group').click()
    await p.getByTestId('report-builder-group-add').click()
    await chooseOption(p, 'report-builder-group-0-col', 'status')
    return { p, close }
}

test.describe('grouping and pivoting a report', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a group level with subtotals opens and closes on the report view', async ({ page }) => {
        const { p, close } = await salesByStatus(page, 'rptgroup')
        try {
            await saveNewReport(p, uniqueName('By status'))
            // A closed group shows its figures.
            await expect(await cellOf(p, 'amount', 'Open')).toHaveText('30.00')
            await expect(p.getByTestId('report-fold-all')).toHaveText('Expand all')

            await rowOf(p, 'Open').click()
            await expect(p.getByTestId('report-table').getByText('Alpha', { exact: true })).toBeVisible()
            await expect(await cellOf(p, 'amount', 'Total Open')).toHaveText('$30.00')
            // An open group with a subtotal row shows no figures of its own.
            await expect(await cellOf(p, 'amount', 'Open')).toHaveText('')
            await expect(p.getByTestId('report-fold-all')).toHaveText('Collapse all')

            await p.getByTestId('report-fold-all').click()
            await expect(p.getByTestId('report-table').getByText('Alpha', { exact: true })).toHaveCount(0)
            await p.getByTestId('report-fold-all').click()
            await expect(p.getByTestId('report-table').getByText('Gamma', { exact: true })).toBeVisible()
        } finally {
            await close()
        }
    })

    test('a pivot splits the total by month', async ({ page }) => {
        const { p, close } = await salesByStatus(page, 'rptpivot')
        try {
            await p.getByTestId('report-builder-section-pivot').click()
            await p.getByTestId('report-builder-pivot-on').click()
            await p.getByTestId('report-builder-pivot-field').click()
            await p.getByTestId('report-builder-picker').getByTestId('report-builder-pick-field-day').click()
            await chooseOption(p, 'report-builder-pivot-bucket', 'month')
            await chooseOption(p, 'report-builder-pivot-cell', 'amount')
            await expect(p.getByTestId('report-heading-amount.p0')).toHaveText('2026-01')
            await expect(p.getByTestId('report-heading-amount.p1')).toHaveText('2026-02')
            await expect(await cellOf(p, 'amount.p0', 'Open')).toHaveText('10.00')
            await expect(await cellOf(p, 'amount.p1', 'Open')).toHaveText('20.00')
            await expect(await cellOf(p, 'amount.p0', 'Paid')).toHaveText('50.00')
        } finally {
            await close()
        }
    })
})
```

- [ ] **Step 2: Run it to see it fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-group-pivot.spec.ts`
Expected: FAIL: `report-builder-section-group` is not found.

- [ ] **Step 3: Write the group and sort section**

`core/reports/components/builder/GroupSortSection.tsx`:

```tsx
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { ButtonGroup, SelectInput, Toggle, useWatch } from '@tinycld/core/ui/form'
import { ArrowDown, ArrowUp, Trash2 } from 'lucide-react-native'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Pressable, Text, View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import { nextColumnId } from '../../lib/builder/columns'
import { columnChoices } from '../../lib/builder/draft'
import type { Problem } from '../../lib/types'
import { REPORT_TAP_TARGET } from '../ReportHeader'
import { BuilderSection } from './BuilderSection'
import { moved } from './ColumnsSection'

const BUCKET_OPTIONS = [
    { value: '', label: 'Each value' },
    { value: 'day', label: 'Day' },
    { value: 'month', label: 'Month' },
    { value: 'quarter', label: 'Quarter' },
    { value: 'year', label: 'Year' },
    { value: 'fiscalYear', label: 'Fiscal year' },
]
const DIR_OPTIONS = [
    { value: 'asc', label: 'A to Z' },
    { value: 'desc', label: 'Z to A' },
]

function IconButton({ label, testID, isVisible, icon: Icon, onPress }: { label: string; testID: string; isVisible: boolean; icon: typeof Trash2; onPress: () => void }) {
    const color = useThemeColor('muted-foreground')
    if (!isVisible) return null
    return (
        <Pressable testID={testID} accessibilityRole="button" accessibilityLabel={label} onPress={onPress} className="items-center justify-center" style={{ width: REPORT_TAP_TARGET, height: REPORT_TAP_TARGET }}>
            <Icon size={16} color={color} />
        </Pressable>
    )
}

function useChoices(builder: ReportBuilder) {
    const { control } = builder.form
    const columns = useWatch({ control, name: 'columns' })
    return columnChoices({ source: builder.base.key, columns }, builder.catalog)
}

function GroupRows({ builder }: { builder: ReportBuilder }) {
    const { control } = builder.form
    const groups = useWatch({ control, name: 'groups' })
    const choices = useChoices(builder)
    const groupable = choices.filter(choice => !choice.isNested).map(choice => ({ value: choice.key, label: choice.label }))
    const rows = groups.map((group, index) => {
        const testID = `report-builder-group-${index}`
        const isDate = choices.find(choice => choice.key === group.col)?.isDate === true
        const bucket = isDate ? <SelectInput name={`groups.${index}.bucket`} control={control} label="By" options={BUCKET_OPTIONS} testID={`${testID}-bucket`} /> : null
        return (
            <View key={`${index}:${group.id}`} testID={testID} className="gap-1 rounded-lg border border-border p-2">
                <View className="flex-row items-center">
                    <Text className="flex-1 text-xs text-muted-foreground">{`Level ${index + 1}`}</Text>
                    <IconButton label="Move level up" testID={`${testID}-up`} isVisible={index > 0} icon={ArrowUp} onPress={() => builder.setGroups(moved(groups, index, index - 1))} />
                    <IconButton label="Move level down" testID={`${testID}-down`} isVisible={index < groups.length - 1} icon={ArrowDown} onPress={() => builder.setGroups(moved(groups, index, index + 1))} />
                    <IconButton label="Remove level" testID={`${testID}-remove`} isVisible icon={Trash2} onPress={() => builder.setGroups(groups.filter((_, i) => i !== index))} />
                </View>
                <SelectInput name={`groups.${index}.col`} control={control} label="Group by" options={groupable} testID={`${testID}-col`} />
                {bucket}
                <Toggle name={`groups.${index}.subtotals`} control={control} label="Subtotals" testID={`${testID}-subtotals`} />
            </View>
        )
    })
    const add = () =>
        builder.setGroups([
            ...groups,
            { id: nextColumnId(groups.map(g => g.id), 'level'), col: groupable[0]?.value ?? '', bucket: '', subtotals: true },
        ])
    return (
        <View className="gap-2">
            {rows}
            <Button testID="report-builder-group-add" variant="outline" size="sm" onPress={add} className="self-start">
                <ButtonText>Add group level</ButtonText>
            </Button>
        </View>
    )
}

function SortRows({ builder }: { builder: ReportBuilder }) {
    const { control } = builder.form
    const sort = useWatch({ control, name: 'sort' })
    const choices = useChoices(builder).map(choice => ({ value: choice.key, label: choice.label }))
    const rows = sort.map((key, index) => {
        const testID = `report-builder-sort-${index}`
        return (
            <View key={`${index}:${key.col}`} testID={testID} className="gap-1 rounded-lg border border-border p-2">
                <View className="flex-row items-center">
                    <Text className="flex-1 text-xs text-muted-foreground">{`Sort ${index + 1}`}</Text>
                    <IconButton label="Remove sort" testID={`${testID}-remove`} isVisible icon={Trash2} onPress={() => builder.setSort(sort.filter((_, i) => i !== index))} />
                </View>
                <SelectInput name={`sort.${index}.col`} control={control} label="Sort by" options={choices} testID={`${testID}-col`} />
                <ButtonGroup name={`sort.${index}.dir`} control={control} label="Order" horizontal options={DIR_OPTIONS} />
            </View>
        )
    })
    const add = () => builder.setSort([...sort, { col: choices[0]?.value ?? '', dir: 'asc' }])
    return (
        <View className="gap-2">
            {rows}
            <Button testID="report-builder-sort-add" variant="outline" size="sm" onPress={add} className="self-start">
                <ButtonText>Add sort</ButtonText>
            </Button>
        </View>
    )
}

export function GroupSortSection({ builder, problems }: { builder: ReportBuilder; problems: Problem[] }) {
    return (
        <BuilderSection title="Group and sort" section="group" problems={problems}>
            <Text className="text-xs font-semibold uppercase text-muted-foreground">Groups</Text>
            <GroupRows builder={builder} />
            <Text className="text-xs font-semibold uppercase text-muted-foreground">Sort</Text>
            <SortRows builder={builder} />
        </BuilderSection>
    )
}
```

- [ ] **Step 4: Write the pivot section**

`core/reports/components/builder/PivotSection.tsx`:

```tsx
import { NumberInput, SelectInput, Toggle, useWatch } from '@tinycld/core/ui/form'
import { View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import { describePath, pathLabel } from '../../lib/builder/columns'
import { columnChoices } from '../../lib/builder/draft'
import type { Problem } from '../../lib/types'
import { BuilderSection } from './BuilderSection'
import { ColumnPicker } from './ColumnPicker'

const BUCKET_OPTIONS = [
    { value: '', label: 'Each value' },
    { value: 'day', label: 'Day' },
    { value: 'month', label: 'Month' },
    { value: 'quarter', label: 'Quarter' },
    { value: 'year', label: 'Year' },
    { value: 'fiscalYear', label: 'Fiscal year' },
]

function PivotFields({ builder }: { builder: ReportBuilder }) {
    const { control } = builder.form
    const pivot = useWatch({ control, name: 'pivot' })
    const columns = useWatch({ control, name: 'columns' })
    if (!pivot.on) return null
    const info = describePath(builder.catalog, builder.base.key, pivot.path)
    const isDate = info?.field.type === 'date' || info?.field.type === 'datetime'
    const cells = columnChoices({ source: builder.base.key, columns }, builder.catalog)
        .filter(choice => choice.hasAggregate)
        .map(choice => ({ value: choice.key, label: choice.label }))
    const bucket = isDate ? <SelectInput name="pivot.bucket" control={control} label="By" options={BUCKET_OPTIONS} testID="report-builder-pivot-bucket" /> : null
    return (
        <View className="gap-2">
            <ColumnPicker catalog={builder.catalog} baseKey={builder.base.key} onPick={builder.setPivotPath} label={info ? pathLabel(info) : 'Choose a field'} testID="report-builder-pivot-field" toOneOnly />
            {bucket}
            <SelectInput name="pivot.cell" control={control} label="Cells show" options={cells} testID="report-builder-pivot-cell" />
            <NumberInput name="pivot.maxColumns" control={control} label="At most this many columns" min={1} max={200} testID="report-builder-pivot-max" />
        </View>
    )
}

// A pivot spreads a column with a total across one column per value of a field
// (a month, a status). It needs a group level.
export function PivotSection({ builder, problems }: { builder: ReportBuilder; problems: Problem[] }) {
    return (
        <BuilderSection title="Pivot" section="pivot" problems={problems}>
            <Toggle name="pivot.on" control={builder.form.control} label="Pivot by a field" testID="report-builder-pivot-on" />
            <PivotFields builder={builder} />
        </BuilderSection>
    )
}
```

In `ReportBuilderPanel.tsx`, `Sections`: import both and add `<GroupSortSection builder={builder} problems={problems} />` and `<PivotSection builder={builder} problems={problems} />` after `<FilterSection … />`.

- [ ] **Step 5: Format, lint, typecheck**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome format --write core/reports tests/e2e/reports-group-pivot.spec.ts && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Expected: clean.

- [ ] **Step 6: Run the spec**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-group-pivot.spec.ts`
Expected: 2 passed.

- [ ] **Step 7: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/reports tests/e2e/reports-group-pivot.spec.ts
git commit -m "feat(core): report builder group, sort and pivot"
```

---

### Task 16: Builder formulas, to-many totals and child rows, and layout

**Files:**
- Modify: `core/reports/components/builder/ColumnsSection.tsx` (formula editor, Add formula)
- Create: `core/reports/components/builder/LayoutSection.tsx`
- Modify: `core/reports/components/builder/ReportBuilderPanel.tsx` (`Sections`: add `LayoutSection` last)
- Create: `tests/e2e/reports-formula-layout.spec.ts`

**Interfaces:**
- Consumes: `FormulaEditor` (`@tinycld/core/components/forms-builder/FormulaEditor`), `formulaOperands` (Task 11), `columnChoices` (Task 11), `ReportBuilder.setColumns`, `setLayout` (Task 13).
- Produces: test ids `report-builder-add-formula`, `report-builder-column-<i>-formula` (FormulaEditor's own: `-root-part`, `-<path>-part`, `-<path>-field`, …), `report-builder-column-<i>-formula-fixed`; `report-builder-layout-add-section`, `report-builder-layout-add-row`, `report-builder-layout-<i>` with `-title`, `-key` (items `-key-<value>`), `-label`, `-formula`, `-up`, `-down`, `-remove`.

- [ ] **Step 1: Write the failing spec**

`tests/e2e/reports-formula-layout.spec.ts`:

```ts
import { expect, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub } from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    cellOf,
    chooseOption,
    openReportStub,
    openReportsHub,
    SALES_SOURCE,
    startReport,
    TEAMS_SOURCE,
    uniqueName,
} from './reports-helpers'

/**
 * Formula columns (the forms engine's formula editor), to-many totals and child
 * rows, and a statement layout, against the salesstub package. A fresh invited
 * member owns the sales; shared teams are scoped by a name stem.
 */
test.describe('formulas, related rows and layouts', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a formula column subtracts cost from amount', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptformula')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, { team, customer: 'Acme', amount: 1000, cost: 400, day: '2026-01-05' })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['amount'])
            await addColumn(p, ['cost'])
            await p.getByTestId('report-builder-add-formula').click()
            await p.getByTestId('report-builder-column-3-label').fill('Profit')
            const formula = 'report-builder-column-3-formula'
            await chooseOption(p, `${formula}-root-part`, 'sub')
            await chooseOption(p, `${formula}-0-part`, 'field')
            await chooseOption(p, `${formula}-0-field`, 'amount')
            await chooseOption(p, `${formula}-1-part`, 'field')
            await chooseOption(p, `${formula}-1-field`, 'cost')
            await expect(p.getByTestId(`${formula}-summary`)).toHaveText('= Amount − Cost')
            await expect(p.getByTestId('report-heading-formula')).toHaveText('Profit')
            await expect(await cellOf(p, 'formula', 'Acme')).toHaveText('6.00')
        } finally {
            await close()
        }
    })

    test('a to-many link adds a total per row or shows child rows', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptmany')
        try {
            const stem = uniqueName('M')
            const team = `${stem} team`
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, { team, customer: 'First buyer', amount: 100, cost: 0, day: '2026-01-05' })
            await addSale(p, { team, customer: 'Second buyer', amount: 200, cost: 0, day: '2026-01-06' })
            await openReportsHub(p)
            await startReport(p, TEAMS_SOURCE)
            await p.getByTestId('report-builder-section-filter').click()
            await p.getByTestId('report-builder-filter-add-condition').click()
            await p.getByTestId('report-builder-picker').getByTestId('report-builder-pick-field-name').click()
            await chooseOption(p, 'report-builder-filter-0-op', 'startsWith')
            await p.getByTestId('report-builder-filter-0-value').fill(stem)

            await addColumn(p, ['sales', 'id'], 'total')
            await expect(p.getByTestId('report-builder-column-1-aggregate')).toContainText('Count')
            await expect(await cellOf(p, 'salesId', team)).toHaveText('2')

            await addColumn(p, ['sales', 'customer'], 'nest')
            const table = p.getByTestId('report-table')
            await expect(table.getByText('First buyer', { exact: true })).toBeVisible()
            await expect(table.getByText('Second buyer', { exact: true })).toBeVisible()
        } finally {
            await close()
        }
    })

    test('a layout shows sections by status and a computed row', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptlayout')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, { team, customer: 'Owed', amount: 3000, cost: 0, day: '2026-01-05' })
            await addSale(p, { team, customer: 'Settled', amount: 5000, cost: 0, day: '2026-01-06', status: 'paid' })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['status'])
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-2-aggregate', 'sum')
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'status')

            await p.getByTestId('report-builder-section-layout').click()
            await p.getByTestId('report-builder-layout-add-section').click()
            await p.getByTestId('report-builder-layout-0-title').fill('Open sales')
            await chooseOption(p, 'report-builder-layout-0-key', 'open')
            await p.getByTestId('report-builder-layout-add-section').click()
            await p.getByTestId('report-builder-layout-1-title').fill('Paid sales')
            await chooseOption(p, 'report-builder-layout-1-key', 'paid')
            await p.getByTestId('report-builder-layout-add-row').click()
            await p.getByTestId('report-builder-layout-2-label').fill('Open less paid')
            const formula = 'report-builder-layout-2-formula'
            await chooseOption(p, `${formula}-root-part`, 'sub')
            await chooseOption(p, `${formula}-0-part`, 'field')
            await chooseOption(p, `${formula}-0-field`, 'section')
            await chooseOption(p, `${formula}-1-part`, 'field')
            await chooseOption(p, `${formula}-1-field`, 'section2')

            await expect(await cellOf(p, 'amount', 'Total Open sales')).toHaveText('$30.00')
            await expect(await cellOf(p, 'amount', 'Total Paid sales')).toHaveText('$50.00')
            await expect(await cellOf(p, 'amount', 'Open less paid')).toHaveText('-$20.00')
        } finally {
            await close()
        }
    })
})
```

- [ ] **Step 2: Run it to see it fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-formula-layout.spec.ts`
Expected: FAIL: `report-builder-add-formula` is not found (the to-many test passes its first half already; its child-rows half and the layout test fail on the missing parts).

- [ ] **Step 3: Add the formula editor to the columns**

In `core/reports/components/builder/ColumnsSection.tsx`:

Add imports:

```tsx
import { FormulaEditor } from '@tinycld/core/components/forms-builder/FormulaEditor'
import type { ComputeExpr } from '@tinycld/core/lib/forms/expression'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { columnChoices } from '../../lib/builder/draft'
import { formulaOperands } from '../../lib/builder/formula'
```

Replace the placeholder `ColumnFormula` with:

```tsx
function FixedFormula({ testID }: { testID: string }) {
    return (
        <Text testID={testID} className="text-xs text-muted-foreground">
            This formula uses parts the builder cannot edit. It is kept as it is.
        </Text>
    )
}

// The forms engine's structured formula editor (spec §4): parts picked from menus,
// reading the other numeric columns (R15).
function ColumnFormula({ builder, columns, index }: ColumnRowProps) {
    const column = columns[index]
    if (column?.kind !== 'formula') return null
    const testID = `report-builder-column-${index}-formula`
    if (column.formula.kind === 'fixed') return <FixedFormula testID={`${testID}-fixed`} />
    const choices = columnChoices({ source: builder.base.key, columns }, builder.catalog)
    const onChange = (expr: ComputeExpr) =>
        builder.setColumns(columns.map((c, i) => (i === index && c.kind === 'formula' ? { ...c, formula: { kind: 'editable', expr } } : c)))
    return <FormulaEditor value={column.formula.expr} onChange={onChange} operands={formulaOperands(choices, column.id)} valueNoun="Column" testID={testID} />
}
```


In `ColumnsSection`, add the formula button beside the picker:

```tsx
    const onAddFormula = () =>
        builder.setColumns([
            ...columns,
            { kind: 'formula', id: nextColumnId(columns.map(c => c.id), 'formula'), label: 'Formula', formula: { kind: 'editable', expr: 0 }, aggregate: '', scale: null, format: '' },
        ])
```

```tsx
            <View className="flex-row flex-wrap gap-2">
                <ColumnPicker catalog={builder.catalog} baseKey={builder.base.key} onPick={onPick} label="Add column" testID="report-builder-add-column" />
                <Button testID="report-builder-add-formula" variant="outline" size="sm" onPress={onAddFormula}>
                    <ButtonText>Add formula</ButtonText>
                </Button>
            </View>
```

- [ ] **Step 4: Write the layout section**

`core/reports/components/builder/LayoutSection.tsx`:

```tsx
import { FormulaEditor } from '@tinycld/core/components/forms-builder/FormulaEditor'
import type { ComputeExpr } from '@tinycld/core/lib/forms/expression'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { SelectInput, TextInput, useWatch } from '@tinycld/core/ui/form'
import { ArrowDown, ArrowUp, Trash2 } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import { nextColumnId } from '../../lib/builder/columns'
import { columnChoices, type LayoutItemDraft } from '../../lib/builder/draft'
import { formulaOperands } from '../../lib/builder/formula'
import type { Problem } from '../../lib/types'
import { REPORT_TAP_TARGET } from '../ReportHeader'
import { BuilderSection } from './BuilderSection'
import { moved } from './ColumnsSection'

function IconButton({ label, testID, isVisible, icon: Icon, onPress }: { label: string; testID: string; isVisible: boolean; icon: typeof Trash2; onPress: () => void }) {
    const color = useThemeColor('muted-foreground')
    if (!isVisible) return null
    return (
        <Pressable testID={testID} accessibilityRole="button" accessibilityLabel={label} onPress={onPress} className="items-center justify-center" style={{ width: REPORT_TAP_TARGET, height: REPORT_TAP_TARGET }}>
            <Icon size={16} color={color} />
        </Pressable>
    )
}

function NeedsGroup({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return <Text className="text-sm text-muted-foreground">A layout shows the first group level's values as sections. Add a group level first.</Text>
}

interface ItemProps {
    builder: ReportBuilder
    items: LayoutItemDraft[]
    index: number
    keyOptions: { value: string; label: string }[]
}

// A section's key is a value of the first group level: a select's options when the
// group column is a select, else typed.
function SectionKey({ builder, index, keyOptions }: ItemProps) {
    const testID = `report-builder-layout-${index}-key`
    const { control } = builder.form
    if (keyOptions.length > 0) return <SelectInput name={`layout.${index}.key`} control={control} label="Shows the group" options={keyOptions} testID={testID} />
    return <TextInput name={`layout.${index}.key`} control={control} label="Shows the group" testID={testID} />
}

function RowFormula({ builder, items, index }: ItemProps) {
    const item = items[index]
    if (item?.kind !== 'row') return null
    const testID = `report-builder-layout-${index}-formula`
    if (item.formula.kind === 'fixed') {
        return <Text testID={`${testID}-fixed`} className="text-xs text-muted-foreground">This formula uses parts the builder cannot edit. It is kept as it is.</Text>
    }
    // A computed row reads the sections and computed rows above it, by name.
    const earlier = items.slice(0, index).map(other => ({ key: other.id, label: other.kind === 'section' ? other.title : other.label, isNumeric: true }))
    const onChange = (expr: ComputeExpr) =>
        builder.setLayout(items.map((other, i) => (i === index && other.kind === 'row' ? { ...other, formula: { kind: 'editable', expr } } : other)))
    return <FormulaEditor value={item.formula.expr} onChange={onChange} operands={formulaOperands(earlier, item.id)} valueNoun="Section or row" testID={testID} />
}

function ItemFields(props: ItemProps) {
    const { builder, items, index } = props
    const item = items[index]
    const { control } = builder.form
    if (!item) return null
    if (item.kind === 'section') {
        return (
            <>
                <TextInput name={`layout.${index}.title`} control={control} label="Section title" testID={`report-builder-layout-${index}-title`} maxLength={200} />
                <SectionKey {...props} />
            </>
        )
    }
    return (
        <>
            <TextInput name={`layout.${index}.label`} control={control} label="Row label" testID={`report-builder-layout-${index}-label`} maxLength={200} />
            <RowFormula {...props} />
        </>
    )
}

function LayoutItem(props: ItemProps) {
    const { builder, items, index } = props
    const item = items[index]
    if (!item) return null
    const testID = `report-builder-layout-${index}`
    return (
        <View testID={testID} className="gap-1 rounded-lg border border-border p-2">
            <View className="flex-row items-center">
                <Text className="flex-1 text-xs text-muted-foreground">{item.kind === 'section' ? 'Section' : 'Computed row'}</Text>
                <IconButton label="Move up" testID={`${testID}-up`} isVisible={index > 0} icon={ArrowUp} onPress={() => builder.setLayout(moved(items, index, index - 1))} />
                <IconButton label="Move down" testID={`${testID}-down`} isVisible={index < items.length - 1} icon={ArrowDown} onPress={() => builder.setLayout(moved(items, index, index + 1))} />
                <IconButton label="Remove" testID={`${testID}-remove`} isVisible icon={Trash2} onPress={() => builder.setLayout(items.filter((_, i) => i !== index))} />
            </View>
            <ItemFields {...props} />
        </View>
    )
}

// Statements: an ordered list of sections, each the rows of one first-level group
// with its subtotal, and computed rows over sections by name (spec §2 "layout").
export function LayoutSection({ builder, problems }: { builder: ReportBuilder; problems: Problem[] }) {
    const { control } = builder.form
    const items = useWatch({ control, name: 'layout' })
    const groups = useWatch({ control, name: 'groups' })
    const columns = useWatch({ control, name: 'columns' })
    const firstGroup = columnChoices({ source: builder.base.key, columns }, builder.catalog).find(choice => choice.key === groups[0]?.col)
    const keyOptions = firstGroup?.options ?? []
    const ids = items.map(item => item.id)
    const rows = items.map((item, index) => (
        <LayoutItem key={`${index}:${item.id}`} builder={builder} items={items} index={index} keyOptions={keyOptions} />
    ))
    const addSection = () => builder.setLayout([...items, { kind: 'section', id: nextColumnId(ids, 'section'), title: 'Section', key: '' }])
    const addRow = () => builder.setLayout([...items, { kind: 'row', id: nextColumnId(ids, 'row'), label: 'Computed row', formula: { kind: 'editable', expr: 0 } }])
    return (
        <BuilderSection title="Layout" section="layout" problems={problems}>
            <NeedsGroup isVisible={groups.length === 0} />
            {rows}
            <View className="flex-row flex-wrap gap-2">
                <Button testID="report-builder-layout-add-section" variant="outline" size="sm" onPress={addSection}>
                    <ButtonText>Add section</ButtonText>
                </Button>
                <Button testID="report-builder-layout-add-row" variant="outline" size="sm" onPress={addRow}>
                    <ButtonText>Add computed row</ButtonText>
                </Button>
            </View>
        </BuilderSection>
    )
}
```

In `ReportBuilderPanel.tsx`, `Sections`: import `LayoutSection` and add `<LayoutSection builder={builder} problems={problems} />` last.

- [ ] **Step 5: Format, lint, typecheck**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome format --write core/reports tests/e2e/reports-formula-layout.spec.ts && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Expected: clean.

- [ ] **Step 6: Run the spec**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-formula-layout.spec.ts`
Expected: 3 passed.

- [ ] **Step 7: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/reports tests/e2e/reports-formula-layout.spec.ts
git commit -m "feat(core): report builder formulas, related totals and child rows, and layout"
```

---

### Task 17: The report view's params, Customize, notices, Copy link and live re-runs

**Files:**
- Create: `core/reports/lib/param-forms.ts`, test `core/reports/lib/__tests__/param-forms.test.ts`
- Create: `core/reports/components/ParamChip.tsx`, `core/reports/components/ParamPeriodControl.tsx`, `core/reports/components/ParamPickers.tsx`, `core/reports/components/ReportControls.tsx`, `core/reports/components/ReportNotices.tsx`, `core/reports/components/ReportCustomizePanel.tsx`
- Modify: `core/reports/components/ReportView.tsx` (controls, notices, Customize and Copy link in the toolbar, the panel)
- Create: `tests/e2e/reports-view.spec.ts`

**Interfaces:**
- Consumes: `ReportViewModel` (`controls`, `resolved`, `surface`, `onOpenSurface`, `onCloseSurface`, `onCloseSurfaceOf`, `onApplyParam(id, value, keepOpen?)`, `onRemoveParam`, `display`, `allColumns`, `onDisplayChange`, `onResetDisplay`, `isCustomizeOpen`, `onOpenCustomize`, `onCloseCustomize`, `onCopyLink`, `notices`, `isUpdating`) (Task 13); `chipText`, `presetsFor`, `PRESET_LABELS`, `CUSTOM_LABEL`, `isDateParam`, `rangeValue`, `MAX_TEXT_PARAM` (Task 8); core `Popover`, `Menu`, `Drawer*`, `Sheet`, `DateField`, `Toggle`, `ButtonGroup`, `TextInput`, `NumberInput`.
- Produces (`param-forms.ts`): `DATE_FORMAT_MESSAGE`, `FROM_AFTER_TO_MESSAGE`, `CUSTOM`, `PeriodValues`, `periodSchema(def)`, `periodDefaults(value, resolved)`, `periodValueOf(def, values)`, `textValueSchema`, `numberValueSchema`, `choicesSchema`.
- Produces test ids: `report-controls`, `report-param-<id>` (chip), `report-filter-remove-<id>`, `report-add-filter` (items `report-add-filter-<id>`), `report-period-popover`, `report-period-preset-<preset|custom>`, `report-period-from`, `report-period-to`, `report-period-as-of`, `report-period-error`, `report-filter-picker`, `report-filter-option-<value>`, `report-filter-value`, `report-filter-apply`, `report-notices`, `report-notice-<code>`, `report-updating`, `report-customize`, `report-customize-open`, `report-customize-close`, `report-customize-section-number-format|columns`, `report-option-divide-by-1000`, `report-option-without-cents`, `report-option-negative-red`, `report-option-column-<i>`, `report-customize-reset`, `report-copy-link`.

- [ ] **Step 1: Write the failing spec**

`tests/e2e/reports-view.spec.ts`:

```ts
import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, loginAs, skipWithoutReportsStub } from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    cellOf,
    chooseOption,
    openReportStub,
    openReportsHub,
    pressToolbar,
    SALES_SOURCE,
    saveNewReport,
    startReport,
    uniqueName,
} from './reports-helpers'

/**
 * The report view against the salesstub package: the period chip and filter
 * chips edit the report's params, Customize changes the number format and the
 * columns, a copied link reopens the report as it was, and a sale added in
 * another browser shows in the open report. A fresh invited member owns every sale.
 * Days are in the current calendar year, which is the stub source's fiscal year.
 */
const YEAR = new Date().getFullYear()

async function makeReport(page: Page, label: string, askDay: boolean) {
    const invited = await createInvitedUser(page, label)
    const p = invited.inviteePage
    const team = uniqueName('Team')
    await openReportStub(p)
    await addTeam(p, team)
    await addSale(p, { team, customer: 'Alpha', amount: 150000, cost: 10000, day: `${YEAR}-01-10` })
    await addSale(p, { team, customer: 'Beta', amount: 20000, cost: 5000, day: `${YEAR}-02-10` })
    await addSale(p, { team, customer: 'Gamma', amount: 30000, cost: 5000, day: `${YEAR}-03-10` })
    await openReportsHub(p)
    await startReport(p, SALES_SOURCE)
    await addColumn(p, ['amount'])
    await chooseOption(p, 'report-builder-column-1-aggregate', 'sum')
    await addColumn(p, ['cost'])
    if (askDay) {
        await p.getByTestId('report-builder-section-filter').click()
        await p.getByTestId('report-builder-filter-add-condition').click()
        await p.getByTestId('report-builder-picker').getByTestId('report-builder-pick-field-day').click()
        await chooseOption(p, 'report-builder-filter-0-op', 'between')
        await p.getByTestId('report-builder-filter-0-ask').click()
        await chooseOption(p, 'report-builder-filter-0-default', 'thisFiscalYear')
        await p.getByTestId('report-builder-filter-add-condition').click()
        await p.getByTestId('report-builder-picker').getByTestId('report-builder-pick-field-customer').click()
        await chooseOption(p, 'report-builder-filter-1-op', 'contains')
        await p.getByTestId('report-builder-filter-1-ask').click()
    }
    await saveNewReport(p, uniqueName('View'))
    return invited
}

test.describe('the report view', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('the period chip and a filter chip edit the run, and a copied link keeps them', async ({ page, browser }) => {
        const { inviteePage: p, user, close } = await makeReport(page, 'rptchips', true)
        try {
            const table = p.getByTestId('report-table')
            await expect(p.getByTestId('report-param-day')).toHaveText(new RegExp(`^Day: This fiscal year · Jan 1 – Dec 31, ${YEAR}$`))
            await expect(table.getByText('Gamma', { exact: true })).toBeVisible()

            await p.getByTestId('report-param-day').click()
            await p.getByTestId('report-period-preset-custom').click()
            await p.getByTestId('report-period-from').fill(`${YEAR}-02-01`)
            await p.getByTestId('report-period-to').fill(`${YEAR}-02-28`)
            await expect.poll(() => p.url()).toContain(`p-day=${YEAR}-02-01..${YEAR}-02-28`)
            await p.keyboard.press('Escape')
            await expect(table.getByText('Beta', { exact: true })).toBeVisible()
            await expect(table.getByText('Alpha', { exact: true })).toHaveCount(0)
            await expect(p.getByTestId('report-document-period')).toHaveText(`February 1 – February 28, ${YEAR}`)

            await p.getByTestId('report-add-filter').click()
            await p.getByTestId('report-add-filter-customer').click()
            await p.getByTestId('report-filter-value').fill('Gam')
            await p.getByTestId('report-filter-apply').click()
            await expect(p.getByTestId('report-param-customer')).toHaveText('Customer: “Gam”')
            await expect(p.getByTestId('report-empty-message')).toHaveText(`No rows for February 1 – February 28, ${YEAR}`)
            await p.getByTestId('report-empty-action').click()
            await expect(p.getByTestId('report-param-customer')).toHaveCount(0)
            await expect(table.getByText('Beta', { exact: true })).toBeVisible()

            // The copied link is the report's full address with this run's options.
            await p.context().grantPermissions(['clipboard-read', 'clipboard-write'])
            await pressToolbar(p, 'report-copy-link', 'Copy link')
            const link = await p.evaluate(() => navigator.clipboard.readText())
            expect(link).toContain(`p-day=${YEAR}-02-01..${YEAR}-02-28`)
            const other = await browser.newContext()
            try {
                const q = await other.newPage()
                await loginAs(q, user.username, user.password)
                // Opening a pasted link is a full load by nature.
                await q.goto(link)
                await expect(q.getByTestId('report-param-day')).toHaveText(new RegExp(`^Day: Feb 1 – Feb 28, ${YEAR}$`))
                await expect(q.getByTestId('report-table').getByText('Beta', { exact: true })).toBeVisible()
            } finally {
                await other.close()
            }
        } finally {
            await close()
        }
    })

    test('Customize changes the number format and hides a column', async ({ page }) => {
        const { inviteePage: p, close } = await makeReport(page, 'rptcustomize', false)
        try {
            await expect(await cellOf(p, 'amount', 'Alpha')).toHaveText('1,500.00')
            await pressToolbar(p, 'report-customize-open', 'Customize')
            await p.getByTestId('report-option-divide-by-1000').click()
            await expect(await cellOf(p, 'amount', 'Alpha')).toHaveText('1.50')
            await expect(p.getByTestId('report-thousands-note')).toBeVisible()
            await p.getByTestId('report-customize-section-columns').click()
            await p.getByTestId('report-option-column-1').click()
            await expect(p.getByTestId('report-heading-cost')).toHaveCount(0)
            await expect.poll(() => p.url()).toContain('hide=cost')
            await p.getByTestId('report-customize-reset').click()
            await expect(p.getByTestId('report-heading-cost')).toBeVisible()
            await expect(p.getByTestId('report-thousands-note')).toHaveCount(0)
        } finally {
            await close()
        }
    })

    test('a sale added in another browser shows in the open report', async ({ page, browser }) => {
        const { inviteePage: p, user, close } = await makeReport(page, 'rptlive', false)
        const other = await browser.newContext()
        try {
            const q = await other.newPage()
            await loginAs(q, user.username, user.password)
            await openReportStub(q)
            const team = uniqueName('Live team')
            await addTeam(q, team)
            await addSale(q, { team, customer: 'Live Co', amount: 999, cost: 0, day: `${YEAR}-04-01` })
            await expect(p.getByTestId('report-table').getByText('Live Co', { exact: true })).toBeVisible()
        } finally {
            await other.close()
            await close()
        }
    })

    test('a pivot over its column cap says how many values were left out', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptcap')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, { team, customer: 'One', amount: 100, cost: 0, day: `${YEAR}-01-10` })
            await addSale(p, { team, customer: 'Two', amount: 100, cost: 0, day: `${YEAR}-02-10` })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['status'])
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-2-aggregate', 'sum')
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'status')
            await p.getByTestId('report-builder-section-pivot').click()
            await p.getByTestId('report-builder-pivot-on').click()
            await p.getByTestId('report-builder-pivot-field').click()
            await p.getByTestId('report-builder-picker').getByTestId('report-builder-pick-field-day').click()
            await chooseOption(p, 'report-builder-pivot-bucket', 'month')
            await chooseOption(p, 'report-builder-pivot-cell', 'amount')
            await p.getByTestId('report-builder-pivot-max').fill('1')
            await expect(p.getByTestId('report-notice-pivotCap')).toHaveText('The pivot shows its first 1 columns; 1 more are left out.')
        } finally {
            await close()
        }
    })

    test('on a phone the chips open sheets and Customize is a sheet', async ({ page }) => {
        const { inviteePage: p, close } = await makeReport(page, 'rptviewphone', true)
        try {
            await p.setViewportSize({ width: 390, height: 844 })
            await p.getByTestId('report-param-day').click()
            await expect(p.getByText('Day', { exact: true }).last()).toBeVisible()
            await p.getByTestId('report-period-preset-lastFiscalYear').click()
            await expect(p.getByTestId('report-param-day')).toHaveText(new RegExp(`^Day: Last fiscal year · Jan 1 – Dec 31, ${YEAR - 1}$`))
            await expect(p.getByTestId('report-empty')).toBeVisible()
            await pressToolbar(p, 'report-customize-open', 'Customize')
            await expect(p.getByTestId('report-customize')).toBeVisible()
            await p.getByTestId('report-option-without-cents').click()
            await p.getByTestId('report-customize-close').click()
            await expect(p.getByTestId('report-customize')).toBeHidden()
            await expect.poll(() => p.url()).toContain('nocents=1')
        } finally {
            await close()
        }
    })
})
```

- [ ] **Step 2: Write the failing unit test**

`core/reports/lib/__tests__/param-forms.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { DATE_FORMAT_MESSAGE, FROM_AFTER_TO_MESSAGE, periodDefaults, periodSchema, periodValueOf } from '../param-forms'
import type { ParamDef } from '../spec'

const RANGE: ParamDef = { id: 'day', type: 'dateRange', default: { preset: 'thisMonth' } }
const AS_OF: ParamDef = { id: 'asOf', type: 'asOf' }

const issues = (def: ParamDef, values: unknown) => {
    const parsed = periodSchema(def).safeParse(values)
    return parsed.success ? [] : parsed.error.issues.map(issue => `${issue.path.join('.')}: ${issue.message}`)
}

describe('the period form', () => {
    it('checks typed days only for Custom', () => {
        expect(issues(RANGE, { preset: 'thisMonth', from: 'bad', to: null })).toEqual([])
        expect(issues(RANGE, { preset: 'custom', from: '2026-02-30', to: '2026-03-01' })).toEqual([`from: ${DATE_FORMAT_MESSAGE}`])
        expect(issues(RANGE, { preset: 'custom', from: '2026-03-02', to: '2026-03-01' })).toEqual([`from: ${FROM_AFTER_TO_MESSAGE}`])
        expect(issues(AS_OF, { preset: 'custom', from: null, to: '' })).toEqual([`to: ${DATE_FORMAT_MESSAGE}`])
    })

    it('turns the form into a param value', () => {
        expect(periodValueOf(RANGE, { preset: 'lastMonth', from: null, to: null })).toEqual({ kind: 'preset', preset: 'lastMonth' })
        expect(periodValueOf(RANGE, { preset: 'custom', from: '2026-02-01', to: '2026-02-28' })).toEqual({ kind: 'range', from: '2026-02-01', to: '2026-02-28' })
        expect(periodValueOf(AS_OF, { preset: 'custom', from: null, to: '2026-06-30' })).toEqual({ kind: 'day', day: '2026-06-30' })
        expect(periodValueOf(RANGE, { preset: 'custom', from: '2026-03-01', to: '2026-02-01' })).toBeNull()
    })

    it('starts from the value shown, with a preset’s resolved days', () => {
        expect(periodDefaults({ kind: 'preset', preset: 'thisMonth' }, { id: 'day', type: 'dateRange', from: '2026-10-01', to: '2026-10-31' })).toEqual({
            preset: 'thisMonth',
            from: '2026-10-01',
            to: '2026-10-31',
        })
        expect(periodDefaults({ kind: 'day', day: '2026-06-30' }, undefined)).toEqual({ preset: 'custom', from: null, to: '2026-06-30' })
        expect(periodDefaults(null, undefined)).toEqual({ preset: 'custom', from: null, to: null })
    })
})
```

- [ ] **Step 3: Run both to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run reports/lib/__tests__/param-forms.test.ts`
Expected: FAIL: `../param-forms` does not resolve.

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-view.spec.ts`
Expected: FAIL: `report-param-day` is not found (the live test may already pass: live re-runs came with Task 10).

- [ ] **Step 4: Write the param forms**

`core/reports/lib/param-forms.ts`:

```ts
import { z } from '@tinycld/core/ui/form'
import { isIsoDay } from './dates'
import { MAX_TEXT_PARAM, type ParamValue, presetsFor, rangeValue } from './params'
import type { ParamDef } from './spec'
import type { ResolvedParam } from './types'

// The forms of the param chips. A preset applies at once; Custom days are typed and
// apply once they make a valid period (the Books shell's rules).

export const DATE_FORMAT_MESSAGE = 'Enter a date as YYYY-MM-DD.'
export const FROM_AFTER_TO_MESSAGE = 'From must be on or before To.'
export const CUSTOM = 'custom'

export type PeriodValues = { preset: string; from: string | null; to: string | null }

const isDay = (value: string | null): value is string => value !== null && isIsoDay(value)

export function periodSchema(def: ParamDef) {
    return z
        .object({ preset: z.string(), from: z.string().nullable(), to: z.string().nullable() })
        .superRefine(({ preset, from, to }, ctx) => {
            // A preset resolves its own days, so a stale typed date never blocks it.
            if (preset !== CUSTOM) return
            if (!isDay(to)) ctx.addIssue({ code: 'custom', path: ['to'], message: DATE_FORMAT_MESSAGE })
            if (def.type !== 'dateRange') return
            if (!isDay(from)) ctx.addIssue({ code: 'custom', path: ['from'], message: DATE_FORMAT_MESSAGE })
            else if (isDay(to) && from > to) ctx.addIssue({ code: 'custom', path: ['from'], message: FROM_AFTER_TO_MESSAGE })
        })
}

export function periodDefaults(value: ParamValue | null, resolved: ResolvedParam | undefined): PeriodValues {
    if (value?.kind === 'preset') {
        const day = typeof resolved?.value === 'string' ? resolved.value : null
        return { preset: value.preset, from: resolved?.from ?? null, to: resolved?.to ?? day }
    }
    if (value?.kind === 'range') return { preset: CUSTOM, from: value.from, to: value.to }
    if (value?.kind === 'day') return { preset: CUSTOM, from: null, to: value.day }
    return { preset: CUSTOM, from: null, to: null }
}

export function periodValueOf(def: ParamDef, values: PeriodValues): ParamValue | null {
    if (values.preset !== CUSTOM) {
        const preset = presetsFor(def).find(p => p === values.preset)
        return preset ? { kind: 'preset', preset } : null
    }
    if (def.type === 'dateRange') return values.from && values.to ? rangeValue(values.from, values.to) : null
    return isDay(values.to) ? { kind: 'day', day: values.to } : null
}

export const textValueSchema = z.object({ text: z.string().trim().min(1, 'Enter some text').max(MAX_TEXT_PARAM) })
export const numberValueSchema = z.object({ number: z.number().refine(Number.isFinite, 'Enter a number') })
export const choicesSchema = z.object({ values: z.array(z.string()).min(1, 'Choose at least one') })
```

- [ ] **Step 5: Write the chips and pickers**

`core/reports/components/ParamChip.tsx`:

```tsx
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Calendar, X } from 'lucide-react-native'
import type { Ref } from 'react'
import { Pressable, Text, View, type ViewInstance } from 'react-native'
import { REPORT_TAP_TARGET } from './ReportHeader'

const TARGET_STYLE = { minHeight: REPORT_TAP_TARGET, marginVertical: -1 }
const REMOVE_STYLE = { ...TARGET_STYLE, minWidth: REPORT_TAP_TARGET }

function RemoveControl({ isVisible, label, testID, onRemove }: { isVisible: boolean; label: string; testID: string; onRemove: () => void }) {
    const muted = useThemeColor('muted-foreground')
    if (!isVisible) return null
    return (
        <Pressable testID={testID} accessibilityRole="button" accessibilityLabel={label} onPress={onRemove} className="items-center justify-center" style={REMOVE_STYLE}>
            <X size={14} color={muted} />
        </Pressable>
    )
}

function ChipIcon({ isVisible }: { isVisible: boolean }) {
    const muted = useThemeColor('muted-foreground')
    if (!isVisible) return null
    return <Calendar size={14} color={muted} />
}

interface ParamChipProps {
    paramId: string
    text: string
    label: string
    isDate: boolean
    isRemovable: boolean
    onPress: () => void
    onRemove: () => void
    /** Set by the picker's Popover, which clones its trigger with an anchor ref. */
    ref?: Ref<ViewInstance>
}

// A chip shows its value at rest (the shell's spark 3). Its label opens the picker;
// a removable chip has its own 44pt remove target.
export function ParamChip({ paramId, text, label, isDate, isRemovable, onPress, onRemove, ref }: ParamChipProps) {
    return (
        <View ref={ref} className={`flex-row items-center gap-1.5 rounded-full border border-border bg-secondary pl-3 ${isRemovable ? '' : 'pr-3'}`} style={{ minHeight: REPORT_TAP_TARGET }}>
            <ChipIcon isVisible={isDate} />
            <Pressable testID={`report-param-${paramId}`} accessibilityRole="button" accessibilityLabel={`${label}: ${text}`} onPress={onPress} className="justify-center" style={TARGET_STYLE}>
                <Text numberOfLines={1} className="text-sm text-secondary-foreground">
                    {text}
                </Text>
            </Pressable>
            <RemoveControl isVisible={isRemovable} label={`Remove ${label} filter`} testID={`report-filter-remove-${paramId}`} onRemove={onRemove} />
        </View>
    )
}
```

`core/reports/components/ParamPeriodControl.tsx`:

```tsx
import { useForm, useWatch, zodResolver } from '@tinycld/core/ui/form'
import { DateField } from '@tinycld/core/ui/forms/fields/DateField'
import { Popover } from '@tinycld/core/ui/popover'
import type { ReactElement } from 'react'
import { Pressable, Text, View } from 'react-native'
import { CUSTOM, periodDefaults, periodSchema, periodValueOf, type PeriodValues } from '../lib/param-forms'
import { CUSTOM_LABEL, type ParamValue, PRESET_LABELS, presetsFor } from '../lib/params'
import type { ParamDef } from '../lib/spec'
import type { ResolvedParam } from '../lib/types'
import { REPORT_TAP_TARGET } from './ReportHeader'

const POPOVER_WIDTH = 360
const PILL_TONE = {
    selected: { box: 'border-primary bg-primary', text: 'text-primary-foreground' },
    idle: { box: 'border-border bg-transparent', text: 'text-foreground' },
}

function PresetPill({ id, label, isSelected, onPress }: { id: string; label: string; isSelected: boolean; onPress: () => void }) {
    const tone = isSelected ? PILL_TONE.selected : PILL_TONE.idle
    return (
        <Pressable testID={`report-period-preset-${id}`} accessibilityRole="radio" accessibilityState={{ checked: isSelected }} aria-checked={isSelected} onPress={onPress} className={`justify-center rounded-full border px-3 ${tone.box}`} style={{ minHeight: REPORT_TAP_TARGET }}>
            <Text className={`text-sm font-medium ${tone.text}`}>{label}</Text>
        </Pressable>
    )
}

function FieldError({ message }: { message: string | undefined }) {
    if (!message) return null
    return (
        <Text testID="report-period-error" role="alert" className="text-xs text-danger">
            {message}
        </Text>
    )
}

function PeriodDate({ label, value, error, testID, onChange }: { label: string; value: string | null; error: string | undefined; testID: string; onChange: (value: string | null) => void }) {
    return (
        <View className="gap-1.5">
            <Text className="text-sm font-semibold text-foreground">{label}</Text>
            <DateField value={value} onChange={onChange} withTime={false} accessibilityLabel={label} testID={testID} />
            <FieldError message={error} />
        </View>
    )
}

interface BodyProps {
    def: ParamDef
    value: ParamValue | null
    resolved: ResolvedParam | undefined
    onApply(value: ParamValue, keepOpen: boolean): void
}

// The form lives while the popover is open, so each open starts from the value shown,
// and closing with a bad typed date discards it.
function OpenPeriodBody({ def, value, resolved, onApply }: BodyProps) {
    const form = useForm<PeriodValues>({ resolver: zodResolver(periodSchema(def)), mode: 'onChange', defaultValues: periodDefaults(value, resolved) })
    const [preset, from, to] = useWatch({ control: form.control, name: ['preset', 'from', 'to'] })
    const errors = form.formState.errors
    const apply = (keepOpen: boolean) =>
        void form.handleSubmit(values => {
            const next = periodValueOf(def, values)
            if (next) onApply(next, keepOpen)
        })()
    const onPreset = (id: string) => {
        form.setValue('preset', id)
        if (id !== CUSTOM) apply(false)
    }
    const onDate = (field: 'from' | 'to', next: string | null) => {
        form.setValue('preset', CUSTOM)
        form.setValue(field, next)
        apply(true)
    }
    const presetPills = presetsFor(def).map(id => (
        <PresetPill key={id} id={id} label={PRESET_LABELS[id]} isSelected={id === preset} onPress={() => onPreset(id)} />
    ))
    const pills = [
        ...presetPills,
        <PresetPill key={CUSTOM} id={CUSTOM} label={CUSTOM_LABEL} isSelected={preset === CUSTOM} onPress={() => onPreset(CUSTOM)} />,
    ]
    const fromField = def.type === 'dateRange' ? <PeriodDate label="From" value={from} error={errors.from?.message} testID="report-period-from" onChange={next => onDate('from', next)} /> : null
    return (
        <View className="gap-3 p-2">
            <View accessibilityRole="radiogroup" className="flex-row flex-wrap gap-2">
                {pills}
            </View>
            {fromField}
            <PeriodDate label={def.type === 'dateRange' ? 'To' : 'As of'} value={to} error={errors.to?.message} testID={def.type === 'dateRange' ? 'report-period-to' : 'report-period-as-of'} onChange={next => onDate('to', next)} />
        </View>
    )
}

function PeriodBody({ isVisible, ...props }: BodyProps & { isVisible: boolean }) {
    if (!isVisible) return null
    return <OpenPeriodBody {...props} />
}

interface ParamPeriodControlProps extends BodyProps {
    trigger: ReactElement
    isOpen: boolean
    onOpenChange(open: boolean): void
}

export function ParamPeriodControl({ trigger, isOpen, onOpenChange, ...body }: ParamPeriodControlProps) {
    return (
        <Popover trigger={trigger} isOpen={isOpen} onOpenChange={onOpenChange} title={body.def.label ?? 'Report period'} sheetSide="bottom" width={POPOVER_WIDTH} testID="report-period-popover">
            <PeriodBody isVisible={isOpen} {...body} />
        </Popover>
    )
}
```


`core/reports/components/ParamPickers.tsx`:

```tsx
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { NumberInput, TextInput, useForm, useWatch, zodResolver } from '@tinycld/core/ui/form'
import { Popover } from '@tinycld/core/ui/popover'
import { Check } from 'lucide-react-native'
import type { ReactElement } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { choicesSchema, numberValueSchema, textValueSchema } from '../lib/param-forms'
import type { ParamValue } from '../lib/params'
import type { ParamDef } from '../lib/spec'
import { REPORT_TAP_TARGET } from './ReportHeader'

const PICKER_WIDTH = 340
const LIST_MAX_HEIGHT = 360

function ApplyButton({ isDisabled, onPress }: { isDisabled: boolean; onPress: () => void }) {
    return (
        <View className="border-t border-border pt-2">
            <Button testID="report-filter-apply" style={{ minHeight: REPORT_TAP_TARGET }} isDisabled={isDisabled} onPress={onPress}>
                <ButtonText>Apply</ButtonText>
            </Button>
        </View>
    )
}

function OptionRow({ value, label, isChecked, isRadio, onPress }: { value: string; label: string; isChecked: boolean; isRadio: boolean; onPress: () => void }) {
    const checkColor = useThemeColor('primary-foreground')
    const box = isChecked ? 'border-primary bg-primary' : 'border-border bg-transparent'
    const check = isChecked ? <Check size={14} color={checkColor} /> : null
    return (
        <Pressable testID={`report-filter-option-${value}`} accessibilityRole={isRadio ? 'radio' : 'checkbox'} accessibilityState={{ checked: isChecked }} aria-checked={isChecked} onPress={onPress} className="flex-row items-center gap-3 rounded-md px-2" style={{ minHeight: REPORT_TAP_TARGET }}>
            <View className={`h-5 w-5 items-center justify-center border ${isRadio ? 'rounded-full' : 'rounded'} ${box}`}>{check}</View>
            <Text numberOfLines={1} className="min-w-0 flex-1 text-sm text-foreground">
                {label}
            </Text>
        </Pressable>
    )
}

interface BodyProps {
    def: ParamDef
    value: ParamValue | null
    onApply(value: ParamValue): void
}

// One choice applies when it is pressed.
function SingleChoiceBody({ def, value, onApply }: BodyProps) {
    const current = value?.kind === 'choice' ? value.values[0] : undefined
    const rows = (def.options ?? []).map(option => (
        <OptionRow key={option.value} value={option.value} label={option.label} isChecked={option.value === current} isRadio onPress={() => onApply({ kind: 'choice', values: [option.value] })} />
    ))
    return <ScrollView style={{ maxHeight: LIST_MAX_HEIGHT }} nestedScrollEnabled>{rows}</ScrollView>
}

// Several choices apply on Apply; an empty selection is not a filter.
function MultiChoiceBody({ def, value, onApply }: BodyProps) {
    const form = useForm({ resolver: zodResolver(choicesSchema), mode: 'onChange', defaultValues: { values: value?.kind === 'choice' ? value.values : [] } })
    const values = useWatch({ control: form.control, name: 'values' })
    const toggle = (option: string) =>
        form.setValue('values', values.includes(option) ? values.filter(v => v !== option) : [...values, option], { shouldValidate: true })
    const rows = (def.options ?? []).map(option => (
        <OptionRow key={option.value} value={option.value} label={option.label} isChecked={values.includes(option.value)} isRadio={false} onPress={() => toggle(option.value)} />
    ))
    const apply = () => void form.handleSubmit(submitted => onApply({ kind: 'choice', values: submitted.values }))()
    return (
        <View className="gap-2">
            <ScrollView style={{ maxHeight: LIST_MAX_HEIGHT }} nestedScrollEnabled>
                {rows}
            </ScrollView>
            <ApplyButton isDisabled={!form.formState.isValid} onPress={apply} />
        </View>
    )
}

function TextBody({ def, value, onApply }: BodyProps) {
    const form = useForm({ resolver: zodResolver(textValueSchema), mode: 'onChange', defaultValues: { text: value?.kind === 'text' ? value.text : '' } })
    const apply = () => void form.handleSubmit(submitted => onApply({ kind: 'text', text: submitted.text }))()
    return (
        <View className="gap-2">
            <TextInput name="text" control={form.control} label={def.label ?? 'Value'} testID="report-filter-value" onSubmitEditing={apply} />
            <ApplyButton isDisabled={!form.formState.isValid} onPress={apply} />
        </View>
    )
}

function NumberBody({ def, value, onApply }: BodyProps) {
    const form = useForm({ resolver: zodResolver(numberValueSchema), mode: 'onChange', defaultValues: { number: value?.kind === 'number' ? value.number : 0 } })
    const apply = () => void form.handleSubmit(submitted => onApply({ kind: 'number', number: submitted.number }))()
    return (
        <View className="gap-2">
            <NumberInput name="number" control={form.control} label={def.label ?? 'Value'} allowDecimal testID="report-filter-value" />
            <ApplyButton isDisabled={!form.formState.isValid} onPress={apply} />
        </View>
    )
}

function PickerBody({ isVisible, ...props }: BodyProps & { isVisible: boolean }) {
    if (!isVisible) return null
    if (props.def.type === 'choice') return props.def.multiple ? <MultiChoiceBody {...props} /> : <SingleChoiceBody {...props} />
    if (props.def.type === 'number') return <NumberBody {...props} />
    return <TextBody {...props} />
}

interface ParamPickerProps extends BodyProps {
    trigger: ReactElement
    isOpen: boolean
    onOpenChange(open: boolean): void
}

// The picker of a choice, text or number param. Closing without Apply changes nothing.
export function ParamPicker({ trigger, isOpen, onOpenChange, ...body }: ParamPickerProps) {
    return (
        <Popover trigger={trigger} isOpen={isOpen} onOpenChange={onOpenChange} title={body.def.label ?? 'Filter'} sheetSide="bottom" width={PICKER_WIDTH} testID="report-filter-picker">
            <PickerBody isVisible={isOpen} {...body} />
        </Popover>
    )
}
```

`core/reports/components/ReportControls.tsx`:

```tsx
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Menu } from '@tinycld/core/ui/menu'
import { Plus } from 'lucide-react-native'
import type { ReactElement } from 'react'
import { Pressable, ScrollView, Text } from 'react-native'
import type { ReportViewModel } from '../hooks/useReportView'
import { chipText, type ControlItem, isDateParam, type ParamValue } from '../lib/params'
import type { ParamDef } from '../lib/spec'
import { ParamChip } from './ParamChip'
import { ParamPeriodControl } from './ParamPeriodControl'
import { ParamPicker } from './ParamPickers'
import { REPORT_TAP_TARGET } from './ReportHeader'

function openParamId(model: ReportViewModel): string | null {
    return model.surface?.kind === 'param' ? model.surface.paramId : null
}

interface PickerProps {
    model: ReportViewModel
    def: ParamDef
    value: ParamValue | null
    trigger: ReactElement
}

// The period control for a date param, the picker for any other.
function ParamSurface({ model, def, value, trigger }: PickerProps) {
    const isOpen = openParamId(model) === def.id
    const onOpenChange = (open: boolean) => {
        if (open) model.onOpenSurface({ kind: 'param', paramId: def.id })
        else model.onCloseSurfaceOf('param')
    }
    if (isDateParam(def)) {
        return (
            <ParamPeriodControl
                trigger={trigger}
                isOpen={isOpen}
                onOpenChange={onOpenChange}
                def={def}
                value={value}
                resolved={model.resolved[def.id]}
                onApply={(next, keepOpen) => model.onApplyParam(def.id, next, keepOpen)}
            />
        )
    }
    return <ParamPicker trigger={trigger} isOpen={isOpen} onOpenChange={onOpenChange} def={def} value={value} onApply={next => model.onApplyParam(def.id, next)} />
}

function ParamControl({ model, def, value, isRemovable }: { model: ReportViewModel; def: ParamDef; value: ParamValue | null; isRemovable: boolean }) {
    const chip = (
        <ParamChip
            paramId={def.id}
            text={chipText(def, value, model.resolved[def.id])}
            label={def.label ?? def.id}
            isDate={isDateParam(def)}
            isRemovable={isRemovable}
            onPress={() => model.onOpenSurface({ kind: 'param', paramId: def.id })}
            onRemove={() => model.onRemoveParam(def.id)}
        />
    )
    return <ParamSurface model={model} def={def} value={value} trigger={chip} />
}

// An element, not a component: the Menu and the Popover clone their trigger.
function addFilterChip(iconColor: string) {
    return (
        <Pressable testID="report-add-filter" accessibilityRole="button" accessibilityLabel="Add filter" className="flex-row items-center gap-1.5 rounded-full border border-border bg-secondary px-3" style={{ minHeight: REPORT_TAP_TARGET }}>
            <Plus size={14} color={iconColor} />
            <Text numberOfLines={1} className="text-sm text-secondary-foreground">
                Add filter
            </Text>
        </Pressable>
    )
}

// The picker of a param that is not set yet anchors on the "Add filter" chip, which
// then stands in for the menu that opened it.
function AddFilterControl({ model, defs }: { model: ReportViewModel; defs: ParamDef[] }) {
    const muted = useThemeColor('muted-foreground')
    const trigger = addFilterChip(muted)
    const picking = defs.find(def => def.id === openParamId(model))
    if (picking) return <ParamSurface model={model} def={picking} value={null} trigger={trigger} />
    const onOpenChange = (open: boolean) => {
        if (open) model.onOpenSurface({ kind: 'add-filter' })
        else model.onCloseSurfaceOf('add-filter')
    }
    const items = defs.map(def => (
        <Menu.Item key={def.id} label={def.label ?? def.id} testID={`report-add-filter-${def.id}`} onSelect={() => model.onOpenSurface({ kind: 'param', paramId: def.id })} />
    ))
    return (
        <Menu trigger={trigger} isOpen={model.surface?.kind === 'add-filter'} onOpenChange={onOpenChange} title="Add filter" sheetSide="bottom" testID="report-add-filter-menu">
            {items}
        </Menu>
    )
}

function Control({ item, model }: { item: ControlItem; model: ReportViewModel }) {
    if (item.kind === 'add-filter') return <AddFilterControl model={model} defs={item.defs} />
    return <ParamControl model={model} def={item.def} value={item.value} isRemovable={item.isRemovable} />
}

// One line of chips under the header (direction C): it scrolls sideways on a phone
// and never wraps.
export function ReportControls({ model }: { model: ReportViewModel }) {
    if (model.controls.length === 0) return null
    const controls = model.controls.map(item => (
        <Control key={item.kind === 'param' ? item.def.id : 'add-filter'} item={item} model={model} />
    ))
    return (
        <ScrollView horizontal testID="report-controls" showsHorizontalScrollIndicator={false} style={{ flexGrow: 0, flexShrink: 0 }} contentContainerClassName="flex-row items-center gap-2 px-4 py-2">
            {controls}
        </ScrollView>
    )
}
```

`core/reports/components/ReportNotices.tsx`:

```tsx
import { Text, View } from 'react-native'
import type { Notice } from '../lib/types'

function Updating({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text testID="report-updating" className="text-xs text-muted-foreground">
            Updating…
        </Text>
    )
}

// What the run says about itself: caps that applied, mixed currencies, an upgraded
// spec, renamed fields (spec "Errors": a notice above the table).
export function ReportNotices({ notices, isUpdating }: { notices: Notice[]; isUpdating: boolean }) {
    if (notices.length === 0 && !isUpdating) return null
    const lines = notices.map(notice => (
        <Text key={notice.code} testID={`report-notice-${notice.code}`} className="text-sm text-foreground">
            {notice.message}
        </Text>
    ))
    return (
        <View testID="report-notices" className="gap-1 border-t border-border bg-surface-secondary px-4 py-2">
            {lines}
            <Updating isVisible={isUpdating} />
        </View>
    )
}
```

`core/reports/components/ReportCustomizePanel.tsx`:

```tsx
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { Drawer, DrawerBackdrop, DrawerBody, DrawerCloseButton, DrawerContent, DrawerHeader } from '@tinycld/core/ui/drawer'
import { ButtonGroup, Toggle, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { Sheet } from '@tinycld/core/ui/sheet'
import { ChevronDown, ChevronRight, X } from 'lucide-react-native'
import { type ReactNode, useState } from 'react'
import { Pressable, Text, View } from 'react-native'
import type { ReportViewModel } from '../hooks/useReportView'
import { REPORT_TAP_TARGET } from './ReportHeader'

const NEGATIVE_OPTIONS = [
    { value: 'minus', label: '-100' },
    { value: 'parens', label: '(100)' },
]
const CLOSE_TARGET = { width: REPORT_TAP_TARGET, height: REPORT_TAP_TARGET }

const displaySchema = z.object({
    divideBy1000: z.boolean(),
    withoutCents: z.boolean(),
    negative: z.enum(['minus', 'parens']),
    negativeRed: z.boolean(),
    columns: z.array(z.object({ id: z.string(), label: z.string(), isShown: z.boolean() })),
})
type DisplayValues = z.infer<typeof displaySchema>

// Customize is a form over the URL's display options: `values` keeps it in step with
// the URL, and every change writes the URL at once (live apply, no Save).
function useDisplayForm(model: ReportViewModel) {
    const values: DisplayValues = {
        divideBy1000: model.display.divideBy1000,
        withoutCents: model.display.withoutCents,
        negative: model.display.negative,
        negativeRed: model.display.negativeRed,
        columns: model.allColumns.slice(1).map(column => ({ id: column.id, label: column.label, isShown: !model.display.hidden.includes(column.id) })),
    }
    const form = useForm<DisplayValues>({ resolver: zodResolver(displaySchema), values })
    const write = () =>
        void form.handleSubmit(next =>
            model.onDisplayChange({
                divideBy1000: next.divideBy1000,
                withoutCents: next.withoutCents,
                negative: next.negative,
                negativeRed: next.negativeRed,
                hidden: next.columns.filter(column => !column.isShown).map(column => column.id),
            })
        )()
    return { control: form.control, columns: values.columns, write }
}
type DisplayForm = ReturnType<typeof useDisplayForm>

function PanelSection({ id, title, isOpenAtStart, children }: { id: string; title: string; isOpenAtStart: boolean; children: ReactNode }) {
    // The fold state is this panel's own: nothing else reads it.
    const [isOpen, setIsOpen] = useState(isOpenAtStart)
    const color = useThemeColor('muted-foreground')
    const Caret = isOpen ? ChevronDown : ChevronRight
    const body = isOpen ? <View className="pb-2">{children}</View> : null
    return (
        <View className="border-b border-border">
            <Pressable testID={`report-customize-section-${id}`} accessibilityRole="button" aria-expanded={isOpen} onPress={() => setIsOpen(open => !open)} className="flex-row items-center gap-2" style={{ minHeight: REPORT_TAP_TARGET }}>
                <Caret size={16} color={color} />
                <Text className="text-sm font-semibold text-foreground">{title}</Text>
            </Pressable>
            {body}
        </View>
    )
}

function NumberFormatFields({ form }: { form: DisplayForm }) {
    return (
        <>
            <Toggle name="divideBy1000" control={form.control} label="Divide by 1000" testID="report-option-divide-by-1000" onValueChange={form.write} />
            <Toggle name="withoutCents" control={form.control} label="Without cents" testID="report-option-without-cents" onValueChange={form.write} />
            <ButtonGroup name="negative" control={form.control} label="Negative numbers" horizontal options={NEGATIVE_OPTIONS} onValueChange={form.write} />
            <Toggle name="negativeRed" control={form.control} label="Show negative numbers in red" testID="report-option-negative-red" onValueChange={form.write} />
        </>
    )
}

function ColumnFields({ form }: { form: DisplayForm }) {
    const toggles = form.columns.map((column, index) => (
        <Toggle key={column.id} name={`columns.${index}.isShown`} control={form.control} label={column.label} testID={`report-option-column-${index}`} onValueChange={form.write} />
    ))
    return <>{toggles}</>
}

function CustomizeFields({ model }: { model: ReportViewModel }) {
    const form = useDisplayForm(model)
    return (
        <View>
            <PanelSection id="number-format" title="Number format" isOpenAtStart>
                <NumberFormatFields form={form} />
            </PanelSection>
            <PanelSection id="columns" title="Columns" isOpenAtStart={false}>
                <ColumnFields form={form} />
            </PanelSection>
            <View className="items-start pt-3">
                <Button testID="report-customize-reset" variant="outline" style={{ minHeight: REPORT_TAP_TARGET }} onPress={model.onResetDisplay}>
                    <ButtonText>Reset to defaults</ButtonText>
                </Button>
            </View>
        </View>
    )
}

function PanelTitle() {
    return <Text className="text-lg font-semibold text-foreground">Customize</Text>
}

function CloseButton({ onPress }: { onPress: () => void }) {
    const color = useThemeColor('foreground')
    return (
        <Pressable testID="report-customize-close" accessibilityRole="button" accessibilityLabel="Close" onPress={onPress} className="items-center justify-center" style={CLOSE_TARGET}>
            <X size={18} color={color} />
        </Pressable>
    )
}

function CustomizeSheet({ model }: { model: ReportViewModel }) {
    return (
        <Sheet isOpen={model.isCustomizeOpen} onClose={model.onCloseCustomize} testID="report-customize">
            <Sheet.Body>
                <View className="flex-row items-center justify-between">
                    <PanelTitle />
                    <CloseButton onPress={model.onCloseCustomize} />
                </View>
                <CustomizeFields model={model} />
            </Sheet.Body>
        </Sheet>
    )
}

// Wide: inline beside the report, which updates behind it. Tablet: an overlay.
function CustomizeDrawer({ model }: { model: ReportViewModel }) {
    const closeColor = useThemeColor('foreground')
    const isInline = model.breakpoint === 'desktop'
    return (
        <Drawer isOpen={model.isCustomizeOpen} onClose={model.onCloseCustomize} presentation={isInline ? 'inline' : 'overlay'} anchor="right" size="md">
            <DrawerBackdrop />
            <DrawerContent testID="report-customize" role={isInline ? 'complementary' : 'dialog'} accessibilityLabel="Customize" className={isInline ? 'w-[360px]' : undefined}>
                <DrawerHeader className="border-b border-border">
                    <PanelTitle />
                    <DrawerCloseButton testID="report-customize-close" accessibilityLabel="Close" className="items-center justify-center" style={CLOSE_TARGET}>
                        <X size={18} color={closeColor} />
                    </DrawerCloseButton>
                </DrawerHeader>
                <DrawerBody style={{ flexShrink: 1 }}>
                    <CustomizeFields model={model} />
                </DrawerBody>
            </DrawerContent>
        </Drawer>
    )
}

export function ReportCustomizePanel({ model }: { model: ReportViewModel }) {
    if (model.isEditing) return null
    if (model.breakpoint === 'mobile') return <CustomizeSheet model={model} />
    return <CustomizeDrawer model={model} />
}
```

- [ ] **Step 6: Put them on the view**

In `core/reports/components/ReportView.tsx`:

Add imports `ReportControls`, `ReportNotices`, `ReportCustomizePanel`.

In `toolbarItems`, after the fold button, add:

```tsx
    if (!model.isEditing) items.push(toolbarButton('customize', 'Customize', 'report-customize-open', model.onOpenCustomize))
```

and at the end, before `return items`:

```tsx
    if (model.onCopyLink) items.push(toolbarButton('copy-link', 'Copy link', 'report-copy-link', model.onCopyLink))
```

In `ReportView`, render the controls and notices between the header and the document header, and the panel beside the column:

```tsx
        <View testID="report-screen" className="flex-1 flex-row bg-background">
            <View className="min-w-0 flex-1">
                <ReportHeader title={model.title} onBack={model.onBack} toolbar={toolbar} />
                <ReportControls model={model} />
                <ReportNotices notices={model.notices} isUpdating={model.isUpdating} />
                <ReportDocumentHeader orgName={model.orgName} title={model.title} periodWords={model.periodWords} isThousands={model.display.divideBy1000} />
                <ReportBody model={model} />
            </View>
            <ReportCustomizePanel model={model} />
            {side}
        </View>
```

- [ ] **Step 7: Run the unit tests, format, lint, typecheck**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run reports && cd .. && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome format --write core/reports tests/e2e/reports-view.spec.ts && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Expected: all pass.

- [ ] **Step 8: Run the e2e spec**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-view.spec.ts`
Expected: 5 passed.

- [ ] **Step 9: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/reports tests/e2e/reports-view.spec.ts
git commit -m "feat(core): report view params, Customize, notices, copy link and live runs"
```

---

### Task 18: Hub actions, sharing, publishing, Save as, lost access and broken reports

**Files:**
- Create: `core/reports/hooks/useHubActions.ts`, `core/reports/hooks/useReportShares.ts`
- Create: `core/reports/components/hub/HubRowMenu.tsx`, `core/reports/components/share/ReportShareDialog.tsx`
- Modify: `core/reports/components/hub/ReportsHub.tsx` (row menus, package banner, share dialog)
- Modify: `core/reports/hooks/useReportView.ts` (`onShare`, `onSaveAs` in the input and the model)
- Modify: `core/reports/components/ReportView.tsx` (Share and Save as in the toolbar)
- Modify: `core/reports/screens/ReportScreen.tsx` (`SavedReport`: share dialog and the Save as prompt)
- Modify: `tests/e2e/reports-helpers.ts` (`superuserClient` for the one seeded fixture)
- Create: `tests/e2e/reports-share.spec.ts`

**Interfaces:**
- Consumes: `useReportRecord` (Task 10), `reportAccess`, `SHARE_ROLES`, `ShareRole` (Task 5), `useReportUIStore.openShare/closeShare` (Task 9), `copyTitle` (Task 13), core `useGroupGrants` (`@tinycld/core/lib/groups/use-group-grants`), `GroupShareSection` (`@tinycld/core/components/groups/GroupShareSection`), `Dialog`, `Menu`, `PickMenu`, `PlainInput`, `Switch`, `ConfirmDialog`, `PromptDialog`, `useToastStore`, `errorToString`.
- Produces: `useHubActions()` → `{ duplicate(report), saveCopy(report, title), remove(report), setPublished(report, published), isPending }`; `useReportShares(reportId)` → `{ title, owner, published, access, direct, groups, candidates, search, setSearch, role, setRole, add(userId), changeRole(shareId, role), remove(shareId), setPublished(published) }`; `ReportShareDialog()` (reads `shareReportId` from the store); `HubRowMenu({ row, actions, onOpen })`; `ReportViewInput.onShare?`, `ReportViewInput.onSaveAs?`; test ids `reports-hub-row-menu-<id>`, `reports-hub-action-open|duplicate|share|publish|unpublish|delete`, `reports-hub-filter`, `reports-hub-show-all`, `report-share-open`, `report-save-as`, `report-share-dialog`, `report-share-search`, `report-share-role`, `report-share-candidate-<userId>`, `report-share-row-<shareId>`, `report-share-row-role-<shareId>`, `report-share-row-remove-<shareId>`, `report-share-publish`, `report-share-note`, `report-share-done`.

- [ ] **Step 1: Add the superuser helper to the e2e helpers**

Add `import PocketBase from 'pocketbase'` to the imports at the top of `tests/e2e/reports-helpers.ts`, then append:

```ts
// A PocketBase client signed in as the e2e server's superuser, for the one fixture
// reports-share.spec.ts seeds (its header says why). scripts/e2e-serve.ts creates
// this superuser from the same variables and defaults.
export async function superuserClient(): Promise<PocketBase> {
    const pb = new PocketBase(`http://127.0.0.1:${process.env.E2E_PORT ?? 7200}`)
    pb.autoCancellation(false)
    await pb.collection('_superusers').authWithPassword(process.env.ADMIN_USER_LOGIN || 'admin@tinycld.org', process.env.ADMIN_USER_PW || 'AdminPass1234!')
    return pb
}
```


- [ ] **Step 2: Write the failing spec**

`tests/e2e/reports-share.spec.ts`:

```ts
import { expect, type Page, test } from '@playwright/test'
import { login, signInAsCollaborator, skipWithoutReportsStub, TEST_COLLABORATOR_NAME, TEST_USER_USERNAME } from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    chooseOption,
    hubRow,
    openReportStub,
    openReportsHub,
    pressToolbar,
    SALES_SOURCE,
    saveNewReport,
    startReport,
    superuserClient,
    uniqueName,
} from './reports-helpers'

/**
 * Sharing, publishing and the hub's actions, against the salesstub package. The
 * seeded owner (an admin) shares and publishes; the seeded collaborator (a member)
 * receives. Each adds a sale whose customer is unique to the test, so "sees only
 * their data" is checked by name: salesstub_sales lets each user read only their
 * own rows.
 *
 * One fixture is seeded by a superuser write (the CLAUDE.md exception): a report
 * whose source no longer exists. A report becomes broken when a package is removed
 * or a source is renamed without an alias and the server restarts, which CI cannot
 * do. The test therefore covers the hub's Broken badge and the report's
 * missing-parts state, not the start-up check that writes the mark (Go tests cover
 * that: broken_test.go).
 */
async function ownerReport(page: Page, customer: string) {
    await login(page)
    const team = uniqueName('Team')
    await openReportStub(page)
    await addTeam(page, team)
    await addSale(page, { team, customer, amount: 1000, cost: 0, day: '2026-01-05' })
    await openReportsHub(page)
    await startReport(page, SALES_SOURCE)
    await addColumn(page, ['amount'])
    await chooseOption(page, 'report-builder-column-1-aggregate', 'sum')
    const title = uniqueName('Shared report')
    await saveNewReport(page, title)
    return title
}

async function openShare(page: Page) {
    await pressToolbar(page, 'report-share-open', 'Share')
    await expect(page.getByTestId('report-share-dialog')).toBeVisible()
}

test.describe('sharing reports', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a viewer sees only their own rows, cannot edit, saves a copy, and loses a removed share at once', async ({ page }) => {
        const ownerCustomer = uniqueName('Owner customer')
        const title = await ownerReport(page, ownerCustomer)
        await openShare(page)
        await expect(page.getByTestId('report-share-note')).toHaveText('Each person sees only the data they can access.')
        await page.getByTestId('report-share-search').fill(TEST_COLLABORATOR_NAME.slice(0, 6))
        await page.getByTestId('report-share-dialog').locator('[data-testid^="report-share-candidate-"]').filter({ hasText: TEST_COLLABORATOR_NAME }).click()
        const shareRow = page.getByTestId('report-share-dialog').locator('[data-testid^="report-share-row-"]').filter({ hasText: TEST_COLLABORATOR_NAME })
        await expect(shareRow).toContainText('Viewer')
        await page.getByTestId('report-share-done').click()

        const { page: c, close } = await signInAsCollaborator(page)
        try {
            const collabCustomer = uniqueName('Collab customer')
            const team = uniqueName('Collab team')
            await openReportStub(c)
            await addTeam(c, team)
            await addSale(c, { team, customer: collabCustomer, amount: 500, cost: 0, day: '2026-01-06' })
            await openReportsHub(c)
            await c.getByTestId('reports-hub-tab-shared').click()
            await hubRow(c, title).click()
            const table = c.getByTestId('report-table')
            await expect(table.getByText(collabCustomer, { exact: true })).toBeVisible()
            await expect(table.getByText(ownerCustomer, { exact: true })).toHaveCount(0)
            await expect(c.getByTestId('report-edit-open')).toHaveCount(0)

            // Save as keeps a copy the viewer owns and can edit.
            await pressToolbar(c, 'report-save-as', 'Save as…')
            await c.getByLabel('Save as', { exact: true }).fill(`${title} copy`)
            await c.getByLabel('Save as', { exact: true }).press('Enter')
            await expect(c.getByTestId('report-title')).toHaveText(`${title} copy`)
            await expect(c.getByTestId('report-edit-open')).toBeVisible()

            // Back goes to the shared report the copy was made from; the owner removes the share.
            await c.getByTestId('report-back').click()
            await expect(c.getByTestId('report-title')).toHaveText(title)
            await expect(c.getByTestId('report-table')).toBeVisible()
            await openShare(page)
            await shareRow.locator('[data-testid^="report-share-row-remove-"]').click()
            await expect(shareRow).toHaveCount(0)
            await expect(c.getByTestId('report-no-access')).toHaveText('You no longer have access to this report')
        } finally {
            await close()
        }
    })

    test('an admin publishes a report to every member, and unpublishes it from the hub', async ({ page }) => {
        const title = await ownerReport(page, uniqueName('Published customer'))
        await openShare(page)
        await page.getByTestId('report-share-publish').click()
        await page.getByTestId('report-share-done').click()

        const { page: c, close } = await signInAsCollaborator(page)
        try {
            await openReportsHub(c)
            await c.getByTestId('reports-hub-tab-published').click()
            await expect(hubRow(c, title)).toBeVisible()

            await page.getByTestId('report-back').click()
            const row = hubRow(page, title)
            await row.locator('[data-testid^="reports-hub-row-menu-"]').click()
            await page.getByTestId('reports-hub-action-unpublish').click()
            await expect(hubRow(c, title)).toHaveCount(0)
        } finally {
            await close()
        }
    })

    test('Duplicate opens a copy, and Delete removes it after a confirm', async ({ page }) => {
        const title = await ownerReport(page, uniqueName('Copied customer'))
        await page.getByTestId('report-back').click()
        await hubRow(page, title).first().locator('[data-testid^="reports-hub-row-menu-"]').click()
        await page.getByTestId('reports-hub-action-duplicate').click()
        await expect(page.getByTestId('report-title')).toHaveText(`Copy of ${title}`)
        await page.getByTestId('report-back').click()
        const copy = hubRow(page, `Copy of ${title}`)
        await copy.locator('[data-testid^="reports-hub-row-menu-"]').click()
        await page.getByTestId('reports-hub-action-delete').click()
        await page.getByText('Delete', { exact: true }).last().click()
        await expect(copy).toHaveCount(0)
    })

    test('a report whose source is gone is marked broken and names what is missing', async ({ page }) => {
        const pb = await superuserClient()
        const owner = await pb.collection('users').getFirstListItem(pb.filter('username = {:name}', { name: TEST_USER_USERNAME }))
        const title = uniqueName('Broken report')
        const seeded = await pb.collection('reports').create({
            owner: owner.id,
            title,
            spec: { version: 1, source: 'salesstub.gone', columns: [{ id: 'name', path: [{ field: 'name' }] }] },
            source: 'salesstub.gone',
            broken: { status: 422, problems: { source: { code: 'unknown_source', message: 'the source "salesstub.gone" does not exist' } } },
        })
        try {
            await login(page)
            await openReportsHub(page)
            const row = hubRow(page, title)
            await expect(row.locator('[data-testid^="reports-hub-broken-"]')).toHaveText('Broken')
            await row.click()
            await expect(page.getByTestId('report-missing')).toContainText('the source "salesstub.gone" does not exist')
        } finally {
            await pb.collection('reports').delete(seeded.id)
        }
    })

    test('a package link shows only that package’s reports and sources', async ({ page }) => {
        await login(page, { startAt: '/a/reports?package=salesstub' })
        await expect(page.getByTestId('reports-hub-filter')).toContainText('Sales Stub')
        await page.getByTestId('reports-hub-new').click()
        await expect(page.getByTestId('reports-new-dialog').locator('[data-testid^="reports-new-source-"]')).toHaveCount(2)
        await page.keyboard.press('Escape')
        await page.getByTestId('reports-hub-show-all').click()
        await expect(page.getByTestId('reports-hub-filter')).toHaveCount(0)
    })
})
```

- [ ] **Step 3: Run it to see it fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-share.spec.ts`
Expected: FAIL: `report-share-open` and `reports-hub-row-menu-*` are not found (the broken-report test may already pass).

- [ ] **Step 4: Write the hub actions and the row menu**

`core/reports/hooks/useHubActions.ts`:

```ts
import { useAuth } from '@tinycld/core/lib/auth'
import { errorToString } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useToastStore } from '@tinycld/core/lib/stores/toast-store'
import type { Reports } from '@tinycld/core/types/pbSchema'
import { router } from 'expo-router'
import { newRecordId } from 'pbtsdb'
import { copyTitle } from './useReportBuilder'

function toastError(title: string, error: unknown) {
    useToastStore.getState().addToast({ title, body: errorToString(error), variant: 'error', duration: 6000 })
}

// The hub's and the report's actions on a saved report. Duplicate and Save as are a
// plain create with the stored spec (the sharing contracts); the server refuses what
// the reader may not do, and says why in a toast.
export function useHubActions() {
    const { user } = useAuth()
    const orgHref = useOrgHref()
    const [reportsCollection] = useStore('reports')
    const copy = useMutation({
        mutationFn: function* ({ report, title }: { report: Reports; title: string }) {
            const id = newRecordId()
            yield reportsCollection.insert({ id, owner: user.id, title, description: report.description, spec: report.spec, published: false })
            return id
        },
        onSuccess: id => router.push(orgHref('reports/[id]', { id })),
        onError: error => toastError('Could not copy the report', error),
    })
    const remove = useMutation({
        mutationFn: function* (report: Reports) {
            yield reportsCollection.delete(report.id)
        },
        onError: error => toastError('Could not delete the report', error),
    })
    const publish = useMutation({
        mutationFn: function* ({ report, published }: { report: Reports; published: boolean }) {
            yield reportsCollection.update(report.id, row => {
                row.published = published
            })
        },
        onError: error => toastError('Could not change who sees the report', error),
    })
    return {
        duplicate: (report: Reports) => copy.mutate({ report, title: copyTitle(report.title) }),
        saveCopy: (report: Reports, title: string) => copy.mutate({ report, title }),
        remove: (report: Reports) => remove.mutate(report),
        setPublished: (report: Reports, published: boolean) => publish.mutate({ report, published }),
        isPending: copy.isPending || remove.isPending || publish.isPending,
    }
}

export type HubActions = ReturnType<typeof useHubActions>
```

`core/reports/components/hub/HubRowMenu.tsx`:

```tsx
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ConfirmDialog } from '@tinycld/core/ui/ConfirmDialog'
import { Menu } from '@tinycld/core/ui/menu'
import { EllipsisVertical } from 'lucide-react-native'
import { type ReactNode, useState } from 'react'
import { type GestureResponderEvent, Pressable } from 'react-native'
import type { HubActions } from '../../hooks/useHubActions'
import type { HubRow } from '../../hooks/useReportsHub'
import { useReportUIStore } from '../../stores/report-ui-store'

function swallow(event: GestureResponderEvent) {
    event.stopPropagation()
}

// The menu's content is portaled, but React still bubbles its presses to the row's
// Pressable, which would open the report behind the action just chosen.
function PressBoundary({ children }: { children: ReactNode }) {
    return (
        <Pressable accessible={false} focusable={false} onPress={swallow}>
            {children}
        </Pressable>
    )
}

interface MenuItemSpec {
    id: string
    label: string
    isVisible: boolean
    isDestructive?: boolean
    onSelect: () => void
}

// Each action shows only when the reader may take it (R13); a shipped report offers
// Unpublish instead of Delete.
export function HubRowMenu({ row, actions, onOpen }: { row: HubRow; actions: HubActions; onOpen: () => void }) {
    const color = useThemeColor('muted-foreground')
    const openShare = useReportUIStore(s => s.openShare)
    // The delete confirm is a modal toggle: local UI state.
    const [isConfirmingDelete, setConfirmingDelete] = useState(false)
    const { report, access } = row
    const specs: MenuItemSpec[] = [
        { id: 'open', label: 'Open', isVisible: true, onSelect: onOpen },
        { id: 'duplicate', label: 'Duplicate', isVisible: true, onSelect: () => actions.duplicate(report) },
        { id: 'share', label: 'Share', isVisible: access.canShare, onSelect: () => openShare(report.id) },
        { id: 'publish', label: 'Publish to everyone', isVisible: access.canPublish && !report.published, onSelect: () => actions.setPublished(report, true) },
        { id: 'unpublish', label: 'Unpublish', isVisible: access.canPublish && report.published, onSelect: () => actions.setPublished(report, false) },
        { id: 'delete', label: 'Delete', isVisible: access.canDelete, isDestructive: true, onSelect: () => setConfirmingDelete(true) },
    ]
    const items = specs
        .filter(spec => spec.isVisible)
        .map(spec => <Menu.Item key={spec.id} testID={`reports-hub-action-${spec.id}`} label={spec.label} isDestructive={spec.isDestructive} onSelect={spec.onSelect} />)
    return (
        <PressBoundary>
            <Menu
                title={report.title}
                placement="bottom-end"
                sheetSide="bottom"
                trigger={
                    <Pressable testID={`reports-hub-row-menu-${report.id}`} accessibilityRole="button" accessibilityLabel={`Actions for ${report.title}`} onPress={swallow} className="items-center justify-center" style={{ width: 44, minHeight: 44 }}>
                        <EllipsisVertical size={18} color={color} />
                    </Pressable>
                }
            >
                {items}
            </Menu>
            <ConfirmDialog
                isOpen={isConfirmingDelete}
                onClose={() => setConfirmingDelete(false)}
                onConfirm={() => {
                    setConfirmingDelete(false)
                    actions.remove(report)
                }}
                title={`Delete “${report.title}”?`}
                message="Everyone it is shared with loses it. This cannot be undone."
                confirmLabel="Delete"
                isDestructive
            />
        </PressBoundary>
    )
}
```

- [ ] **Step 5: Write the share hook and dialog**

`core/reports/hooks/useReportShares.ts`:

```ts
import { and, eq, like } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { errorToString } from '@tinycld/core/lib/errors'
import { useGroupGrants } from '@tinycld/core/lib/groups/use-group-grants'
import { useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useToastStore } from '@tinycld/core/lib/stores/toast-store'
import { newRecordId } from 'pbtsdb'
import { useState } from 'react'
import { SHARE_ROLES, type ShareRole } from '../lib/access'
import { useReportRecord } from './useReportRecord'

const CANDIDATE_LIMIT = 20

function toastError(error: unknown) {
    useToastStore.getState().addToast({ title: 'Could not change sharing', body: errorToString(error), variant: 'error', duration: 6000 })
}

// People the report can be shared with, searched on the server by name prefix. A
// guest may receive a share (the sharing contracts), so roles are not filtered.
function useShareCandidates(search: string, exclude: ReadonlySet<string>) {
    const [usersCollection] = useStore('users')
    // A backslash would break PocketBase's filter literal; it means nothing in a name search.
    const term = search.trim().replace(/\\/g, '')
    const { data } = useLiveQuery({
        queryKey: ['reports.share-candidates', term],
        query: q =>
            term
                ? q
                      .from({ u: usersCollection })
                      .where(({ u }) => and(eq(u.disabled, false), like(u.name, `${term}%`)))
                      .orderBy(({ u }) => u.name)
                      .limit(CANDIDATE_LIMIT)
                      .select(({ u }) => ({ id: u.id, name: u.name }))
                : null,
    })
    return (data ?? []).filter(candidate => !exclude.has(candidate.id))
}

// The share dialog's data: direct shares (a user, no group), group grants through
// core's group share section, the people search and the publish switch. Each
// change is written at once; a refusal is a toast.
export function useReportShares(reportId: string) {
    const record = useReportRecord(reportId)
    const [sharesCollection, reportsCollection] = useStore('report_shares', 'reports')
    const { data: direct } = useLiveQuery({
        queryKey: ['reports.shares.direct', reportId],
        query: q => q.from({ share: sharesCollection }).where(({ share }) => and(eq(share.report, reportId), eq(share.group, ''))),
    })
    const groups = useGroupGrants({
        collection: sharesCollection,
        roles: SHARE_ROLES,
        isForResource: row => row.report === reportId,
        buildRow: grant => ({ ...grant, report: reportId }),
    })
    // The search box's text and the role a pick adds: local UI state.
    const [search, setSearch] = useState('')
    const [role, setRole] = useState<ShareRole>('viewer')
    const directRows = direct ?? []
    const exclude = new Set([record.record?.owner ?? '', ...directRows.map(row => row.user)])
    const candidates = useShareCandidates(search, exclude)
    const add = useMutation({
        mutationFn: function* (userId: string) {
            yield sharesCollection.insert({ id: newRecordId(), report: reportId, user: userId, group: '', role })
        },
        onSuccess: () => setSearch(''),
        onError: toastError,
    })
    const changeRole = useMutation({
        mutationFn: function* ({ shareId, next }: { shareId: string; next: ShareRole }) {
            yield sharesCollection.update(shareId, row => {
                row.role = next
            })
        },
        onError: toastError,
    })
    const remove = useMutation({
        mutationFn: function* (shareId: string) {
            yield sharesCollection.delete(shareId)
        },
        onError: toastError,
    })
    const publish = useMutation({
        mutationFn: function* (published: boolean) {
            yield reportsCollection.update(reportId, row => {
                row.published = published
            })
        },
        onError: toastError,
    })
    return {
        title: record.record?.title ?? '',
        owner: record.record?.owner ?? '',
        published: record.record?.published === true,
        access: record.access,
        direct: directRows,
        groups,
        candidates,
        search,
        setSearch,
        role,
        setRole,
        add: (userId: string) => add.mutate(userId),
        changeRole: (shareId: string, next: ShareRole) => changeRole.mutate({ shareId, next }),
        remove: (shareId: string) => remove.mutate(shareId),
        setPublished: (published: boolean) => publish.mutate(published),
    }
}

export type ReportShares = ReturnType<typeof useReportShares>
```


`core/reports/components/share/ReportShareDialog.tsx`:

```tsx
import { GroupShareSection } from '@tinycld/core/components/groups/GroupShareSection'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useRecord } from '@tinycld/core/lib/use-record'
import { Dialog } from '@tinycld/core/ui/dialog'
import { type PickItem, PickMenu } from '@tinycld/core/ui/pick-menu'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { Switch } from '@tinycld/core/ui/switch'
import { Trash2 } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { type ReportShares, useReportShares } from '../../hooks/useReportShares'
import { SHARE_ROLES, type ShareRole } from '../../lib/access'
import { useReportUIStore } from '../../stores/report-ui-store'
import { REPORT_TAP_TARGET } from '../ReportHeader'

function roleLabel(role: ShareRole): string {
    return SHARE_ROLES.find(r => r.value === role)?.label ?? role
}

function roleItems(current: ShareRole, onPick: (role: ShareRole) => void): PickItem[] {
    return SHARE_ROLES.map(r => ({ id: r.value, label: r.label, isSelected: r.value === current, onSelect: () => onPick(r.value) }))
}

function UserName({ userId, className }: { userId: string; className: string }) {
    const [users] = useStore('users')
    const { record } = useRecord(users, userId)
    return (
        <Text numberOfLines={1} className={className}>
            {record?.name ?? ''}
        </Text>
    )
}

function SearchRow({ share }: { share: ReportShares }) {
    const muted = useThemeColor('muted-foreground')
    return (
        <View className="flex-row items-center gap-2 px-5 pb-3">
            <PlainInput
                value={share.search}
                onChangeText={share.setSearch}
                placeholder="Add people by name"
                placeholderTextColor={muted}
                accessibilityLabel="Add people by name"
                testID="report-share-search"
                className="flex-1 rounded-md border border-border px-3 py-2 text-sm text-foreground"
            />
            <PickMenu label={roleLabel(share.role)} purpose="Role for people you add" items={roleItems(share.role, share.setRole)} testID="report-share-role" />
        </View>
    )
}

function Candidates({ share }: { share: ReportShares }) {
    if (share.candidates.length === 0) return null
    const rows = share.candidates.map(candidate => (
        <Pressable key={candidate.id} testID={`report-share-candidate-${candidate.id}`} accessibilityRole="button" onPress={() => share.add(candidate.id)} className="justify-center rounded-md px-2" style={{ minHeight: REPORT_TAP_TARGET }}>
            <Text className="text-sm text-foreground">{candidate.name}</Text>
        </Pressable>
    ))
    return <View className="gap-1 pb-3">{rows}</View>
}

function DirectRow({ share, row }: { share: ReportShares; row: ReportShares['direct'][number] }) {
    const color = useThemeColor('muted-foreground')
    return (
        <View testID={`report-share-row-${row.id}`} className="flex-row items-center gap-2" style={{ minHeight: REPORT_TAP_TARGET }}>
            <UserName userId={row.user} className="min-w-0 flex-1 text-sm text-foreground" />
            <PickMenu label={roleLabel(row.role)} purpose="Role" items={roleItems(row.role, next => share.changeRole(row.id, next))} testID={`report-share-row-role-${row.id}`} />
            <Pressable testID={`report-share-row-remove-${row.id}`} accessibilityRole="button" accessibilityLabel="Remove access" onPress={() => share.remove(row.id)} className="items-center justify-center" style={{ width: REPORT_TAP_TARGET, height: REPORT_TAP_TARGET }}>
                <Trash2 size={16} color={color} />
            </Pressable>
        </View>
    )
}

function People({ share }: { share: ReportShares }) {
    const rows = share.direct.map(row => <DirectRow key={row.id} share={share} row={row} />)
    return (
        <View className="gap-1">
            <Text className="text-xs font-semibold uppercase text-muted-foreground">People with access</Text>
            <View className="flex-row items-center gap-2" style={{ minHeight: REPORT_TAP_TARGET }}>
                <UserName userId={share.owner} className="min-w-0 flex-1 text-sm text-foreground" />
                <Text className="text-sm text-muted-foreground">Owner</Text>
            </View>
            {rows}
        </View>
    )
}

function PublishRow({ share }: { share: ReportShares }) {
    if (!share.access.canPublish) return null
    return (
        <View className="flex-row items-center gap-3 border-t border-border pt-3">
            <View className="min-w-0 flex-1">
                <Text className="text-sm font-medium text-foreground">Publish to everyone</Text>
                <Text className="text-xs text-muted-foreground">Every member sees it under Published. Guests do not.</Text>
            </View>
            <Switch value={share.published} onValueChange={share.setPublished} accessibilityLabel="Publish to everyone" testID="report-share-publish" />
        </View>
    )
}

function OpenShareDialog({ reportId, onClose }: { reportId: string; onClose: () => void }) {
    const share = useReportShares(reportId)
    return (
        <Dialog isOpen onClose={onClose} title={`Share “${share.title}”`} size="lg" testID="report-share-dialog">
            <SearchRow share={share} />
            <Dialog.Body contentClassName="px-5 gap-4">
                <Candidates share={share} />
                <People share={share} />
                <GroupShareSection
                    grants={share.groups.grants}
                    isReady={share.groups.isReady}
                    roles={SHARE_ROLES}
                    canManage={share.access.canShare}
                    isPending={share.groups.isPending}
                    onAdd={share.groups.onAdd}
                    onRoleChange={share.groups.onRoleChange}
                    onRemove={share.groups.onRemove}
                />
                <PublishRow share={share} />
                <Text testID="report-share-note" className="text-sm text-muted-foreground">
                    Each person sees only the data they can access.
                </Text>
            </Dialog.Body>
            <Dialog.Footer>
                <Dialog.ActionButton label="Done" onPress={onClose} requiresServer={false} testID="report-share-done" />
            </Dialog.Footer>
        </Dialog>
    )
}

// Mounted once per screen; opened from the hub or a report through the store.
export function ReportShareDialog() {
    const reportId = useReportUIStore(s => s.shareReportId)
    const closeShare = useReportUIStore(s => s.closeShare)
    if (!reportId) return null
    return <OpenShareDialog reportId={reportId} onClose={closeShare} />
}
```

- [ ] **Step 6: Wire the hub, the view and the saved report**

In `core/reports/components/hub/ReportsHub.tsx`, replace `ReportsHub` with:

```tsx
function PackageBanner({ hub }: { hub: ReportsHubModel }) {
    if (!hub.packageSlug) return null
    return (
        <View testID="reports-hub-filter" className="flex-row flex-wrap items-center justify-between gap-2 rounded-lg border border-border bg-surface-secondary px-3 py-2">
            <Text className="text-sm text-foreground">{`Showing reports from ${hub.packageLabel}`}</Text>
            <Button testID="reports-hub-show-all" variant="outline" size="sm" onPress={hub.onShowAll}>
                <ButtonText>Show all reports</ButtonText>
            </Button>
        </View>
    )
}

export function ReportsHub() {
    const hub = useReportsHub()
    const actions = useHubActions()
    return (
        <>
            <ReportsHubView hub={hub} menuFor={row => <HubRowMenu row={row} actions={actions} onOpen={() => hub.onOpen(row.report)} />} banner={<PackageBanner hub={hub} />} />
            <ReportShareDialog />
        </>
    )
}
```

with imports `useHubActions` (`../../hooks/useHubActions`), `HubRowMenu` (`./HubRowMenu`), `ReportShareDialog` (`../share/ReportShareDialog`).

In `core/reports/hooks/useReportView.ts`: add `onShare?(): void` and `onSaveAs?(): void` to `ReportViewInput`, and to the returned model `onShare: input.onShare ?? null,` and `onSaveAs: input.onSaveAs ?? null,` (next to `onEdit`).

In `core/reports/components/ReportView.tsx`, `toolbarItems`, before the Copy link item add:

```tsx
    if (model.onShare && model.access.canShare) items.push(toolbarButton('share', 'Share', 'report-share-open', model.onShare))
    if (model.onSaveAs && !model.isEditing) items.push(toolbarButton('save-as', 'Save as…', 'report-save-as', model.onSaveAs))
```

In `core/reports/screens/ReportScreen.tsx`, replace `SavedReport` with:

```tsx
function SavedReport({ record, onBack }: { record: Record; onBack: () => void }) {
    const report = record.record
    const openShare = useReportUIStore(s => s.openShare)
    const actions = useHubActions()
    // The Save as prompt is a modal toggle: local UI state.
    const [isSavingAs, setSavingAs] = useState(false)
    const model = useReportView({
        reportKey: report?.id ?? '',
        title: report?.title ?? '',
        defs: record.spec?.params ?? [],
        source: report && record.spec ? { kind: 'saved', reportId: report.id } : null,
        access: record.access,
        broken: record.broken ?? (record.spec ? null : UNREADABLE),
        isEditing: false,
        onBack,
        onEdit: () => router.setParams({ edit: '1' }),
        onShare: report ? () => openShare(report.id) : undefined,
        onSaveAs: () => setSavingAs(true),
    })
    return (
        <>
            <ReportView model={model} />
            <ReportShareDialog />
            <PromptDialog
                isOpen={isSavingAs}
                onClose={() => setSavingAs(false)}
                onSubmit={title => {
                    setSavingAs(false)
                    if (report) actions.saveCopy(report, title)
                }}
                title="Save as"
                placeholder="Report title"
                confirmLabel="Save copy"
                defaultValue={copyTitle(report?.title ?? '')}
                maxLength={200}
                required
            />
        </>
    )
}
```

with imports `useState` (`react`), `PromptDialog` (`@tinycld/core/ui/PromptDialog`), `ReportShareDialog` (`../components/share/ReportShareDialog`), `useHubActions` (`../hooks/useHubActions`), `copyTitle` (`../hooks/useReportBuilder`), `useReportUIStore` (`../stores/report-ui-store`).

- [ ] **Step 7: Format, lint, typecheck**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome format --write core/reports tests/e2e/reports-helpers.ts tests/e2e/reports-share.spec.ts && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Expected: clean.

- [ ] **Step 8: Run the spec**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-share.spec.ts`
Expected: 5 passed.

- [ ] **Step 9: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/reports tests/e2e/reports-helpers.ts tests/e2e/reports-share.spec.ts
git commit -m "feat(core): report sharing, publishing, hub actions and Save as"
```

---

### Task 19: Help topic and package-author docs

**Files:**
- Create: `core/help/reports.md`
- Modify: `core/reports/components/hub/ReportsHub.tsx` (a help icon by the title), `core/reports/components/builder/ReportBuilderPanel.tsx` (a help icon by the panel title)
- Modify: `docs/packages.md` (section "Shipped and saved reports": the hub link and the catalog)

**Interfaces:**
- Consumes: `HelpIcon` (`@tinycld/core/components/help/HelpIcon`); the generator's core help step (`scripts/generate.ts`: `core/help` is read as package `core`).
- Produces: help topic `core:reports`.

- [ ] **Step 1: Write the help topic**

`core/help/reports.md`:

```markdown
---
title: Reports
summary: Make a report from your data, run it, share it, and publish it to everyone
tags: [reports, report builder, share report, publish, filter, group, pivot, formula]
order: 40
---

Open **Reports** from the rail (on a phone: **More → Reports**). The hub has three
tabs: **Mine** (reports you own), **Shared with me**, and **Published** (reports an
admin published for everyone). **Last run** shows when you last ran a report on this
device.

## Make a report

1. Select **New report** and choose what to report on.
2. In the panel beside the report, build it. The report updates as you change it.
   - **Columns**: select **Add column**. A field of a linked record (such as a
     sale's team) opens in place. Where each row has many linked records, choose
     **Add as total** (a count or a sum on each row) or **Show as child rows**.
     Rename a column, choose its **Total**, and move it up or down.
   - **Filter**: add conditions and choose **All of**, **Any of** or **None of**.
     **Add related rows** keeps rows that have (or have no) linked records that
     match their own conditions. Turn on **Ask when the report runs** to let each
     reader set the value from a chip above the report.
   - **Group and sort**: add group levels, each with or without subtotals, and the
     order of the rows.
   - **Pivot**: spread a column with a total across one column per value, such as
     one column per month.
   - **Add formula**: build a calculated column from parts picked from menus.
   - **Layout**: for statements, show each value of the first group level as a
     section, and add computed rows such as "Income less expenses".
3. Enter a **Title** under **Details** and select **Save**.

To change a report you own or edit, open it and select **Edit report**. To keep a
copy of any report you can see, select **Save as…**.

## Run a report

- The chips above the report set its period and filters. A link copied with
  **Copy link** opens the report with the same choices.
- **Customize** changes how numbers look (thousands, cents, negatives) and which
  columns show. It never changes the figures.
- Select a group row to open or close it; **Expand all** and **Collapse all** do
  every group.
- A report that is open stays current: when someone changes the data it reads, it
  runs again.

## Share and publish

Select **Share** to add people or groups as **Viewer** (can run it) or **Editor**
(can also change and share it). Each person sees only the data they can access: a
share gives the design of the report, never your data.

An admin who owns or edits a report can turn on **Publish to everyone**. Every
member then finds it under **Published**; guests do not.

## A report marked Broken

A report is marked **Broken** when something it uses is no longer installed, for
example after a package was removed. Open it to see what is missing; its owner or an
editor can fix it with **Edit report**.
```

- [ ] **Step 2: Link the topic from the screens**

In `core/reports/components/hub/ReportsHub.tsx`, `ReportsHubView`, replace the title `Text` with:

```tsx
                <View className="flex-row items-center gap-2">
                    <Text className="text-2xl font-semibold text-foreground">Reports</Text>
                    <HelpIcon topic="core:reports" />
                </View>
```

In `core/reports/components/builder/ReportBuilderPanel.tsx`, `PanelTitle`, return:

```tsx
    return (
        <View className="flex-row items-center gap-2">
            <Text className="text-lg font-semibold text-foreground">{builder.mode === 'new' ? 'New report' : 'Edit report'}</Text>
            <HelpIcon topic="core:reports" />
        </View>
    )
```

(import `HelpIcon` from `@tinycld/core/components/help/HelpIcon` in both).

- [ ] **Step 3: Document the hub link and the catalog for package authors**

In `docs/packages.md`, at the end of "### Shipped and saved reports" (before "### Test build tag"), add:

````markdown
#### Linking to a package's reports

Core's Reports hub is at `/a/reports`. To show only your package's reports and
report sources (for a sidebar "Reports" entry), link to it with your slug:

```ts
router.push(orgHref('reports', { package: '<slug>' }))
```

The hub then lists the saved reports and offers the sources whose key starts with
`<slug>.` — so give every source a key that starts with your slug. The builder reads
the sources from `GET /api/reports/sources` (signed-in users, not API tokens): each
source's fields with their types and aggregates, and its links. It shows no record
data.
````

- [ ] **Step 4: Regenerate the help index and check**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && pnpm run packages:generate`
Expected: it completes; `grep -n "core:reports\|reports.md" lib/generated/package-help.ts` finds the topic.

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome format --write core/reports && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/help/reports.md core/reports docs/packages.md
git commit -m "docs(core): reports help topic and the hub link for package authors"
```

---

### Task 20: Full checks: core, Go, Books and core isolation

**Files:** none (fix what fails at its root, in the file the failure names).

- [ ] **Step 1: Core TypeScript**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Expected: Biome, tsc and vitest pass for the `tinycld` member (app shell and core).

- [ ] **Step 2: Go**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go vet -tags no_ui ./... && go test -tags no_ui -count=1 ./... && go test -tags no_ui -count=1 -race ./reports/...`
Expected: `ok` for every package (the race run of `reports` takes about two minutes).

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/server && go vet . && go build -o /dev/null .`
Expected: clean (the app server builds with the stub's Go module linked by Task 4).

- [ ] **Step 3: Books still builds and passes**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Expected: Biome, tsc and vitest pass (this unit changes nothing in Books; a failure here is a core change Books relies on, fixed in core).

- [ ] **Step 4: Core names no package**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tsx scripts/check-core-isolation.ts`
Expected: exit 0, no references. (The stub and the specs live under `tests/`, which the check does not scan.)

- [ ] **Step 5: Commit any fix**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git status --short
git commit -am "fix(core): full-check fixes for the reports UI"
```
(Only when Steps 1–4 needed a change; name the fix in the message.)

---

### Task 21: Core feature e2e, Books report e2e, and push

**Files:** none.

- [ ] **Step 1: Every reports spec together, then the whole core suite**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-create.spec.ts tests/e2e/reports-filter.spec.ts tests/e2e/reports-group-pivot.spec.ts tests/e2e/reports-formula-layout.spec.ts tests/e2e/reports-view.spec.ts tests/e2e/reports-share.spec.ts`
Expected: 20 passed.

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e`
Expected: the whole core suite passes (the rail and the More sheet gained a Reports entry; `help.spec.ts`, `keyboard-shortcuts.spec.ts` and `search-palette.spec.ts` touch the same chrome). A failure is diagnosed and fixed at its root.

- [ ] **Step 2: Books' report shell and chrome e2e**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/report-shell-frame.spec.ts tests/e2e/report-shell-period.spec.ts tests/e2e/report-shell-filters.spec.ts tests/e2e/report-shell-customize.spec.ts tests/e2e/report-shell-table.spec.ts tests/e2e/books-sidebar-navigation.spec.ts tests/e2e/mobile-drawer.spec.ts`
Expected: all pass: Books' shell is untouched, and the new core rail entry and More row do not collide with Books' own Reports entries.

- [ ] **Step 3: Push**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && git push origin books/p5-report-builder`
Expected: the branch is pushed; CI runs the full suite with the new "Scaffold reports stub package" step.

---

## Spec coverage

| Spec item (§4 and what it relies on) | Where |
|---|---|
| The shell (frame, period and param chips, filter chips, Customize panel, table, folding, live apply, the row renderer) moves to `@tinycld/core/reports` | Tasks 6 (format), 7 (rows, folding), 8 (params, URL), 12 (table), 13 (frame, states), 17 (chips, Customize, live apply) |
| Books keeps no copy | `p5-trial-balance` removes it (spec "Books changes"); this unit leaves Books untouched (R27) and runs its checks (Tasks 20, 21) |
| Hub at `/a/reports`: tabs Mine, Shared with me, Published | Task 13 (`useReportsHub`, tabs), Task 18 e2e (shared, published) |
| Rows show title, source, owner, last run | Task 13 (`HubList`), R12 (last run per device) |
| Actions: New report (pick a source), Open, Duplicate, Share, Delete (owner), Publish (admin) | Task 13 (New report, Open), Task 18 (`HubRowMenu`, share, publish) |
| A broken report shows its error badge | Task 13 (`BrokenBadge`), Task 18 e2e |
| A package may link to the hub filtered to its sources | Task 13 (`?package=`), Task 18 (banner, e2e), Task 19 (docs) |
| Report view renders the run result; "Edit report" for owners and editors | Tasks 13 (view, Edit report), 10 (run) |
| A viewer changes params and Customize for the run; a copied link keeps them | Tasks 8 (URL), 17 (chips, Customize, Copy link, e2e) |
| Builder: side panel on desktop, full-screen sheet on phone, live preview | Task 13 (panel per width, preview), R14 |
| Columns: tree of fields and links; to-one expands; to-many "Add as total" or "Show as child rows"; reorder, rename, format | Task 13 (`ColumnPicker`, `ColumnsSection`), Task 16 e2e (to-many) |
| Filter: and/or/not, params, has / has no related | Task 14 |
| Group and sort with subtotals on or off | Task 15 |
| Pivot: a field, a date bucket, a cell value and its aggregate | Task 15 |
| Formulas: the forms engine's structured formula editor | Task 16 (`FormulaEditor`), R15 |
| Layout: sections and computed rows | Task 16 |
| Save; Save as when the user cannot edit | Task 13 (Save), Task 18 (Save as from the view), R17 |
| Share dialog modelled on Drive's: users or groups as viewer or editor, remove, admins publish, the data note | Task 18 |
| Live: subscribe to topics, re-run debounced, one follow-up | Tasks 9 (scheduler), 10 (`useLiveTopics`), 17 e2e |
| Help topic for building, sharing and publishing | Task 19 |
| The source catalog the builder reads (left to this unit by the engine plan) | Task 1 |
| The broken mark in the hub and the missing-parts state | Tasks 5 (`parseBroken`), 13, 18 |
| Errors: invalid spec → field errors in the builder, not saved | Tasks 11 (`errors.ts`), 13 (section problems, save errors) |
| Errors: unknown reference → error state naming the part; hub marks it broken | Tasks 13 (`ProblemsBody`), 18 |
| Errors: run failure or timeout → error state with Retry | Task 13 (`report-error`, `report-timeout`) |
| Errors: caps → a notice above the table | Task 17 (`ReportNotices`, pivot cap e2e) |
| Errors: no access → "You no longer have access to this report" | Tasks 13, 18 e2e (R30) |
| Security: a share gives the design only; each reader runs with their own rights | Server (p5-report-sharing); shown in the dialog note (Task 18) and checked by e2e (Task 18) |
| Testing: e2e on desktop and phone with data set up through the UI: hub tabs and actions; builder columns through to-one and to-many links, filter with exists, group, pivot, formula; share with a user who sees only their data; admin publish; live update; broken report | Tasks 13–18 (phone runs in Tasks 13 and 17; R22 for the broken fixture) |
| Paging of a long report | Task 7 (`mergePages`), Task 12 (near-end loading), Task 10 (`fetchNextPage`); not reachable in e2e (a page is 200 rows; seeding 200 rows through the UI per run is out of a test's time budget), so pinned by the unit tests |
| Money display exact near 2^53; per-cell currency | Task 6 |
| Platform: web and native at all widths | every component uses core primitives; [platform] items: none new |

## Self-review notes

- Placeholders: none; each code step carries the code. Where a task edits a file an earlier task wrote, the step names the exact function and gives the new code.
- Names checked across tasks: `useReportView` input (`onEdit`, `onShare`, `onSaveAs`), `ReportBuilder` setters (`setColumns`, `setGroups`, `setSort`, `setLayout`, `setFilter`, `setParams`, `setPivotPath`), `onApplyParam(id, value, keepOpen?)`, `periodDefaults(value, resolved)`, `columnChoices({ source, columns }, catalog)`, test ids in the specs against the components.
- The `lib/` modules, the stores, the data hooks, every component and the e2e specs and helpers were extracted from this plan into a probe outside the repos (with a copy of `core/lib/pocketbase.ts` that registers the two collections as Task 5 does, and with the edits of Tasks 16–18 applied): vitest 126 passed, `tsc` clean, `biome lint` clean (format only, see Global Constraints). The Go code of Tasks 1–2 was run in a scratch copy of `core/server`: the new tests pass and the whole `reports` and `coreserver` packages pass (`gofmt` realigns two structs). The scaffolder and the e2e specs were typechecked but not run (no servers, no install).
