import { expect, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub, waitForTableFit } from './helpers'
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
 * Spec v4 column fit set in the builder: a figure is never cut, the report is one
 * line only from its one-line width, an arrangement is used only from its Use from
 * width, and a phone below both gets automatic lines.
 */
test.describe('report column fit', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('one line from 1040, a tablet arrangement from 600, automatic lines on a phone', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptcfit')
        try {
            const team = uniqueName('Fit Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, {
                team,
                customer: 'Fitco 01',
                amount: 12345,
                cost: 100,
                day: '2026-01-05',
                status: 'paid',
            })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['amount'])
            // An amount is never cut: it offers only Extra room.
            await expect(p.getByTestId('report-builder-column-1-fit-never-cut')).toBeVisible()
            await expect(p.getByTestId('report-builder-column-1-fit-min')).toHaveCount(0)
            await expect(p.getByTestId('report-builder-column-1-fit-grow')).toContainText('None')
            // As the label column the amount takes a share of 1 (Normal), and None can be set.
            await p.getByTestId('report-builder-column-1-up').click()
            await expect(p.getByTestId('report-builder-column-0-fit-grow')).toContainText('Normal')
            await chooseOption(p, 'report-builder-column-0-fit-grow', '0')
            await expect(p.getByTestId('report-builder-column-0-fit-grow')).toContainText('None')
            await chooseOption(p, 'report-builder-column-0-fit-grow', '1')
            await p.getByTestId('report-builder-column-0-down').click()
            await chooseOption(p, 'report-builder-column-0-fit-min', '12')
            await expect(p.getByTestId('report-builder-column-0-fit-min')).toContainText(
                '12 letters'
            )
            await p.getByTestId('report-builder-section-arrange').click()
            await chooseOption(p, 'report-builder-arrange-one-line', '1040')
            await p.getByTestId('report-builder-arrange-width-tablet').click()
            await p.getByTestId('report-builder-arrange-on').click()
            await chooseOption(p, 'report-builder-arrange-use-from', '600')
            // Turning an arrangement on keeps the one-line width.
            await expect(p.getByTestId('report-builder-arrange-one-line')).toContainText('1040 px')
            await saveNewReport(p, uniqueName('Column fit'))

            // Rows on /a/reports are the window less the 64 px rail: 1216 and 960.
            await p.setViewportSize({ width: 1280, height: 800 })
            expect(await waitForTableFit(p)).toBe('one-line')
            await p.setViewportSize({ width: 1024, height: 800 })
            expect(await waitForTableFit(p)).toBe('tablet')
            // A phone has no rail: 390 is under both gates.
            await p.setViewportSize({ width: 390, height: 844 })
            expect(await waitForTableFit(p)).toBe('auto')
        } finally {
            await close()
        }
    })

    test('long text set to wrap takes two lines on a phone and one on a wide screen', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptwrap2')
        try {
            const team = uniqueName('Wrap Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSales(p, {
                team,
                customerPrefix:
                    'Fitco Wholesale Partners of the Greater Northwest Region Cooperative',
                count: 1,
                firstDay: '2026-01-05',
                firstAmount: 12345,
                cost: 100,
                quantity: 1,
            })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-0-fit-wrap', '2')
            await saveNewReport(p, uniqueName('Wrap'))
            // The customer is the label of each detail row; its text is the row's first text.
            const label = p
                .locator('[data-testid^="report-row-d:"]')
                .first()
                .locator('div[dir="auto"]')
            const height = async () => (await label.boundingBox())?.height ?? 0

            await p.setViewportSize({ width: 1280, height: 800 })
            await waitForTableFit(p)
            // text-sm is 20 px a line.
            await expect.poll(height).toBeLessThan(26)
            // Two columns still fit one line on a phone (Σmin ≈ 230 < 375), so the label
            // wraps inside its one-line box (about 260 px of room for about 500 px of text).
            await p.setViewportSize({ width: 390, height: 844 })
            expect(await waitForTableFit(p)).toBe('one-line')
            await expect.poll(height).toBeGreaterThan(30)
            await expect.poll(height).toBeLessThan(46)
        } finally {
            await close()
        }
    })

    test('a wrapping column with a narrow least width never splits a word on a phone', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptword')
        try {
            const team = uniqueName('Word Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSales(p, {
                team,
                customerPrefix: 'Fitco Supercalifragilisticexpialidocious Wholesale',
                count: 1,
                firstDay: '2026-01-05',
                firstAmount: 12345,
                cost: 100,
                quantity: 1,
            })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            // Three figures are never cut and fill most of a phone's line, so the label
            // gets little more than its least width: six letters, much less than
            // "Supercalifragilisticexpialidocious".
            await addColumn(p, ['amount'])
            await addColumn(p, ['cost'])
            await addColumn(p, ['quantity'])
            await chooseOption(p, 'report-builder-column-0-fit-wrap', '2')
            await chooseOption(p, 'report-builder-column-0-fit-min', '6')
            await saveNewReport(p, uniqueName('Word'))
            const label = p
                .locator('[data-testid^="report-row-d:"]')
                .first()
                .locator('div[dir="auto"]')

            await p.setViewportSize({ width: 390, height: 844 })
            await waitForTableFit(p)
            await expect(label).toContainText('Wholesale')
            // Each word is one DOM range: a word split across lines has two client rects.
            const words = () =>
                label.evaluate(element => {
                    const node = element.firstChild
                    const text = node?.textContent ?? ''
                    const box = element.getBoundingClientRect().width
                    const found = [...text.matchAll(/\S+/g)].map(match => {
                        const range = document.createRange()
                        range.setStart(node as Node, match.index)
                        range.setEnd(node as Node, match.index + match[0].length)
                        const rects = range.getClientRects()
                        return { word: match[0], rects: rects.length, width: rects[0]?.width ?? 0 }
                    })
                    return { box, found }
                })
            await expect
                .poll(async () => (await words()).found.filter(w => w.rects !== 1).map(w => w.word))
                .toEqual([])
            const { box, found } = await words()
            // A word a few px wider than its line is clipped, as CSS does, never split.
            expect(await label.evaluate(element => getComputedStyle(element).overflowWrap)).toBe(
                'normal'
            )
            const widest = Math.max(...found.map(w => w.width))
            expect(box).toBeGreaterThanOrEqual(widest)
            // The label wraps: it is narrower than its whole text.
            expect(box).toBeLessThan(found.reduce((sum, w) => sum + w.width, 0))
        } finally {
            await close()
        }
    })
})
