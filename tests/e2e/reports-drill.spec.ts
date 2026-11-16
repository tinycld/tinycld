import type { Browser, Locator, Page } from '@playwright/test'
import { inStatement, skipWithoutReportsStub, waitForTableFit } from './helpers'
import { expect, test } from './member-fixtures'
import {
    cellOf,
    DRILL_TARGET_TITLE,
    DRILLED_PIVOT_TITLE,
    DRILLED_TITLE,
    drillPanel,
    hubRow,
    loginAsShippedOwner,
    openDrilled,
    openReportStub,
    openReportsHub,
    pressToolbar,
    rowOf,
    seedStub,
    setPeriod,
    uniqueName,
} from './reports-helpers'

/**
 * Report drills (D1), open rows (D2), the back link to the source (D3) and the
 * drill panel (D4) on the stub's "Stub drilled sales" and "Stub drilled by month",
 * whose Amount opens "Stub sales by team" (tests/scripts/reports-stub/server/
 * register.go.txt). The target is unpublished, so the shipped owner opens it and a
 * member's figures stay plain. Each test seeds its own East teams in its own year,
 * because every test that signs in as the shipped owner reads the same sales.
 */

const WIDE = { width: 1440, height: 900 }
const NARROW = { width: 1280, height: 900 }
const PHONE = { width: 390, height: 844 }

const BAD_REQUEST = {
    status: 400,
    contentType: 'application/json',
    body: JSON.stringify({ status: 400, message: 'Bad request.', data: {} }),
}

interface SeedTeam {
    name: string
    sales: { customer: string; amount: number; day: string }[]
}

async function seed(page: Page, teams: SeedTeam[]) {
    await seedStub(page, {
        teams: teams.map(team => ({ name: team.name, region: 'east' as const })),
        sales: teams.flatMap(team =>
            team.sales.map(sale => ({ team: team.name, cost: 0, ...sale }))
        ),
    })
}

/** The report id of a report screen's URL (`…/reports/<id>`). */
function reportIdOf(url: string): string {
    const id = new URL(url).pathname.split('/').pop()
    if (!id) throw new Error(`no report id in ${url}`)
    return id
}

function param(page: Page, key: string): string | null {
    return new URL(page.url()).searchParams.get(key)
}

// react-native-web draws a link's text as its first child.
function decorationOf(link: Locator): Promise<string> {
    return link.evaluate(el => getComputedStyle(el.firstElementChild ?? el).textDecorationLine)
}

/** Presses Tab until `target` has the focus. */
async function tabTo(page: Page, target: Locator) {
    for (let i = 0; i < 60; i += 1) {
        if (await target.evaluate(el => el === document.activeElement)) return
        await page.keyboard.press('Tab')
    }
    throw new Error('Tab never reached the figure')
}

/** The bodies of every run of a report other than `sourceId`, from now on. */
function watchRuns(page: Page, sourceId: string): string[] {
    const bodies: string[] = []
    page.on('request', request => {
        if (!request.url().includes('/api/reports/run')) return
        const body = request.postDataJSON() as { reportId?: string } | null
        if (body && body.reportId !== sourceId) bodies.push(JSON.stringify(body))
    })
    return bodies
}

/** The detail rows' labels in the panel, in screen order. */
function panelDetails(panel: Locator): Promise<string[]> {
    return panel
        .getByTestId('report-table')
        .locator('[data-testid^="report-row-d:"]')
        .allInnerTexts()
}

// React does not touch an attribute it does not render, so the mark lasts as long
// as the element: a remount drops it.
async function markTable(panel: Locator) {
    await panel.getByTestId('report-table').evaluate(el => el.setAttribute('data-e2e-kept', '1'))
}

/**
 * A link pasted into a new tab has no history. With the source's title read, Back
 * names it and opens the source with its defaults; while that read is held, and
 * after it fails, Back reads "Reports" and still returns to the source. (A source the
 * reader cannot view sends Back to the hub: the member case of the access test.)
 */
