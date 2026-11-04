import { expect, type Page, type Route, test } from '@playwright/test'
import { skipWithoutReportsStub } from './helpers'
import {
    addSale,
    addTeam,
    hubRow,
    loginAsShippedOwner,
    openReportStub,
    openReportsHub,
    rowOf,
    TEAM_TOTALS_TITLE,
    uniqueName,
} from './reports-helpers'

/**
 * The report view's failure states and their Retry, on the stub's "Stub team
 * totals" (owned by the shipped reports' owner) over a team and a sale this test
 * adds through the stub's screen. The failures are made by answering the report's
 * own requests in the browser (page.route); nothing is written outside the UI.
 *
 * A failed request is answered with a 404: a status the run and the options do not
 * retry (useReportRun and useParamOptions retry only a 5xx or a lost connection,
 * twice, before they show the error) and that is not one of the run's refusals
 * (run-client.ts refusalOf), so one failed request shows the error state. Every
 * request fails until the test lifts the failure just before it presses Retry, so
 * a second request the screen might send cannot clear the state before it is read.
 */

const NOT_FOUND = {
    status: 404,
    contentType: 'application/json',
    body: JSON.stringify({ status: 404, message: 'Not found.', data: {} }),
}

/** Answers every request to `url` with `answer` until the returned lift is called. */
async function failUntilLifted(
    page: Page,
    url: string,
    answer: Parameters<Route['fulfill']>[0]
): Promise<() => void> {
    const failure = { isOn: true }
    await page.route(url, route => (failure.isOn ? route.fulfill(answer) : route.continue()))
    return () => {
        failure.isOn = false
    }
}

async function seedTeam(page: Page): Promise<string> {
    await loginAsShippedOwner(page)
    const team = uniqueName('Failing team')
    await openReportStub(page)
    await addTeam(page, team)
    await addSale(page, { team, customer: 'Fay', amount: 100, cost: 0, day: '2026-01-05' })
    return team
}

async function openTeamTotals(page: Page) {
    await openReportsHub(page)
    await page.getByTestId('reports-hub-tab-mine').click()
    await hubRow(page, TEAM_TOTALS_TITLE).click()
    await expect(page.getByTestId('report-title')).toHaveText(TEAM_TOTALS_TITLE)
}

test.describe('report failure states', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a run that fails shows the error, and Retry runs it again', async ({ page }) => {
        const team = await seedTeam(page)
        const lift = await failUntilLifted(page, '**/api/reports/run', NOT_FOUND)
        await openTeamTotals(page)
        await expect(page.getByTestId('report-error-message')).toHaveText(
            'Could not load this report'
        )
        lift()
        await page.getByTestId('report-retry').click()
        await expect(rowOf(page, team)).toBeVisible()
        await expect(page.getByTestId('report-error')).toHaveCount(0)
    })

    test('a run that takes too long says so, and Retry runs it again', async ({ page }) => {
        const team = await seedTeam(page)
        const lift = await failUntilLifted(page, '**/api/reports/run', {
            status: 408,
            contentType: 'application/json',
            body: JSON.stringify({ status: 408, message: 'The report took too long.', data: {} }),
        })
        await openTeamTotals(page)
        await expect(page.getByTestId('report-timeout-message')).toHaveText(
            'This report took too long to run'
        )
        lift()
        await page.getByTestId('report-retry').click()
        await expect(rowOf(page, team)).toBeVisible()
        await expect(page.getByTestId('report-timeout')).toHaveCount(0)
    })

    test('options that fail to load say so in the picker, and its Retry loads them', async ({
        page,
    }) => {
        const team = await seedTeam(page)
        await openTeamTotals(page)
        await expect(rowOf(page, team)).toBeVisible()
        const lift = await failUntilLifted(page, '**/api/reports/param-options', NOT_FOUND)
        await page.getByTestId('report-add-filter').click()
        await page.getByTestId('report-add-filter-teams').click()
        const picker = page.getByTestId('report-filter-picker')
        await expect(picker.getByTestId('report-filter-load-error')).toHaveText(
            'The options could not be loaded.'
        )
        lift()
        await picker.getByTestId('report-filter-retry').click()
        await expect(picker.getByTestId('report-filter-load-error')).toHaveCount(0)
        await picker.getByTestId('report-filter-search').fill(team)
        await expect(picker.getByRole('checkbox', { name: team, exact: true })).toBeVisible()
    })
})
