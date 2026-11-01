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
    await page
        .getByRole('radio', { name: sale.status === 'paid' ? 'Paid' : 'Open', exact: true })
        .click()
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
