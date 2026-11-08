import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub } from './helpers'
import {
    addColumn,
    addSale,
    addSales,
    addTeam,
    chooseOption,
    openReportStub,
    openReportsHub,
    SALES_SOURCE,
    saveNewReport,
    startReport,
    uniqueName,
} from './reports-helpers'

/**
 * The layout fit goal: at 390, 768, 1024 and 1280 a ledger-like report (a label,
 * text columns, a date and amounts, grouped with subtotals and a Total) fits its
 * view with core defaults only: nothing scrolls sideways, no figure, date or
 * heading word is cut, every group, subtotal and Total label is whole, and every
 * figure stands under its heading.
 */
const WIDTHS = [
    { width: 390, height: 844 },
    { width: 768, height: 1024 },
    { width: 1024, height: 800 },
    { width: 1280, height: 800 },
]
const FIGURES = ['day', 'amount', 'cost', 'quantity']

async function fitProblems(p: Page) {
    return p.getByTestId('report-table').evaluate((table, figures) => {
        const cut = (el: Element | null) =>
            el instanceof HTMLElement &&
            (el.scrollWidth > el.clientWidth + 0.5 || el.scrollHeight > el.clientHeight + 0.5)
        const textOf = (el: Element) => el.querySelector('div[dir="auto"]')
        const problems: string[] = []
        if (table.scrollWidth > table.clientWidth) problems.push('table scrolls sideways')
        for (const col of figures) {
            const heading = table.querySelector(`[data-testid="report-heading-${col}"]`)
            if (!heading) {
                problems.push(`heading ${col} missing`)
                continue
            }
            if (cut(textOf(heading))) problems.push(`heading ${col} cut`)
            const head = heading.getBoundingClientRect()
            let drawn = 0
            for (const cell of table.querySelectorAll(`[data-testid^="report-cell-${col}-"]`)) {
                // A group row leaves the cells of columns it does not sum blank.
                const text = textOf(cell)
                if (!text?.textContent) continue
                drawn++
                const id = cell.getAttribute('data-testid')
                if (cut(text)) problems.push(`${id} cut`)
                const box = text.getBoundingClientRect()
                if (box.left < head.left - 1 || box.right > head.right + 1)
                    problems.push(`${id} outside its heading`)
            }
            if (drawn === 0) problems.push(`no ${col} figure drawn`)
        }
        for (const row of table.querySelectorAll('[data-testid^="report-row-"]')) {
            const id = row.getAttribute('data-testid') ?? ''
            if (id.startsWith('report-row-d:')) continue
            const text = textOf(row)
            if (!text?.textContent) problems.push(`${id} label missing`)
            else if (cut(text)) problems.push(`${id} label cut`)
        }
        return problems
    }, FIGURES)
}

test.describe('report fit at four widths', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a ledger-like report fits 390, 768, 1024 and 1280 with nothing cut that must not be', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptfit')
        try {
            const team = uniqueName('Northwest Regional Wholesale')
            await openReportStub(p)
            await addTeam(p, team)
            // Six-figure amounts: wider than the measure's worst case, so the
            // columns must take their width from the rows.
            await addSales(p, {
                team,
                customerPrefix: 'Fitco Wholesale Partners of the Northwest',
                count: 5,
                firstDay: '2026-01-05',
                firstAmount: 12345678,
                cost: 4000000,
                quantity: 12,
            })
            await addSale(p, {
                team,
                customer: 'Fitco Wholesale Partners of the Northwest 05',
                amount: 12345683,
                cost: 4000000,
                quantity: 12,
                day: '2026-01-10',
                status: 'paid',
            })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            for (const keys of [
                ['status'],
                ['currency'],
                ['team', 'name'],
                ['day'],
                ['amount'],
                ['cost'],
                ['quantity'],
            ]) {
                await addColumn(p, keys)
            }
            await chooseOption(p, 'report-builder-column-5-aggregate', 'sum')
            await chooseOption(p, 'report-builder-column-6-aggregate', 'sum')
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'status')
            await p.getByTestId('report-builder-group-0-open').click()
            await saveNewReport(p, uniqueName('Fit'))

            for (const size of WIDTHS) {
                await p.setViewportSize(size)
                await expect(p.getByTestId('report-table')).toHaveCSS('opacity', '1')
                // The checks below read the drawn cells: there must be some.
                // The Total's id carries its currency (total:["USD"]).
                await expect(p.locator('[data-testid^="report-row-total"]').first()).toBeVisible()
                await expect(
                    p.locator('[data-testid^="report-cell-amount-"]').first()
                ).toBeVisible()
                await expect
                    .poll(() => fitProblems(p), { message: `fit at ${size.width}` })
                    .toEqual([])
                expect(
                    await p.evaluate(() => document.documentElement.scrollWidth)
                ).toBeLessThanOrEqual(size.width)
            }
        } finally {
            await close()
        }
    })
})