async function expectFreshBack(browser: Browser, targetUrl: string, sourceId: string) {
    const context = await browser.newContext({ viewport: NARROW })
    const q = await context.newPage()
    try {
        await loginAsShippedOwner(q)
        // Opening a pasted link is a full load by nature.
        await q.goto(targetUrl)
        await expect(q.getByTestId('report-back')).toHaveText(DRILLED_TITLE)
        await q.getByTestId('report-back').click()
        await expect(q.getByTestId('report-title')).toHaveText(DRILLED_TITLE)
        expect(new URL(q.url()).searchParams.get('p-period')).toBeNull()

        const read = { mode: 'hold' as 'hold' | 'fail' | 'pass' }
        let release = () => {}
        const held = new Promise<void>(resolve => {
            release = resolve
        })
        let answered = () => {}
        const failed = new Promise<void>(resolve => {
            answered = resolve
        })
        await q.route('**/api/collections/reports/records**', async route => {
            const isSource = decodeURIComponent(route.request().url()).includes(sourceId)
            if (read.mode === 'pass' || !isSource) return route.continue()
            if (read.mode === 'hold') await held
            await route.fulfill(BAD_REQUEST)
            answered()
        })
        // Opening a pasted link is a full load by nature.
        await q.goto(targetUrl)
        await expect(q.getByTestId('report-title')).toHaveText(DRILL_TARGET_TITLE)
        await expect(q.getByTestId('report-back')).toHaveText('Reports')
        read.mode = 'fail'
        release()
        await failed
        await expect(q.getByTestId('report-back')).toHaveText('Reports')
        read.mode = 'pass'
        await q.getByTestId('report-back').click()
        await expect(q.getByTestId('report-title')).toHaveText(DRILLED_TITLE)
    } finally {
        await context.close()
    }
}

