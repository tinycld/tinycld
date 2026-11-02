import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, login, skipWithoutReportsStub } from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    chooseOption,
    chooseParamOption,
    hubRow,
    loginAsShippedOwner,
    openReportStub,
    openReportsHub,
    pressToolbar,
    rowOf,
    SALES_SOURCE,
    STUB_HEADING,
    startReport,
    TEAM_TOTALS_TITLE,
    uniqueName,
} from './reports-helpers'

/**
 * Choice params whose options come from a source (E8), the source heading (E9)
 * and an unpublished shipped report reached through a share (E10), against the
 * stub's "Stub team totals" and a draft over sales. A shipped report is owned by
 * the first owner (shipped.go firstAdmin), the seed's admin app user, so the specs
 * that open it sign in as that user; it is unpublished, so no one else sees it
 * until it is shared. Each user reads only their own stub sales and teams, and a
 * user's sales are shared by several tests, so rows are found by team names
 * unique to the test.
 */

async function seedRegions(p: Page, stem: string) {
    await openReportStub(p)
    await addTeam(p, `${stem} N1`, 'north')
    await addTeam(p, `${stem} N2`, 'north')
    await addTeam(p, `${stem} S1`, 'south')
    for (const [team, customer] of [
        [`${stem} N1`, 'Nia'],
        [`${stem} N2`, 'Ned'],
        [`${stem} S1`, 'Sol'],
    ] as const) {
        await addSale(p, { team, customer, amount: 100, cost: 0, day: '2026-01-05' })
    }
}

async function openTeamTotals(p: Page) {
    await openReportsHub(p)
    await p.getByTestId('reports-hub-tab-mine').click()
    await hubRow(p, TEAM_TOTALS_TITLE).click()
    await expect(p.getByTestId('report-title')).toHaveText(TEAM_TOTALS_TITLE)
}

