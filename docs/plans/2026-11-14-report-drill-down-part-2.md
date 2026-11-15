# Report drills, open links and the drill panel — Part 2 (Tasks 9–16)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The `salesstub` e2e package ships a drill target and two drilled reports, the e2e helpers look in the statement or in a drill panel, and core's report view draws drilled figures and open rows as links, names the source on the back link of a drilled report (D3), runs a second, prefixed report view in a 480 px drill panel beside the report on a window of 1360 px or more (D4), and the builder, help and docs name the drill and the open link.

**Architecture:** See the outline, `docs/plans/2026-11-14-report-drill-down.md` (Architecture, Names changed N1–N4, Rulings R1–R16). Part 1 (Tasks 1–8) built the engine and the client libraries this part uses: `drill.ts` (`drillHref`, `drillPanelParams`, `isDrillShown`, `useOpenDrill`), `view-url.ts` (`FROM_KEY`, `DRILL_KEY`, `DRILL_PREFIX`, the `prefix` argument, `clearedDrillKeys`), the per-scope `report-ui-store`, `detail-surface`. Here: a `useRowLinks` hook gives the table its links (absent in the builder's preview, so figures stay plain); `RowCell` draws a drilled figure as expo-router `Link asChild` around a `Pressable` whose press runs `useOpenDrill` and whose `href` is the full report; `useReportBack` reads the `from` report's title; `useReportView` takes `urlPrefix`, `uiScope` and `viewWidth`; `ReportDrillPanel` is a second view in `ReportView`'s `side` slot that changes only style values when the window crosses 1360 px.

**Tech Stack:** TypeScript (React Native + web, expo-router 58, NativeWind), Zustand, vitest, Playwright; Go only in the stub template.

**Spec:** `/Users/nas/tmp/tinycld/books/books/docs/specs/report-drill-down-design.md` §3, §4, §9, §10. Brief: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-drill-down/cp/cp0.md` ("Tests it must add", Do step 2).

Paths are relative to `/Users/nas/tmp/tinycld/books/tinycld` (branch `books/p5-drill-down`; do not change branches). Vitest, from that directory: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run <file>`. Typecheck, from the same directory: `PATH=/Users/nas/tmp/tinycld/books/node_modules/.bin:$PATH tinycld-pkg typecheck`. Biome on your own files only: `/Users/nas/tmp/tinycld/books/node_modules/.bin/biome check <files>`. E2E, from the same directory, one file (with `-g` where a step says so), after the port check:

```bash
cd /Users/nas/tmp/tinycld/books/tinycld && { lsof -iTCP -sTCP:LISTEN -nP | grep -qE ':(7230|7231) ' && { echo PORTS-BUSY; exit 1; }; } ; E2E_PORT=7230 E2E_PORT_2=7231 E2E_IMAP_PORT=1230 E2E_IMAP_PORT_2=1231 pnpm exec tinycld-pkg test:e2e -- <name>.spec.ts
```

A run that takes more than about 4 minutes is the driver's: hand back `NEEDS-RUN: <command>` instead of waiting. A failing e2e points at the server: search `/Users/nas/tmp/tinycld/books/tinycld/tmp/e2e-serve-7230.log` with `command grep -E`.

## Global Constraints

- Core names no package: the e2e uses the `salesstub` package only; nothing in `core/` names it.
- Every new view input has a default that keeps today's view: `drill` absent → plain figures and rows; `urlPrefix` `''`, `uiScope` `'main'`, `viewWidth` absent → the window; `backLabel` absent → "Reports". `ReportViewModel`'s new fields (`backLabel`, `viewWidth`) are optional in the type, so a hand-built model (Books' static reference, `books/tinycld/books/screens/helix-mockup.tsx`) keeps compiling.
- A drilled figure: `text-foreground`, tabular; web hover underline and pointer cursor; native pressed dim; `bg-accent` (the selected-row token) and `aria-selected` while its panel shows exactly its params (a change of the panel's sort keeps the mark); named "Show transactions: <row label>, <column heading>, <figure>"; its `href` is always the full report (⌘-click and middle-click open a new tab; platform items 1 and 2 are signed off).
- An open row: its label is a link named "Open: <row label>"; a press anywhere on the row opens it.
- The panel switches between panel and page by style values only (no remount, no rerun). Escape closes it only while it is open (`LayerEscape`: `useShortcutScope('modal')` + `useRegisterShortcut`, standing approval).
- Biome (4 spaces, single quotes), no `any`, no biome-ignore, no `console.*`; comments say why; JSX minimal; no `useState` + `useEffect` sync.
- E2E: no raw PocketBase writes (sales through the stub screen); `page.goto` only through the helpers, or for a pasted link in a fresh page with the comment "Opening a pasted link is a full load by nature"; ports 7230/7231.
- Make no commits: each task's last step hands the diff to the driver.

## Review Focus

1. A modified click (⌘/Ctrl/⇧/⌥, or a middle click) on a drilled figure is left to the browser (a new tab with the full report, no panel); a plain press runs the drill and never also follows the `href` — Task 11 `link-press.test.ts` "leaves a modified click to the browser", e2e test 4 "the panel closes with ✕, Escape and Customize…" (Task 10, green in Task 15).
2. The e2e helpers on the page never read the drill panel: with the panel open, `rowOf`, `cellOf`, `chooseParamOption` and `waitForTableFit` with no scope find the statement's own row, cell, chip and table, though the same test ids are drawn twice — Task 9 helpers, exercised by e2e tests 3–6.
3. Crossing 1360 px with the panel open neither remounts nor reruns the panel's report (R14) — e2e test 5 "crossing 1360 px keeps the panel's report…" (Task 10, green in Task 14).
4. A `from` report whose read is held or fails gives the back link "Reports" and still returns to the source; one the reader cannot view returns to the hub — Task 12 `report-back.test.ts`, e2e test 1 (green in Task 12).
5. Escape with a panel chip's popover open closes the popover only; the panel stays — e2e test 4 (green in Task 15).

---

### Task 9: The stub's drill reports, its sale screen, and the e2e helpers' scope

**Files:**
- Modify: `tests/scripts/reports-stub/server/register.go.txt`
- Create: `tests/scripts/reports-stub/screens/sale/[id].tsx.txt`
- Modify: `tests/scripts/scaffold-reports-stub.ts` (TEMPLATES)
- Modify: `tests/unit/scaffold-reports-stub.test.ts`
- Modify: `tests/e2e/helpers.ts` (`inStatement`, `waitForTableFit`)
- Modify: `tests/e2e/report-view-helpers.ts` (`rowOf`, `cellOf`, `chooseParamOption`)
- Modify: `tests/e2e/reports-helpers.ts` (drill titles, `openDrilled`, `setPeriod`, `drillPanel`, header comment)

**Interfaces:**
- Consumes: `reports.RegisterShipped`, the spec fields of Tasks 1–5 (`detail.open`, `columns[].drill`, `groupKey`, `pivotBound`, `pivotCells`); `openReportsHub`, `hubRow`, `navigateToPackage`.
- Produces: shipped reports `salesstub.drill_target` ("Stub sales by team", unpublished), `salesstub.drilled` ("Stub drilled sales", published), `salesstub.drilled_pivot` ("Stub drilled by month", published) (N4: the target filters by `teams`, not `customers`); route `salesstub/sale/[id]` (test ids `stub-sale-screen`, `stub-sale-back`, `stub-sale-customer`); `inStatement(page: Page, testId: string): Locator` (helpers.ts, N4); `waitForTableFit(page: Page, scope?: Locator): Promise<string>`; `rowOf(page, label, scope?: Locator): Locator`; `cellOf(page, columnId, label, scope?: Locator): Promise<Locator>`; `chooseParamOption(page, paramId, pick, scope?: Locator)`; reports-helpers `DRILLED_TITLE`, `DRILLED_PIVOT_TITLE`, `DRILL_TARGET_TITLE`, `openDrilled(page: Page, title: string)`, `setPeriod(page: Page, from: string, to: string, scope?: Locator)`, `drillPanel(page: Page): Locator` (N4: shared with reports-failure-states.spec.ts, so in reports-helpers, not spec-local).

- [ ] **Step 1: Write the failing unit test**

`tests/unit/scaffold-reports-stub.test.ts`, in `it('re-writes a member stub from the templates', …)`, after the `register.go` expectation:

```ts
        const saleScreen = fs.readFileSync(
            path.join(TEMPLATES, 'screens', 'sale', '[id].tsx.txt'),
            'utf8'
        )
        expect(readStub('tinycld/salesstub/screens/sale/[id].tsx')).toBe(saleScreen)
```

- [ ] **Step 2: Run it to see it fail**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run tests/unit/scaffold-reports-stub.test.ts`
Expected: FAIL: ENOENT on `screens/sale/[id].tsx.txt`.

- [ ] **Step 3: The sale screen template and its copy**

Create `tests/scripts/reports-stub/screens/sale/[id].tsx.txt`:

```tsx
import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { router, useLocalSearchParams } from 'expo-router'
import { Pressable, Text, View } from 'react-native'

// E2E fixture screen: the record a report's detail row opens (detail.open,
// "salesstub/sale/<id>"), with a Back that returns to the report.
export default function StubSale() {
    const { id } = useLocalSearchParams<{ id: string }>()
    const [sales] = useStore('salesstub_sales')
    const { data: sale } = useLiveQuery({
        queryKey: ['salesstub.sale', id],
        query: q =>
            q
                .from({ sale: sales })
                .where(({ sale }) => eq(sale.id, id))
                .findOne(),
    })
    return (
        <View testID="stub-sale-screen" className="flex-1 gap-2 bg-background p-4">
            <Pressable
                testID="stub-sale-back"
                accessibilityRole="link"
                onPress={() => router.back()}
                className="self-start"
            >
                <Text className="text-sm text-primary">Back</Text>
            </Pressable>
            <Text testID="stub-sale-customer" className="text-lg font-semibold text-foreground">
                {sale?.customer ?? ''}
            </Text>
        </View>
    )
}
```

`tests/scripts/scaffold-reports-stub.ts`, TEMPLATES, after the `screens/index.tsx.txt` entry:

```ts
    ['screens/sale/[id].tsx.txt', `tinycld/${STUB_SLUG}/screens/sale/[id].tsx`],
```

The package's `exports` (`'./screens/*'`) already covers `screens/sale/[id]`. In the file's header comment, after "its landing screen adds rows through the UI, as the specs must", add: "and its sale screen is the record a report's detail row opens (`detail.open`)".

- [ ] **Step 4: Run the unit test**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run tests/unit/scaffold-reports-stub.test.ts`
Expected: PASS.

- [ ] **Step 5: The three shipped reports**

`tests/scripts/reports-stub/server/register.go.txt`, before `return nil` in `Register`:

```go
	// The drill reports (D1, D2; tests/e2e/reports-drill.spec.ts). "Stub sales by
	// team" is the target: unpublished, so only the shipped owner may open it and a
	// member's figures stay plain. Its detail rows open the stub's sale screen, and
	// below 600 px its rows wrap, so the 480 px drill panel shows its Sort chip.
	reports.RegisterShipped(reports.Shipped{
		Key: "salesstub.drill_target", Title: "Stub sales by team", Revision: 1, Unpublished: true,
		Description: "Each team's sales in a period, line by line.",
		Spec: json.RawMessage(strings.ReplaceAll(`{
			"version": 4,
			"source": "salesstub.sales",
			"params": [
				{"id": "period", "type": "dateRange", "label": "Period", "default": {"preset": "thisMonthToDate"}},
				{"id": "teams", "type": "choice", "label": "Teams", "multiple": true,
				 "optionsFrom": {"source": "salesstub.teams", "value": [{"field": "id"}], "label": [{"field": "name"}], "group": [{"field": "region"}], "groupLabel": "All {group} teams"}}
			],
			"columns": [
				{"id": "team", "label": "Team", "path": [{"link": "team"}]},
				{"id": "day", "label": "Day", "path": [{"field": "day"}]},
				{"id": "amount", "label": "Amount", "path": [{"field": "amount"}], "aggregate": "sum"}
			],
			"filter": {"op": "and", "args": [
				{"op": "between", "args": [{"path": [{"field": "day"}]}, {"param": "period"}]},
				{"op": "in", "args": [{"path": [{"link": "team"}]}, {"param": "teams"}]}
			]},
			"groups": [{"id": "byTeam", "col": "team", "open": true, "subtotals": true,
				"label": {"record": "byTeam", "path": [{"field": "name"}]}}],
			"sort": [{"col": "day"}],
			"detail": {"label": {"path": [{"field": "customer"}]}, "open": SALE_OPEN},
			"display": {
				"oneLine": {"minWidth": "600px"},
				"layouts": [{"width": "mobile", "tracks": 2, "cells": [
					{"col": "team", "line": 1, "span": 2},
					{"col": "day", "line": 2},
					{"col": "amount", "line": 2}
				]}]
			}
		}`, "SALE_OPEN", saleOpen)),
	})
	// Sales by region, then team; an Amount opens the target filtered to the group
	// (a region as a whole group of teams, a team by its id), with the period the
	// reader ran.
	reports.RegisterShipped(reports.Shipped{
		Key: "salesstub.drilled", Title: "Stub drilled sales", Revision: 1,
		Description: "Sales by region and team; an amount opens the sales behind it.",
		Spec: json.RawMessage(`{
			"version": 4,
			"source": "salesstub.sales",
			"params": [{"id": "period", "type": "dateRange", "label": "Period", "default": {"preset": "thisFiscalYear"}}],
			"columns": [
				{"id": "team", "label": "Team", "path": [{"link": "team"}]},
				{"id": "region", "hidden": true, "path": [{"link": "team"}, {"field": "region"}]},
				{"id": "amount", "label": "Amount", "path": [{"field": "amount"}], "aggregate": "sum",
				 "drill": {"report": "salesstub.drill_target", "levels": {
					"byRegion": {"period": {"param": "period"}, "teams": {"group": {"groupKey": "byRegion"}}},
					"byTeam": {"period": {"param": "period"}, "teams": {"record": "byTeam", "path": [{"field": "id"}]}}}}}
			],
			"filter": {"op": "between", "args": [{"path": [{"field": "day"}]}, {"param": "period"}]},
			"groups": [
				{"id": "byRegion", "col": "region", "open": true, "subtotals": true,
				 "label": {"op": "case", "args": [
					{"op": "eq", "args": [{"col": "region"}, {"value": "north"}]}, {"value": "North"},
					{"op": "eq", "args": [{"col": "region"}, {"value": "south"}]}, {"value": "South"},
					{"value": "East"}]}},
				{"id": "byTeam", "col": "team", "label": {"record": "byTeam", "path": [{"field": "name"}]}}
			],
			"sort": [{"formula": {"record": "byTeam", "path": [{"field": "name"}]}}]
		}`),
	})
	// A month pivot of each team's sales; a month cell opens the target cut to that
	// month inside the period, the team's total with the whole period.
	reports.RegisterShipped(reports.Shipped{
		Key: "salesstub.drilled_pivot", Title: "Stub drilled by month", Revision: 1,
		Description: "Each team's sales in every month; a month opens its sales.",
		Spec: json.RawMessage(`{
			"version": 4,
			"source": "salesstub.sales",
			"params": [{"id": "period", "type": "dateRange", "label": "Period", "default": {"preset": "thisFiscalYear"}}],
			"columns": [
				{"id": "team", "label": "Team", "path": [{"link": "team"}]},
				{"id": "amount", "label": "Total", "path": [{"field": "amount"}], "aggregate": "sum",
				 "drill": {"report": "salesstub.drill_target",
					"levels": {"byTeam": {"period": {"param": "period"}, "teams": {"record": "byTeam", "path": [{"field": "id"}]}}},
					"pivotCells": {"period": {"range": {"from": {"pivotBound": "from"}, "to": {"pivotBound": "to"}}}}}}
			],
			"filter": {"op": "between", "args": [{"path": [{"field": "day"}]}, {"param": "period"}]},
			"groups": [{"id": "byTeam", "col": "team", "label": {"record": "byTeam", "path": [{"field": "name"}]}}],
			"pivot": {"path": [{"field": "day"}], "bucket": "month", "cell": "amount", "range": "period", "order": "spec"}
		}`),
	})
```

After `const teamOpening = …` at the end of the file:

```go

// saleOpen is the stub sale screen a detail row opens (detail.open).
const saleOpen = `{"op": "concat", "args": [{"value": "salesstub/sale/"}, {"path": [{"field": "id"}]}]}`
```

In the file's header comment, after "shipped reports (published and not)", add ", and drill reports whose figures open another shipped report".

- [ ] **Step 6: The e2e helpers' scope**

`tests/e2e/helpers.ts`: add `type Locator` to its `@playwright/test` import if it is not there. Before `waitForTableFit`, add:

```ts
/**
 * A test id in the report itself, never in its drill panel (D4): with the panel
 * open, the panel draws its own chips, table, rows and cells under the same ids.
 */
export function inStatement(page: Page, testId: string): Locator {
    return page.locator(`[data-testid="${testId}"]:not([data-testid="report-drill-panel"] *)`)
}
```

Replace `waitForTableFit` with:

```ts
/**
 * Waits until the report table's layout is fitted to the width its rows have now
 * (the table's data-fit-width equals the row probe's width), and returns the rule
 * that laid it out: 'one-line', 'desktop', 'tablet', 'mobile' or 'auto'. Call it
 * after each setViewportSize before reading the table's geometry. With no scope it
 * reads the report's own table, not its drill panel's; pass the panel to read that.
 */
export async function waitForTableFit(page: Page, scope?: Locator): Promise<string> {
    const table = scope ? scope.getByTestId('report-table') : inStatement(page, 'report-table')
    let layout = ''
    await expect
        .poll(
            async () => {
                // evaluateAll does not wait for the table: a poll reads what is drawn now.
                layout = await table.evaluateAll(tables => {
                    const [element] = tables
                    if (!(element instanceof HTMLElement)) return ''
                    const probe = element.querySelector<HTMLElement>(
                        '[data-testid="report-width-probe"]'
                    )
                    if (!probe || element.dataset.fitWidth === undefined) return ''
                    const drawn = probe.getBoundingClientRect().width
                    const isSettled = Math.abs(drawn - Number(element.dataset.fitWidth)) < 1
                    return isSettled ? (element.dataset.layout ?? '') : ''
                })
                return layout
            },
            { message: 'the report table fits its layout to the width its rows have' }
        )
        .not.toBe('')
    return layout
}
```

`tests/e2e/report-view-helpers.ts`: import `inStatement` with `navigateToPackage` from `./helpers.ts`. Replace `rowOf`, `cellOf` and the start of `chooseParamOption`:

```ts
// The report's own table, or the table inside `scope` (the drill panel).
function tableOf(page: Page, scope?: Locator): Locator {
    return scope ? scope.getByTestId('report-table') : inStatement(page, 'report-table')
}

/**
 * The label cell of the row whose label is `label` (a group row's is its toggle),
 * in the report's own table, or in `scope` (the drill panel).
 */
export function rowOf(page: Page, label: string, scope?: Locator): Locator {
    return tableOf(page, scope)
        .locator('[data-testid^="report-row-"]')
        .filter({ has: page.getByText(label, { exact: true }) })
        .first()
}

/** The cell of `columnId` (not the first column) in the row whose label is `label`. */
export async function cellOf(
    page: Page,
    columnId: string,
    label: string,
    scope?: Locator
): Promise<Locator> {
    const labelCell = rowOf(page, label, scope)
    await expect(labelCell).toBeVisible()
    const testId = await labelCell.getAttribute('data-testid')
    if (!testId) throw new Error(`no row labelled ${label}`)
    return tableOf(page, scope).getByTestId(
        `report-cell-${columnId}-${testId.replace('report-row-', '')}`
    )
}
```

```ts
export async function chooseParamOption(
    page: Page,
    paramId: string,
    pick: ParamPick,
    scope?: Locator
) {
    // The chip and Add filter are the view's own; the picker is one, on the page.
    const own = (testId: string) => (scope ? scope.getByTestId(testId) : inStatement(page, testId))
    if (pick.from === 'chip') await own(`report-param-${paramId}`).click()
    else {
        await own('report-add-filter').click()
        await page.getByTestId(`report-add-filter-${paramId}`).click()
    }
```

The rest of `chooseParamOption` stays. Every existing caller passes no scope and keeps working: with no panel on screen, `inStatement` finds what `page.getByTestId` found.

- [ ] **Step 7: The drill helpers**

`tests/e2e/reports-helpers.ts`: import `inStatement` with `loginAs, navigateToPackage` from `./helpers`. In the header comment, after `unpublished "Stub team totals" and "Stub team balance"`, write: `, and the drill reports: published "Stub drilled sales" and "Stub drilled by month", whose Amount opens the unpublished "Stub sales by team" (only the shipped owner may open it)`. After `SHIPPED_FORMULA`, add:

```ts
/** The drill reports (register.go.txt): two drilled sources and the report they open. */
export const DRILLED_TITLE = 'Stub drilled sales'
export const DRILLED_PIVOT_TITLE = 'Stub drilled by month'
export const DRILL_TARGET_TITLE = 'Stub sales by team'

/** Opens a published report from the hub. */
export async function openDrilled(page: Page, title: string) {
    await openReportsHub(page)
    await page.getByTestId('reports-hub-tab-published').click()
    await hubRow(page, title).click()
    await expect(inStatement(page, 'report-title')).toHaveText(title)
}

/**
 * Sets a Custom period on the period chip of the report, or of `scope` (the drill
 * panel, whose keys carry the `d.` prefix), and closes its popover.
 */
export async function setPeriod(page: Page, from: string, to: string, scope?: Locator) {
    const chip = scope
        ? scope.getByTestId('report-param-period')
        : inStatement(page, 'report-param-period')
    await chip.click()
    await page.getByTestId('report-period-preset-custom').click()
    await page.getByTestId('report-period-from').fill(from)
    await page.getByTestId('report-period-to').fill(to)
    const key = scope ? 'd.p-period' : 'p-period'
    await expect.poll(() => new URL(page.url()).searchParams.get(key)).toBe(`${from}..${to}`)
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('report-period-popover')).toHaveCount(0)
}

/** The drill panel (D4); its own helpers' scope. */
export function drillPanel(page: Page): Locator {
    return page.getByTestId('report-drill-panel')
}
```

- [ ] **Step 8: Bring the new route in, and boot the stub**

The new template needs one run of the scaffolder (it runs `packages:generate`, which links the new route). No e2e may be running.

Run: `cd /Users/nas/tmp/tinycld/books && pnpm exec tsx tinycld/tests/scripts/scaffold-reports-stub.ts`
Expected: ends with `[scaffold-reports-stub] done`.

Run (port check first): `… pnpm exec tinycld-pkg test:e2e -- reports-shipped.spec.ts`, then `… reports-period-pivot.spec.ts`
Expected: PASS (the shipped specs boot with the three new reports; `rowOf`, `cellOf`, `waitForTableFit` with no scope keep working).

Run: `command grep -nE 'panic|shipped spec|salesstub.drill' /Users/nas/tmp/tinycld/books/tinycld/tmp/e2e-serve-7230.log | head -20`
Expected: no panic and no "skipped" line for a `salesstub.drill*` key (a refused shipped spec is logged and skipped, shipped.go). A refusal there is a defect of this step's spec text or of Tasks 1–5: fix it at its source.

- [ ] **Step 9: Biome**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/biome check tests/scripts/scaffold-reports-stub.ts tests/unit/scaffold-reports-stub.test.ts tests/e2e/helpers.ts tests/e2e/report-view-helpers.ts tests/e2e/reports-helpers.ts`
Expected: no errors.

- [ ] **Step 10: Hand the diff to the driver** (no commit).

---

### Task 10: The `reports: drill` e2e, written red

**Files:**
- Create: `tests/e2e/reports-drill.spec.ts`

**Interfaces:**
- Consumes: Task 9 (`openDrilled`, `setPeriod`, `drillPanel`, `inStatement`, the drill titles, `rowOf`/`cellOf` with `scope`), `loginAsShippedOwner`, `createInvitedUser`, `pressToolbar`, `addTeam`, `addSale`, `openReportStub`, `uniqueName`, `waitForTableFit`, `skipWithoutReportsStub`.
- Produces: `tests/e2e/reports-drill.spec.ts`, `test.describe('reports: drill')`, six tests (titles below); local helpers `seed`, `reportIdOf`, `param`, `decorationOf`, `tabTo`, `watchRuns`, `panelDetails`, `markTable`, `expectFreshBack`.

The tests are the brief's tests 1–6. Each signs in as the shipped owner (the target is unpublished) and seeds its own teams in region East in its own year (2031–2036), because the shipped owner's sales are shared by every test that signs in as it: a test's Custom period holds its own sales only. Test 6 ends with a fresh member, who may read the published drilled report but not its target.

- [ ] **Step 1: Write the spec**

Create `tests/e2e/reports-drill.spec.ts`:

```ts
import { type Browser, expect, type Locator, type Page, test } from '@playwright/test'
import { createInvitedUser, inStatement, skipWithoutReportsStub, waitForTableFit } from './helpers'
import {
    addSale,
    addTeam,
    cellOf,
    DRILL_TARGET_TITLE,
    DRILLED_PIVOT_TITLE,
    DRILLED_TITLE,
    drillPanel,
    hubRow,
    loginAsShippedOwner,
    openDrilled,
    openReportStub,
    openReportsHub,
    pressToolbar,
    rowOf,
    setPeriod,
    uniqueName,
} from './reports-helpers'

/**
 * Report drills (D1), open rows (D2), the back link to the source (D3) and the
 * drill panel (D4) on the stub's "Stub drilled sales" and "Stub drilled by month",
 * whose Amount opens "Stub sales by team" (tests/scripts/reports-stub/server/
 * register.go.txt). The target is unpublished, so the shipped owner opens it and a
 * member's figures stay plain. Each test seeds its own East teams in its own year,
 * because every test that signs in as the shipped owner reads the same sales.
 */

const WIDE = { width: 1440, height: 900 }
const NARROW = { width: 1280, height: 900 }
const PHONE = { width: 390, height: 844 }

const BAD_REQUEST = {
    status: 400,
    contentType: 'application/json',
    body: JSON.stringify({ status: 400, message: 'Bad request.', data: {} }),
}

interface SeedTeam {
    name: string
    sales: { customer: string; amount: number; day: string }[]
}

async function seed(page: Page, teams: SeedTeam[]) {
    await openReportStub(page)
    for (const team of teams) {
        await addTeam(page, team.name, 'east')
        for (const sale of team.sales) await addSale(page, { team: team.name, cost: 0, ...sale })
    }
}

/** The report id of a report screen's URL (`…/reports/<id>`). */
function reportIdOf(url: string): string {
    const id = new URL(url).pathname.split('/').pop()
    if (!id) throw new Error(`no report id in ${url}`)
    return id
}

function param(page: Page, key: string): string | null {
    return new URL(page.url()).searchParams.get(key)
}

// react-native-web draws a link's text as its first child.
function decorationOf(link: Locator): Promise<string> {
    return link.evaluate(el => getComputedStyle(el.firstElementChild ?? el).textDecorationLine)
}

/** Presses Tab until `target` has the focus. */
async function tabTo(page: Page, target: Locator) {
    for (let i = 0; i < 60; i += 1) {
        if (await target.evaluate(el => el === document.activeElement)) return
        await page.keyboard.press('Tab')
    }
    throw new Error('Tab never reached the figure')
}

/** The bodies of every run of a report other than `sourceId`, from now on. */
function watchRuns(page: Page, sourceId: string): string[] {
    const bodies: string[] = []
    page.on('request', request => {
        if (!request.url().includes('/api/reports/run')) return
        const body = request.postDataJSON() as { reportId?: string } | null
        if (body && body.reportId !== sourceId) bodies.push(JSON.stringify(body))
    })
    return bodies
}

/** The detail rows' labels in the panel, in screen order. */
function panelDetails(panel: Locator): Promise<string[]> {
    return panel
        .getByTestId('report-table')
        .locator('[data-testid^="report-row-d:"]')
        .allInnerTexts()
}

// React does not touch an attribute it does not render, so the mark lasts as long
// as the element: a remount drops it.
async function markTable(panel: Locator) {
    await panel.getByTestId('report-table').evaluate(el => el.setAttribute('data-e2e-kept', '1'))
}

/**
 * A link pasted into a new tab has no history. With the source's title read, Back
 * names it and opens the source with its defaults; while that read is held, and
 * after it fails, Back reads "Reports" and still returns to the source; a source the
 * reader cannot open (the client sees no record, as for an id that does not exist)
 * sends Back to the hub.
 */
async function expectFreshBack(browser: Browser, targetUrl: string, sourceId: string) {
    const context = await browser.newContext({ viewport: NARROW })
    const q = await context.newPage()
    try {
        await loginAsShippedOwner(q)
        // Opening a pasted link is a full load by nature.
        await q.goto(targetUrl)
        await expect(q.getByTestId('report-back')).toHaveText(DRILLED_TITLE)
        await q.getByTestId('report-back').click()
        await expect(q.getByTestId('report-title')).toHaveText(DRILLED_TITLE)
        expect(new URL(q.url()).searchParams.get('p-period')).toBeNull()

        const read = { mode: 'hold' as 'hold' | 'fail' | 'pass' }
        let release = () => {}
        const held = new Promise<void>(resolve => {
            release = resolve
        })
        let answered = () => {}
        const failed = new Promise<void>(resolve => {
            answered = resolve
        })
        await q.route('**/api/collections/reports/records**', async route => {
            const isSource = decodeURIComponent(route.request().url()).includes(sourceId)
            if (read.mode === 'pass' || !isSource) return route.continue()
            if (read.mode === 'hold') await held
            await route.fulfill(BAD_REQUEST)
            answered()
        })
        // Opening a pasted link is a full load by nature.
        await q.goto(targetUrl)
        await expect(q.getByTestId('report-title')).toHaveText(DRILL_TARGET_TITLE)
        await expect(q.getByTestId('report-back')).toHaveText('Reports')
        read.mode = 'fail'
        release()
        await failed
        await expect(q.getByTestId('report-back')).toHaveText('Reports')
        read.mode = 'pass'
        await q.getByTestId('report-back').click()
        await expect(q.getByTestId('report-title')).toHaveText(DRILLED_TITLE)

        const unknown = new URL(targetUrl)
        unknown.searchParams.set('from', 'zzzzzzzzzzzzzzz')
        // Opening a pasted link is a full load by nature.
        await q.goto(unknown.toString())
        await expect(q.getByTestId('report-title')).toHaveText(DRILL_TARGET_TITLE)
        await expect(q.getByTestId('report-back')).toHaveText('Reports')
        await q.getByTestId('report-back').click()
        await expect(q.getByTestId('reports-hub')).toBeVisible()
    } finally {
        await context.close()
    }
}

test.describe('reports: drill', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a figure opens its report as a screen, and the back link names the source', async ({
        page,
        browser,
    }) => {
        await loginAsShippedOwner(page)
        const ann = uniqueName('Drill Ann')
        const bo = uniqueName('Drill Bo')
        await seed(page, [
            {
                name: ann,
                sales: [
                    { customer: 'Cara', amount: 300, day: '2031-03-12' },
                    { customer: 'Dan', amount: 100, day: '2031-04-02' },
                ],
            },
            { name: bo, sales: [{ customer: 'Eve', amount: 0, day: '2031-05-01' }] },
        ])
        await page.setViewportSize(NARROW)
        await openDrilled(page, DRILLED_TITLE)
        await setPeriod(page, '2031-03-10', '2031-05-20')
        const sourceId = reportIdOf(page.url())

        // At rest a drilled figure reads as a figure; the pointer underlines it.
        const figure = await cellOf(page, 'amount', ann)
        await expect(
            page.getByRole('link', { name: `Show transactions: ${ann}, Amount, 4.00` })
        ).toBeVisible()
        await page.mouse.move(0, 0)
        await expect.poll(() => decorationOf(figure)).toBe('none')
        await figure.hover()
        await expect.poll(() => decorationOf(figure)).toBe('underline')
        expect(await figure.evaluate(el => getComputedStyle(el).cursor)).toBe('pointer')
        // A figure of 0.00 still opens its rows.
        await expect(
            page.getByRole('link', { name: `Show transactions: ${bo}, Amount, 0.00` })
        ).toBeVisible()

        await figure.click()
        await expect(page.getByTestId('report-title')).toHaveText(DRILL_TARGET_TITLE)
        await expect(page.getByTestId('report-drill-panel')).toHaveCount(0)
        expect(param(page, 'from')).toBe(sourceId)
        expect(param(page, 'p-period')).toBe('2031-03-10..2031-05-20')
        await expect(page.getByTestId('report-param-teams')).toHaveText(`Teams: ${ann}`)
        await expect(rowOf(page, 'Cara')).toBeVisible()
        await expect(rowOf(page, 'Dan')).toBeVisible()
        await expect(rowOf(page, 'Eve')).toHaveCount(0)
        const targetUrl = page.url()

        await expect(page.getByTestId('report-back')).toHaveText(DRILLED_TITLE)
        await page.getByTestId('report-back').click()
        await expect(page.getByTestId('report-title')).toHaveText(DRILLED_TITLE)
        await expect.poll(() => param(page, 'p-period')).toBe('2031-03-10..2031-05-20')

        // A month cell opens the target with that month, cut to the period.
        await openDrilled(page, DRILLED_PIVOT_TITLE)
        await setPeriod(page, '2031-03-10', '2031-05-20')
        await (await cellOf(page, 'amount.p0', ann)).click()
        await expect(page.getByTestId('report-title')).toHaveText(DRILL_TARGET_TITLE)
        expect(param(page, 'p-period')).toBe('2031-03-10..2031-03-31')
        await expect(page.getByTestId('report-param-period')).toHaveText(
            'Period: Mar 10 – Mar 31, 2031'
        )
        await expect(rowOf(page, 'Cara')).toBeVisible()
        await expect(rowOf(page, 'Dan')).toHaveCount(0)

        await expectFreshBack(browser, targetUrl, sourceId)
    })

    test('on a phone a figure opens its report as a screen', async ({ page }) => {
        await loginAsShippedOwner(page)
        const team = uniqueName('Drill Phone')
        await seed(page, [{ name: team, sales: [{ customer: 'Gil', amount: 500, day: '2032-02-02' }] }])
        await page.setViewportSize(NARROW)
        await openDrilled(page, DRILLED_TITLE)
        await setPeriod(page, '2032-01-01', '2032-12-31')
        await page.setViewportSize(PHONE)
        await waitForTableFit(page)
        await (await cellOf(page, 'amount', team)).click()
        await expect(page.getByTestId('report-title')).toHaveText(DRILL_TARGET_TITLE)
        await expect(page.getByTestId('report-drill-panel')).toHaveCount(0)
        await expect(page.getByTestId('report-back')).toHaveText(DRILLED_TITLE)
        await expect(rowOf(page, 'Gil')).toBeVisible()
    })

    test('on a wide window a figure opens its report in a panel beside the source', async ({
        page,
    }) => {
        await loginAsShippedOwner(page)
        const ann = uniqueName('Drill Panel A')
        const bo = uniqueName('Drill Panel B')
        await seed(page, [
            {
                name: ann,
                sales: [
                    { customer: 'Hal', amount: 300, day: '2033-03-01' },
                    { customer: 'Ida', amount: 100, day: '2033-03-02' },
                ],
            },
            { name: bo, sales: [{ customer: 'Jo', amount: 700, day: '2033-03-03' }] },
        ])
        await page.setViewportSize(WIDE)
        await openDrilled(page, DRILLED_TITLE)
        await setPeriod(page, '2033-01-01', '2033-12-31')
        const sourceId = reportIdOf(page.url())
        const sourcePath = new URL(page.url()).pathname
        const sourceChip = await inStatement(page, 'report-param-period').textContent()
        const figureA = await cellOf(page, 'amount', ann)
        // ⌘-click and middle-click open the full report (platform item 2).
        await expect(figureA).toHaveAttribute(
            'href',
            new RegExp(`/reports/[^?]+\\?.*from=${sourceId}`)
        )
        await figureA.click()

        const panel = drillPanel(page)
        await expect(panel).toBeVisible()
        await expect
            .poll(async () => Math.round((await panel.boundingBox())?.width ?? 0))
            .toBe(480)
        await expect(inStatement(page, 'report-title')).toHaveText(DRILLED_TITLE)
        expect(new URL(page.url()).pathname).toBe(sourcePath)
        expect(param(page, 'd')).toBeTruthy()
        await expect(panel.getByTestId('report-drill-panel-title')).toHaveText(DRILL_TARGET_TITLE)
        await expect(panel.getByTestId('report-drill-panel-title')).toBeFocused()
        await expect(figureA).toHaveAttribute('aria-selected', 'true')
        for (const id of [
            'report-customize-open',
            'report-share-open',
            'report-save-as',
            'report-copy-link',
            'report-edit-open',
            'toolbar-more-button',
        ]) {
            await expect(panel.getByTestId(id)).toHaveCount(0)
        }
        await expect(panel.getByTestId('report-param-teams')).toHaveText(`Teams: ${ann}`)

        // The panel's Sort chip sorts the panel's rows only.
        await expect.poll(() => panelDetails(panel)).toEqual(['Hal', 'Ida'])
        await panel.getByTestId('report-sort-menu').click()
        await page.getByTestId('report-sort-choice-amount-asc').click()
        await expect.poll(() => param(page, 'd.s')).toBe('amount:asc')
        expect(param(page, 's')).toBeNull()
        await expect.poll(() => panelDetails(panel)).toEqual(['Ida', 'Hal'])

        // The URL holds the panel, so a reload keeps it.
        await page.reload()
        await expect(panel).toBeVisible()
        await expect(panel.getByTestId('report-param-teams')).toHaveText(`Teams: ${ann}`)
        await expect.poll(() => panelDetails(panel)).toEqual(['Ida', 'Hal'])

        // A second figure replaces the panel's report: one panel, its chips, its mark.
        const figureB = await cellOf(page, 'amount', bo)
        await figureB.click()
        await expect(panel).toHaveCount(1)
        await expect(panel.getByTestId('report-param-teams')).toHaveText(`Teams: ${bo}`)
        await expect(figureB).toHaveAttribute('aria-selected', 'true')
        await expect(await cellOf(page, 'amount', ann)).not.toHaveAttribute('aria-selected', 'true')
        expect(param(page, 'd.s')).toBeNull()

        // The panel's Period chip opens its own popover; the source's chip stays closed.
        await panel.getByTestId('report-param-period').click()
        await expect(page.getByTestId('report-period-popover')).toHaveCount(1)
        await expect(inStatement(page, 'report-param-period')).not.toHaveAttribute(
            'aria-expanded',
            'true'
        )
        await page.keyboard.press('Escape')
        await expect(page.getByTestId('report-period-popover')).toHaveCount(0)

        // The panel's Period runs the panel's report only, and no figure is marked after it.
        await setPeriod(page, '2033-03-03', '2033-03-31', panel)
        await expect(inStatement(page, 'report-param-period')).toHaveText(sourceChip ?? '')
        expect(param(page, 'p-period')).toBe('2033-01-01..2033-12-31')
        await expect(page.locator('[data-testid^="report-cell-"][aria-selected="true"]')).toHaveCount(0)

        // Open as full report: the panel's chips, and a back link that names the source.
        await panel.getByTestId('report-drill-open-full').click()
        await expect(page.getByTestId('report-title')).toHaveText(DRILL_TARGET_TITLE)
        expect(param(page, 'p-period')).toBe('2033-03-03..2033-03-31')
        expect(param(page, 'from')).toBe(sourceId)
        await expect(page.getByTestId('report-param-teams')).toHaveText(`Teams: ${bo}`)
        await expect(page.getByTestId('report-back')).toHaveText(DRILLED_TITLE)
        await page.getByTestId('report-back').click()
        await expect(panel).toBeVisible()
        await expect(panel.getByTestId('report-param-teams')).toHaveText(`Teams: ${bo}`)

        // Opening and replacing the panel added no history entry: Back leaves the report.
        await (await cellOf(page, 'amount', ann)).click()
        await expect(panel.getByTestId('report-param-teams')).toHaveText(`Teams: ${ann}`)
        await page.goBack()
        await expect.poll(() => new URL(page.url()).pathname).not.toBe(sourcePath)
    })

    test('the panel closes with ✕, Escape and Customize, and focus goes back to its figure', async ({
        page,
    }) => {
        await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
        await loginAsShippedOwner(page)
        const team = uniqueName('Drill Close')
        await seed(page, [{ name: team, sales: [{ customer: 'Kim', amount: 200, day: '2034-06-01' }] }])
        await page.setViewportSize(WIDE)
        await openDrilled(page, DRILLED_TITLE)
        await setPeriod(page, '2034-01-01', '2034-12-31')
        const figure = await cellOf(page, 'amount', team)
        const panel = drillPanel(page)

        // A modified click is the browser's: a new tab with the full report, no panel.
        const [tab] = await Promise.all([
            page.context().waitForEvent('page'),
            figure.click({ modifiers: ['ControlOrMeta'] }),
        ])
        await tab.waitForURL(/from=/)
        await tab.close()
        await expect(panel).toHaveCount(0)

        await figure.click()
        await expect(panel).toBeVisible()
        await panel.getByTestId('report-drill-close').click()
        await expect(panel).toHaveCount(0)
        await expect(figure).toBeFocused()
        expect(param(page, 'd')).toBeNull()

        // With a chip's popover open in the panel, Escape closes the popover only.
        await figure.click()
        await expect(panel).toBeVisible()
        await panel.getByTestId('report-param-period').click()
        await expect(page.getByTestId('report-period-popover')).toBeVisible()
        await page.keyboard.press('Escape')
        await expect(page.getByTestId('report-period-popover')).toHaveCount(0)
        await expect(panel).toBeVisible()
        await page.keyboard.press('Escape')
        await expect(panel).toHaveCount(0)
        await expect(figure).toBeFocused()

        // Customize and the panel are one side surface: each closes the other.
        await figure.click()
        await expect(panel).toBeVisible()
        await pressToolbar(page, 'report-customize-open', 'Customize')
        await expect(page.getByTestId('report-customize')).toBeVisible()
        await expect(panel).toHaveCount(0)
        expect(param(page, 'd')).toBeNull()
        await figure.click()
        await expect(panel).toBeVisible()
        await expect(page.getByTestId('report-customize')).toHaveCount(0)

        // A copied link never holds the panel.
        await pressToolbar(page, 'report-copy-link', 'Copy link')
        const link = new URL(await page.evaluate(() => navigator.clipboard.readText()))
        const keys = [...link.searchParams.keys()]
        expect(keys.filter(key => key === 'd' || key.startsWith('d.') || key === 'from')).toEqual([])
        expect(link.searchParams.get('p-period')).toBe('2034-01-01..2034-12-31')
    })

    test("crossing 1360 px keeps the panel's report mounted and does not run it again", async ({
        page,
    }) => {
        await loginAsShippedOwner(page)
        const team = uniqueName('Drill Cross')
        await seed(page, [{ name: team, sales: [{ customer: 'Lou', amount: 900, day: '2035-02-02' }] }])
        await page.setViewportSize(WIDE)
        await openDrilled(page, DRILLED_TITLE)
        await setPeriod(page, '2035-01-01', '2035-12-31')
        const runs = watchRuns(page, reportIdOf(page.url()))
        await (await cellOf(page, 'amount', team)).click()
        const panel = drillPanel(page)
        await expect(rowOf(page, 'Lou', panel)).toBeVisible()
        await markTable(panel)
        const screen = inStatement(page, 'report-screen')

        // Narrower: the panel's report covers the source as a page (R14: the mark
        // proves no remount; every run after the crossing is the same run).
        const atNarrow = runs.length
        await page.setViewportSize(NARROW)
        await expect
            .poll(async () => {
                const [p, s] = [await panel.boundingBox(), await screen.boundingBox()]
                return p && s ? Math.round(p.width) === Math.round(s.width) && Math.round(p.x) === Math.round(s.x) : false
            })
            .toBe(true)
        await expect(panel.getByTestId('report-back')).toHaveText(DRILLED_TITLE)
        await expect(panel.getByTestId('report-drill-close')).toHaveCount(0)
        await expect(panel.getByTestId('report-drill-panel-title')).toHaveText(DRILL_TARGET_TITLE)
        await expect(panel.getByTestId('report-drill-open-full')).toBeVisible()
        await expect(panel.getByTestId('report-loading')).toHaveCount(0)
        await expect(panel.locator('[data-e2e-kept="1"]')).toHaveCount(1)
        await expect(rowOf(page, 'Lou', panel)).toBeVisible()
        expect(runs.slice(atNarrow).every(body => body === runs[atNarrow - 1])).toBe(true)

        // Wider again: a 480 px panel, the same table and rows.
        const atWide = runs.length
        await page.setViewportSize(WIDE)
        await expect
            .poll(async () => Math.round((await panel.boundingBox())?.width ?? 0))
            .toBe(480)
        await expect(panel.getByTestId('report-drill-close')).toBeVisible()
        await expect(panel.getByTestId('report-loading')).toHaveCount(0)
        await expect(panel.locator('[data-e2e-kept="1"]')).toHaveCount(1)
        await expect(rowOf(page, 'Lou', panel)).toBeVisible()
        expect(runs.slice(atWide).every(body => body === runs[atWide - 1])).toBe(true)

        // The page's back link closes it; wider again shows no panel.
        await page.setViewportSize(NARROW)
        await panel.getByTestId('report-back').click()
        await expect(panel).toHaveCount(0)
        await expect(inStatement(page, 'report-title')).toHaveText(DRILLED_TITLE)
        await page.setViewportSize(WIDE)
        await expect(panel).toHaveCount(0)
    })

    test('rows open their record, the keyboard opens a figure, and a reader without the target sees plain figures', async ({
        page,
    }) => {
        await loginAsShippedOwner(page)
        const team = uniqueName('Drill Rows')
        await seed(page, [
            {
                name: team,
                sales: [
                    { customer: 'Max', amount: 400, day: '2036-04-04' },
                    { customer: 'Ned', amount: 600, day: '2036-04-05' },
                ],
            },
        ])
        await page.setViewportSize(NARROW)
        await openDrilled(page, DRILLED_TITLE)
        await setPeriod(page, '2036-01-01', '2036-12-31')

        // Tab reaches a drilled figure, and Enter opens it.
        const figure = await cellOf(page, 'amount', team)
        await inStatement(page, 'report-param-period').focus()
        await tabTo(page, figure)
        await page.keyboard.press('Enter')
        await expect(page.getByTestId('report-title')).toHaveText(DRILL_TARGET_TITLE)

        // A line opens its record: its label is the link, and any cell of it opens it too.
        const max = rowOf(page, 'Max')
        await expect(page.getByRole('link', { name: 'Open: Max' })).toBeVisible()
        await max.hover()
        await expect.poll(() => decorationOf(max)).toBe('underline')
        await max.click()
        await expect(page.getByTestId('stub-sale-customer')).toHaveText('Max')
        await page.getByTestId('stub-sale-back').click()
        await expect(page.getByTestId('report-title')).toHaveText(DRILL_TARGET_TITLE)
        await (await cellOf(page, 'day', 'Ned')).click()
        await expect(page.getByTestId('stub-sale-customer')).toHaveText('Ned')
        await page.getByTestId('stub-sale-back').click()
        await expect(page.getByTestId('report-title')).toHaveText(DRILL_TARGET_TITLE)

        // A line in the panel opens its record; Back returns to the source with the panel.
        await page.getByTestId('report-back').click()
        await expect(page.getByTestId('report-title')).toHaveText(DRILLED_TITLE)
        await page.setViewportSize(WIDE)
        await (await cellOf(page, 'amount', team)).click()
        const panel = drillPanel(page)
        await rowOf(page, 'Max', panel).click()
        await expect(page.getByTestId('stub-sale-customer')).toHaveText('Max')
        await page.getByTestId('stub-sale-back').click()
        await expect(panel).toBeVisible()
        await expect(inStatement(page, 'report-title')).toHaveText(DRILLED_TITLE)

        // In the builder the preview's figures are plain, and Columns names the target.
        await panel.getByTestId('report-drill-close').click()
        await pressToolbar(page, 'report-edit-open', 'Edit report')
        await expect(page.getByTestId('report-builder')).toBeVisible()
        await expect(page.getByTestId('report-builder-column-2-drill')).toHaveText(
            `Figures open ${DRILL_TARGET_TITLE}`
        )
        const preview = await cellOf(page, 'amount', team)
        await expect(page.getByRole('link', { name: /^Show transactions:/ })).toHaveCount(0)
        const editPath = new URL(page.url()).pathname
        await preview.click()
        await expect(drillPanel(page)).toHaveCount(0)
        expect(new URL(page.url()).pathname).toBe(editPath)

        // The target's Details section says its rows open their record.
        await openReportsHub(page)
        await page.getByTestId('reports-hub-tab-mine').click()
        await hubRow(page, DRILL_TARGET_TITLE).click()
        await pressToolbar(page, 'report-edit-open', 'Edit report')
        await expect(page.getByTestId('report-builder-detail-open')).toHaveText(
            'Rows open their record'
        )

        // A member may read the published source but not its target: plain figures.
        const member = await createInvitedUser(page, 'drill')
        try {
            const p = member.inviteePage
            const own = uniqueName('Drill Member')
            await seed(p, [{ name: own, sales: [{ customer: 'Oz', amount: 800, day: '2036-07-07' }] }])
            await p.setViewportSize(WIDE)
            await openDrilled(p, DRILLED_TITLE)
            await setPeriod(p, '2036-01-01', '2036-12-31')
            const plain = await cellOf(p, 'amount', own)
            await expect(plain).toHaveText('8.00')
            await expect(p.getByRole('link', { name: /^Show transactions:/ })).toHaveCount(0)
            const path = new URL(p.url()).pathname
            await plain.click()
            await expect(drillPanel(p)).toHaveCount(0)
            expect(new URL(p.url()).pathname).toBe(path)
        } finally {
            await member.close()
        }
    })
})
```

`createInvitedUser(page, …)` signs `page` in as the shared test user, so the member part comes last.

- [ ] **Step 2: Run it to see it fail**

Run (port check first): `… pnpm exec tinycld-pkg test:e2e -- reports-drill.spec.ts`
Expected: 6 failures, each for a missing feature, not a broken spec: tests 1, 3, 4, 5, 6 time out on the `Show transactions:` link, the `href` or the panel (`report-drill-panel` absent); test 2 at `report-title` (the press on a group figure folds the group, so the target never opens). In test 1 a later timeout at `report-title` reading the target during the held read means the route also held the target's record (one request for both): match the source alone in the predicate (`id="<sourceId>"`). A failure in a step before the first drill assertion (seed, `openDrilled`, `setPeriod`) is a defect of this spec or of Task 9: fix it now. If the run takes more than about 4 minutes, hand back `NEEDS-RUN: <command>` instead.

- [ ] **Step 3: Biome**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/biome check tests/e2e/reports-drill.spec.ts`
Expected: no errors (run with `--write` on this file only to apply its formatting).

