import { type ElementHandle, expect, type Locator, type Page } from '@playwright/test'
import { deriveUsername } from '@tinycld/core/lib/derive-username'
import PocketBase from 'pocketbase'
import { loginAs, navigateToPackage } from './helpers'
import { chooseParamOption, hubRow, openReportsHub, rowOf } from './report-view-helpers'

export {
    cellOf,
    chooseParamOption,
    hubRow,
    openReportsHub,
    pressToolbar,
    rowOf,
} from './report-view-helpers'

// Helpers for the reports specs. Fixture names come from the salesstub package
// (tests/scripts/scaffold-reports-stub.ts): sources salesstub.sales ("Stub sales":
// customer, amount, cost, quantity, currency, day, booked (a day kept as text),
// status; link team) and salesstub.teams ("Stub teams": name, region; to-many link
// sales), both headed "Stub Sales Co", and the shipped reports "Stub larger side"
// (published) and "Stub team totals" (unpublished) over sales
// (reports-stub/server/register.go.txt).
export const SALES_SOURCE = 'salesstub.sales'
export const TEAMS_SOURCE = 'salesstub.teams'
export const SHIPPED_TITLE = 'Stub larger side'
export const TEAM_TOTALS_TITLE = 'Stub team totals'
/** The heading the stub's sources print above a report's title. */
export const STUB_HEADING = 'Stub Sales Co'
/** The shipped report's formula column (index 3, id `larger`), as register.go.txt has it. */
export const SHIPPED_FORMULA = {
    op: 'if',
    args: [
        { op: 'gt', args: [{ col: 'amount' }, { col: 'cost' }] },
        { col: 'amount' },
        { col: 'cost' },
    ],
}

let counter = 0

/** A name no other spec, worker or run uses. */
export function uniqueName(prefix: string): string {
    counter += 1
    return `${prefix} ${process.env.TEST_WORKER_INDEX ?? '0'}${Date.now().toString(36)}${counter}`
}

export async function openReportStub(page: Page) {
    await navigateToPackage(page, 'salesstub', { waitFor: page.getByTestId('salesstub-landing') })
}

const REGION_LABELS = { north: 'North', south: 'South', east: 'East' } as const

export async function addTeam(
    page: Page,
    name: string,
    region: keyof typeof REGION_LABELS = 'north'
) {
    await page.getByTestId('salesstub-team-name').fill(name)
    // The team form's radios come first on the screen, so the first match is its own.
    await page.getByRole('radio', { name: REGION_LABELS[region], exact: true }).first().click()
    await page.getByTestId('salesstub-team-create').click()
    await expect(page.getByTestId('salesstub-team-saved')).toHaveText(`Saved ${name}`)
}

export interface StubSale {
    team: string
    customer: string
    /** Cents. */
    amount: number
    cost: number
    quantity?: number
    day: string
    status?: 'open' | 'paid'
}

export async function addSale(page: Page, sale: StubSale) {
    await page.getByTestId('salesstub-sale-team').click()
    await page.getByRole('menuitem', { name: sale.team, exact: true }).click()
    await page.getByTestId('salesstub-sale-customer').fill(sale.customer)
    await page.getByTestId('salesstub-sale-amount').fill(String(sale.amount))
    await page.getByTestId('salesstub-sale-cost').fill(String(sale.cost))
    await page.getByTestId('salesstub-sale-quantity').fill(String(sale.quantity ?? 0))
    await page.getByTestId('salesstub-sale-day').fill(sale.day)
    await page
        .getByRole('radio', { name: sale.status === 'paid' ? 'Paid' : 'Open', exact: true })
        .click()
    await page.getByTestId('salesstub-sale-create').click()
    await expect(page.getByTestId('salesstub-sale-saved')).toHaveText(`Saved ${sale.customer}`)
}

export interface StubSales {
    team: string
    /** Customers are named `${customerPrefix} 00`, `${customerPrefix} 01`, … */
    customerPrefix: string
    count: number
    /** The first sale's day; each next sale is one day later. */
    firstDay: string
    /** The first sale's amount; each next sale's is one more. */
    firstAmount: number
    cost: number
    quantity: number
}

/**
 * Adds `count` open sales in one save through the stub's "Many sales" form: one
 * form, not `count` rounds of the single-sale form, which took most of a spec's
 * time budget.
 */
