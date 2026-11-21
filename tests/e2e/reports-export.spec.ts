import fs from 'node:fs'
import { type Download, expect, type Locator, type Page, test } from '@playwright/test'
import { authStorageKey } from './auth-key-helpers'
import { openSettingsItem, skipWithoutReportsStub, waitForTableFit } from './helpers'
import { downloadReport, openExportMenu } from './report-view-helpers'
import {
    cellOf,
    holdRuns,
    loginAsShippedOwner,
    openTeamBalance,
    pressToolbar,
    rowOf,
    seedStub,
    setPeriod,
    TEAM_BALANCE_TITLE,
    uniqueName,
} from './reports-helpers'

/**
 * Report exports (download): the Export button of a saved report's toolbar, its
 * menu's three downloads, "Preparing…", refusals as a toast, the states that
 * disable it, and its rows inside "⋯" on a phone. Each test signs in as the
 * shipped reports' owner (an admin, so Settings → Audit log opens) and adds one
 * sale with a rep (customer) no other test uses, through the stub's screen.
 *
 * The report is the stub's shipped "Stub team balance" (owned by that admin): the
 * shipped "Stub larger side" has no period param, so it cannot take a period, and
 * a period with no sales is how the empty state is reached.
 *
 * `page.route` fakes only what the server cannot be made to give: a held export,
 * an export refusal (422, 500) and an invalid run (400). Files are read back in the
 * test process from the download event.
 */

const DESKTOP = { width: 1600, height: 900 }
const PHONE = { width: 390, height: 844 }
// Days no other report spec uses, so the period holds this spec's sales alone.
const FROM = '2039-05-01'
const TO = '2039-05-31'
const SALE_DAY = '2039-05-14'
const EMPTY_FROM = '1902-01-01'
const EMPTY_TO = '1902-01-31'
/** 1 234.50 in cents: its raw CSV text is `1234.50`. */
const SALE_AMOUNT = 123450
const FILE_STEM = 'stub-team-balance'
const PDF_LIMIT = 'This report has more than 5,000 rows. Narrow the period or download CSV.'
const TOAST_TITLE = 'Could not create the file'

/** Signs in, adds a team and one sale in the period, and returns the sale's rep. */
async function seedSale(page: Page, prefix: string): Promise<string> {
    await loginAsShippedOwner(page)
    const team = uniqueName(`${prefix} team`)
    const customer = uniqueName(`${prefix} rep`)
    await seedStub(page, {
        teams: [{ name: team }],
        sales: [{ team, customer, amount: SALE_AMOUNT, cost: 100, day: SALE_DAY }],
    })
    return customer
}

/** Opens the report on the period that holds the sale, and waits for the sale's row. */
async function openOnPeriod(page: Page, customer: string) {
    await page.setViewportSize(DESKTOP)
    await openTeamBalance(page)
    await setPeriod(page, FROM, TO)
    await expect(rowOf(page, customer)).toBeVisible()
}

/** Holds every export request until the returned release is called. */
async function holdExports(page: Page): Promise<() => void> {
    let release = () => {}
    const held = new Promise<void>(resolve => {
        release = resolve
    })
    await page.route('**/api/reports/export', async route => {
        await held
        await route.continue()
    })
    return release
}

async function fileBytes(download: Download): Promise<Buffer> {
    return fs.readFileSync(await download.path())
}

/**
 * The count of `report.export` audit rows of a CSV file of this report, read with
 * the signed-in admin's own token (a read only). The format is in the row's
 * metadata, which the Audit log screen does not show.
 */
async function csvExportAuditRows(page: Page): Promise<number> {
    const key = await authStorageKey(page)
    const token = await page.evaluate(k => JSON.parse(localStorage.getItem(k) ?? '{}').token, key)
    const filter = `action = "report.export" && resource_label = "${TEAM_BALANCE_TITLE}" && metadata.format = "csv"`
    const res = await page.request.get(
        `/api/collections/audit_logs/records?perPage=1&filter=${encodeURIComponent(filter)}`,
        { headers: { Authorization: token } }
    )
    expect(res.ok()).toBe(true)
    return (await res.json()).totalItems as number
}