- [ ] **Step 4: Hand the diff to the driver** (no commit).

---

### Task 11: Figure links and row links (D1 and D2 in the view)

**Files:**
- Create: `core/reports/lib/link-press.ts`, `core/reports/lib/__tests__/link-press.test.ts`
- Create: `core/reports/hooks/useRowLinks.ts`
- Create: `core/reports/components/ReportLinks.tsx`
- Modify: `core/reports/components/ReportRow.tsx`
- Modify: `core/reports/components/ReportTable.tsx`
- Modify: `core/reports/components/ReportView.tsx`
- Modify: `core/reports/screens/ReportScreen.tsx` (`SavedReportView` passes `drill`)

**Interfaces:**
- Consumes: `drillHref`, `isDrillShown`, `useOpenDrill` (Task 8), `DRILL_KEY`, `DRILL_PREFIX`, `SearchParams` (Task 7), `ResultColumn.drill`, `ResultRow.open` (Task 6), `useOrgHref`, `useStable` (`../hooks/useStable`), `rowLabelText`, `headingText`, `cellDisplay` (lib/format), expo-router `Link`.
- Produces (N4): `isModifiedClick(event: PressLike): boolean`, `linkPress(action: () => void): (event: PressLike) => void`, `interface PressLike` (link-press.ts); `interface RowDrill { sourceId: string; hasPanel: boolean }`, `interface RowLinks { figureHref(column, row): Href | null; onFigure(column, row): void; isFigureShown(column, row): boolean; rowHref(row): Href | null; onRow(row): void }`, `useRowLinks(drill: RowDrill | undefined): RowLinks | undefined` (useRowLinks.ts); `DrillFigure`, `OpenRowLabel`, `RowFrame`, `linkTone(state)` (ReportLinks.tsx); `TableRowProps.links?: RowLinks`; `ReportTableProps.drill?: RowDrill`; `ReportBody` / `ReportView` prop `drill?: RowDrill`.

