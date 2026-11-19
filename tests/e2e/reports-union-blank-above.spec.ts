import type { Page } from '@playwright/test'
import { skipWithoutReportsStub } from './helpers'
import { expect, test } from './member-fixtures'
import { cellOf, rowOf } from './report-view-helpers'
import {
    ACTIVITY_SOURCE,
    addColumn,
    chooseOption,
    openReportsHub,
    saveNewReport,
    seedStub,
    startReport,
    uniqueName,
} from './reports-helpers'

/**
 * The stub's union source (salesstub.activity: sales and returns, each with its
 * kind) and a column totalled up to a group (blankAbove). A fresh invited user owns
 * the rows, so the report holds only this test's sales and returns (R14).
 */
type Shot = { id: string; amount: string | null }

// Every drawn row's id and amount cell text. The view draws no amount cell on a row that
// has none (the Total up to a group, a group header); null is an absent cell.
function amounts(page: Page): Promise<Shot[]> {
    return page.getByTestId('report-table').evaluate(table =>
        [...table.querySelectorAll('[data-testid^="report-row-"]')].map(row => {
            const id = row.getAttribute('data-testid')?.replace('report-row-', '') ?? ''
            const cell = table.querySelector(`[data-testid="report-cell-amount-${CSS.escape(id)}"]`)
            return { id, amount: cell ? (cell.textContent ?? '').trim() : null }
        })
    )
}

// A group row's key (g: and s: ids) is its groups' keys then its currency, so the
// deeper level's rows carry the longer key: the team's, against the kind's.
function keyLength(s: Shot) {
    return (JSON.parse(s.id.slice(2)) as unknown[]).length
}

function totals(shots: Shot[]) {
    const subtotals = shots.filter(s => s.id.startsWith('s:'))
    const deepest = Math.max(0, ...subtotals.map(keyLength))
    const isTeam = (s: Shot) => keyLength(s) === deepest
    const headers = shots.filter(s => s.id.startsWith('g:'))
    const shallowest = Math.min(...headers.map(keyLength))
    return {
        team: subtotals
            .filter(isTeam)
            .map(s => (s.amount ?? 'absent').replace('$', ''))
            .sort(),
        kind: subtotals.filter(s => !isTeam(s)).map(s => s.amount),
        total: shots.filter(s => s.id.startsWith('total')).map(s => s.amount),
        // The kind level's header rows, above the team level.
        kindHeaders: headers.filter(s => keyLength(s) === shallowest).map(s => s.amount),
    }
}

