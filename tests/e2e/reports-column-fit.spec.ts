import { expect, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub, waitForTableFit } from './helpers'
import {
    addColumn,
    addSale,
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
})