- [ ] **Step 1: Write the failing test**

Create `core/reports/lib/__tests__/link-press.test.ts`:

```ts
import { describe, expect, it, vi } from 'vitest'
import { linkPress } from '../link-press'

function press(event: Parameters<ReturnType<typeof linkPress>>[0]) {
    const action = vi.fn()
    const preventDefault = vi.fn()
    const stopPropagation = vi.fn()
    linkPress(action)({ preventDefault, stopPropagation, ...event })
    return { action, preventDefault, stopPropagation }
}

describe('linkPress', () => {
    it('runs the action in place of the href on a plain click', () => {
        const { action, preventDefault, stopPropagation } = press({ button: 0 })
        expect(action).toHaveBeenCalledOnce()
        expect(preventDefault).toHaveBeenCalledOnce()
        // A row's own press must not run as well when its label link is pressed.
        expect(stopPropagation).toHaveBeenCalledOnce()
    })

    it('leaves a modified click to the browser', () => {
        for (const key of ['metaKey', 'ctrlKey', 'shiftKey', 'altKey'] as const) {
            const { action, preventDefault } = press({ button: 0, [key]: true })
            expect(action).not.toHaveBeenCalled()
            expect(preventDefault).not.toHaveBeenCalled()
        }
    })

    it('leaves a middle click to the browser', () => {
        const { action, preventDefault } = press({ button: 1 })
        expect(action).not.toHaveBeenCalled()
        expect(preventDefault).not.toHaveBeenCalled()
    })

    it('runs on a native press, which has no keys or button', () => {
        expect(press({}).action).toHaveBeenCalledOnce()
    })
})
```

