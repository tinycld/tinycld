import { expect, type Page, test } from '@playwright/test'
import { inStatement, login, skipWithoutReportsStub } from './helpers'
import {
    addSale,
    addTeam,
    openDrilled,
    openReportStub,
    pressToolbar,
    recordPanel,
    superuserClient,
    uniqueName,
    ZONE_TITLE,
} from './reports-helpers'

/**
 * Date-time cells in the reader's time zone (T1) and a detail row's record in the
 * side panel (D5), on the stub's "Stub sales by time"
 * (tests/scripts/reports-stub/server/register.go.txt), whose rows open the stub's
 * record panel (path `panel`). The browser reads America/Chicago; the server is UTC.
 * A sale's `created` is set by the server, so the expected text is read back with a
 * read-only GET and written here in Node; the edges of local days are pinned in Go
 * (core/server/reports/zonerange_test.go), not here.
 */

const WIDE = { width: 1440, height: 900 }
const NARROW = { width: 1280, height: 900 }
const ZONE = 'America/Chicago'
// The report filters on `created`, not `day`: a day no other spec uses keeps these
// sales out of every other report.
const SALE_DAY = '2047-07-07'

interface Sale {
    id: string
    created: string
}

/** Adds `count` sales through the stub's screen and reads back their ids and `created`. */
async function seedSales(page: Page, count: number): Promise<Sale[]> {
    await openReportStub(page)
    const team = uniqueName('Zone Team')
    await addTeam(page, team, 'east')
    const customers = Array.from({ length: count }, (_, i) => uniqueName(`Zone ${i}`))
    for (const customer of customers) {
        await addSale(page, { team, customer, amount: 100, cost: 0, day: SALE_DAY })
    }
    // Read-only: the server sets `created`, which the cells must show in the zone.
    const pb = await superuserClient()
    return Promise.all(
        customers.map(async customer => {
            const sale = await pb
                .collection('salesstub_sales')
                .getFirstListItem(pb.filter('customer = {:customer}', { customer }))
            return { id: sale.id, created: String(sale.created) }
        })
    )
}

/**
 * The text a reader in Chicago sees for an instant. Node's ICU writes a narrow
 * no-break space before AM/PM; the app writes a plain space.
 */
function chicagoText(created: string): string {
    return new Date(created.replace(' ', 'T'))
        .toLocaleString('en-US', {
            timeZone: ZONE,
            month: 'short',
            day: 'numeric',
            year: 'numeric',
            hour: 'numeric',
            minute: '2-digit',
        })
        .replace(/\s+/g, ' ')
}

/** A detail row's label: its record link, named by its id. */
function rowLabel(page: Page, sale: Sale) {
    return page.getByTestId(`report-row-d:${sale.id}`)
}

function markedRows(page: Page) {
    return page.locator('[data-testid^="report-row-d:"][aria-selected="true"]')
}

function param(page: Page, key: string): string | null {
    return new URL(page.url()).searchParams.get(key)
}

