import { type Download, expect, type Locator, type Page } from '@playwright/test'
import { inStatement } from './helpers.ts'

// Helpers for any spec that drives the reports hub and core's report view: they
// name no source, so a package's own report e2e uses them through
// @tinycld/core/e2e-helpers (E11). None opens the hub: core's navigation has no
// entry for it, so a spec opens it through its package's own link. The `.ts` extension is required there (see
// core/e2e-helpers.ts).

export function hubRow(page: Page, title: string): Locator {
    return page.locator('[data-testid^="reports-hub-row-"]').filter({ hasText: title })
}

// Present in a fitted toolbar row: drawn (no box around it at opacity 0, which is
// how the row measures), inside the window, and every item of its row inside
// the row (a row still fitted to an earlier width runs past its edge).
function isInFittedRow(locator: Locator): Promise<boolean> {
    return locator.evaluateAll(elements =>
        elements.some(element => {
            const row = element.closest('[data-testid="toolbar-row"]')
            if (!row) return false
            for (let node: Element | null = element; node; node = node.parentElement) {
                if (getComputedStyle(node).opacity === '0') return false
            }
            const box = element.getBoundingClientRect()
            const rowBox = row.getBoundingClientRect()
            const inWindow = box.width > 0 && box.left >= 0 && box.right <= window.innerWidth
            const rowFits = [...row.children].every(child => {
                const c = child.getBoundingClientRect()
                return c.left >= rowBox.left - 1 && c.right <= rowBox.right + 1
            })
            return inWindow && rowFits
        })
    )
}

/**
 * Where a toolbar action is: its own button in the fitted row, or folded into
 * "More actions". The toolbar draws after the report loads, and after a width
 * change it refits its items (until then they can run past the row's edge), so
 * wait for a fitted row before choosing a branch.
 */
async function toolbarBranch(page: Page, testId: string): Promise<'button' | 'more'> {
    const button = page.getByTestId(testId)
    const more = page.getByTestId('toolbar-more-button')
    // The branch is the one the poll accepted: asking again could see a refit.
    const found: { branch: 'button' | 'more' | null } = { branch: null }
    const fitted = async () => {
        if (await isInFittedRow(button)) found.branch = 'button'
        else if ((await button.count()) === 0 && (await isInFittedRow(more))) found.branch = 'more'
        else found.branch = null
        return found.branch
    }
    await expect.poll(fitted).not.toBeNull()
    return found.branch === 'button' ? 'button' : 'more'
}

/**
 * Whether the fitted toolbar row shows "More actions". As `toolbarBranch`, it
 * waits for a fitted row first, so a row still refitting after a width change
 * is never read.
 */
export async function toolbarHasMore(page: Page): Promise<boolean> {
    const more = page.getByTestId('toolbar-more-button')
    const items = page.locator('[data-testid="toolbar-row"] [data-testid^="toolbar-item-"]')
    // The answer is the one the poll accepted: asking again could see a refit.
    const found: { hasMore: boolean | null } = { hasMore: null }
    const fitted = async () => {
        if (await isInFittedRow(more)) found.hasMore = true
        else if ((await more.count()) === 0 && (await isInFittedRow(items))) found.hasMore = false
        else found.hasMore = null
        return found.hasMore
    }
    await expect.poll(fitted).not.toBeNull()
    return found.hasMore === true
}

/** A toolbar action, folded into "More actions" when the row is narrow. */
export async function pressToolbar(page: Page, testId: string, label: string) {
    if ((await toolbarBranch(page, testId)) === 'button') await page.getByTestId(testId).click()
    else {
        await page.getByTestId('toolbar-more-button').click()
        await page.getByRole('menuitem', { name: label }).click()
    }
}

export type ReportExportFormat = 'pdf' | 'xlsx' | 'csv'

/**
 * Opens the menu that holds the export rows: the Export button's own menu when it
 * is in the fitted row, else "More actions", where the rows are entries of their own.
 */
export async function openExportMenu(page: Page) {
    const branch = await toolbarBranch(page, 'report-export-open')
    await page
        .getByTestId(branch === 'button' ? 'report-export-open' : 'toolbar-more-button')
        .click()
}

/** Downloads the report in `format` through the export menu, and returns the download. */
export async function downloadReport(page: Page, format: ReportExportFormat): Promise<Download> {
    await openExportMenu(page)
    const download = page.waitForEvent('download')
    await page.getByTestId(`report-export-${format}`).click()
    return download
}

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

export interface ParamPick {
    /** How the picker opens: the param's chip, or Add filter for a param not set yet. */
    from: 'chip' | 'add-filter'
    search?: string
    /** Option labels to tick (a multiple choice) or press (a single choice). */
    labels?: string[]
    /** A group value whose "All <group>" row to press. */
    group?: string
}

/**
 * Opens a param's picker, searches, picks options or a whole group, and applies. A
 * multiple choice always has Apply; a single choice applies when its option is
 * pressed and has none. The kind is read from the options' role (a checkbox or a
 * radio) before the press, never from whether Apply happens to be on screen.
 */
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
    const picker = page.getByTestId('report-filter-picker')
    await expect(picker).toBeVisible()
    if (pick.search !== undefined)
        await picker.getByTestId('report-filter-search').fill(pick.search)
    // Only a multiple choice picks whole groups.
    let isMultiple = pick.group !== undefined
    // The blank group's value is ''.
    if (pick.group !== undefined) {
        await picker.getByTestId(`report-filter-group-${pick.group}`).click()
    }
    for (const label of pick.labels ?? []) {
        const option = picker
            .getByRole('checkbox', { name: label, exact: true })
            .or(picker.getByRole('radio', { name: label, exact: true }))
        await expect(option).toBeVisible()
        if ((await option.getAttribute('role')) === 'checkbox') isMultiple = true
        await option.click()
    }
    if (isMultiple) await picker.getByTestId('report-filter-apply').click()
    await expect(picker).toBeHidden()
}