- [ ] **Step 2: Run it to see it fail**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run core/reports/lib/__tests__/link-press.test.ts`
Expected: FAIL: `../link-press` does not exist.

- [ ] **Step 3: `link-press.ts`**

Create `core/reports/lib/link-press.ts`:

```ts
/** The parts of a press event a link reads; a native press has no keys or button. */
export interface PressLike {
    preventDefault?(): void
    stopPropagation?(): void
    metaKey?: boolean
    ctrlKey?: boolean
    shiftKey?: boolean
    altKey?: boolean
    button?: number
}

/** A click the browser opens in a new tab or window: with a key held, or not the main button. */
export function isModifiedClick(event: PressLike): boolean {
    const hasKey = Boolean(event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)
    return hasKey || (event.button !== undefined && event.button !== 0)
}

/**
 * A link's press. On web the Pressable's own click handler replaces Link's, so a
 * plain press prevents the anchor's default and runs the action (a panel, or a push
 * the view chooses); on native Link's onPress runs after this one and skips a
 * default-prevented event. A modified click is left to the browser, which opens the
 * link's href (react-native-web never calls onPress for an ⌥-click). The press stops
 * here, so a row that opens on a press does not open again under its label link.
 */
export function linkPress(action: () => void) {
    return (event: PressLike) => {
        if (isModifiedClick(event)) return
        event.preventDefault?.()
        event.stopPropagation?.()
        action()
    }
}
```

- [ ] **Step 4: `useRowLinks`**

Create `core/reports/hooks/useRowLinks.ts`:

```ts
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { type Href, useLocalSearchParams, useRouter } from 'expo-router'
import { useMemo, useRef } from 'react'
import { drillHref, isDrillShown, useOpenDrill } from '../lib/drill'
import type { ResultColumn, ResultRow } from '../lib/types'
import { DRILL_KEY, DRILL_PREFIX, type SearchParams } from '../lib/view-url'
import { useStable } from './useStable'

/** The report a view shows, for its figures' drills, and whether its screen hosts a drill panel. */
export interface RowDrill {
    sourceId: string
    hasPanel: boolean
}

/** What a table's figures and rows open. */
export interface RowLinks {
    figureHref(column: ResultColumn, row: ResultRow): Href | null
    onFigure(column: ResultColumn, row: ResultRow): void
    isFigureShown(column: ResultColumn, row: ResultRow): boolean
    rowHref(row: ResultRow): Href | null
    onRow(row: ResultRow): void
}

// Only the drill keys mark a figure, so the links change only when they do.
function drillSearch(search: SearchParams): SearchParams {
    return Object.fromEntries(
        Object.entries(search).filter(([key]) => key === DRILL_KEY || key.startsWith(DRILL_PREFIX))
    )
}

/**
 * The table's links, or undefined where figures and rows are plain (the builder's
 * preview passes no drill). They keep their identity while the drill keys stay, so
 * FlashList redraws the rows (extraData) only when a figure's mark may change.
 */
export function useRowLinks(drill: RowDrill | undefined): RowLinks | undefined {
    const router = useRouter()
    const orgHref = useOrgHref()
    const shown = useStable(drillSearch(useLocalSearchParams()))
    const openDrill = useOpenDrill({
        sourceId: drill?.sourceId ?? '',
        hasPanel: drill?.hasPanel ?? false,
    })
    // The press reads the newest handler, so the links need not change with it.
    const latest = useRef(openDrill)
    latest.current = openDrill
    const sourceId = drill?.sourceId
    return useMemo<RowLinks | undefined>(() => {
        if (sourceId === undefined) return undefined
        return {
            figureHref: (column, row) => {
                const href = drillHref(column, row, sourceId)
                return href === null ? null : orgHref(href)
            },
            onFigure: (column, row) => latest.current(column, row),
            isFigureShown: (column, row) => isDrillShown(column, row, shown),
            rowHref: row => (row.open ? orgHref(row.open) : null),
            onRow: row => {
                if (row.open) router.push(orgHref(row.open))
            },
        }
    }, [sourceId, shown, orgHref, router])
}
```

- [ ] **Step 5: `ReportLinks.tsx`**

Create `core/reports/components/ReportLinks.tsx`:

```tsx
import { type Href, Link } from 'expo-router'
import type { ReactNode } from 'react'
import {
    Platform,
    Pressable,
    type PressableStateCallbackType,
    Text,
    View,
    type ViewStyle,
} from 'react-native'
import type { RowLinks } from '../hooks/useRowLinks'
import { linkPress } from '../lib/link-press'
import type { ResultRow } from '../lib/types'

// react-native-web adds `hovered` to a Pressable's state; native has no pointer.
function isHovered(state: PressableStateCallbackType): boolean {
    return 'hovered' in state && state.hovered === true
}

/** A link's text: underlined under the pointer on web, dimmed while pressed on native. */
export function linkTone(state: PressableStateCallbackType): string {
    if (isHovered(state)) return 'underline'
    return Platform.OS !== 'web' && state.pressed ? 'opacity-60' : ''
}

interface DrillFigureProps {
    href: Href
    testID: string
    style: { width: number | undefined; minHeight: number; flexGrow: number }
    /** "Show transactions: <row label>, <column heading>, <figure>". */
    name: string
    text: string
    textClass: string
    lines: number
    /** The drill panel shows exactly this figure's params. */
    isShown: boolean
    /** Part of a stuck copy: no tab stop, hidden from assistive technology. */
    isCopy: boolean
    onPress(): void
}

/**
 * A figure that opens the rows behind it. Its href is always the full report, so
 * ⌘-click and middle-click open it in a new tab; a plain press runs the view's
 * drill (a panel, or the full report as a screen).
 */
export function DrillFigure(props: DrillFigureProps) {
    const marked = props.isShown ? 'bg-accent' : ''
    return (
        <Link href={props.href} asChild>
            <Pressable
                testID={props.testID}
                accessibilityRole="link"
                accessibilityLabel={props.name}
                aria-selected={props.isShown}
                accessible={!props.isCopy}
                focusable={!props.isCopy}
                tabIndex={props.isCopy ? -1 : 0}
                onPress={linkPress(props.onPress)}
                className={`justify-center px-3 web:cursor-pointer ${marked}`}
                style={props.style}
            >
                {state => (
                    <Text
                        numberOfLines={props.lines}
                        className={`${props.textClass} ${linkTone(state)}`}
                    >
                        {props.text}
                    </Text>
                )}
            </Pressable>
        </Link>
    )
}