test.describe('reports: union source and blank above', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('sales and returns in one report; a total up to the team leaves the kinds and the Total blank, also on a phone', async ({
        freshMember,
    }) => {
        const p = freshMember.page
        const east = uniqueName('East')
        const west = uniqueName('West')
        const sold = uniqueName('Sold')
        const soldWest = uniqueName('Sold west')
        const returned = uniqueName('Returned')
        await seedStub(p, {
            teams: [{ name: east }, { name: west, region: 'south' }],
            sales: [
                { team: east, customer: sold, amount: 1000, cost: 0, day: '2026-07-02' },
                { team: west, customer: soldWest, amount: 400, cost: 0, day: '2026-07-02' },
            ],
            returns: [{ team: west, customer: returned, amount: 250, day: '2026-07-03' }],
        })

        // Columns customer (0), kind (1), team (2), amount summed (3); grouped by
        // kind, then team, both open, with subtotals.
        await openReportsHub(p)
        await startReport(p, ACTIVITY_SOURCE)
        await addColumn(p, ['kind'])
        await p.getByTestId('report-builder-add-column').click()
        await p
            .getByTestId('report-builder-picker')
            .getByTestId('report-builder-pick-record-team')
            .click()
        await addColumn(p, ['amount'])
        await chooseOption(p, 'report-builder-column-3-aggregate', 'sum')
        await p.getByTestId('report-builder-section-group').click()
        await p.getByTestId('report-builder-group-add').click()
        await chooseOption(p, 'report-builder-group-0-col', 'kind')
        await p.getByTestId('report-builder-group-0-open').click()
        await p.getByTestId('report-builder-group-add').click()
        await chooseOption(p, 'report-builder-group-1-col', 'team')
        await p.getByTestId('report-builder-group-1-open').click()

        // One report lists the rows of both collections, each with its kind.
        await expect(await cellOf(p, 'kind', sold)).toHaveText('Sale')
        await expect(await cellOf(p, 'kind', soldWest)).toHaveText('Sale')
        await expect(await cellOf(p, 'kind', returned)).toHaveText('Return')
        // Precondition: both kinds' subtotals and the Total are drawn with a figure.
        await expect
            .poll(async () => {
                const t = totals(await amounts(p))
                return t.kind.length === 2 && t.total.length === 1
                    ? [...t.kind, ...t.total].every(a => a !== null && a !== '')
                    : false
            })
            .toBe(true)

        // Totals up to the team: the team totals keep the figure; the kinds' totals,
        // the kind headers and the Total show no figure. A subtotal draws its amount
        // cell empty; the Total and a group header draw none.
        const upToTeam = {
            team: ['10.00', '2.50', '4.00'],
            kind: ['', ''],
            total: [null],
            kindHeaders: [null, null],
        }
        await chooseOption(p, 'report-builder-column-3-blank-above', 'up-1')
        await expect(p.getByTestId('report-builder-column-3-blank-above')).toHaveText(/Up to Team/)
        await expect.poll(async () => totals(await amounts(p))).toEqual(upToTeam)

        // A removed group leaves the choice in view with its refusal; "Every row"
        // sets it back, and the team group returns for the rest of the flow.
        await p.getByTestId('report-builder-group-1-remove').click()
        await expect(p.getByTestId('report-builder-column-3-blank-above')).toHaveText(
            /Choose a group/
        )
        await expect(p.getByTestId('report-builder-column-3-blank-above-error')).toBeVisible()
        await chooseOption(p, 'report-builder-column-3-blank-above', 'every')
        await expect(p.getByTestId('report-builder-column-3-blank-above')).toHaveText(
            /On every row/
        )
        await expect(p.getByTestId('report-builder-column-3-blank-above-error')).toHaveCount(0)
        await p.getByTestId('report-builder-group-add').click()
        await chooseOption(p, 'report-builder-group-1-col', 'team')
        await p.getByTestId('report-builder-group-1-open').click()
        await chooseOption(p, 'report-builder-column-3-blank-above', 'up-1')
        await expect.poll(async () => totals(await amounts(p))).toEqual(upToTeam)

        await saveNewReport(p, uniqueName('Union up to team'))
        await p.setViewportSize({ width: 390, height: 844 })
        const table = p.getByTestId('report-table')
        await expect.poll(() => table.evaluate(el => el.scrollWidth - el.clientWidth)).toBe(0)
        await expect
            .poll(() =>
                p.evaluate(
                    () =>
                        document.documentElement.scrollWidth - document.documentElement.clientWidth
                )
            )
            .toBe(0)
        // The phone's layout draws no cell for a blank figure, so a kind's subtotal
        // cell is absent there where the desktop draws it empty.
        await expect
            .poll(async () => totals(await amounts(p)))
            .toEqual({ ...upToTeam, kind: [null, null] })
    })

    test('a day shown only up to the team is whole when a closed kind is opened', async ({
        freshMember,
    }) => {
        const p = freshMember.page
        const team = uniqueName('Day team')
        await seedStub(p, {
            teams: [{ name: team }],
            sales: [
                {
                    team,
                    customer: uniqueName('Day sold'),
                    amount: 1000,
                    cost: 0,
                    day: '2026-09-28',
                },
            ],
        })

        // Columns customer (0), kind (1), team (2), the earliest day (3); grouped by
        // kind, then team, neither open; the day shows only up to the team.
        await openReportsHub(p)
        await startReport(p, ACTIVITY_SOURCE)
        await addColumn(p, ['kind'])
        await p.getByTestId('report-builder-add-column').click()
        await p
            .getByTestId('report-builder-picker')
            .getByTestId('report-builder-pick-record-team')
            .click()
        await addColumn(p, ['day'])
        await chooseOption(p, 'report-builder-column-3-aggregate', 'min')
        await p.getByTestId('report-builder-section-group').click()
        await p.getByTestId('report-builder-group-add').click()
        await chooseOption(p, 'report-builder-group-0-col', 'kind')
        await p.getByTestId('report-builder-group-add').click()
        await chooseOption(p, 'report-builder-group-1-col', 'team')
        await chooseOption(p, 'report-builder-column-3-blank-above', 'up-1')
        await saveNewReport(p, uniqueName('Day up to team'))
        await p.setViewportSize({ width: 1280, height: 800 })

        // The run lands with every kind closed: no drawn row shows a day, so the
        // column is measured with no value in it.
        const kind = rowOf(p, 'Sale')
        await expect(kind).toBeVisible()
        const shownDays = () =>
            p
                .getByTestId('report-table')
                .evaluate(table =>
                    [...table.querySelectorAll('[data-testid^="report-cell-day-"]')]
                        .map(cell => (cell.textContent ?? '').trim())
                        .filter(text => text !== '')
                )
        expect(await shownDays()).toEqual([])

        // Opened, the kind shows its team, whose day is drawn whole.
        await kind.click()
        await expect(rowOf(p, team)).toBeVisible()
        const day = await cellOf(p, 'day', team)
        await expect(day).toHaveText(/2026$/)
        await expect
            .poll(() =>
                day
                    .locator('div[dir="auto"]')
                    .evaluate(element => element.scrollWidth - element.clientWidth)
            )
            .toBeLessThanOrEqual(0)
    })
})