test.describe('params from a source, headings and unpublished shipped reports', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('an unpublished shipped report prints its heading and reaches only who it is shared with', async ({
        page,
    }) => {
        // A fresh reader each run, with a name no one else has, so a re-run on the
        // same database finds them as a candidate (F24). The shared test user
        // invites; the shipped report's owner then signs in on the same page.
        const readerName = uniqueName('Reader')
        const { inviteePage: c, close } = await createInvitedUser(page, 'rptshr', {
            name: readerName,
        })
        try {
            await loginAsShippedOwner(page)
            await openReportsHub(page)
            await page.getByTestId('reports-hub-tab-published').click()
            await expect(page.getByTestId('reports-hub-tab-published')).toHaveAttribute(
                'aria-selected',
                'true'
            )
            await expect(hubRow(page, TEAM_TOTALS_TITLE)).toHaveCount(0)
            await openTeamTotals(page)
            await expect(page.getByTestId('report-business-name')).toHaveText(STUB_HEADING)

            await openReportsHub(c)
            await c.getByTestId('reports-hub-tab-published').click()
            await expect(c.getByTestId('reports-hub-tab-published')).toHaveAttribute(
                'aria-selected',
                'true'
            )
            await expect(hubRow(c, TEAM_TOTALS_TITLE)).toHaveCount(0)

            await pressToolbar(page, 'report-share-open', 'Share')
            await page.getByTestId('report-share-search').fill(readerName)
            await page
                .getByTestId('report-share-dialog')
                .locator('[data-testid^="report-share-candidate-"]')
                .filter({ hasText: readerName })
                .click()
            await page.getByTestId('report-share-done').click()

            await c.getByTestId('reports-hub-tab-shared').click()
            await hubRow(c, TEAM_TOTALS_TITLE).click()
            await expect(c.getByTestId('report-title')).toHaveText(TEAM_TOTALS_TITLE)
            await expect(c.getByTestId('report-business-name')).toHaveText(STUB_HEADING)
        } finally {
            await close()
        }
    })

    test('a choice from a source searches, picks one record, then a whole group', async ({
        page,
    }) => {
        await loginAsShippedOwner(page)
        const stem = uniqueName('Src')
        await seedRegions(page, stem)
        await openTeamTotals(page)
        await chooseParamOption(page, 'teams', {
            from: 'add-filter',
            search: stem,
            labels: [`${stem} N1`],
        })
        await expect(page.getByTestId('report-param-teams')).toHaveText(`Teams: ${stem} N1`)
        const rows = page.getByTestId('report-table').locator('[data-testid^="report-row-g:"]')
        await expect(rows).toHaveText([`${stem} N1`])
        await chooseParamOption(page, 'teams', { from: 'chip', search: stem, group: 'north' })
        await expect(page.getByTestId('report-param-teams')).toHaveText(
            `Teams: ${stem} N1, All North`
        )
        await expect(rowOf(page, `${stem} N2`)).toBeVisible()
        await expect(rows.filter({ hasText: `${stem} S1` })).toHaveCount(0)
    })

    test('on a phone the source picker is a sheet and the page does not scroll sideways', async ({
        page,
    }) => {
        await loginAsShippedOwner(page)
        const stem = uniqueName('SrcPh')
        await seedRegions(page, stem)
        await openTeamTotals(page)
        await page.setViewportSize({ width: 390, height: 844 })
        await page.getByTestId('report-add-filter').click()
        await page.getByTestId('report-add-filter-teams').click()
        const picker = page.getByTestId('report-filter-picker')
        await expect(picker.getByTestId('sheet-handle')).toBeVisible()
        await expect.poll(async () => (await picker.boundingBox())?.width).toBe(390)
        await page.keyboard.press('Escape')
        await expect(picker).toBeHidden()
        await chooseParamOption(page, 'teams', {
            from: 'add-filter',
            search: stem,
            labels: [`${stem} S1`],
        })
        await expect(rowOf(page, `${stem} S1`)).toBeVisible()
        await expect
            .poll(() => page.evaluate(() => document.documentElement.scrollWidth))
            .toBeLessThanOrEqual(390)
    })

    test('a condition on a linked team picks a record, and Ask when run lists the teams', async ({
        page,
    }) => {
        await login(page)
        const stem = uniqueName('Rel')
        await seedRegions(page, stem)
        await openReportsHub(page)
        await startReport(page, SALES_SOURCE)
        await page.getByTestId('report-builder-section-filter').click()
        await page.getByTestId('report-builder-filter-add-condition').click()
        await page
            .getByTestId('report-builder-picker')
            .getByTestId('report-builder-pick-record-team')
            .click()
        await page.getByTestId('report-builder-filter-0-value').click()
        await page.getByTestId('report-filter-search').fill(`${stem} S1`)
        await page
            .getByTestId('report-filter-picker')
            .getByRole('radio', { name: `${stem} S1`, exact: true })
            .click()
        await expect(page.getByTestId('report-builder-filter-0-value')).toHaveText(`${stem} S1`)
        const table = page.getByTestId('report-table')
        await expect(table.getByText('Sol', { exact: true })).toBeVisible()
        await expect(table.getByText('Nia', { exact: true })).toHaveCount(0)

        // "Is any of" shows each picked record as a chip that removes it.
        await chooseOption(page, 'report-builder-filter-0-op', 'in')
        await page.getByTestId('report-builder-filter-0-value').click()
        await page.getByTestId('report-filter-search').fill(stem)
        const picker = page.getByTestId('report-filter-picker')
        await picker.getByRole('checkbox', { name: `${stem} N1`, exact: true }).click()
        await picker.getByRole('checkbox', { name: `${stem} N2`, exact: true }).click()
        await picker.getByTestId('report-filter-apply').click()
        const chips = page.locator('[data-testid^="report-builder-filter-0-value-"]')
        await expect(chips).toHaveText([`${stem} N1`, `${stem} N2`])
        await expect(table.getByText('Ned', { exact: true })).toBeVisible()
        await chips.filter({ hasText: `${stem} N2` }).click()
        await expect(chips).toHaveText([`${stem} N1`])
        await expect(table.getByText('Ned', { exact: true })).toHaveCount(0)
        await expect(table.getByText('Nia', { exact: true })).toBeVisible()

        await page.getByTestId('report-builder-filter-0-ask').click()
        await chooseParamOption(page, 'team', {
            from: 'add-filter',
            search: stem,
            labels: [`${stem} N1`, `${stem} N2`],
        })
        await expect(table.getByText('Nia', { exact: true })).toBeVisible()
        await expect(table.getByText('Ned', { exact: true })).toBeVisible()
        await expect(table.getByText('Sol', { exact: true })).toHaveCount(0)
    })

    test('a having on a money total asks an amount, kept to its currency', async ({ page }) => {
        await login(page)
        const stem = uniqueName('Amt')
        await seedRegions(page, stem)
        // N1 adds up to 4.00 USD, N2 to 1.00 USD.
        await addSale(page, {
            team: `${stem} N1`,
            customer: 'Nia',
            amount: 300,
            cost: 0,
            day: '2026-01-06',
        })
        await openReportsHub(page)
        await startReport(page, SALES_SOURCE)
        await page.getByTestId('report-builder-add-column').click()
        await page
            .getByTestId('report-builder-picker')
            .getByTestId('report-builder-pick-record-team')
            .click()
        // A new draft starts with the customer column, so team is column 1.
        await addColumn(page, ['amount'])
        await chooseOption(page, 'report-builder-column-2-aggregate', 'sum')
        await page.getByTestId('report-builder-section-filter').click()
        await page.getByTestId('report-builder-filter-add-condition').click()
        await page
            .getByTestId('report-builder-picker')
            .getByTestId('report-builder-pick-link-team')
            .click()
        await page
            .getByTestId('report-builder-picker')
            .getByTestId('report-builder-pick-field-name')
            .click()
        await chooseOption(page, 'report-builder-filter-0-op', 'startsWith')
        await page.getByTestId('report-builder-filter-0-value').fill(stem)
        await page.getByTestId('report-builder-section-group').click()
        await page.getByTestId('report-builder-group-add').click()
        await chooseOption(page, 'report-builder-group-0-col', 'team')
        await page.getByTestId('report-builder-group-0-having-add-total').click()
        await page
            .getByTestId('report-builder-picker')
            .getByTestId('report-builder-pick-field-amount')
            .click()
        await chooseOption(page, 'report-builder-group-0-having-0-aggregate', 'sum')
        await chooseOption(page, 'report-builder-group-0-having-0-op', 'gte')
        await page.getByTestId('report-builder-group-0-having-0-ask').click()

        const table = page.getByTestId('report-table')
        const chip = page.getByTestId('report-param-amount')
        await page.getByTestId('report-add-filter').click()
        await page.getByTestId('report-add-filter-amount').click()
        const picker = page.getByTestId('report-filter-picker')
        await picker.getByTestId('report-filter-value').fill('2.00')
        await picker.getByTestId('report-filter-currency').fill('usd')
        await picker.getByTestId('report-filter-apply').click()
        await expect(chip).toHaveText('Sum of Amount: USD 2.00')
        await expect(rowOf(page, `${stem} N1`)).toBeVisible()
        await expect(table.getByText(`${stem} N2`, { exact: true })).toHaveCount(0)

        // A typo is refused in the picker and applies nothing.
        await chip.click()
        await picker.getByTestId('report-filter-value').fill('2.0.0')
        await expect(picker.getByText('Type an amount, like 1500.00')).toBeVisible()
        await expect(picker.getByTestId('report-filter-apply')).toBeDisabled()

        // The sales are in USD: a EUR amount compares with none of them (R29).
        await picker.getByTestId('report-filter-value').fill('2.00')
        await picker.getByTestId('report-filter-currency').fill('EUR')
        await picker.getByTestId('report-filter-apply').click()
        await expect(chip).toHaveText('Sum of Amount: EUR 2.00')
        await expect(table.getByText(`${stem} N1`, { exact: true })).toHaveCount(0)
    })
})