/**
 * The Audit log screen's rows of a `report.export` of this report: the innermost
 * box that holds both the action badge and the report's title (its ancestors
 * hold both too).
 */
function auditRowsOf(page: Page) {
    const both = (locator: Locator) =>
        locator
            .filter({ has: page.getByText('report.export', { exact: true }) })
            .filter({ has: page.getByText(TEAM_BALANCE_TITLE, { exact: true }) })
    return both(page.locator('div')).filter({ hasNot: both(page.locator('div')) })
}

function isExportPost(url: string, method: string): boolean {
    return method === 'POST' && url.includes('/api/reports/export')
}

test.describe('report-exports: download', () => {
    test.skip(() => skipWithoutReportsStub(), 'needs the salesstub package')

    test('Download CSV saves the view as it is, and writes an audit row', async ({ page }) => {
        const customer = await seedSale(page, 'Export csv')
        await openOnPeriod(page, customer)

        // The reader's sort and a hidden column are part of the view the file holds.
        await page.getByTestId('report-sort-amount').click()
        await expect.poll(() => new URL(page.url()).searchParams.get('s')).toBe('amount:asc')
        await pressToolbar(page, 'report-customize-open', 'Customize')
        await page.getByTestId('report-customize-section-columns').click()
        await page.getByTestId('report-option-column-cost').click()
        await expect(page.getByTestId('report-heading-cost')).toHaveCount(0)
        await page.getByTestId('report-customize-close').click()
        await expect(page.getByTestId('report-customize')).toHaveCount(0)

        // Export is the toolbar's button right after Copy link.
        const exportButton = page.getByTestId('report-export-open')
        await expect(exportButton).toBeVisible()
        await expect
            .poll(async () => {
                const copy = await page.getByTestId('report-copy-link').boundingBox()
                const exp = await exportButton.boundingBox()
                return copy && exp ? exp.x > copy.x : null
            })
            .toBe(true)
        expect(
            await exportButton.evaluate(
                element =>
                    element
                        .closest('[data-testid^="toolbar-item-"]')
                        ?.previousElementSibling?.getAttribute('data-testid') ?? null
            )
        ).toBe('toolbar-item-copy-link')

        const auditRowsBefore = await csvExportAuditRows(page)
        const sent = page.waitForRequest(r => isExportPost(r.url(), r.method()))
        const download = await downloadReport(page, 'csv')
        const body = (await sent).postDataJSON()
        expect(body.format).toBe('csv')
        expect(body.options.sort).toEqual([{ col: 'amount', dir: 'asc' }])
        expect(body.options.display.hidden).toContain('cost')
        expect(typeof body.options.timeZone).toBe('string')
        expect(typeof body.hour12).toBe('boolean')

        expect(download.suggestedFilename()).toBe(`${FILE_STEM}-${FROM}-to-${TO}.csv`)
        const bytes = await fileBytes(download)
        expect([...bytes.subarray(0, 3)]).toEqual([0xef, 0xbb, 0xbf])
        const lines = bytes.subarray(3).toString('utf8').split('\r\n')
        const headings = lines[0].split(',')
        expect(headings[0]).toBe('Customer')
        expect(headings).toContain('Amount')
        expect(headings).not.toContain('Cost')
        const saleLine = lines.find(line => line.startsWith(`${customer},`))
        expect(saleLine, `a line for ${customer}`).toBeDefined()
        expect(saleLine?.split(',')).toContain('1234.50')
        await expect(exportButton).toHaveText('Export')

        // This download wrote one audit row of its own: a CSV export of this report.
        // The file's tests run one after another in one worker, and no other spec
        // exports this report, so nothing else adds such a row meanwhile.
        await expect.poll(() => csvExportAuditRows(page)).toBe(auditRowsBefore + 1)
        await openSettingsItem(page, 'Audit Log')
        await page.getByTestId('audit-search').fill('report.export')
        await expect(auditRowsOf(page).first()).toBeVisible()
    })

    test('while the file is prepared the button says so, and the report does not run again', async ({
        page,
    }) => {
        // Another worker's sale would rerun the report through its live topic; the
        // run count below is about the export alone, so topic subscriptions are
        // answered here and never reach the server. Realtime is cut before login,
        // so no subscription made before the route reaches the server.
        await page.route('**/api/realtime', route =>
            route.request().method() === 'POST' ? route.fulfill({ status: 204 }) : route.fallback()
        )
        const customer = await seedSale(page, 'Export held')
        await openOnPeriod(page, customer)
        const amountBefore = await (await cellOf(page, 'amount', customer)).textContent()
        const exportButton = page.getByTestId('report-export-open')
        await expect(exportButton).toBeVisible()

        const release = await holdExports(page)
        let runs = 0
        page.on('request', request => {
            if (request.method() === 'POST' && request.url().includes('/api/reports/run')) runs++
        })
        const downloaded = page.waitForEvent('download')
        await openExportMenu(page)
        await page.getByTestId('report-export-pdf').click()

        await expect(exportButton).toBeDisabled()
        await expect(exportButton).toHaveText('Preparing…')
        await expect(page.getByTestId('report-preparing')).toHaveText('Preparing…')
        await expect(rowOf(page, customer)).toBeVisible()
        await expect(await cellOf(page, 'amount', customer)).toHaveText(amountBefore ?? '')

        release()
        const download = await downloaded
        await page.unrouteAll({ behavior: 'wait' })
        expect(runs).toBe(0)
        await expect(exportButton).toHaveText('Export')
        await expect(exportButton).toBeEnabled()
        await expect(page.getByTestId('report-preparing')).toHaveCount(0)
        const bytes = await fileBytes(download)
        expect(bytes.subarray(0, 5).toString('latin1')).toBe('%PDF-')
    })

    test('a refused export shows a toast and leaves the report as it was', async ({ page }) => {
        const customer = await seedSale(page, 'Export refused')
        await openOnPeriod(page, customer)
        const amountBefore = await (await cellOf(page, 'amount', customer)).textContent()
        let downloads = 0
        page.on('download', () => {
            downloads++
        })
        const exportButton = page.getByTestId('report-export-open')
        await expect(exportButton).toBeVisible()

        await page.route('**/api/reports/export', route =>
            route.fulfill({
                status: 422,
                contentType: 'application/json',
                body: JSON.stringify({ status: 422, message: PDF_LIMIT, data: {} }),
            })
        )
        await openExportMenu(page)
        await page.getByTestId('report-export-pdf').click()
        const limitToast = page.getByTestId('toast-card').filter({ hasText: PDF_LIMIT })
        await expect(limitToast).toContainText(TOAST_TITLE)
        await expect(limitToast.getByTestId('toast-variant-error')).toBeVisible()
        await expect(rowOf(page, customer)).toBeVisible()
        await expect(await cellOf(page, 'amount', customer)).toHaveText(amountBefore ?? '')
        await expect(exportButton).toBeEnabled()
        await expect(exportButton).toHaveText('Export')
        await page.unrouteAll({ behavior: 'wait' })

        // A failure with no message of its own names the status.
        await page.route('**/api/reports/export', route =>
            route.fulfill({ status: 500, contentType: 'text/plain', body: '' })
        )
        await openExportMenu(page)
        await page.getByTestId('report-export-pdf').click()
        const failedToast = page
            .getByTestId('toast-card')
            .filter({ hasText: 'The server answered 500.' })
        await expect(failedToast).toContainText(TOAST_TITLE)
        await expect(failedToast.getByTestId('toast-variant-error')).toBeVisible()
        await expect(exportButton).toBeEnabled()
        await page.unrouteAll({ behavior: 'wait' })
        expect(downloads).toBe(0)
    })

    test('Export works only when the report can be exported, an empty one included', async ({
        page,
        context,
    }) => {
        const customer = await seedSale(page, 'Export states')
        await page.setViewportSize(DESKTOP)
        const exportButton = page.getByTestId('report-export-open')

        // An invalid report (the run refused with 400).
        await page.route('**/api/reports/run', route =>
            route.fulfill({
                status: 400,
                contentType: 'application/json',
                body: JSON.stringify({
                    status: 400,
                    message: 'The report is not valid.',
                    data: {},
                }),
            })
        )
        await openTeamBalance(page)
        await expect(page.getByTestId('report-invalid')).toBeVisible()
        await expect(exportButton).toBeDisabled()
        await page.unrouteAll({ behavior: 'wait' })
        await setPeriod(page, FROM, TO)
        await expect(rowOf(page, customer)).toBeVisible()
        await expect(exportButton).toBeEnabled()

        // Offline (connection-indicator.spec.ts waits for the same notice).
        const offlineNotice = page.getByText('Offline — waiting for a connection', { exact: true })
        await context.setOffline(true)
        await expect(offlineNotice).toBeVisible({ timeout: 3_000 })
        await expect(exportButton).toBeDisabled()
        await context.setOffline(false)
        await expect(offlineNotice).toBeHidden({ timeout: 2_000 })
        await expect(exportButton).toBeEnabled()

        // While a changed run loads, the view still shows the last result, whose
        // period would name the file: Export waits. The empty period is a
        // body this test has not run, so the run is not served from the cache.
        const releaseRun = await holdRuns(page)
        await setPeriod(page, EMPTY_FROM, EMPTY_TO)
        await expect(page.getByTestId('report-updating')).toBeVisible()
        await expect(rowOf(page, customer)).toBeVisible()
        await expect(exportButton).toBeDisabled()
        releaseRun()
        await page.unrouteAll({ behavior: 'wait' })

        // An empty report exports its headings.
        await expect(page.getByTestId('report-empty')).toBeVisible()
        await expect(exportButton).toBeEnabled()
        const download = await downloadReport(page, 'xlsx')
        expect(download.suggestedFilename()).toBe(`${FILE_STEM}-${EMPTY_FROM}-to-${EMPTY_TO}.xlsx`)
        const bytes = await fileBytes(download)
        expect(bytes.subarray(0, 2).toString('latin1')).toBe('PK')
    })

    test('on a phone the export rows are entries of "⋯"', async ({ page }) => {
        const customer = await seedSale(page, 'Export phone')
        await openOnPeriod(page, customer)
        await expect(page.getByTestId('report-export-open')).toBeVisible()

        await page.setViewportSize(PHONE)
        await waitForTableFit(page)
        const more = page.getByTestId('toolbar-more-button')
        await expect(more).toBeVisible()
        await expect(page.getByTestId('report-export-open')).toHaveCount(0)

        await more.click()
        for (const [testId, label] of [
            ['report-export-pdf', 'Download PDF'],
            ['report-export-xlsx', 'Download Excel'],
            ['report-export-csv', 'Download CSV'],
        ]) {
            const row = page.getByTestId(testId)
            await expect(row).toHaveAttribute('role', 'menuitem')
            await expect(row).toContainText(label)
        }
        await expect(page.getByRole('menuitem', { name: 'Export', exact: true })).toHaveCount(0)
        const downloaded = page.waitForEvent('download')
        await page.getByTestId('report-export-csv').click()
        expect((await downloaded).suggestedFilename()).toMatch(/\.csv$/)

        // While a second file is prepared, the status slot says so and the rows wait.
        const release = await holdExports(page)
        const second = page.waitForEvent('download')
        await more.click()
        await page.getByTestId('report-export-pdf').click()
        await expect(page.getByTestId('report-preparing')).toHaveText('Preparing…')
        await more.click()
        for (const testId of ['report-export-pdf', 'report-export-xlsx', 'report-export-csv']) {
            await expect(page.getByTestId(testId)).toBeDisabled()
        }
        await page.keyboard.press('Escape')
        release()
        await second
        await page.unrouteAll({ behavior: 'wait' })
        await expect(page.getByTestId('report-preparing')).toHaveCount(0)
    })
})