test.describe('reports: drill', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a figure opens its report as a screen, and the back link names the source', async ({
        page,
        browser,
    }) => {
        await loginAsShippedOwner(page)
        const ann = uniqueName('Drill Ann')
        const bo = uniqueName('Drill Bo')
        await seed(page, [
            {
                name: ann,
                sales: [
                    { customer: 'Cara', amount: 300, day: '2031-03-12' },
                    { customer: 'Dan', amount: 100, day: '2031-04-02' },
                ],
            },
            { name: bo, sales: [{ customer: 'Eve', amount: 0, day: '2031-05-01' }] },
        ])
        await page.setViewportSize(NARROW)
        await openDrilled(page, DRILLED_TITLE)
        await setPeriod(page, '2031-03-10', '2031-05-20')
        const sourceId = reportIdOf(page.url())

        // At rest a drilled figure reads as a figure; the pointer underlines it.
        const figure = await cellOf(page, 'amount', ann)
        await expect(
            page.getByRole('link', { name: `Show transactions: ${ann}, Amount, 4.00` })
        ).toBeVisible()
        await page.mouse.move(0, 0)
        await expect.poll(() => decorationOf(figure)).toBe('none')
        await figure.hover()
        await expect.poll(() => decorationOf(figure)).toBe('underline')
        expect(await figure.evaluate(el => getComputedStyle(el).cursor)).toBe('pointer')
        // A figure of 0.00 still opens its rows.
        await expect(
            page.getByRole('link', { name: `Show transactions: ${bo}, Amount, 0.00` })
        ).toBeVisible()

        await figure.click()
        await expect(page.getByTestId('report-title')).toHaveText(DRILL_TARGET_TITLE)
        await expect(page.getByTestId('report-drill-panel')).toHaveCount(0)
        expect(param(page, 'from')).toBe(sourceId)
        expect(param(page, 'p-period')).toBe('2031-03-10..2031-05-20')
        await expect(page.getByTestId('report-param-teams')).toHaveText(`Teams: ${ann}`)
        await expect(rowOf(page, 'Cara')).toBeVisible()
        await expect(rowOf(page, 'Dan')).toBeVisible()
        await expect(rowOf(page, 'Eve')).toHaveCount(0)
        const targetUrl = page.url()

        await expect(page.getByTestId('report-back')).toHaveText(DRILLED_TITLE)
        await page.getByTestId('report-back').click()
        await expect(page.getByTestId('report-title')).toHaveText(DRILLED_TITLE)
        await expect.poll(() => param(page, 'p-period')).toBe('2031-03-10..2031-05-20')

        // A month cell opens the target with that month, cut to the period.
        await openDrilled(page, DRILLED_PIVOT_TITLE)
        await setPeriod(page, '2031-03-10', '2031-05-20')
        await (await cellOf(page, 'amount.p0', ann)).click()
        await expect(page.getByTestId('report-title')).toHaveText(DRILL_TARGET_TITLE)
        expect(param(page, 'p-period')).toBe('2031-03-10..2031-03-31')
        await expect(page.getByTestId('report-param-period')).toHaveText(
            'Period: Mar 10 – Mar 31, 2031'
        )
        await expect(rowOf(page, 'Cara')).toBeVisible()
        await expect(rowOf(page, 'Dan')).toHaveCount(0)

        await expectFreshBack(browser, targetUrl, sourceId)
    })

    test('on a phone a figure opens its report as a screen', async ({ page }) => {
        await loginAsShippedOwner(page)
        const team = uniqueName('Drill Phone')
        await seed(page, [
            { name: team, sales: [{ customer: 'Gil', amount: 500, day: '2032-02-02' }] },
        ])
        await page.setViewportSize(NARROW)
        await openDrilled(page, DRILLED_TITLE)
        await setPeriod(page, '2032-01-01', '2032-12-31')
        await page.setViewportSize(PHONE)
        await waitForTableFit(page)
        await (await cellOf(page, 'amount', team)).click()
        await expect(page.getByTestId('report-title')).toHaveText(DRILL_TARGET_TITLE)
        await expect(page.getByTestId('report-drill-panel')).toHaveCount(0)
        await expect(page.getByTestId('report-back')).toHaveText(DRILLED_TITLE)
        await expect(rowOf(page, 'Gil')).toBeVisible()
    })

    test('on a wide window a figure opens its report in a panel beside the source', async ({
        page,
    }) => {
        await loginAsShippedOwner(page)
        const ann = uniqueName('Drill Panel A')
        const bo = uniqueName('Drill Panel B')
        await seed(page, [
            {
                name: ann,
                sales: [
                    { customer: 'Hal', amount: 300, day: '2033-03-01' },
                    { customer: 'Ida', amount: 100, day: '2033-03-02' },
                ],
            },
            { name: bo, sales: [{ customer: 'Jo', amount: 700, day: '2033-03-03' }] },
        ])
        await page.setViewportSize(WIDE)
        await openDrilled(page, DRILLED_TITLE)
        await setPeriod(page, '2033-01-01', '2033-12-31')
        const sourceId = reportIdOf(page.url())
        const sourcePath = new URL(page.url()).pathname
        const sourceChip = await inStatement(page, 'report-param-period').textContent()
        const figureA = await cellOf(page, 'amount', ann)
        // ⌘-click and middle-click open the full report (platform item 2).
        await expect(figureA).toHaveAttribute(
            'href',
            new RegExp(`/reports/[^?]+\\?.*from=${sourceId}`)
        )
        await figureA.click()

        const panel = drillPanel(page)
        await expect(panel).toBeVisible()
        await expect.poll(async () => Math.round((await panel.boundingBox())?.width ?? 0)).toBe(480)
        await expect(inStatement(page, 'report-title')).toHaveText(DRILLED_TITLE)
        expect(new URL(page.url()).pathname).toBe(sourcePath)
        expect(param(page, 'd')).toBeTruthy()
        await expect(panel.getByTestId('report-drill-panel-title')).toHaveText(DRILL_TARGET_TITLE)
        await expect(panel.getByTestId('report-drill-panel-title')).toBeFocused()
        await expect(figureA).toHaveAttribute('aria-selected', 'true')
        for (const id of [
            'report-customize-open',
            'report-share-open',
            'report-save-as',
            'report-copy-link',
            'report-edit-open',
            'toolbar-more-button',
        ]) {
            await expect(panel.getByTestId(id)).toHaveCount(0)
        }
        // The drill panel has no Export; the report beside it has its one.
        await expect(panel.getByTestId('report-export-open')).toHaveCount(0)
        await expect(page.getByTestId('report-export-open')).toHaveCount(1)
        await expect(panel.getByTestId('report-param-teams')).toHaveText(`Teams: ${ann}`)

        // The panel's Sort chip sorts the panel's rows only.
        await expect.poll(() => panelDetails(panel)).toEqual(['Hal', 'Ida'])
        await panel.getByTestId('report-sort-menu').click()
        await page.getByTestId('report-sort-choice-amount-asc').click()
        await expect.poll(() => param(page, 'd.s')).toBe('amount:asc')
        expect(param(page, 's')).toBeNull()
        await expect.poll(() => panelDetails(panel)).toEqual(['Ida', 'Hal'])
        // A sort is not a chip: the panel still shows the figure's params, so it stays marked.
        await expect(figureA).toHaveAttribute('aria-selected', 'true')

        // The URL holds the panel, so a reload keeps it.
        await page.reload()
        await expect(panel).toBeVisible()
        await expect(panel.getByTestId('report-param-teams')).toHaveText(`Teams: ${ann}`)
        await expect.poll(() => panelDetails(panel)).toEqual(['Ida', 'Hal'])

        // A second figure replaces the panel's report: one panel, its chips, its mark.
        const figureB = await cellOf(page, 'amount', bo)
        await figureB.click()
        await expect(panel).toHaveCount(1)
        await expect(panel.getByTestId('report-param-teams')).toHaveText(`Teams: ${bo}`)
        await expect(figureB).toHaveAttribute('aria-selected', 'true')
        await expect(await cellOf(page, 'amount', ann)).not.toHaveAttribute('aria-selected', 'true')
        expect(param(page, 'd.s')).toBeNull()
        // A figure press with the panel already open moves the focus to the panel title.
        await expect(panel.getByTestId('report-drill-panel-title')).toBeFocused()

        // The panel's Period chip opens its own popover; the source's chip stays closed.
        await panel.getByTestId('report-param-period').click()
        await expect(page.getByTestId('report-period-popover')).toHaveCount(1)
        await expect(inStatement(page, 'report-param-period')).not.toHaveAttribute(
            'aria-expanded',
            'true'
        )
        await page.keyboard.press('Escape')
        await expect(page.getByTestId('report-period-popover')).toHaveCount(0)

        // The panel's Period runs the panel's report only, and no figure is marked after it.
        await setPeriod(page, '2033-03-03', '2033-03-31', panel)
        await expect(inStatement(page, 'report-param-period')).toHaveText(sourceChip ?? '')
        expect(param(page, 'p-period')).toBe('2033-01-01..2033-12-31')
        await expect(
            page.locator('[data-testid^="report-cell-"][aria-selected="true"]')
        ).toHaveCount(0)
        // A change in the panel's own controls keeps the focus on the control the reader
        // used (the Period chip, back from its popover); only a figure press moves it to the title.
        await expect(panel.getByTestId('report-param-period')).toBeFocused()
        await expect(panel.getByTestId('report-drill-panel-title')).not.toBeFocused()

        // Open as full report: the panel's chips, and a back link that names the source.
        await panel.getByTestId('report-drill-open-full').click()
        await expect(page.getByTestId('report-title')).toHaveText(DRILL_TARGET_TITLE)
        expect(param(page, 'p-period')).toBe('2033-03-03..2033-03-31')
        expect(param(page, 'from')).toBe(sourceId)
        await expect(page.getByTestId('report-param-teams')).toHaveText(`Teams: ${bo}`)
        await expect(page.getByTestId('report-back')).toHaveText(DRILLED_TITLE)
        await page.getByTestId('report-back').click()
        await expect(panel).toBeVisible()
        await expect(panel.getByTestId('report-param-teams')).toHaveText(`Teams: ${bo}`)

        // Opening and replacing the panel added no history entry: Back leaves the report.
        await (await cellOf(page, 'amount', ann)).click()
        await expect(panel.getByTestId('report-param-teams')).toHaveText(`Teams: ${ann}`)
        await page.goBack()
        await expect.poll(() => new URL(page.url()).pathname).not.toBe(sourcePath)
    })

    test('the panel closes with ✕, Escape and Customize, and focus goes back to its figure', async ({
        page,
    }) => {
        await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
        await loginAsShippedOwner(page)
        const team = uniqueName('Drill Close')
        await seed(page, [
            { name: team, sales: [{ customer: 'Kim', amount: 200, day: '2034-06-01' }] },
        ])
        await page.setViewportSize(WIDE)
        await openDrilled(page, DRILLED_TITLE)
        await setPeriod(page, '2034-01-01', '2034-12-31')
        const figure = await cellOf(page, 'amount', team)
        const panel = drillPanel(page)

        // A modified click is the browser's: a new tab with the full report, no panel.
        const [tab] = await Promise.all([
            page.context().waitForEvent('page'),
            figure.click({ modifiers: ['ControlOrMeta'] }),
        ])
        await tab.waitForURL(/from=/)
        await tab.close()
        await expect(panel).toHaveCount(0)

        await figure.click()
        await expect(panel).toBeVisible()
        await panel.getByTestId('report-drill-close').click()
        await expect(panel).toHaveCount(0)
        await expect(figure).toBeFocused()
        expect(param(page, 'd')).toBeNull()

        // With a chip's popover open in the panel, Escape closes the popover only.
        await figure.click()
        await expect(panel).toBeVisible()
        await panel.getByTestId('report-param-period').click()
        await expect(page.getByTestId('report-period-popover')).toBeVisible()
        await page.keyboard.press('Escape')
        await expect(page.getByTestId('report-period-popover')).toHaveCount(0)
        await expect(panel).toBeVisible()
        await page.keyboard.press('Escape')
        await expect(panel).toHaveCount(0)
        await expect(figure).toBeFocused()

        // Customize and the panel are one side surface: each closes the other.
        await figure.click()
        await expect(panel).toBeVisible()
        await pressToolbar(page, 'report-customize-open', 'Customize')
        await expect(page.getByTestId('report-customize')).toBeVisible()
        await expect(panel).toHaveCount(0)
        expect(param(page, 'd')).toBeNull()
        await figure.click()
        await expect(panel).toBeVisible()
        await expect(page.getByTestId('report-customize')).toHaveCount(0)

        // A copied link never holds the panel.
        await pressToolbar(page, 'report-copy-link', 'Copy link')
        const link = new URL(await page.evaluate(() => navigator.clipboard.readText()))
        const keys = [...link.searchParams.keys()]
        expect(keys.filter(key => key === 'd' || key.startsWith('d.') || key === 'from')).toEqual(
            []
        )
        expect(link.searchParams.get('p-period')).toBe('2034-01-01..2034-12-31')
    })

    test("crossing 1360 px keeps the panel's report mounted and does not run it again", async ({
        page,
    }) => {
        await loginAsShippedOwner(page)
        const team = uniqueName('Drill Cross')
        await seed(page, [
            { name: team, sales: [{ customer: 'Lou', amount: 900, day: '2035-02-02' }] },
        ])
        await page.setViewportSize(WIDE)
        await openDrilled(page, DRILLED_TITLE)
        await setPeriod(page, '2035-01-01', '2035-12-31')
        const runs = watchRuns(page, reportIdOf(page.url()))
        await (await cellOf(page, 'amount', team)).click()
        const panel = drillPanel(page)
        await expect(rowOf(page, 'Lou', panel)).toBeVisible()
        await markTable(panel)
        const screen = inStatement(page, 'report-screen')

        // Narrower: the panel's report covers the source as a page (R14: the mark
        // proves no remount; the run count proves no rerun).
        const atNarrow = runs.length
        await page.setViewportSize(NARROW)
        await expect
            .poll(async () => {
                const [p, s] = [await panel.boundingBox(), await screen.boundingBox()]
                return p && s
                    ? Math.round(p.width) === Math.round(s.width) &&
                          Math.round(p.x) === Math.round(s.x)
                    : false
            })
            .toBe(true)
        await expect(panel.getByTestId('report-back')).toHaveText(DRILLED_TITLE)
        await expect(panel.getByTestId('report-drill-close')).toHaveCount(0)
        await expect(panel.getByTestId('report-drill-panel-title')).toHaveText(DRILL_TARGET_TITLE)
        await expect(panel.getByTestId('report-drill-open-full')).toBeVisible()
        await expect(panel.getByTestId('report-loading')).toHaveCount(0)
        await expect(panel.locator('[data-e2e-kept="1"]')).toHaveCount(1)
        await expect(rowOf(page, 'Lou', panel)).toBeVisible()
        // The report under the page is out of the accessibility tree and the tab order.
        const statementFigure = page.getByRole('link', { name: `Show transactions: ${team},` })
        await expect(statementFigure).toHaveCount(0)
        await expect(screen.locator('[inert]')).toHaveCount(1)
        expect(atNarrow).toBeGreaterThan(0)
        expect(runs.length).toBe(atNarrow)

        // Wider again: a 480 px panel, the same table and rows.
        const atWide = runs.length
        await page.setViewportSize(WIDE)
        await expect.poll(async () => Math.round((await panel.boundingBox())?.width ?? 0)).toBe(480)
        await expect(panel.getByTestId('report-drill-close')).toBeVisible()
        await expect(panel.getByTestId('report-loading')).toHaveCount(0)
        await expect(panel.locator('[data-e2e-kept="1"]')).toHaveCount(1)
        await expect(rowOf(page, 'Lou', panel)).toBeVisible()
        // As a panel, the report beside it is reachable again.
        await expect(statementFigure).toHaveCount(1)
        expect(atWide).toBe(atNarrow)
        expect(runs.length).toBe(atWide)

        // The page's back link closes it; wider again shows no panel.
        await page.setViewportSize(NARROW)
        await panel.getByTestId('report-back').click()
        await expect(panel).toHaveCount(0)
        await expect(inStatement(page, 'report-title')).toHaveText(DRILLED_TITLE)
        await expect(statementFigure).toHaveCount(1)
        await expect(screen.locator('[inert]')).toHaveCount(0)
        await page.setViewportSize(WIDE)
        await expect(panel).toHaveCount(0)
    })

    test('rows open their record, the keyboard opens a figure, and a reader without the target sees plain figures', async ({
        page,
        freshMember,
    }) => {
        await loginAsShippedOwner(page)
        const team = uniqueName('Drill Rows')
        await seed(page, [
            {
                name: team,
                sales: [
                    { customer: 'Max', amount: 400, day: '2036-04-04' },
                    { customer: 'Ned', amount: 600, day: '2036-04-05' },
                ],
            },
        ])
        await page.setViewportSize(NARROW)
        await openDrilled(page, DRILLED_TITLE)
        await setPeriod(page, '2036-01-01', '2036-12-31')
        const sourcePath = new URL(page.url()).pathname
        const sourceSearch = new URL(page.url()).searchParams.toString()

        // Tab reaches a drilled figure, and Enter opens it.
        const figure = await cellOf(page, 'amount', team)
        await inStatement(page, 'report-param-period').focus()
        await tabTo(page, figure)
        await page.keyboard.press('Enter')
        await expect(page.getByTestId('report-title')).toHaveText(DRILL_TARGET_TITLE)

        // A line opens its record: its label is the link, and any cell of it opens it too.
        // Back from the record shows the report again at its own URL: the reports tab
        // stays mounted while hidden, so its title alone would pass behind the record.
        const targetUrl = page.url()
        const targetId = reportIdOf(targetUrl)
        const max = rowOf(page, 'Max')
        await expect(page.getByRole('link', { name: 'Open: Max' })).toBeVisible()
        await max.hover()
        await expect.poll(() => decorationOf(max)).toBe('underline')
        await max.click()
        await expect(page.getByTestId('stub-sale-customer')).toHaveText('Max')
        await page.getByTestId('stub-sale-back').click()
        await expect(page).toHaveURL(targetUrl)
        await expect(page.getByTestId('report-title')).toBeVisible()
        await expect(page.getByTestId('report-title')).toHaveText(DRILL_TARGET_TITLE)
        await (await cellOf(page, 'day', 'Ned')).click()
        await expect(page.getByTestId('stub-sale-customer')).toHaveText('Ned')
        await page.getByTestId('stub-sale-back').click()
        await expect(page).toHaveURL(targetUrl)
        await expect(page.getByTestId('report-title')).toBeVisible()
        await expect(page.getByTestId('report-title')).toHaveText(DRILL_TARGET_TITLE)

        // A line in the panel opens its record; Back returns to the source with the panel.
        await page.getByTestId('report-back').click()
        await expect.poll(() => new URL(page.url()).pathname).toBe(sourcePath)
        // The source's own params (its period) survive the round trip.
        await expect.poll(() => new URL(page.url()).searchParams.toString()).toBe(sourceSearch)
        await expect(page.getByTestId('report-title')).toBeVisible()
        await expect(page.getByTestId('report-title')).toHaveText(DRILLED_TITLE)
        await page.setViewportSize(WIDE)
        await (await cellOf(page, 'amount', team)).click()
        const panel = drillPanel(page)
        await expect(panel).toBeVisible()
        const panelUrl = page.url()
        await rowOf(page, 'Max', panel).click()
        await expect(page.getByTestId('stub-sale-customer')).toHaveText('Max')
        await page.getByTestId('stub-sale-back').click()
        await expect(page).toHaveURL(panelUrl)
        await expect(panel).toBeVisible()
        await expect(inStatement(page, 'report-title')).toBeVisible()
        await expect(inStatement(page, 'report-title')).toHaveText(DRILLED_TITLE)

        // In the builder the preview's figures are plain, and Columns names the target.
        await panel.getByTestId('report-drill-close').click()
        await pressToolbar(page, 'report-edit-open', 'Edit report')
        await expect(page.getByTestId('report-builder')).toBeVisible()
        await expect(page.getByTestId('report-builder-column-2-drill')).toHaveText(
            `Figures open ${DRILL_TARGET_TITLE}`
        )
        const preview = await cellOf(page, 'amount', team)
        await expect(page.getByRole('link', { name: /^Show transactions:/ })).toHaveCount(0)
        const editPath = new URL(page.url()).pathname
        await preview.click()
        await expect(drillPanel(page)).toHaveCount(0)
        expect(new URL(page.url()).pathname).toBe(editPath)

        // The target's Details section says its rows open their record.
        await openReportsHub(page)
        await page.getByTestId('reports-hub-tab-mine').click()
        await hubRow(page, DRILL_TARGET_TITLE).click()
        await pressToolbar(page, 'report-edit-open', 'Edit report')
        await expect(page.getByTestId('report-builder-detail-open')).toHaveText(
            'Rows open their record'
        )

        // A member may read the published source but not its target: plain figures.
        const p = freshMember.page
        const own = uniqueName('Drill Member')
        await seed(p, [{ name: own, sales: [{ customer: 'Oz', amount: 800, day: '2036-07-07' }] }])
        await p.setViewportSize(WIDE)
        await openDrilled(p, DRILLED_TITLE)
        await setPeriod(p, '2036-01-01', '2036-12-31')
        const plain = await cellOf(p, 'amount', own)
        await expect(plain).toHaveText('8.00')
        await expect(p.getByRole('link', { name: /^Show transactions:/ })).toHaveCount(0)
        const path = new URL(p.url()).pathname
        await plain.click()
        await expect(drillPanel(p)).toHaveCount(0)
        expect(new URL(p.url()).pathname).toBe(path)

        // A back link to a report the reader cannot view reads "Reports" and opens the hub.
        // Opening a pasted link is a full load by nature.
        const pasted = new URL(p.url())
        pasted.searchParams.set('from', targetId)
        await p.goto(pasted.toString())
        await expect(p.getByTestId('report-title')).toHaveText(DRILLED_TITLE)
        await expect(rowOf(p, own)).toBeVisible()
        await expect(p.getByTestId('report-back')).toHaveText('Reports')
        await p.getByTestId('report-back').click()
        await expect(p.getByTestId('reports-hub')).toBeVisible()
    })

    test("a record opened from a report keeps the package's own screens, and Back returns to the report", async ({
        page,
    }) => {
        await loginAsShippedOwner(page)
        const team = uniqueName('Drill Keep')
        await seed(page, [
            { name: team, sales: [{ customer: 'Pia', amount: 700, day: '2037-03-03' }] },
        ])
        // The package tab holds a form with typed input that is not saved.
        const draft = page.getByTestId('salesstub-sale-customer')
        await draft.fill('Unsaved draft')
        await page.setViewportSize(NARROW)
        await openDrilled(page, DRILLED_TITLE)
        await setPeriod(page, '2037-01-01', '2037-12-31')
        await (await cellOf(page, 'amount', team)).click()
        await expect(page.getByTestId('report-title')).toHaveText(DRILL_TARGET_TITLE)
        const targetUrl = page.url()

        await rowOf(page, 'Pia').click()
        await expect(page.getByTestId('stub-sale-customer')).toHaveText('Pia')
        await page.getByTestId('stub-sale-back').click()
        await expect(page).toHaveURL(targetUrl)
        await expect(page.getByTestId('report-title')).toBeVisible()
        await expect(page.getByTestId('report-title')).toHaveText(DRILL_TARGET_TITLE)

        await openReportStub(page)
        await expect(draft).toHaveValue('Unsaved draft')
    })
})
