import { expect, type Locator, type Page } from '@playwright/test'
import { navigateToPackage } from './helpers.ts'

// Helpers for any spec that drives the reports hub and core's report view: they
// name no source, so a package's own report e2e uses them through
// @tinycld/core/e2e-helpers (E11). The `.ts` extension is required there (see
// core/e2e-helpers.ts).

export async function openReportsHub(page: Page) {
    await navigateToPackage(page, 'reports', { waitFor: page.getByTestId('reports-hub') })
}

export function hubRow(page: Page, title: string): Locator {
    return page.locator('[data-testid^="reports-hub-row-"]').filter({ hasText: title })
}

/**
 * A toolbar action, folded into "More actions" when the row is narrow. The toolbar
 * draws after the report loads, so wait for the button or the More button before
 * choosing a branch.
 */
export async function pressToolbar(page: Page, testId: string, label: string) {
    const button = page.getByTestId(testId)
    const more = page.getByTestId('toolbar-more-button')
    await expect(button.or(more).first()).toBeVisible()
    if (await button.isVisible()) await button.click()
    else {
        await more.click()
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
export async function chooseParamOption(page: Page, paramId: string, pick: ParamPick) {
    if (pick.from === 'chip') await page.getByTestId(`report-param-${paramId}`).click()
    else {
        await page.getByTestId('report-add-filter').click()
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
