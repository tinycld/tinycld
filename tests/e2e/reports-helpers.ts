import { type ElementHandle, expect, type Locator, type Page } from '@playwright/test'
import { deriveUsername } from '@tinycld/core/lib/derive-username'
import PocketBase from 'pocketbase'
import { loginAs, navigateToPackage } from './helpers'

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