interface OpenRowLabelProps {
    href: Href
    testID: string
    text: string
    textClass: string
    lines: number
    style: ViewStyle
    onPress(): void
}

/** A row's label that opens its record (D2), named "Open: <label>". */
export function OpenRowLabel(props: OpenRowLabelProps) {
    return (
        <Link href={props.href} asChild>
            <Pressable
                testID={props.testID}
                accessibilityRole="link"
                accessibilityLabel={`Open: ${props.text}`}
                onPress={linkPress(props.onPress)}
                className="flex-row items-center pr-3 web:cursor-pointer"
                style={props.style}
            >
                {state => (
                    <Text
                        numberOfLines={props.lines}
                        className={`${props.textClass} ${linkTone(state)}`}
                    >
                        {props.text}
                    </Text>
                )}
            </Pressable>
        </Link>
    )
}

interface RowFrameProps {
    row: ResultRow
    links: RowLinks | undefined
    isCopy: boolean | undefined
    className: string
    children: ReactNode
}

/**
 * A row's box. A row that opens its record takes a press anywhere on it; its label
 * link is the one control in the tab order and for assistive technology.
 */
export function RowFrame({ row, links, isCopy, className, children }: RowFrameProps) {
    const href = links && !isCopy ? links.rowHref(row) : null
    if (!links || href === null) return <View className={className}>{children}</View>
    return (
        <Pressable
            accessible={false}
            focusable={false}
            tabIndex={-1}
            onPress={linkPress(() => links.onRow(row))}
            className={`${className} web:cursor-pointer`}
        >
            {children}
        </Pressable>
    )
}
```

If the typecheck refuses `aria-selected` on `Pressable`, use `accessibilityState={{ selected: props.isShown }}` and check in the browser that react-native-web writes `aria-selected` (e2e test 3 reads it); never cast.

- [ ] **Step 6: `ReportRow.tsx`**

Imports: add `headingText` to the `../lib/format` import; add `import type { RowLinks } from '../hooks/useRowLinks'` and `import { DrillFigure, OpenRowLabel, RowFrame } from './ReportLinks'`.

Replace `LabelText` with a class helper it uses, so the open label draws its text the same way:

```tsx
const labelTextClass = (row: ResultRow) =>
    `min-w-0 flex-1 text-sm ${WHOLE_WORDS} ${labelTone(row.kind)} ${ROW_WEIGHT[row.kind]}`

function LabelText({ row, text, lines = 1 }: { row: ResultRow; text: string; lines?: number }) {
    return (
        <Text numberOfLines={lines} className={labelTextClass(row)}>
            {wrappedText(text, lines)}
        </Text>
    )
}
```

`RowLabelProps`: add

```ts
    /** What the row opens; absent: a plain label (the builder's preview). */
    links?: RowLinks
```

In `RowLabel`, after the `isToggle` line and the destructuring, before the `return`:

```tsx
    const href = props.links && !isCopy ? props.links.rowHref(item.row) : null
    if (props.links && href !== null) {
        const links = props.links
        return (
            <OpenRowLabel
                href={href}
                testID={labelId(item.row, isCopy)}
                text={text}
                textClass={labelTextClass(item.row)}
                lines={lines ?? 1}
                style={labelStyle(item.row, justify, minHeight)}
                onPress={() => links.onRow(item.row)}
            />
        )
    }
```

(`OpenRowLabel` names itself `Open: <text>`; `text` is `rowLabelText(item.row, format)`, the same text the row shows. A wrapped label keeps its colon breaks only in `LabelText`; an open row's label is a detail row's, which the view draws on its lines as the fit gives them.)

`RowCellProps`: add `links?: RowLinks`. In `RowCell`, destructure `links` and, after `const style = cellBoxStyle(width, minHeight)`, before `if (!item.isToggle)`:

```tsx
    // A figure whose column drills, on a row with a @drill cell, opens the rows
    // behind it; on a group row this press replaces the fold (the label still folds).
    const href = links ? links.figureHref(column, row) : null
    if (links && href !== null) {
        const name = `Show transactions: ${rowLabelText(row, format)}, ${headingText(column, format)}, ${display.text}`
        return (
            <DrillFigure
                href={href}
                testID={testID}
                style={style}
                name={name}
                text={shown}
                textClass={cellTextClass(display, justify, row.kind, isFigure)}
                lines={lines}
                isShown={links.isFigureShown(column, row)}
                isCopy={isCopy}
                onPress={() => links.onFigure(column, row)}
            />
        )
    }
```

The comment above `RowCell` ("A group row's label is its one accessible toggle…") gains: "A drilled figure is a link instead (D1)."

`TableRowProps`: add

```ts
    /** What figures and rows open (useRowLinks); absent: all plain. */
    links?: RowLinks
```

`OneLineRow` and `BlocksRow`: destructure `links`; pass `links={links}` to every `RowLabel` and `RowCell` they draw; replace each row's outer `<View className={…}>…</View>` with `<RowFrame row={item.row} links={links} isCopy={isCopy} className={…}>…</RowFrame>` (same class string). `StickyGroup` passes `{...props}` on, so its copy gets `links` and draws them as copies (`isCopy`).

- [ ] **Step 7: `ReportTable.tsx` and `ReportView.tsx`**

`ReportTable.tsx`: import `{ type RowDrill, useRowLinks } from '../hooks/useRowLinks'`. In `useTableListProps`, destructure `links` and make `extraData`:

```ts
    const extraData = useMemo<TableData>(
        () => ({ columns, layout, format, onToggleGroup, links, rows }),
        [columns, layout, format, onToggleGroup, links, rows]
    )
```

`ReportTableProps`: add

```ts
    /**
     * The report shown, for its figures' drills and its rows' records (D1, D2). Not
     * given (the builder's preview): figures and rows are plain and do not open.
     */
    drill?: RowDrill
```

In `ReportTable`, after `const sorting = useHeadingSort(…)` (before the early `return null`, so the hook order stays): `const links = useRowLinks(props.drill)`; pass `links={links}` to `TableList`.

`ReportView.tsx`: import `type RowDrill` from `../hooks/useRowLinks`. `BodyProps`: add `drill?: RowDrill` with the doc "The report shown, so its figures and rows open; not given (the builder's preview), they are plain.". `ReportBody` and `ViewBody` pass `drill` on; `TableBody` passes `drill={drill}` to `ReportTable`. `ReportView({ model, side, drill }: { model: ReportViewModel; side?: ReactNode; drill?: RowDrill })` passes `drill` to `ViewBody`. The builder panel's `ReportBody` (ReportBuilderPanel.tsx:270) passes none and stays plain.

- [ ] **Step 8: The saved report passes its drill**

`ReportScreen.tsx`, `SavedReportView`: `<ReportView model={model} drill={drill} />` with, above the return:

```tsx
    // No drill panel yet on this screen: a figure opens its report as its own screen.
    const drill = report ? { sourceId: report.id, hasPanel: false } : undefined
```

(Task 14 sets `hasPanel: true` when it mounts the panel.)

- [ ] **Step 9: Run the tests, the typecheck and a view e2e**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run core/reports/lib/__tests__/link-press.test.ts core/reports/lib/__tests__/drill.test.ts`
Expected: PASS.

Run: `cd /Users/nas/tmp/tinycld/books/tinycld && PATH=/Users/nas/tmp/tinycld/books/node_modules/.bin:$PATH tinycld-pkg typecheck`
Expected: no errors. If `PressableStateCallbackType` has no `hovered` key, `'hovered' in state` still narrows; never cast.

Run (port check first): `… test:e2e -- reports-wrap.spec.ts`, then `… test:e2e -- reports-period-pivot.spec.ts`
Expected: PASS (rows, folds and the sticky copy draw as before: no stub report there has a drill or an open path).