export async function addSales(page: Page, sales: StubSales) {
    await page.getByTestId('salesstub-sales-team').click()
    await page.getByRole('menuitem', { name: sales.team, exact: true }).click()
    await page.getByTestId('salesstub-sales-prefix').fill(sales.customerPrefix)
    await page.getByTestId('salesstub-sales-count').fill(String(sales.count))
    await page.getByTestId('salesstub-sales-first-day').fill(sales.firstDay)
    await page.getByTestId('salesstub-sales-first-amount').fill(String(sales.firstAmount))
    await page.getByTestId('salesstub-sales-cost').fill(String(sales.cost))
    await page.getByTestId('salesstub-sales-quantity').fill(String(sales.quantity))
    await page.getByTestId('salesstub-sales-create').click()
    await expect(page.getByTestId('salesstub-sales-saved')).toHaveText(`Saved ${sales.count} sales`)
}

/** New report → pick a source → the builder opens on a draft. */
export async function startReport(page: Page, sourceKey: string) {
    await page.getByTestId('reports-hub-new').click()
    await page.getByTestId(`reports-new-source-${sourceKey}`).click()
    await expect(page.getByTestId('report-builder')).toBeVisible()
}

/**
 * Adds a column through the picker: link keys, then a field key. `mode` answers the
 * to-many choice of the last link; the links before it must be to-one.
 */
export async function addColumn(page: Page, keys: string[], mode?: 'total' | 'nest') {
    await page.getByTestId('report-builder-add-column').click()
    const picker = page.getByTestId('report-builder-picker')
    const links = keys.slice(0, -1)
    for (const [i, key] of links.entries()) {
        await picker.getByTestId(`report-builder-pick-link-${key}`).click()
        if (mode && i === links.length - 1) {
            await picker.getByTestId(`report-builder-pick-${mode}`).click()
        }
    }
    await picker.getByTestId(`report-builder-pick-field-${keys[keys.length - 1]}`).click()
    await expect(picker).toBeHidden()
}

/** Holds every report run until the returned release is called. */
export async function holdRuns(page: Page): Promise<() => void> {
    let release = () => {}
    const held = new Promise<void>(resolve => {
        release = resolve
    })
    await page.route('**/api/reports/run', async route => {
        await held
        await route.continue()
    })
    return release
}

/** The report list's own scroller: the row's nearest vertical (overflow-y: auto) ancestor. */
export async function listScroller(row: Locator): Promise<ElementHandle<HTMLElement>> {
    return row.evaluateHandle(element => {
        let scroller = element.parentElement
        while (scroller && getComputedStyle(scroller).overflowY !== 'auto') {
            scroller = scroller.parentElement
        }
        if (!scroller) throw new Error('the row has no vertical scroller')
        return scroller
    })
}

/** Scrolls the report list so the row's top edge lands `below` px under the list's top edge. */
export async function scrollRowTo(row: Locator, below = 0) {
    const scroller = await listScroller(row)
    await row.evaluate(
        (element, { list, offset }) => {
            const top = element.getBoundingClientRect().top - list.getBoundingClientRect().top
            list.scrollTop += top - offset
        },
        { list: scroller, offset: below }
    )
}

// The stub's shipped "Stub team balance" (salesstub.team_balance): sales grouped by
// team, each team open by default, with an opening row ("Before the period"), a
// running balance and a subtotal row ("Total <team>"). Shipped reports belong to
// the seed's admin, so these helpers run after loginAsShippedOwner.
export const TEAM_BALANCE_TITLE = 'Stub team balance'
const YEAR = new Date().getFullYear()

export async function openTeamBalance(page: Page) {
    await openReportsHub(page)
    await page.getByTestId('reports-hub-tab-mine').click()
    await hubRow(page, TEAM_BALANCE_TITLE).click()
    await expect(page.getByTestId('report-title')).toHaveText(TEAM_BALANCE_TITLE)
}

export async function showTeams(page: Page, search: string, teams: string[]) {
    await chooseParamOption(page, 'teams', { from: 'add-filter', search, labels: teams })
}

/**
 * The rows' ids in screen order, read in one snapshot. A row drawn twice is listed
 * twice, so a repeat can be counted (a sticky copy has its own test id and is not
 * listed).
 */