test.describe('reports: zone times and record panel', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')
    test.use({ timezoneId: ZONE, locale: 'en-US' })

    test("date-time cells and the zone line are in the reader's time zone", async ({ page }) => {
        await login(page)
        const sales = await seedSales(page, 2)
        await page.setViewportSize(WIDE)
        await openDrilled(page, ZONE_TITLE)
        for (const sale of sales) {
            await expect(rowLabel(page, sale)).toHaveText(chicagoText(sale.created))
        }
        await expect(page.getByTestId('report-document-zone')).toHaveCount(1)
        await expect(inStatement(page, 'report-document-zone')).toHaveText('Times in Central Time')
    })

    test('on a wide window a row opens its record beside the report', async ({ page }) => {
        await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
        await login(page)
        const [a, b] = await seedSales(page, 2)
        await page.setViewportSize(WIDE)
        await openDrilled(page, ZONE_TITLE)
        const panel = recordPanel(page)
        const title = panel.getByTestId('report-record-panel-title')

        // The press opens the panel beside the report, named by the package's panel.
        await rowLabel(page, a).click()
        await expect(panel).toBeVisible()
        await expect.poll(() => panel.evaluate(el => el.getBoundingClientRect().width)).toBe(480)
        await expect(title).toHaveText(`Stub record ${a.id}`)
        await expect(panel.getByTestId('stub-record-panel-body')).toHaveText(a.id)
        expect(param(page, 'dr')).toBe(`salesstub/panel/${a.id}`)
        await expect(rowLabel(page, a)).toHaveAttribute('aria-selected', 'true')
        await expect(markedRows(page)).toHaveCount(1)

        // Another row replaces the record; the mark moves with it.
        await rowLabel(page, b).click()
        await expect(title).toHaveText(`Stub record ${b.id}`)
        await expect(rowLabel(page, b)).toHaveAttribute('aria-selected', 'true')
        await expect(markedRows(page)).toHaveCount(1)

        // ✕ closes it, and the focus goes back to the row's label.
        await panel.getByTestId('report-record-panel-close').click()
        await expect(panel).toHaveCount(0)
        expect(param(page, 'dr')).toBeNull()
        await expect(rowLabel(page, b)).toBeFocused()
        await expect(markedRows(page)).toHaveCount(0)

        // Escape closes it too.
        await rowLabel(page, b).click()
        await expect(panel).toBeVisible()
        await page.keyboard.press('Escape')
        await expect(panel).toHaveCount(0)
        await expect(rowLabel(page, b)).toBeFocused()

        // A reload keeps the panel (it is in the URL); no row is marked after it (Q1).
        await rowLabel(page, b).click()
        await expect(title).toHaveText(`Stub record ${b.id}`)
        await page.reload()
        await expect(panel).toBeVisible()
        await expect(title).toHaveText(`Stub record ${b.id}`)
        await expect(rowLabel(page, b)).toBeVisible()
        await expect(markedRows(page)).toHaveCount(0)

        // A copied link never holds the panel.
        await pressToolbar(page, 'report-copy-link', 'Copy link')
        const link = new URL(await page.evaluate(() => navigator.clipboard.readText()))
        expect(link.searchParams.has('dr')).toBe(false)

        // Customize closes the panel: one side surface at a time (B7).
        await pressToolbar(page, 'report-customize-open', 'Customize')
        await expect(panel).toHaveCount(0)
        await expect.poll(() => param(page, 'dr')).toBeNull()

        // A row press opens the record again (and closes Customize).
        await rowLabel(page, b).click()
        await expect(title).toHaveText(`Stub record ${b.id}`)

        // Open as full screen pushes the record's own screen.
        const screen = page.getByTestId('stub-panel-screen')
        await panel.getByTestId('report-record-panel-full').click()
        await expect(screen).toBeVisible()
        await expect
            .poll(() => new URL(page.url()).pathname)
            .toMatch(new RegExp(`/salesstub/panel/${b.id}$`))
        await expect(screen.getByTestId('stub-record-panel-body')).toHaveText(b.id)
    })

    test('below 1360 px a row opens its record as a screen', async ({ page }) => {
        await login(page)
        const [sale] = await seedSales(page, 1)
        await page.setViewportSize(NARROW)
        await openDrilled(page, ZONE_TITLE)
        await rowLabel(page, sale).click()
        const screen = page.getByTestId('stub-panel-screen')
        await expect(screen).toBeVisible()
        await expect
            .poll(() => new URL(page.url()).pathname)
            .toMatch(new RegExp(`/salesstub/panel/${sale.id}$`))
        // The push leaves the report route, so this reads the pushed screen's URL, not the
        // report's. The recordPanel count below is what proves no panel opened on the report.
        await expect.poll(() => param(page, 'dr')).toBeNull()
        await expect(recordPanel(page)).toHaveCount(0)
    })
})