Run (port check first): `… test:e2e -- reports-drill.spec.ts -g "rows open their record"`
Expected: FAIL only after "Tab reaches a drilled figure" and the record steps pass, at the drill panel (Task 14). If Enter on the focused figure opens the target twice or follows the `href` (a full load), react-native-web handles Enter on a focused link itself: make `linkPress` the one handler that runs (read how RN-web's PressResponder treats a keyboard press on an element with `href`) and add the case to `link-press.test.ts`.

- [ ] **Step 10: Biome**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/biome check core/reports/lib/link-press.ts core/reports/lib/__tests__/link-press.test.ts core/reports/hooks/useRowLinks.ts core/reports/components/ReportLinks.tsx core/reports/components/ReportRow.tsx core/reports/components/ReportTable.tsx core/reports/components/ReportView.tsx core/reports/screens/ReportScreen.tsx`
Expected: no errors.

- [ ] **Step 11: Hand the diff to the driver** (no commit).

---

### Task 12: The back link names the source (D3)

**Files:**
- Create: `core/reports/lib/report-back.ts`, `core/reports/lib/__tests__/report-back.test.ts`
- Modify: `core/reports/hooks/useBackToReports.ts` (`useReportBack`)
- Modify: `core/reports/components/ReportHeader.tsx` (`ReportBackLink` exported with `label`; `ReportHeader` `backLabel`)
- Modify: `core/reports/hooks/useReportView.ts` (`backLabel` input and model field)
- Modify: `core/reports/components/ReportView.tsx` (`ReportTop` passes it)
- Modify: `core/reports/screens/ReportScreen.tsx` (reads `from`)

**Interfaces:**
- Consumes: `useReportRecord` (`state`, `record`, `retry`), `FROM_KEY` (Task 7), `useOrgHref`, `router`.
- Produces: `HUB_BACK_LABEL = 'Reports'`, `reportBack(state: RecordState, title: string | undefined): { label: string; to: 'source' | 'hub' }` (report-back.ts, N4); `useReportBack(fromId: string): { label: string; onBack: () => void }` (N4: `fromId` is a `string`; a screen without `from` uses `useBackToReports` and never reads a record); `ReportBackLink({ label, onBack })`; `ReportHeader` prop `backLabel?: string`; `ReportViewInput.backLabel?: string`; `ReportViewModel.backLabel?: string`.

- [ ] **Step 1: Write the failing test**

Create `core/reports/lib/__tests__/report-back.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { HUB_BACK_LABEL, reportBack } from '../report-back'

describe('reportBack', () => {
    it('names the source and returns to it once its title is read', () => {
        expect(reportBack('ready', 'Profit and Loss')).toEqual({ label: 'Profit and Loss', to: 'source' })
    })

    it('reads Reports while the title loads and when its read fails, and still returns to the source', () => {
        expect(reportBack('loading', undefined)).toEqual({ label: HUB_BACK_LABEL, to: 'source' })
        expect(reportBack('error', undefined)).toEqual({ label: HUB_BACK_LABEL, to: 'source' })
    })

    it('reads Reports and goes to the hub when the reader cannot view the source', () => {
        expect(reportBack('missing', undefined)).toEqual({ label: HUB_BACK_LABEL, to: 'hub' })
    })

    it('never shows a blank label', () => {
        expect(reportBack('ready', '')).toEqual({ label: HUB_BACK_LABEL, to: 'source' })
        expect(HUB_BACK_LABEL).toBe('Reports')
    })
})
```

- [ ] **Step 2: Run it to see it fail**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run core/reports/lib/__tests__/report-back.test.ts`
Expected: FAIL: `../report-back` does not exist.

- [ ] **Step 3: `report-back.ts` and `useReportBack`**

Create `core/reports/lib/report-back.ts`:

```ts
import type { RecordState } from '../hooks/useReportRecord'

/** The back link of a report opened from the hub. */
export const HUB_BACK_LABEL = 'Reports'

/**
 * The back link of a report opened from another one (`from`, D3). With the source's
 * title it names the source. While the title loads, or when its read failed, it
 * reads as the hub's (no blank, no spinner) but still returns to the source. A
 * source the reader cannot view has no record: the link goes to the hub.
 */
export function reportBack(
    state: RecordState,
    title: string | undefined
): { label: string; to: 'source' | 'hub' } {
    if (state === 'missing') return { label: HUB_BACK_LABEL, to: 'hub' }
    if (state !== 'ready' || !title) return { label: HUB_BACK_LABEL, to: 'source' }
    return { label: title, to: 'source' }
}
```

`useBackToReports.ts`: add imports `import { reportBack } from '../lib/report-back'` and `import { useReportRecord } from './useReportRecord'`, then:

```ts
/**
 * The back link of a report opened from report `fromId`. It returns to the source as
 * the reader left it (history back), or, opened from a pasted link with no history,
 * opens the source with its defaults.
 */
export function useReportBack(fromId: string): { label: string; onBack: () => void } {
    const orgHref = useOrgHref()
    const source = useReportRecord(fromId)
    const back = reportBack(source.state, source.record?.title)
    const onBack = () => {
        if (back.to === 'hub') {
            router.replace(orgHref('reports'))
            return
        }
        // A failed read stays failed for the next screen's query of the same record
        // until it is recovered, so the source screen would open on its error.
        if (source.state === 'error') source.retry()
        if (router.canGoBack()) router.back()
        else router.replace(orgHref(`reports/${fromId}`))
    }
    return { label: back.label, onBack }
}
```

- [ ] **Step 4: The header and the view**

`ReportHeader.tsx`: import `HUB_BACK_LABEL` from `../lib/report-back`. Replace `ReportBackLink`:

```tsx
/** The link back to where the report was opened from: the hub, or the report it was drilled from. */
export function ReportBackLink({ label, onBack }: { label: string; onBack: () => void }) {
    const muted = useThemeColor('muted-foreground')
    return (
        <Pressable
            testID="report-back"
            accessibilityRole="link"
            onPress={onBack}
            className="flex-row items-center gap-1 self-start"
            style={{ minHeight: REPORT_TAP_TARGET }}
        >
            <ArrowLeft size={16} color={muted} />
            <Text numberOfLines={1} className="text-sm text-muted-foreground">
                {label}
            </Text>
        </Pressable>
    )
}
```

`ReportHeader`: add the prop `backLabel = HUB_BACK_LABEL` (type `backLabel?: string`, doc "The back link's text: the source report's title after a drill (D3)") and render `<ReportBackLink label={backLabel} onBack={onBack} />`.

`useReportView.ts`: `ReportViewInput` gains

```ts
    /** The back link's text; absent: "Reports" (the hub). */
    backLabel?: string
```

and the returned model `backLabel: input.backLabel ?? HUB_BACK_LABEL` (import from `../lib/report-back`). Make it optional in the model type, so a hand-built model needs none:

```ts
export type ReportViewModel = Omit<
    ReturnType<typeof useReportView>,
    'onSetSort' | 'fit' | 'backLabel'
> & {
    onSetSort?(sort: ViewSort | null): void
    fit?: DisplayFit
    backLabel?: string
}
```

Update the comment above it: "onSetSort, fit and backLabel are optional: a hand-built model has none of them; then the Sort chip is not shown, the table takes the default fit and the back link reads Reports."

`ReportView.tsx`, `ReportTop`: pass `backLabel={model.backLabel}` to `ReportHeader`.

- [ ] **Step 5: The screen reads `from`**

`ReportScreen.tsx`: import `FROM_KEY` with `EDIT_KEY` from `../lib/view-url`, `useReportBack` with `useBackToReports` from `../hooks/useBackToReports`, and `HUB_BACK_LABEL` from `../lib/report-back`. `FrameProps` gains `backLabel?: string`, passed to its `ReportHeader`. `SavedReportView` and `EditingBody` take `back: ReportBack` in place of `onBack` and pass `onBack: back.onBack` and `backLabel: back.label` to `useReportView` (and `back.onBack` / `back.label` to `ReportFrame` and `EditingReport`, which passes them to its `useReportView` too). Replace `ReportScreen`:

```tsx
interface ReportBack {
    label: string
    onBack: () => void
}

// A drilled report (`from`) names its source on its back link; the hub's reads
// Reports and reads no record.
export function ReportScreen({ reportId }: { reportId: string }) {
    const search = useLocalSearchParams<{ [FROM_KEY]?: string }>()
    const fromId = search[FROM_KEY]
    if (fromId) return <ReportFromSource reportId={reportId} fromId={fromId} />
    return <ReportFromHub reportId={reportId} />
}

function ReportFromSource({ reportId, fromId }: { reportId: string; fromId: string }) {
    const back = useReportBack(fromId)
    return <ReportScreenBody reportId={reportId} back={back} />
}

function ReportFromHub({ reportId }: { reportId: string }) {
    const onBack = useBackToReports()
    return <ReportScreenBody reportId={reportId} back={{ label: HUB_BACK_LABEL, onBack }} />
}

function ReportScreenBody({ reportId, back }: { reportId: string; back: ReportBack }) {
    const record = useReportRecord(reportId)
    const search = useLocalSearchParams<{ [EDIT_KEY]?: string }>()
    const frame = { onBack: back.onBack, backLabel: back.label }
    if (record.state === 'loading') return <ReportFrame title="" state="loading" {...frame} />
    if (record.state === 'error') {
        return <ReportFrame title="" state="error" onRetry={record.retry} {...frame} />
    }
    if (record.state === 'missing') return <ReportFrame title="" state="no-access" {...frame} />
    if (search[EDIT_KEY] === '1' && record.access.canEdit) {
        return <EditingBody record={record} back={back} />
    }
    return <SavedReportView record={record} back={back} />
}
```

- [ ] **Step 6: Run the tests, the typecheck and e2e tests 1 and 2**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run core/reports/lib/__tests__/report-back.test.ts`
Expected: PASS.

Run: `cd /Users/nas/tmp/tinycld/books/tinycld && PATH=/Users/nas/tmp/tinycld/books/node_modules/.bin:$PATH tinycld-pkg typecheck`
Expected: no errors (the Books scratch model still compiles: `backLabel` is optional).

Run (port check first): `… test:e2e -- reports-drill.spec.ts -g "as a screen"`
Expected: PASS (tests 1 and 2).

- [ ] **Step 7: Biome**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/biome check core/reports/lib/report-back.ts core/reports/lib/__tests__/report-back.test.ts core/reports/hooks/useBackToReports.ts core/reports/components/ReportHeader.tsx core/reports/hooks/useReportView.ts core/reports/components/ReportView.tsx core/reports/screens/ReportScreen.tsx`
Expected: no errors.

- [ ] **Step 8: Hand the diff to the driver** (no commit).

---

### Task 13: Two report views on one route (D4c)

**Files:**
- Modify: `core/components/workspace/useBreakpoint.ts` (`breakpointFor`)
- Create: `core/components/workspace/__tests__/useBreakpoint.test.ts`
- Modify: `core/reports/hooks/useReportView.ts` (`urlPrefix`, `uiScope`, `viewWidth`)
- Modify: `core/reports/components/ReportNotices.tsx` (`width`)
- Modify: `core/reports/components/ReportView.tsx` (passes `model.viewWidth`)

**Interfaces:**
- Consumes: `parseViewOptions(defs, search, prefix)`, `serializeViewOptions(defs, options, prefix)`, `SORT_KEY` (Task 7); store `scopes[scope]` and the scoped actions, `ReportUIScope` (Task 8).
- Produces: `breakpointFor(width: number): Breakpoint` (useBreakpoint.ts, N4: the old private `bucketFor`, exported); `ReportViewInput.urlPrefix?: string` (default `''`), `ReportViewInput.uiScope?: ReportUIScope` (default `'main'`), `ReportViewInput.viewWidth?: number` (N4; absent: the window); `ReportViewModel.viewWidth?: number`; `ReportNotices` prop `width?: number`; with a prefix, `onCopyLink` is `null`.

- [ ] **Step 1: Write the failing test**

Create `core/components/workspace/__tests__/useBreakpoint.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { breakpointFor } from '../useBreakpoint'

describe('breakpointFor', () => {
    it('gives the bucket of a width, so a view can use its own width, not the window', () => {
        expect(breakpointFor(480)).toBe('mobile')
        expect(breakpointFor(767)).toBe('mobile')
        expect(breakpointFor(768)).toBe('tablet')
        expect(breakpointFor(1023)).toBe('tablet')
        expect(breakpointFor(1024)).toBe('desktop')
    })
})
```

- [ ] **Step 2: Run it to see it fail**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run core/components/workspace/__tests__/useBreakpoint.test.ts`
Expected: FAIL: `breakpointFor` is not exported.

- [ ] **Step 3: `breakpointFor`**

`useBreakpoint.ts`: rename `bucketFor` to `breakpointFor`, export it, and add the doc line "The breakpoint of a width: the window's (useBreakpoint), or a view's own (a 480 px drill panel draws as a phone)."; `useBreakpoint` reads `breakpointFor(s.width)`.

- [ ] **Step 4: Prefix, scope and width in `useReportView`**

`useReportView.ts`: import `breakpointFor` with `useBreakpoint`, and `type ReportUIScope` from `../stores/report-ui-store`. `ReportViewInput` gains:

```ts
    /**
     * The key prefix of this view's options in the route (the drill panel's is
     * DRILL_PREFIX), so two views on one route never read or write each other's.
     */
    urlPrefix?: string
    /** The UI store scope of this view's popovers and Customize. */
    uiScope?: ReportUIScope
    /** The view's own width (the 480 px drill panel); absent: the window's. */
    viewWidth?: number
```

In `useReportView`:

```ts
    const prefix = input.urlPrefix ?? ''
    const scope = input.uiScope ?? 'main'
    const options = parseViewOptions(defs, search, prefix)
```

```ts
    const windowBreakpoint = useBreakpoint()
    // A view narrower than the window (the drill panel) lays itself out by its own width.
    const breakpoint =
        input.viewWidth === undefined ? windowBreakpoint : breakpointFor(input.viewWidth)
```

The store reads become `useReportUIStore(s => s.scopes[scope].surface)` and `useReportUIStore(s => s.scopes[scope].isCustomizeOpen) && isFocused`. Every call of a store action in the hook passes `scope`: `closeSurface(scope)` in `onApplyParam`, `onRemoveParam`, `onSetSort` and `closeOnBlur`; `closeCustomize(scope)` in `closeOnBlur`. The model's handlers (Task 8 wrapped them for `'main'`) become:

```ts
        onOpenSurface: (surface: ReportSurface) => openSurface(surface, scope),
        onCloseSurface: () => closeSurface(scope),
        onCloseSurfaceOf: (kind: ReportSurface['kind']) => closeSurfaceOf(kind, scope),
        onOpenCustomize: () => openCustomize(scope),
        onCloseCustomize: () => closeCustomize(scope),
```

(import `type ReportSurface` from `../stores/report-ui-store` if the hook does not have it.)

`write`: `router.setParams(serializeViewOptions(defs, next, prefix))`. `onSortColumn`: `router.setParams({ [`${prefix}${SORT_KEY}`]: next ? encodeSort(next) : undefined })`, with `prefix` in its dependency list.

`onCopyLink`: only the main view copies a link (the panel's options are the full report's, one press away):

```ts
    const onCopyLink =
        source?.kind === 'saved' && prefix === ''
            ? () => { … unchanged … }
            : null
```

The model gains `viewWidth: input.viewWidth`. Add `'viewWidth'` to the `Omit` of `ReportViewModel` and `viewWidth?: number` to its added fields (a hand-built model has none: the window's width).

- [ ] **Step 5: The notices read the view's width**

`ReportNotices.tsx`: `useNoticeBand(notices, isCompact, width?: number)` reads `const windowWidth = useWindowWidth()` and `const bandWidth = (width ?? windowWidth) - BAND_PADDING`, and uses `bandWidth` where it used `width`. `NoticeBand` and `ReportNotices` take `width?: number` (doc "The view's own width (the drill panel); absent: the window's") and pass it on.

`ReportView.tsx`, `ReportTop`: `<ReportNotices … width={model.viewWidth} />`. `isCompact = model.breakpoint === 'mobile'` stays: the breakpoint is now the view's.

- [ ] **Step 6: Run the tests, the typecheck and two view e2e**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run core/components/workspace/__tests__/useBreakpoint.test.ts core/reports/lib/__tests__/view-url.test.ts core/reports/stores/__tests__/report-ui-store.test.ts`
Expected: PASS.

Run: `cd /Users/nas/tmp/tinycld/books/tinycld && PATH=/Users/nas/tmp/tinycld/books/node_modules/.bin:$PATH tinycld-pkg typecheck`
Expected: no errors.

Run (port check first): `… test:e2e -- reports-ledger-v3.spec.ts`, then `… test:e2e -- reports-wrap.spec.ts`
Expected: PASS (the main view's params, sort, Customize, Copy link and the phone's notice band behave as before).

- [ ] **Step 7: Biome**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/biome check core/components/workspace/useBreakpoint.ts core/components/workspace/__tests__/useBreakpoint.test.ts core/reports/hooks/useReportView.ts core/reports/components/ReportNotices.tsx core/reports/components/ReportView.tsx`
Expected: no errors.

- [ ] **Step 8: Hand the diff to the driver** (no commit).

---

### Task 14: The drill panel (D4d), mounted beside the saved report

**Files:**
- Create: `core/reports/lib/figure-focus.ts` (`focusNode`)
- Modify: `core/reports/lib/drill.ts` (`fullReportHref`), `core/reports/lib/__tests__/drill.test.ts`
- Modify: `core/reports/lib/spec.ts` (`UNREADABLE_SPEC`, moved from ReportScreen.tsx)
- Create: `core/reports/hooks/useDrillPanel.ts`
- Create: `core/reports/components/ReportDrillPanel.tsx`
- Modify: `core/reports/screens/ReportScreen.tsx` (`side` panel, `hasPanel: true`)
- Modify: `tests/e2e/reports-failure-states.spec.ts`

**Interfaces:**
- Consumes: `useReportView({ urlPrefix, uiScope, viewWidth })` (Task 13), `useReportRecord`, `useDetailSurface`, `DETAIL_PANEL_WIDTH`, `DRILL_KEY`, `DRILL_PREFIX`, `clearedDrillKeys`, `serializeViewOptions`, `ReportBackLink` (Task 12), `ReportBody` with `drill` (Task 11), `ReportControls`, `ReportNotices` `width`, `ReportDocumentHeader`, `useTableLayout`, `layoutNoticeOf`, `LayerEscape` (`@tinycld/core/ui/overlay`), `LoadingState`, `MessageBody`.
- Produces (N4): `type FocusTarget`, `focusNode(node: FocusTarget): void` (figure-focus.ts); `fullReportHref(reportId: string, defs: readonly ParamDef[], options: ViewOptions, sourceId: string): string` (drill.ts); `UNREADABLE_SPEC: BrokenMark` (spec.ts); `interface DrillPanelInput { targetId: string; sourceId: string; sourceTitle: string }`, `useDrillPanel(input: DrillPanelInput)`, `type DrillPanelModel` (useDrillPanel.ts; N4: the outline's `useDrillPanel({ sourceId, sourceTitle })` also takes `targetId`, which `ReportDrillPanel` reads from `d`, so the hook never runs without a target); `ReportDrillPanel({ sourceId, sourceTitle })`; test ids `report-drill-panel`, `report-drill-panel-header`, `report-drill-panel-title`, `report-drill-open-full`, `report-drill-close`, `report-drill-loading`, `report-drill-error` (+ `report-drill-retry`), `report-drill-no-access`. R13: the panel mounts in this task, not Task 15.

- [ ] **Step 1: Write the failing tests**

`core/reports/lib/__tests__/drill.test.ts`: add `fullReportHref` to the `../drill` import and `DRILL_PREFIX` to the `../view-url` import, then:

```ts
describe('fullReportHref', () => {
    it("writes the panel's options without the prefix, with the source report", () => {
        const options = parseViewOptions(
            TARGET,
            { 'd.p-period': '2026-03-01..2026-03-31', 'd.s': 'amount:asc', 'p-period': 'lastMonth' },
            DRILL_PREFIX
        )
        const href = fullReportHref('r2', TARGET, options, 'r1')
        expect(href.startsWith('reports/r2?')).toBe(true)
        const search = searchOf(href)
        expect(search).toEqual({ 'p-period': '2026-03-01..2026-03-31', s: 'amount:asc', from: 'r1' })
    })
})
```

`tests/e2e/reports-failure-states.spec.ts`: import `createInvitedUser` with `skipWithoutReportsStub` from `./helpers`; add `cellOf`, `DRILLED_TITLE`, `drillPanel`, `openDrilled`, `setPeriod` to the `./reports-helpers` import. In the file's header comment add: "The drill panel's own states run on "Stub drilled sales" at 1440 px, whose figures open the unpublished "Stub sales by team" in a panel." Add, inside the describe:

```ts
    test('the drill panel shows its own loading, its error with Retry, and no access', async ({
        page,
    }) => {
        await loginAsShippedOwner(page)
        const team = uniqueName('Panel states')
        await openReportStub(page)
        await addTeam(page, team, 'east')
        await addSale(page, { team, customer: 'Pia', amount: 100, cost: 0, day: '2037-01-05' })
        await page.setViewportSize({ width: 1440, height: 900 })
        await openDrilled(page, DRILLED_TITLE)
        await setPeriod(page, '2037-01-01', '2037-12-31')
        const sourcePath = new URL(page.url()).pathname
        const sourceId = sourcePath.split('/').pop() ?? ''
        const figure = await cellOf(page, 'amount', team)

        // The panel's runs (any report but the source) are held, then failed, until lifted.
        const panelRuns = { mode: 'hold' as 'hold' | 'fail' | 'pass' }
        let release = () => {}
        const held = new Promise<void>(resolve => {
            release = resolve
        })
        await page.route('**/api/reports/run', async route => {
            const body = route.request().postDataJSON() as { reportId?: string } | null
            if (body?.reportId === sourceId || panelRuns.mode === 'pass') return route.continue()
            if (panelRuns.mode === 'hold') await held
            return route.fulfill(NOT_FOUND)
        })
        await figure.click()
        const panel = drillPanel(page)
        await expect(panel.getByTestId('report-loading')).toBeVisible()
        await expect(rowOf(page, team)).toBeVisible()
        panelRuns.mode = 'fail'
        release()
        await expect(panel.getByTestId('report-error-message')).toHaveText(
            'Could not load this report'
        )
        panelRuns.mode = 'pass'
        await panel.getByTestId('report-retry').click()
        await expect(rowOf(page, 'Pia', panel)).toBeVisible()
        await expect(panel.getByTestId('report-error')).toHaveCount(0)
        const targetId = new URL(page.url()).searchParams.get('d') ?? ''

        // A member may read the source (published) but not its target (unpublished).
        const member = await createInvitedUser(page, 'panelna')
        try {
            const p = member.inviteePage
            await p.setViewportSize({ width: 1440, height: 900 })
            // Opening a pasted link is a full load by nature.
            await p.goto(`${sourcePath}?d=${targetId}`)
            await expect(drillPanel(p).getByTestId('report-drill-no-access')).toBeVisible()
            await expect(p.getByTestId('report-title')).toHaveText(DRILLED_TITLE)
        } finally {
            await member.close()
        }
    })
```

- [ ] **Step 2: Run them to see them fail**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run core/reports/lib/__tests__/drill.test.ts`
Expected: FAIL: `fullReportHref` is not exported.

Run (port check first): `… test:e2e -- reports-failure-states.spec.ts -g "drill panel"`
Expected: FAIL: `report-loading` in `report-drill-panel` never shows (a figure opens a screen).

- [ ] **Step 3: `fullReportHref`, `UNREADABLE_SPEC`, `focusNode`**

`drill.ts`: import `type ParamDef` from `./spec` and `serializeViewOptions`, `type ViewOptions` from `./view-url`; add:

```ts
/**
 * The full report's screen path for the panel's report as the reader left it
 * (Open as full report): its options without the panel's prefix, and the source.
 */
export function fullReportHref(
    reportId: string,
    defs: readonly ParamDef[],
    options: ViewOptions,
    sourceId: string
): string {
    const query = new URLSearchParams()
    for (const [key, value] of Object.entries(serializeViewOptions(defs, options))) {
        if (value !== undefined) query.set(key, value)
    }
    query.set(FROM_KEY, sourceId)
    return `reports/${reportId}?${query.toString()}`
}
```

`spec.ts`: move `UNREADABLE` from `ReportScreen.tsx` here as

```ts
/** A stored spec this app cannot read (a newer spec version) cannot run here. */
export const UNREADABLE_SPEC: BrokenMark = { … the same object … }
```

and use `UNREADABLE_SPEC` in `ReportScreen.tsx` in its place.

Create `core/reports/lib/figure-focus.ts`:

```ts
import type { View } from 'react-native'
import { AccessibilityInfo, Platform } from 'react-native'

export type FocusTarget = React.ComponentRef<typeof View>

/**
 * Moves the focus to a view: the keyboard focus on web, the screen reader's on
 * native, where a view has no keyboard focus to take.
 */
export function focusNode(node: FocusTarget) {
    if (Platform.OS === 'web') node.focus()
    else AccessibilityInfo.sendAccessibilityEvent(node, 'focus')
}
```

(Import `type ComponentRef` from `react` if the global `React` namespace is not in scope.)

- [ ] **Step 4: `useDrillPanel`**

Create `core/reports/hooks/useDrillPanel.ts`:

```ts
import { DETAIL_PANEL_WIDTH } from '@tinycld/core/lib/detail-surface'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useDetailSurface } from '@tinycld/core/lib/use-detail-surface'
import { useLocalSearchParams, useRouter } from 'expo-router'
import { useCallback } from 'react'
import { fullReportHref } from '../lib/drill'
import { type FocusTarget, focusNode } from '../lib/figure-focus'
import { UNREADABLE_SPEC } from '../lib/spec'
import { clearedDrillKeys, DRILL_PREFIX } from '../lib/view-url'
import { useReportRecord } from './useReportRecord'
import { useReportView } from './useReportView'

export interface DrillPanelInput {
    /** The report the panel shows (the route's `d`). */
    targetId: string
    /** The report beside it, for Open as full report's back link. */
    sourceId: string
    sourceTitle: string
}

/**
 * The drill panel's report: a second report view on the source's route, whose
 * options carry DRILL_PREFIX and whose popovers live in the `drill` scope. On a
 * window under 1360 px it is a page over the report; only its width input and the
 * header change, so a crossing neither remounts nor reruns it.
 */
export function useDrillPanel({ targetId, sourceId, sourceTitle }: DrillPanelInput) {
    const router = useRouter()
    const orgHref = useOrgHref()
    const search = useLocalSearchParams()
    const surface = useDetailSurface()
    const record = useReportRecord(targetId)
    // Closing is a view change on the source's route, as opening was: no history entry.
    const onClose = () => router.setParams(clearedDrillKeys(search))
    const view = useReportView({
        reportKey: `${DRILL_PREFIX}${targetId}`,
        title: record.record?.title ?? '',
        defs: record.spec?.params ?? [],
        source: record.record && record.spec ? { kind: 'saved', reportId: targetId } : null,
        access: record.access,
        broken: record.broken ?? (record.spec ? null : UNREADABLE_SPEC),
        isEditing: false,
        onBack: onClose,
        spec: record.spec,
        urlPrefix: DRILL_PREFIX,
        uiScope: 'drill',
        viewWidth: surface === 'panel' ? DETAIL_PANEL_WIDTH : undefined,
    })
    // On open the focus moves to the panel's title, so a keyboard reader continues there.
    const titleRef = useCallback((node: FocusTarget | null) => {
        if (node) focusNode(node)
    }, [])
    return {
        targetId,
        title: record.record?.title ?? '',
        isPage: surface === 'page',
        recordState: record.state,
        retryRecord: record.retry,
        view,
        backLabel: sourceTitle,
        titleRef,
        onClose,
        onOpenFull: () =>
            router.push(orgHref(fullReportHref(targetId, view.defs, view.options, sourceId))),
    }
}

export type DrillPanelModel = ReturnType<typeof useDrillPanel>
```

- [ ] **Step 5: `ReportDrillPanel`**

Create `core/reports/components/ReportDrillPanel.tsx`:

```tsx
import { LoadingState } from '@tinycld/core/components/LoadingState'
import { DETAIL_PANEL_WIDTH } from '@tinycld/core/lib/detail-surface'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { LayerEscape } from '@tinycld/core/ui/overlay'
import { useLocalSearchParams } from 'expo-router'
import { Maximize2, X } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { type DrillPanelInput, type DrillPanelModel, useDrillPanel } from '../hooks/useDrillPanel'
import type { ReportViewModel } from '../hooks/useReportView'
import { useTableLayout } from '../hooks/useTableLayout'
import { layoutNoticeOf } from '../lib/notices'
import { DRILL_KEY } from '../lib/view-url'
import { ReportControls } from './ReportControls'
import { ReportDocumentHeader } from './ReportDocumentHeader'
import { REPORT_TAP_TARGET, ReportBackLink } from './ReportHeader'
import { ReportNotices } from './ReportNotices'
import { MessageBody } from './ReportStates'
import { ReportBody } from './ReportView'

// Beside the report under a left border, or over it as a page: style values only,
// so the panel's report stays mounted when the window crosses 1360 px.
const PANEL_STYLE = { width: DETAIL_PANEL_WIDTH, borderLeftWidth: 1 } as const
const PAGE_STYLE = {
    position: 'absolute',
    top: 0,
    right: 0,
    bottom: 0,
    left: 0,
    borderLeftWidth: 0,
} as const
const TAP_STYLE = { minHeight: REPORT_TAP_TARGET, minWidth: REPORT_TAP_TARGET }

function PageBack({ panel }: { panel: DrillPanelModel }) {
    if (!panel.isPage) return null
    return <ReportBackLink label={panel.backLabel} onBack={panel.onClose} />
}

function CloseButton({ panel }: { panel: DrillPanelModel }) {
    const muted = useThemeColor('muted-foreground')
    if (panel.isPage) return null
    return (
        <Pressable
            testID="report-drill-close"
            accessibilityRole="button"
            accessibilityLabel="Close"
            onPress={panel.onClose}
            className="items-center justify-center"
            style={TAP_STYLE}
        >
            <X size={18} color={muted} />
        </Pressable>
    )
}

function OpenFullButton({ panel }: { panel: DrillPanelModel }) {
    const color = useThemeColor('foreground')
    if (panel.recordState !== 'ready') return null
    return (
        <Pressable
            testID="report-drill-open-full"
            accessibilityRole="button"
            onPress={panel.onOpenFull}
            className="flex-row items-center gap-1.5 px-2"
            style={{ minHeight: REPORT_TAP_TARGET }}
        >
            <Maximize2 size={16} color={color} />
            <Text className="text-sm text-foreground">Open as full report</Text>
        </Pressable>
    )
}

function DrillPanelHeader({ panel }: { panel: DrillPanelModel }) {
    return (
        <View testID="report-drill-panel-header" className="gap-1 border-b border-border px-4 py-2">
            <PageBack panel={panel} />
            <View className="flex-row items-center gap-2">
                <View
                    ref={panel.titleRef}
                    testID="report-drill-panel-title"
                    tabIndex={-1}
                    accessibilityRole="header"
                    className="min-w-0 flex-1"
                >
                    <Text numberOfLines={1} className="text-lg font-semibold text-foreground">
                        {panel.title}
                    </Text>
                </View>
                <OpenFullButton panel={panel} />
                <CloseButton panel={panel} />
            </View>
        </View>
    )
}

function RecordLoading({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View testID="report-drill-loading" className="flex-1">
            <LoadingState />
        </View>
    )
}

// The target's record: its own states before its report can run.
function DrillRecordStates({ panel }: { panel: DrillPanelModel }) {
    const retry = { label: 'Retry', testID: 'report-drill-retry', onPress: panel.retryRecord }
    return (
        <>
            <RecordLoading isVisible={panel.recordState === 'loading'} />
            <MessageBody
                isVisible={panel.recordState === 'error'}
                testID="report-drill-error"
                message="Could not load this report"
                action={retry}
            />
            <MessageBody
                isVisible={panel.recordState === 'missing'}
                testID="report-drill-no-access"
                message="You cannot open this report"
            />
        </>
    )
}

// The target's chips, notices, document header and table, fitted to the panel's
// width. No toolbar: Customize, Share, Save as, Copy link and Edit report are on
// the full report, one press away. Its rows open their records; it has no panel.
function DrillReport({ view, targetId }: { view: ReportViewModel; targetId: string }) {
    const table = useTableLayout(view)
    const layoutNotice = view.state === 'ready' ? layoutNoticeOf(table.layout) : undefined
    return (
        <View className="min-h-0 flex-1">
            <ReportControls model={view} isSortMenuShown={table.layout.kind === 'blocks'} />
            <ReportNotices
                notices={view.notices}
                layoutNotice={layoutNotice}
                isCompact={view.breakpoint === 'mobile'}
                width={view.viewWidth}
            />
            <ReportDocumentHeader
                heading={view.heading}
                title={view.title}
                periodWords={view.periodWords}
                isThousands={view.display.divideBy1000}
            />
            <ReportBody model={view} table={table} drill={{ sourceId: targetId, hasPanel: false }} />
        </View>
    )
}

function DrillPanelBody({ panel }: { panel: DrillPanelModel }) {
    if (panel.recordState !== 'ready') return null
    return <DrillReport view={panel.view} targetId={panel.targetId} />
}

function DrillPanel(props: DrillPanelInput) {
    const panel = useDrillPanel(props)
    return (
        <View
            testID="report-drill-panel"
            role="complementary"
            aria-label={panel.title || 'Report'}
            className="border-border bg-background"
            style={panel.isPage ? PAGE_STYLE : PANEL_STYLE}
        >
            <LayerEscape onEscape={panel.onClose} />
            <DrillPanelHeader panel={panel} />
            <DrillRecordStates panel={panel} />
            <DrillPanelBody panel={panel} />
        </View>
    )
}

const firstOf = (value: string | string[] | undefined) => (Array.isArray(value) ? value[0] : value)

/**
 * The report a figure drilled to (the route's `d`), in ReportView's side slot. It
 * draws nothing without `d`, so Escape is registered only while it is open.
 */
export function ReportDrillPanel({ sourceId, sourceTitle }: { sourceId: string; sourceTitle: string }) {
    const targetId = firstOf(useLocalSearchParams()[DRILL_KEY])
    if (!targetId) return null
    return <DrillPanel targetId={targetId} sourceId={sourceId} sourceTitle={sourceTitle} />
}
```

If `ReportControls` draws a control that is not a chip (an Add filter is a chip control and stays), keep it: the brief removes only the toolbar's actions from the panel.

- [ ] **Step 6: Mount it beside the saved report**

`ReportScreen.tsx`, `SavedReportView`: import `ReportDrillPanel`; replace the Task 11 `drill` and the `ReportView` line:

```tsx
    // The figure opens its report beside this one on a wide window (D4), else as a screen.
    const drill = report ? { sourceId: report.id, hasPanel: true } : undefined
    const side = report ? <ReportDrillPanel sourceId={report.id} sourceTitle={report.title} /> : null
```

```tsx
            <ReportView model={model} drill={drill} side={side} />
```

The editing view (`EditingReport`) keeps its builder in `side` and passes no `drill`: the builder occupies the side.

- [ ] **Step 7: Run the tests, the typecheck and the e2e**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run core/reports/lib/__tests__/drill.test.ts`
Expected: PASS.

Run: `cd /Users/nas/tmp/tinycld/books/tinycld && PATH=/Users/nas/tmp/tinycld/books/node_modules/.bin:$PATH tinycld-pkg typecheck`
Expected: no errors. If the RN `role` type lacks `'complementary'`, set `accessibilityRole` from what RN 0.88 accepts and read the DOM role in test 3's terms; never cast.

Run (port check first): `… test:e2e -- reports-failure-states.spec.ts`
Expected: PASS (the three old tests and the panel's states).

Run (port check first): `… test:e2e -- reports-drill.spec.ts -g "panel beside the source|crossing 1360"`
Expected: PASS (tests 3 and 5). In test 5, a failing `data-e2e-kept` count means the panel remounted at the crossing: find which element changed type or place between the panel and page forms and keep one tree.

- [ ] **Step 8: Biome**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/biome check core/reports/lib/figure-focus.ts core/reports/lib/drill.ts core/reports/lib/__tests__/drill.test.ts core/reports/lib/spec.ts core/reports/hooks/useDrillPanel.ts core/reports/components/ReportDrillPanel.tsx core/reports/screens/ReportScreen.tsx tests/e2e/reports-failure-states.spec.ts`
Expected: no errors.

- [ ] **Step 9: Hand the diff to the driver** (no commit).

---

### Task 15: One side surface at a time, and the focus back to the figure (D4e)

**Files:**
- Modify: `core/reports/stores/report-ui-store.ts` (`drillOrigin`), `core/reports/stores/__tests__/report-ui-store.test.ts`
- Modify: `core/reports/lib/figure-focus.ts` (`figureKey`, `useFigureRef`, `focusFigure`)
- Create: `core/reports/lib/__tests__/figure-focus.test.ts`
- Modify: `core/reports/components/ReportLinks.tsx` (`DrillFigure` ref), `core/reports/components/ReportRow.tsx` (passes the key)
- Modify: `core/reports/hooks/useRowLinks.ts` (records the origin)
- Modify: `core/reports/hooks/useDrillPanel.ts` (focus on close)
- Modify: `core/reports/hooks/useReportView.ts` (Customize clears `d` keys)

**Interfaces:**
- Consumes: `focusNode`, `FocusTarget` (Task 14), `clearedDrillKeys` (Task 7), the scoped store (Task 8), `useOpenDrill`'s `closeCustomize('main')` (Task 8).
- Produces (N4): store `drillOrigin: string | null`, `setDrillOrigin(key: string | null)` (reset by `startVisit`); `figureKey(columnId: string, rowId: string): string`, `useFigureRef(key: string, isCopy: boolean)`, `focusFigure(key: string | null): void` (figure-focus.ts); `DrillFigure` prop `figureKey: string`; the main view's `onOpenCustomize` clears every `d` key.

- [ ] **Step 1: Write the failing tests**

`report-ui-store.test.ts`, inside the describe:

```ts
    it('keeps the figure that opened the drill panel until the next visit', () => {
        state().setDrillOrigin('amount|g:east')
        expect(state().drillOrigin).toBe('amount|g:east')
        state().startVisit()
        expect(state().drillOrigin).toBeNull()
    })
```

Create `core/reports/lib/__tests__/figure-focus.test.ts`:

```ts
import { describe, expect, it, vi } from 'vitest'
import { type FocusTarget, figureKey, focusFigure, registerFigure } from '../figure-focus'

const node = () => ({ focus: vi.fn() }) as unknown as FocusTarget & { focus: ReturnType<typeof vi.fn> }

describe('focusFigure', () => {
    it('focuses the figure drawn under the key', () => {
        const figure = node()
        const forget = registerFigure(figureKey('amount', 'g:1'), figure)
        focusFigure(figureKey('amount', 'g:1'))
        expect(figure.focus).toHaveBeenCalledOnce()
        forget()
    })

    it('does nothing for a figure no longer drawn', () => {
        const figure = node()
        registerFigure(figureKey('amount', 'g:2'), figure)()
        focusFigure(figureKey('amount', 'g:2'))
        expect(figure.focus).not.toHaveBeenCalled()
    })

    it('keeps a newer figure of the key when an older one leaves', () => {
        const older = node()
        const newer = node()
        const forgetOlder = registerFigure(figureKey('amount', 'g:3'), older)
        registerFigure(figureKey('amount', 'g:3'), newer)
        forgetOlder()
        focusFigure(figureKey('amount', 'g:3'))
        expect(newer.focus).toHaveBeenCalledOnce()
    })
})
```

(The vitest run is web: `Platform.OS` is `'web'` there, so `focusNode` calls `focus()`. The cast builds a stand-in node; it is the test's only cast.)

- [ ] **Step 2: Run them to see them fail**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run core/reports/stores/__tests__/report-ui-store.test.ts core/reports/lib/__tests__/figure-focus.test.ts`
Expected: FAIL: `setDrillOrigin` is not a function; `registerFigure` is not exported.

- [ ] **Step 3: The store and the figure registry**

`report-ui-store.ts`: `ReportUIState` gains

```ts
    /** The figure (figureKey) whose press opened the drill panel; its close sends the focus back there. */
    drillOrigin: string | null
    setDrillOrigin: (key: string | null) => void
```

the store `drillOrigin: null`, `setDrillOrigin: key => set({ drillOrigin: key })`, and `startVisit` also sets `drillOrigin: null`.

`figure-focus.ts`: add `import { useCallback } from 'react'` and:

```ts
// The figures drawn now, by key. FlashList recycles a row's views for another row,
// so a figure is filed under its key while it shows that key, and only its own
// entry is removed when it leaves.
const figures = new Map<string, FocusTarget>()

export function figureKey(columnId: string, rowId: string): string {
    return `${columnId}|${rowId}`
}

/** Files a drawn figure; the returned function removes it, unless a newer one took the key. */
export function registerFigure(key: string, node: FocusTarget): () => void {
    figures.set(key, node)
    return () => {
        if (figures.get(key) === node) figures.delete(key)
    }
}

/** A drilled figure's ref: the list's copy, never a stuck copy, takes the focus back. */
export function useFigureRef(key: string, isCopy: boolean) {
    return useCallback(
        (node: FocusTarget | null) => {
            if (!node || isCopy) return
            return registerFigure(key, node)
        },
        [key, isCopy]
    )
}

/** Sends the focus back to a figure, when it is still drawn. */
export function focusFigure(key: string | null) {
    const node = key === null ? undefined : figures.get(key)
    if (node) focusNode(node)
}
```

(A ref callback that returns a function is React 19's ref cleanup.)

- [ ] **Step 4: The figure files itself, the press records it, the close focuses it**

`ReportLinks.tsx`, `DrillFigure`: props gain `figureKey: string`; inside, `const ref = useFigureRef(props.figureKey, props.isCopy)` and `ref={ref}` on the `Pressable` (`Link asChild` passes the ref to its child). `ReportRow.tsx`, `RowCell`: pass `figureKey={figureKey(column.id, row.id)}` (import from `../lib/figure-focus`).

`useRowLinks.ts`: `const setDrillOrigin = useReportUIStore(s => s.setDrillOrigin)` (import the store), and

```ts
            onFigure: (column, row) => {
                setDrillOrigin(figureKey(column.id, row.id))
                latest.current(column, row)
            },
```

with `setDrillOrigin` in the `useMemo` dependencies.

`useDrillPanel.ts`: import `focusFigure` and `useReportUIStore`;

```ts
    // ✕, Escape and the page's back link send the focus back to the figure that
    // opened the panel, when it is still drawn.
    const onClose = () => {
        router.setParams(clearedDrillKeys(search))
        focusFigure(useReportUIStore.getState().drillOrigin)
    }
```

- [ ] **Step 5: Opening Customize closes the panel**

`useReportView.ts`: import `clearedDrillKeys`. The model's `onOpenCustomize`:

```ts
        // One side surface at a time: the main view's Customize closes the drill panel
        // (a figure press closes Customize, useOpenDrill).
        onOpenCustomize: () => {
            openCustomize(scope)
            if (scope === 'main') router.setParams(clearedDrillKeys(search))
        },
```

- [ ] **Step 6: Run the tests, the typecheck and the panel e2e**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run core/reports/stores/__tests__/report-ui-store.test.ts core/reports/lib/__tests__/figure-focus.test.ts core/reports/lib/__tests__/link-press.test.ts`
Expected: PASS.

Run: `cd /Users/nas/tmp/tinycld/books/tinycld && PATH=/Users/nas/tmp/tinycld/books/node_modules/.bin:$PATH tinycld-pkg typecheck`
Expected: no errors.

Run (port check first): `… test:e2e -- reports-drill.spec.ts -g "panel"`
Expected: PASS (tests 3, 4 and 5). If the figure is not focused after ✕, check that the press reached `setDrillOrigin` and that the figure's ref filed the list's copy, not the sticky copy.

- [ ] **Step 7: Biome**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/biome check core/reports/stores/report-ui-store.ts core/reports/stores/__tests__/report-ui-store.test.ts core/reports/lib/figure-focus.ts core/reports/lib/__tests__/figure-focus.test.ts core/reports/components/ReportLinks.tsx core/reports/components/ReportRow.tsx core/reports/hooks/useRowLinks.ts core/reports/hooks/useDrillPanel.ts core/reports/hooks/useReportView.ts`
Expected: no errors.

- [ ] **Step 8: Hand the diff to the driver** (no commit).

---

### Task 16: The builder names the drill and the open link; help and docs

**Files:**
- Modify: `core/reports/components/builder/ColumnsSection.tsx` (`resultColumns`, `DrillNote`)
- Modify: `core/reports/components/builder/ReportBuilderPanel.tsx` (passes `resultColumns`)
- Modify: `core/reports/components/builder/DetailsSection.tsx` (`DetailOpenNote`)
- Modify: `core/help/reports.md`
- Modify: `docs/packages.md`
- Modify: `core/reports/lib/__tests__/statements-docs.test.ts`

**Interfaces:**
- Consumes: `ColumnDraft.drill`, `BuilderDraft.detailOpen` (Task 6), `ResultColumn.drill` (Task 6), `preview.allColumns`.
- Produces: `ColumnsSection` prop `resultColumns: readonly ResultColumn[]`; text "Figures open <report title>" (test id `report-builder-column-<i>-drill`; with no result yet, the shipped key); text "Rows open their record" (test id `report-builder-detail-open`); help sections "Open the rows behind a figure", "Open a row's record"; `docs/packages.md` rows `columns[].drill`, `detail.open` and the drill panel's width rule.

- [ ] **Step 1: Write the failing docs test**

`statements-docs.test.ts`, inside the describe:

```ts
    it('names drills, open links and the drill panel in the help and the docs', () => {
        const help = read('core/help/reports.md')
        expect(help).toContain('## Open the rows behind a figure')
        expect(help).toContain("## Open a row's record")
        expect(help).toContain('1360 pixels')
        const docs = read('docs/packages.md')
        const start = docs.indexOf('**Fields added in version 4.**')
        const table = docs.slice(start, docs.indexOf('\n\n', docs.indexOf('| Field |', start)))
        expect(table).toContain('`columns[].drill`')
        expect(table).toContain('`detail.open`')
        const shippedAt = docs.indexOf('### Shipped and saved reports')
        const shipped = docs.slice(shippedAt, docs.indexOf('\n### ', shippedAt + 1))
        expect(shipped).toContain('DETAIL_PANEL_MIN_WINDOW')
        expect(shipped).toContain('1360 px')
    })
```

- [ ] **Step 2: Run it to see it fail**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run core/reports/lib/__tests__/statements-docs.test.ts`
Expected: FAIL: the help has no "## Open the rows behind a figure".

- [ ] **Step 3: The builder's read-only notes**

`ColumnsSection.tsx`: import `type ResultColumn` from `../../lib/types`. Add:

```tsx
// A drill is set in the spec only (Q10): the builder keeps it as it is and names the
// report its figures open (the run's title for the reader, else the shipped key).
function DrillNote({
    column,
    index,
    resultColumns,
}: {
    column: ColumnDraft
    index: number
    resultColumns: readonly ResultColumn[]
}) {
    if (!column.drill) return null
    const title = resultColumns.find(result => result.id === column.id)?.drill?.title
    return (
        <Text
            testID={`report-builder-column-${index}-drill`}
            className="text-xs text-muted-foreground"
        >
            {`Figures open ${title ?? column.drill.report}`}
        </Text>
    )
}
```

`ColumnRowProps` gains `resultColumns: readonly ResultColumn[]`; `ColumnRow` draws `<DrillNote column={column} index={index} resultColumns={resultColumns} />` after `<ColumnFormula …/>`. `ColumnsSection` takes `resultColumns: readonly ResultColumn[]` and passes it to each `ColumnRow`. If `drill` is declared on one variant of `ColumnDraft` only, read it as `'drill' in column ? column.drill : undefined`; never cast.

`ReportBuilderPanel.tsx`, `Sections`: `<ColumnsSection builder={builder} problems={problems} resultColumns={preview.allColumns} />`.

`DetailsSection.tsx`: import `useWatch` from `@tinycld/core/ui/form` (the module `ColumnsSection` takes it from) and `Text` from `react-native`;

```tsx
// An open link is set in the spec only (Q10): the builder keeps it and says what it does.
function DetailOpenNote({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text testID="report-builder-detail-open" className="text-sm text-muted-foreground">
            Rows open their record
        </Text>
    )
}
```

In `DetailsSection`: `const detailOpen = useWatch({ control, name: 'detailOpen' })` and `<DetailOpenNote isVisible={detailOpen !== undefined} />` after the description input.

- [ ] **Step 4: Help**

`core/help/reports.md`: after the "## Run a report" section (before "## Share and publish"), add:

```markdown
## Open the rows behind a figure

In some reports a figure opens the report that lists the rows behind it, such as the transactions behind an account's total. On a computer, the figure underlines when you point at it. Select it to open that report, filtered to the figure's rows and period.

- On a window 1360 pixels wide or wider, the report opens in a panel at the right, beside the report you are reading. The figure you selected stays marked. Select another figure to show its rows in the same panel. The panel has its own filters and sort, and its lines open their records. Select **Open as full report** to open it as its own screen, or **Close** (or press Esc) to close it. Opening **Customize** closes the panel, and selecting a figure closes **Customize**. A link copied with **Copy link** does not open the panel.
- On a narrower window, and on a phone, the report opens as its own screen. Its back link shows the name of the report you came from and returns to it as you left it.
- To open the report in a new browser tab, ⌘-click the figure.

When you cannot open the report behind a figure, the figure is plain and does not open.

## Open a row's record

In some reports each line opens the record it comes from, such as an entry or an invoice. Select the line, or its name, to open the record. Back returns to the report as you left it.
```

- [ ] **Step 5: Package docs**

`docs/packages.md`, at the end of the "**Fields added in version 4.**" table (its last row, before the first blank line after `| Field |`), add:

```markdown
| `columns[].drill` | `{ report, levels, pivotCells? }`: a figure of this column opens the shipped report `report` (a key of the report's own package) filtered to the figure's rows. `levels.<group id>.<param id>` gives each target param's value on that level's group and subtotal rows: a formula (`groupKey` reads a level's group value; a relation reads as its id), `{ "range": { "from": …, "to": … } }`, `{ "group": <formula> }` (a whole group of the target's `optionsFrom.group`), or `{ "param": "<id>" }` (the value the reader ran, copied). `pivotCells`, on a pivot's cell column only, lays values over the level's for each pivot cell; there `{ "pivotBound": "from" \| "to" }` is the cell's first or last day, cut to the pivot's range. A blank value leaves that figure with no target. Not on the first, a hidden or a percent column. The result column carries `drill: { reportId, title }` and each drilled row `<col>@drill` (the target's params in the run request's form), only for a reader who may view the target. |
| `detail.open` | A text formula: the app path a detail row opens, for example `concat("<slug>/entries/", [id])`. It must start with the report's package slug and a `/` and hold only letters, digits, `-`, `_` and `/`; any other value gives that row no link (logged once per run). A relation inside it reads as its record id. Not on a pivot report. Detail rows carry `open`. |
```

In "### Shipped and saved reports", after the bullet about `Unpublished: true` reports, add the bullet:

```markdown
- A drilled figure (`columns[].drill`) opens its target beside the report in a
  480 px panel (`DETAIL_PANEL_WIDTH`) when the window is 1360 px or wider
  (`DETAIL_PANEL_MIN_WINDOW`, `@tinycld/core/lib/detail-surface`), else as its
  own screen with `from=<source id>`, whose back link names the source. The
  panel's state is in the source's URL (`d=<target id>` and the target's
  options under the `d.` prefix), so a reload keeps it; a copied link never
  holds it. A target the reader may not view gives plain figures.
```

- [ ] **Step 6: Run the tests, the typecheck and e2e test 6**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run core/reports/lib/__tests__/statements-docs.test.ts core/reports/lib/builder/__tests__/draft.test.ts`
Expected: PASS.

Run: `cd /Users/nas/tmp/tinycld/books/tinycld && PATH=/Users/nas/tmp/tinycld/books/node_modules/.bin:$PATH tinycld-pkg typecheck`
Expected: no errors.

Run: `cd /Users/nas/tmp/tinycld/books/tinycld && pnpm run packages:generate` (the help topic changed; no e2e may be running)
Expected: completes.

Run (port check first): `… test:e2e -- reports-drill.spec.ts -g "rows open their record"`
Expected: PASS (test 6).

- [ ] **Step 7: Biome**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/biome check core/reports/components/builder/ColumnsSection.tsx core/reports/components/builder/ReportBuilderPanel.tsx core/reports/components/builder/DetailsSection.tsx core/reports/lib/__tests__/statements-docs.test.ts`
Expected: no errors.

- [ ] **Step 8: Hand the diff to the driver** (no commit). Tell the driver the help topic changed, so the website docs may want the two new sections.