export function rowIds(page: Page) {
    return page
        .getByTestId('report-table')
        .evaluate(table =>
            [...table.querySelectorAll('[data-testid^="report-row-"]')].map(
                el => el.getAttribute('data-testid')?.replace('report-row-', '') ?? ''
            )
        )
}

export function detailLabels(page: Page) {
    return page.getByTestId('report-table').locator('[data-testid^="report-row-d:"]')
}

/** A run request as the client sent it, before openPagedTeams' route shrinks its page. */
export type SentRun = { cursor?: string; limit?: number }

// The client's page sizes: its usual page (RUN_PAGE_SIZE) and the server's most
// (RUN_MAX_PAGE), which a fold run asks for while it looks for the group in view.
export const USUAL_PAGE = 200
export const MAX_PAGE = 1000

export interface PagedTeamsOptions {
    /** Teams of one sale each, after the two teams of six. */
    smallTeams?: number
    /** A next page (a request with a cursor) waits for it. */
    nextPages?: Promise<void>
    /** Team n's sales start at baseAmount × n cents and go up a cent each. */
    baseAmount?: number
    /** The rows a page holds once the route shrinks the client's sizes (default 5). */
    pageRows?: number
    /** Refuses (400) the first next page of the run filtered to the teams, once. */
    refuseFirstNextPage?: boolean
    /**
     * The window height the report opens at (default 1600, which draws every row of
     * two teams). A short window loads no next page until the reader scrolls.
     */
    height?: number
}

// What the server answers for a cursor it cannot read (run.go `invalid`).
const CURSOR_REFUSAL = { status: 400, message: 'The page cursor is not valid.', data: {} }

/**
 * Signs in, seeds two teams of six sales each (team A first), and `smallTeams` more
 * teams of one sale each after them, and opens the report on them alone, in pages
 * of `pageRows` rows. A page counts group, opening, detail and subtotal rows (J1a),
 * so team A alone is 9 rows (group, opening, six sales, subtotal) and a small team 4.
 */
export async function openPagedTeams(page: Page, prefix: string, opts: PagedTeamsOptions = {}) {
    const { smallTeams = 0, nextPages, baseAmount = 100, pageRows = 5, height = 1600 } = opts
    let isRefusing = opts.refuseFirstNextPage === true
    await loginAsShippedOwner(page)
    const stem = uniqueName(prefix)
    const letters = 'ABCDEFGH'.slice(0, 2 + smallTeams)
    const teams = [...letters].map(letter => `${stem} ${letter}`)
    await openReportStub(page)
    for (const [i, team] of teams.entries()) {
        await addTeam(page, team)
        await addSales(page, {
            team,
            customerPrefix: `${stem}${letters[i]}`,
            count: i < 2 ? 6 : 1,
            firstDay: `${YEAR}-02-01`,
            firstAmount: baseAmount * (i + 1),
            cost: 0,
            quantity: 1,
        })
    }
    const cursors: string[] = []
    const sent: SentRun[] = []
    await page.route('**/api/reports/run', async route => {
        const body = route.request().postDataJSON()
        const cursor: string | undefined = body.options?.cursor
        // Only the run filtered to these teams: the first, unfiltered run pages too.
        const isTeamsRun = body.params?.teams !== undefined
        if (cursor && isRefusing && isTeamsRun) {
            isRefusing = false
            await route.fulfill({
                status: 400,
                contentType: 'application/json',
                body: JSON.stringify(CURSOR_REFUSAL),
            })
            return
        }
        if (cursor && nextPages) await nextPages
        if (cursor && isTeamsRun) cursors.push(cursor)
        sent.push({ cursor, limit: body.options?.limit })
        // The client's page sizes become pageRows. A fold run asks for max(200, the
        // rows in view), which is 200 here, and then the server's most while it looks
        // for the group in view, so it also gets pages of pageRows and joins them
        // until it holds the rows that were in view.
        const asked = body.options?.limit
        const limit = asked === USUAL_PAGE || asked === MAX_PAGE ? pageRows : asked
        await route.continue({
            postData: JSON.stringify({ ...body, options: { ...body.options, limit } }),
        })
    })
    // By default tall enough that the list draws every row of both teams at once, so
    // one snapshot holds the whole merged list (the list draws only rows near the view).
    await page.setViewportSize({ width: 1280, height })
    await openTeamBalance(page)
    // The first run has no Teams filter, so it shows these sales too: no row
    // tells the two runs apart. Once the run with the Teams filter has answered
    // and the view no longer says "Updating…", every row is the new run's.
    await expect(detailLabels(page).first()).toBeVisible()
    const teamsRun = page.waitForResponse(
        response =>
            response.url().includes('/api/reports/run') &&
            response.request().postDataJSON()?.params?.teams !== undefined
    )
    await showTeams(page, stem, teams)
    await teamsRun
    await expect(page.getByTestId('report-updating')).toHaveCount(0)
    const firstSale = rowOf(page, `${stem}A 00`)
    await expect(firstSale).toBeVisible()
    return { stem, teams, cursors, sent, firstSale }
}

