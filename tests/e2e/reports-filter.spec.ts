import { expect, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub } from './helpers'
import {
    addSale,
    addTeam,
    openReportStub,
    openReportsHub,
    SALES_SOURCE,
    startReport,
    TEAMS_SOURCE,
    uniqueName,
} from './reports-helpers'

/**
 * The builder's filter against the salesstub package. Teams are shared by every
 * user, so the first test scopes its teams by a name stem; a fresh invited member
 * owns the sales, so related-row totals count only this test's sales.
 */
test.describe('filtering a report', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('has and has no related rows filter teams by their sales', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptfilter')
        try {
            const stem = uniqueName('F')
            const big = `${stem} big`
            const small = `${stem} small`
            await openReportStub(p)
            await addTeam(p, big)
            await addTeam(p, small)
            await addSale(p, {
                team: big,
                customer: 'Large',
                amount: 60000,
                cost: 1000,
                day: '2026-01-05',
            })
            await addSale(p, {
                team: small,
                customer: 'Little',
                amount: 1000,
                cost: 100,
                day: '2026-01-06',
            })

            await openReportsHub(p)
            await startReport(p, TEAMS_SOURCE)
            await p.getByTestId('report-builder-section-filter').click()
            // Name starts with this test's stem.
            await p.getByTestId('report-builder-filter-add-condition').click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-name')
                .click()
            await p.getByTestId('report-builder-filter-0-op').click()
            await p.getByTestId('report-builder-filter-0-op-startsWith').click()
            await p.getByTestId('report-builder-filter-0-value').fill(stem)
            // Has related sales where the amount is over 500.00.
            await p.getByTestId('report-builder-filter-add-related').click()
            await p.getByTestId('report-builder-filter-add-related-sales').click()
            await p.getByTestId('report-builder-filter-1-add-condition').click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-amount')
                .click()
            await p.getByTestId('report-builder-filter-1-0-op').click()
            await p.getByTestId('report-builder-filter-1-0-op-gt').click()
            await p.getByTestId('report-builder-filter-1-0-value').fill('500')

            const table = p.getByTestId('report-table')
            await expect(table.getByText(big, { exact: true })).toBeVisible()
            await expect(table.getByText(small, { exact: true })).toHaveCount(0)

            await p.getByTestId('report-builder-filter-1-has').click()
            await p.getByTestId('report-builder-filter-1-has-no').click()
            await expect(table.getByText(small, { exact: true })).toBeVisible()
            await expect(table.getByText(big, { exact: true })).toHaveCount(0)
        } finally {
            await close()
        }
    })

    test('any of: a date on or after a day, or a status', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptany')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, {
                team,
                customer: 'January open',
                amount: 100,
                cost: 0,
                day: '2026-01-10',
            })
            await addSale(p, {
                team,
                customer: 'March open',
                amount: 200,
                cost: 0,
                day: '2026-03-05',
            })
            await addSale(p, {
                team,
                customer: 'January paid',
                amount: 300,
                cost: 0,
                day: '2026-01-20',
                status: 'paid',
            })

            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await p.getByTestId('report-builder-section-filter').click()
            await p.getByTestId('report-builder-filter-match').click()
            await p.getByTestId('report-builder-filter-match-any').click()
            await p.getByTestId('report-builder-filter-add-condition').click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-day')
                .click()
            await p.getByTestId('report-builder-filter-0-op').click()
            await p.getByTestId('report-builder-filter-0-op-gte').click()
            await p.getByTestId('report-builder-filter-0-value').fill('2026-03-01')
            await p.getByTestId('report-builder-filter-add-condition').click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-status')
                .click()
            await p.getByTestId('report-builder-filter-1-value').click()
            await p.getByTestId('report-builder-filter-1-value-paid').click()

            const table = p.getByTestId('report-table')
            await expect(table.getByText('March open', { exact: true })).toBeVisible()
            await expect(table.getByText('January paid', { exact: true })).toBeVisible()
            await expect(table.getByText('January open', { exact: true })).toHaveCount(0)
        } finally {
            await close()
        }
    })

    test('ask when run turns a value into an asked value with a default', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptask')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, {
                team,
                customer: 'Early sale',
                amount: 100,
                cost: 0,
                day: '2026-01-10',
            })
            await addSale(p, {
                team,
                customer: 'Later sale',
                amount: 200,
                cost: 0,
                day: '2026-03-05',
            })

            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await p.getByTestId('report-builder-section-filter').click()
            await p.getByTestId('report-builder-filter-add-condition').click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-day')
                .click()
            await p.getByTestId('report-builder-filter-0-op').click()
            await p.getByTestId('report-builder-filter-0-op-gte').click()
            await p.getByTestId('report-builder-filter-0-value').fill('2026-03-01')
            const table = p.getByTestId('report-table')
            await expect(table.getByText('Later sale', { exact: true })).toBeVisible()
            await expect(table.getByText('Early sale', { exact: true })).toHaveCount(0)

            // Asked with no default, the reader has not set it: it filters nothing.
            await p.getByTestId('report-builder-filter-0-ask').click()
            await expect(p.getByTestId('report-builder-filter-0-value')).toBeHidden()
            await expect(table.getByText('Early sale', { exact: true })).toBeVisible()
            await expect(table.getByText('Later sale', { exact: true })).toBeVisible()

            // A default of today is later than every sale.
            await p.getByTestId('report-builder-filter-0-default').click()
            await p.getByTestId('report-builder-filter-0-default-today').click()
            await expect(p.getByTestId('report-empty')).toBeVisible()

            // Money cannot be asked.
            await p.getByTestId('report-builder-filter-add-condition').click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-amount')
                .click()
            await expect(p.getByTestId('report-builder-filter-1-op')).toBeVisible()
            await expect(p.getByTestId('report-builder-filter-1-ask')).toHaveCount(0)
        } finally {
            await close()
        }
    })
})
