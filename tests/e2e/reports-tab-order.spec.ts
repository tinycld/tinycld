import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub } from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    chooseOption,
    openReportStub,
    openReportsHub,
    pressToolbar,
    rowOf,
    SALES_SOURCE,
    saveNewReport,
    startReport,
    uniqueName,
} from './reports-helpers'

/**
 * Web: Tab moves through the report's group toggles in the order the screen
 * shows them, also after a sort change reorders the groups and a fold puts new
 * groups between them (the list's DOM order follows its data order).
 */

const TOGGLES = '[data-testid^="report-row-"][tabindex="0"]'

// The toggles' test ids, top to bottom as drawn.
async function screenOrder(p: Page): Promise<string[]> {
    const toggles = p.getByTestId('report-table').locator(TOGGLES)
    const placed = await toggles.evaluateAll(elements =>
        elements.map(element => ({
            id: element.getAttribute('data-testid') ?? '',
            y: element.getBoundingClientRect().top,
        }))
    )
    return placed.sort((a, b) => a.y - b.y).map(toggle => toggle.id)
}

// The toggles' test ids in the order Tab reaches them, from the first one on.
async function tabOrder(p: Page, count: number): Promise<string[]> {
    const toggles = p.getByTestId('report-table').locator(TOGGLES)
    await toggles.first().focus()
    const reached: string[] = []
    for (let i = 0; i < count; i++) {
        if (i > 0) await p.keyboard.press('Tab')
        reached.push(
            await p.evaluate(() => document.activeElement?.getAttribute('data-testid') ?? '')
        )
    }
    return reached
}

async function labelsOf(p: Page, ids: string[]): Promise<string[]> {
    const labels: string[] = []
    for (const id of ids) labels.push((await p.getByTestId(id).innerText()).trim())
    return labels
}

test.describe('tab order on web', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('Tab follows the screen order after a sort change and a fold', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rpttab')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            for (const [customer, status] of [
                ['Tab A', 'open'],
                ['Tab B', 'open'],
                ['Tab C', 'open'],
                ['Tab D', 'paid'],
                ['Tab E', 'paid'],
            ] as const) {
                await addSale(p, {
                    team,
                    customer,
                    amount: 100,
                    cost: 0,
                    day: '2026-01-05',
                    status,
                })
            }
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-1-aggregate', 'sum')
            await addColumn(p, ['status'])
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'status')
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-1-col', 'customer')
            await saveNewReport(p, uniqueName('Tab order'))
            const open = rowOf(p, 'Open')
            await open.click()
            await expect(open).toHaveAttribute('aria-expanded', 'true')

            // Status Z to A puts the Paid group, drawn after Open so far, first.
            await pressToolbar(p, 'report-edit-open', 'Edit report')
            await p.getByTestId('report-builder-section-group').click()
            await chooseOption(p, 'report-builder-sort-add-menu', 'status')
            await p
                .getByTestId('report-builder-sort-0')
                .getByRole('radio', { name: 'Z to A', exact: true })
                .click()
            await p.getByTestId('report-builder-save').click()
            await expect(p.getByTestId('report-builder')).toBeHidden()
            const paid = rowOf(p, 'Paid')
            await paid.click()
            await expect(paid).toHaveAttribute('aria-expanded', 'true')

            const expected = ['Paid', 'Tab D', 'Tab E', 'Open', 'Tab A', 'Tab B', 'Tab C']
            await expect.poll(async () => labelsOf(p, await screenOrder(p))).toEqual(expected)
            const onScreen = await screenOrder(p)
            expect(await tabOrder(p, onScreen.length)).toEqual(onScreen)
        } finally {
            await close()
        }
    })
})