export async function chooseOption(page: Page, selectTestId: string, value: string) {
    await page.getByTestId(selectTestId).click()
    await page.getByTestId(`${selectTestId}-${value}`).click()
}

/**
 * A new report over sales with columns customer (0, which names opened rows), the
 * team record (1, id `team`) and the amount summed (2). The builder stays open.
 */
export async function startTeamSales(page: Page) {
    await startReport(page, SALES_SOURCE)
    await page.getByTestId('report-builder-add-column').click()
    await page
        .getByTestId('report-builder-picker')
        .getByTestId('report-builder-pick-record-team')
        .click()
    await addColumn(page, ['amount'])
    await chooseOption(page, 'report-builder-column-2-aggregate', 'sum')
}

/**
 * Groups startTeamSales's draft by team and adds the team's region, read from the
 * group's record (formula column 3, id `formula`): the group row and every opened
 * row of the team show it. The group has no subtotals.
 */
export async function groupWithTeamRegion(page: Page) {
    await page.getByTestId('report-builder-section-group').click()
    await page.getByTestId('report-builder-group-add').click()
    await chooseOption(page, 'report-builder-group-0-col', 'team')
    // No subtotal rows (they start on), so an open group's row keeps its figures.
    await page.getByTestId('report-builder-group-0-subtotals').click()
    await page.getByTestId('report-builder-add-formula').click()
    const f = 'report-builder-column-3-formula'
    await chooseOption(page, `${f}-root-part`, 'record')
    await expect(page.getByTestId(`${f}-root-record-group`)).toHaveText('Team')
    await page.getByTestId(`${f}-root-record-field`).click()
    await page
        .getByTestId('report-builder-picker')
        .getByTestId('report-builder-pick-field-region')
        .click()
}

/** Saves a new report and waits for its own view (the builder closes, the URL changes). */
export async function saveNewReport(page: Page, title: string) {
    await page.getByTestId('report-builder-title').fill(title)
    await page.getByTestId('report-builder-save').click()
    await expect(page.getByTestId('report-builder')).toBeHidden()
    await expect(page).toHaveURL(/\/a\/reports\/[a-z0-9]{15}(\?|$)/)
    await expect(page.getByTestId('report-title')).toHaveText(title)
}

const ADMIN_LOGIN = process.env.ADMIN_USER_LOGIN || 'admin@tinycld.org'
const ADMIN_PASSWORD = process.env.ADMIN_USER_PW || 'AdminPass1234!'

// A PocketBase client signed in as the e2e server's superuser, for the one fixture
// reports-share.spec.ts seeds (its header says why). scripts/e2e-serve.ts creates
// this superuser from the same variables and defaults.
export async function superuserClient(): Promise<PocketBase> {
    const pb = new PocketBase(`http://127.0.0.1:${process.env.E2E_PORT ?? 7200}`)
    pb.autoCancellation(false)
    await pb.collection('_superusers').authWithPassword(ADMIN_LOGIN, ADMIN_PASSWORD)
    return pb
}

/**
 * Signs in as the owner of the shipped reports: the oldest active owner
 * (shipped.go firstAdmin), which is the seed's admin app user (scripts/seed-db.ts
 * ensureAdminAppUser), created before the shared test user. It signs in by its
 * username, which the seed derives from the email: the e2e server's users sign in
 * by username.
 */
export async function loginAsShippedOwner(page: Page) {
    await loginAs(page, deriveUsername(ADMIN_LOGIN), ADMIN_PASSWORD)
}
