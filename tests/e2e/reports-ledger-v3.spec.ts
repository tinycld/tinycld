import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub } from './helpers'
import {
    addSale,
    addSales,
    addTeam,
    cellOf,
    chooseOption,
    chooseParamOption,
    hubRow,
    listScroller,
    loginAsShippedOwner,
    openReportStub,
    openReportsHub,
    pressToolbar,
    rowOf,
    SALES_SOURCE,
    saveNewReport,
    scrollRowTo,
    startTeamSales,
    uniqueName,
} from './reports-helpers'

/**
 * Report engine v3 (spec version 3). Most tests read the stub's
 * shipped "Stub team balance" (salesstub.team_balance, tests/scripts/reports-stub/
 * server/register.go.txt): a team's sales before the period open each team in an
 * opening row ("Before the period"), a running balance continues from it, the
 * team's Total row shows a rollup (opening + the period's sales), and "Same day"
 * counts the team's sales on the row's own day. A shipped report belongs to the
 * seed's admin and stub sales are read only by their owner, so those tests sign in
 * as that user, add their own team and sales, and show only their team through the
 * Teams filter. The period is this fiscal year (the stub's sources start it in
 * January); each team has one sale in December of last year. The last test builds
 * the same parts in the builder as an invited user.
 */

const TEAM_BALANCE_TITLE = 'Stub team balance'
const YEAR = new Date().getFullYear()
const PHONE = { width: 390, height: 844 }
const OPENING = 'Before the period'

async function openTeamBalance(page: Page) {
    await openReportsHub(page)
    await page.getByTestId('reports-hub-tab-mine').click()
    await hubRow(page, TEAM_BALANCE_TITLE).click()
    await expect(page.getByTestId('report-title')).toHaveText(TEAM_BALANCE_TITLE)
}

/**
 * A team with a 10.00 sale last December and four sales this January: Ann (2.00)
 * and Bea (3.00) on one day, Cy (4.00), Dee (1.00). Opening 10.00; balances 12.00,
 * 15.00, 19.00, 20.00; the Total's balance 20.00.
 */
async function seedTeam(page: Page, team: string) {
    await openReportStub(page)
    await addTeam(page, team)
    const sales = [
        ['Old', 1000, `${YEAR - 1}-12-15`],
        ['Ann', 200, `${YEAR}-01-05`],
        ['Bea', 300, `${YEAR}-01-05`],
        ['Cy', 400, `${YEAR}-01-07`],
        ['Dee', 100, `${YEAR}-01-08`],
    ] as const
    for (const [customer, amount, day] of sales) {
        await addSale(page, { team, customer, amount, cost: 0, day })
    }
}

async function showTeams(page: Page, search: string, teams: string[]) {
    await chooseParamOption(page, 'teams', { from: 'add-filter', search, labels: teams })
}

/** Signs in, seeds one team and opens the report on it alone. */
async function balanceOfOneTeam(page: Page, prefix: string) {
    await loginAsShippedOwner(page)
    const team = uniqueName(prefix)
    await seedTeam(page, team)
    await openTeamBalance(page)
    await showTeams(page, team, [team])
    await expect(rowOf(page, 'Dee')).toBeVisible()
    return team
}

/**
 * The rows' ids in screen order, read in one snapshot. A row drawn twice is listed
 * twice, so a repeat can be counted (a sticky copy has its own test id and is not
 * listed).
 */
function rowIds(page: Page) {
    return page
        .getByTestId('report-table')
        .evaluate(table =>
            [...table.querySelectorAll('[data-testid^="report-row-"]')].map(
                el => el.getAttribute('data-testid')?.replace('report-row-', '') ?? ''
            )
        )
}

const occurrences = (ids: readonly string[], prefix: string) =>
    ids.filter(id => id.startsWith(prefix)).length

function detailLabels(page: Page) {
    return page.getByTestId('report-table').locator('[data-testid^="report-row-d:"]')
}

/** A run request as the client sent it, before the route below shrinks its page. */
type SentRun = { cursor?: string; limit?: number }

// The client's page sizes: its usual page (RUN_PAGE_SIZE) and the server's most
// (RUN_MAX_PAGE), which a fold run asks for while it looks for the group in view.
const USUAL_PAGE = 200
const MAX_PAGE = 1000

/**
 * Signs in, seeds two teams of six sales each (team A first), and `smallTeams` more
 * teams of one sale each after them, and opens the report on them alone, in pages
 * of 5 detail rows, so 12 sales page across team A's end. A next page (a request
 * with a cursor) waits for `nextPages` when it is given.
 */
async function openPagedTeams(
    page: Page,
    prefix: string,
    smallTeams = 0,
    nextPages?: Promise<void>
) {
    await loginAsShippedOwner(page)
    const stem = uniqueName(prefix)
    const letters = 'ABCDEFGH'.slice(0, 2 + smallTeams)
    const teams = [...letters].map(letter => `${stem} ${letter}`)
    await openReportStub(page)
    for (const [i, team] of teams.entries()) {
        await addTeam(page, team)
        await addSales(page, {
            team,
            customerPrefix: `${stem}${letters[i]}`,
            count: i < 2 ? 6 : 1,
            firstDay: `${YEAR}-02-01`,
            firstAmount: 100 * (i + 1),
            cost: 0,
            quantity: 1,
        })
    }
    const cursors: string[] = []
    const sent: SentRun[] = []
    await page.route('**/api/reports/run', async route => {
        const body = route.request().postDataJSON()
        if (body.options?.cursor && nextPages) await nextPages
        if (body.options?.cursor) cursors.push(body.options.cursor)
        sent.push({ cursor: body.options?.cursor, limit: body.options?.limit })
        // The client's page sizes become 5. A fold run asks for max(200, the detail
        // rows in view), which is 200 here, and then the server's most while it looks
        // for the group in view, so it also gets pages of 5 and joins them until it
        // holds the rows that were in view.
        const asked = body.options?.limit
        const limit = asked === USUAL_PAGE || asked === MAX_PAGE ? 5 : asked
        await route.continue({
            postData: JSON.stringify({ ...body, options: { ...body.options, limit } }),
        })
    })
    // Tall enough that the list draws every row of both teams at once, so one
    // snapshot holds the whole merged list (the list draws only rows near the view).
    await page.setViewportSize({ width: 1280, height: 1600 })
    await openTeamBalance(page)
    // The first run has no Teams filter, so it shows these sales too: no row
    // tells the two runs apart. Once the run with the Teams filter has answered
    // and the view no longer says "Updating…", every row is the new run's.
    await expect(detailLabels(page).first()).toBeVisible()
    const teamsRun = page.waitForResponse(
        response =>
            response.url().includes('/api/reports/run') &&
            response.request().postDataJSON()?.params?.teams !== undefined
    )
    await showTeams(page, stem, teams)
    await teamsRun
    await expect(page.getByTestId('report-updating')).toHaveCount(0)
    const firstSale = rowOf(page, `${stem}A 00`)
    await expect(firstSale).toBeVisible()
    return { stem, teams, cursors, sent, firstSale }
}

/**
 * Signs in, seeds two teams of six sales each (team A first), and `smallTeams` more
 * teams of one sale each after them, and opens the report on them alone, in pages
 * of 5 detail rows, so 12 sales page across team A's end. Then scrolls until the
 * last team's Total row is drawn: every page is merged then.
 */
async function pagedTeams(page: Page, prefix: string, smallTeams = 0) {
    const { firstSale, ...paged } = await openPagedTeams(page, prefix, smallTeams)
    const scroller = await listScroller(firstSale)
    await expect
        .poll(async () => {
            await scroller.evaluate(el => {
                el.scrollTop = el.scrollHeight
            })
            return rowOf(page, `Total ${paged.teams[paged.teams.length - 1]}`).count()
        })
        .toBe(1)
    return paged
}

/**
 * The requests of a fold run: its first page (no cursor) and each page it joined
 * while it looked for the group in view (those ask for the server's most). A next
 * page the list asks for has a cursor and the usual size.
 */
const foldRequests = (sent: readonly SentRun[]) =>
    sent.filter(run => !run.cursor || run.limit === MAX_PAGE)

/**
 * Records, at every change of the page from now on, each of these row ids that is
 * missing from the list, so a list that drops rows and loads them again is caught,
 * not only its end state. Read the record with recordedDrops.
 */
function recordDrops(page: Page, ids: readonly string[]) {
    return page.evaluate(watched => {
        const w = window as unknown as { __foldDrops: string[] }
        w.__foldDrops = []
        const has = (id: string) =>
            document.querySelector(`[data-testid="report-row-${CSS.escape(id)}"]`) !== null
        new MutationObserver(() => {
            for (const id of watched) {
                if (!has(id) && !w.__foldDrops.includes(id)) w.__foldDrops.push(id)
            }
        }).observe(document.body, { childList: true, subtree: true })
    }, ids)
}

const recordedDrops = (page: Page) =>
    page.evaluate(() => (window as unknown as { __foldDrops: string[] }).__foldDrops)

const sortParam = (page: Page) => new URL(page.url()).searchParams.get('s')

async function pickIn(page: Page, trigger: string, key: string) {
    await page.getByTestId(trigger).click()
    await page.getByTestId('report-builder-picker').getByTestId(key).click()
}

test.describe('general-ledger: report engine v3', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('an open group starts with its opening row, runs a balance from it and ends with its rollup', async ({
        page,
    }) => {
        const team = await balanceOfOneTeam(page, 'BalOpen')
        const ids = await rowIds(page)
        const group = ids.findIndex(id => id.startsWith('g:'))
        expect(group).toBeGreaterThanOrEqual(0)
        expect(ids[group + 1]).toMatch(/^opening:/)
        expect(ids.slice(group + 2, group + 6).every(id => id.startsWith('d:'))).toBe(true)
        expect(ids[group + 6]).toMatch(/^s:/)
        await expect(await cellOf(page, 'balance', OPENING)).toHaveText(/^\$?10\.00$/)
        await expect(await cellOf(page, 'balance', 'Ann')).toHaveText('12.00')
        await expect(await cellOf(page, 'balance', 'Bea')).toHaveText('15.00')
        await expect(await cellOf(page, 'balance', 'Cy')).toHaveText('19.00')
        await expect(await cellOf(page, 'balance', 'Dee')).toHaveText('20.00')
        // Count and This row's field: Ann and Bea share a day.
        await expect(await cellOf(page, 'sameDay', 'Ann')).toHaveText('2')
        await expect(await cellOf(page, 'sameDay', 'Cy')).toHaveText('1')
        // The rollup: opening + the period's sales; the amount keeps its sum.
        await expect(await cellOf(page, 'balance', `Total ${team}`)).toHaveText(/^\$?20\.00$/)
        await expect(await cellOf(page, 'amount', `Total ${team}`)).toHaveText(/^\$?10\.00$/)
    })

    test('a grouped report scrolled past its first page keeps its running balance', async ({
        page,
    }) => {
        const { stem, cursors } = await pagedTeams(page, 'BalPage')
        expect(cursors.length).toBeGreaterThan(0)
        // Each team's sixth sale: the running sum of six amounts, whichever page it is on.
        await expect(await cellOf(page, 'balance', `${stem}A 05`)).toHaveText('6.15')
        await expect(await cellOf(page, 'balance', `${stem}B 05`)).toHaveText('12.15')
        // The merged rows: every sale once, each team once, each opening its team,
        // no row repeated, and the report's one Total at the end (the stub's spec
        // has no statement layout, so the report ends in one Total per currency).
        const ids = await rowIds(page)
        expect(occurrences(ids, 'd:')).toBe(12)
        expect(occurrences(ids, 'g:')).toBe(2)
        expect(occurrences(ids, 'opening:')).toBe(2)
        expect(occurrences(ids, 's:')).toBe(2)
        expect(new Set(ids).size).toBe(ids.length)
        for (const [i, id] of ids.entries()) {
            if (id.startsWith('g:')) expect(ids[i + 1]).toMatch(/^opening:/)
        }
        expect(occurrences(ids, 'total')).toBe(1)
        // A report over amounts with a currency keys its Total by currency (total:["USD"]).
        expect(ids[ids.length - 1]).toMatch(/^total/)
    })

    // D8, D12: while a later page is left, the list ends in "Loading more rows…", never
    // in the Total; the Total comes with the last page, and "End of report" ends the list.
    // The amount column keeps its width when the Total lands (the measure held it).
    test('the Total waits for the last page, a loading row stands in its place, and an end row follows it', async ({
        page,
    }) => {
        let release = () => {}
        const nextPages = new Promise<void>(resolve => {
            release = resolve
        })
        const { teams, firstSale } = await openPagedTeams(page, 'BalEnd', 0, nextPages)
        const loading = page.getByTestId('report-loading-more')
        await expect(loading).toContainText('Loading more rows…')
        expect(occurrences(await rowIds(page), 'total')).toBe(0)
        await expect(page.getByTestId('report-end')).toHaveCount(0)
        const heading = page.getByTestId('report-heading-amount')
        const widthOf = async () => (await heading.boundingBox())?.width
        await expect.poll(widthOf).toBeGreaterThan(0)
        const amountWidth = await widthOf()

        release()
        const scroller = await listScroller(firstSale)
        await expect
            .poll(async () => {
                await scroller.evaluate(el => {
                    el.scrollTop = el.scrollHeight
                })
                return occurrences(await rowIds(page), 'total')
            })
            .toBe(1)
        await expect(rowOf(page, `Total ${teams[teams.length - 1]}`)).toHaveCount(1)
        await expect(page.getByTestId('report-end')).toHaveText('End of report')
        await expect(loading).toHaveCount(0)
        const ids = await rowIds(page)
        expect(ids[ids.length - 1]).toMatch(/^total/)
        // The Total (18.30) and the team rollups (12.15) are wider than any detail
        // amount (at most 2.05), so the column would widen if the measure ignored the
        // held rows.
        await expect.poll(widthOf).toBe(amountWidth)
    })

    // F11: a fold keeps the reader's place. Folding a section that came on a later
    // page must not drop the list back to the first page: the folded section's row
    // stays in view, and the rows above it stay in the list, at every moment from the
    // press to the folded run (a list that drops rows and loads them again is caught
    // by the recorder, not only by the end state).
    test('folding a section that came on a later page keeps the reader at that section', async ({
        page,
    }) => {
        const { stem, teams } = await pagedTeams(page, 'BalFold')
        const ids = await rowIds(page)
        const groupB = ids.filter(id => id.startsWith('g:'))[1] ?? ''
        const teamARows = ids.slice(0, ids.indexOf(groupB)).filter(id => id.startsWith('d:'))
        expect(teamARows).toHaveLength(6)
        const groupRow = page.getByTestId(`report-row-${groupB}`)
        await expect(groupRow).toHaveText(new RegExp(teams[1]))
        await expect(groupRow).toHaveAttribute('aria-expanded', 'true')

        // Record, at every change of the page, whether team B's group row or any of
        // team A's rows is missing from the list.
        await recordDrops(page, [groupB, ...teamARows])
        await groupRow.click()

        // The folded run has landed once team B's rows are gone under its folded row.
        await expect(groupRow).toHaveAttribute('aria-expanded', 'false')
        await expect(rowOf(page, `${stem}B 00`)).toHaveCount(0)
        await expect(groupRow).toBeInViewport()
        const after = await rowIds(page)
        expect(after).toEqual(expect.arrayContaining([...teamARows, groupB]))
        expect(
            await recordedDrops(page),
            'rows the list dropped between the fold and its run'
        ).toEqual([])
    })

    // F11: Expand all keeps the reader's place too. With every group closed the list
    // holds no detail rows, so the reader's place is the group row at the top of the
    // view. F7: the run loads no further than that place. After Collapse all the first
    // page holds every group row of the report, so a run that joined pages up to the
    // last group row would load the whole report (team F's here). Scrolled to team B,
    // Expand all joins pages up to team B's group row, which stays in the list and in
    // view at every moment from the press to the opened run (team A's first page of 5
    // rows does not reach it), and stops there. Expand all at the top (one request) is
    // covered by the useReportRun unit test: here the list's own next pages have the
    // same requests as a run that joins pages with the usual size.
    test('Expand all from closed groups keeps the reader at the group in view', async ({
        page,
    }) => {
        const { stem, teams, sent } = await pagedTeams(page, 'BalExpand', 4)
        const foldAll = page.getByTestId('report-fold-all')
        await pressToolbar(page, 'report-fold-all', 'Collapse all')
        await expect(foldAll).toHaveText('Expand all')
        await expect(rowOf(page, `${stem}A 00`)).toHaveCount(0)
        const groups = (await rowIds(page)).filter(id => id.startsWith('g:'))
        expect(groups).toHaveLength(teams.length)
        const groupA = page.getByTestId(`report-row-${groups[0]}`)
        const groupB = groups[1] ?? ''
        const groupRow = page.getByTestId(`report-row-${groupB}`)
        await expect(groupRow).toHaveText(new RegExp(teams[1]))
        await expect(groupRow).toHaveAttribute('aria-expanded', 'false')

        // A list three rows tall, scrolled so team B's group row is its top row.
        const scroller = await listScroller(groupRow)
        const listHeight = await scroller.evaluate(el => el.clientHeight)
        const rowHeight = await groupRow.evaluate(el => el.getBoundingClientRect().height)
        const viewport = page.viewportSize()
        if (!viewport) throw new Error('the page has no viewport size')
        await page.setViewportSize({
            width: viewport.width,
            height: Math.round(viewport.height - listHeight + 3 * rowHeight),
        })
        await expect.poll(() => scroller.evaluate(el => el.clientHeight)).toBeLessThan(listHeight)
        await scrollRowTo(groupRow, -1)
        await expect(groupA).not.toBeInViewport()
        await expect(groupRow).toBeInViewport()

        await recordDrops(page, [groupB])
        sent.length = 0
        await pressToolbar(page, 'report-fold-all', 'Expand all')

        // The opened run has landed once team B's first sale is drawn under its group.
        await expect(foldAll).toHaveText('Collapse all')
        await expect(rowOf(page, `${stem}B 00`)).toHaveCount(1)
        await expect(groupRow).toHaveAttribute('aria-expanded', 'true')
        await expect(groupRow).toBeInViewport()
        expect(
            await recordedDrops(page),
            'rows the list dropped between Expand all and its run'
        ).toEqual([])
        // Pages of 5: team A's first five sales, then its sixth and team B's group row.
        expect(foldRequests(sent), 'requests of Expand all at team B').toHaveLength(2)
    })

    test('a heading sorts the rows inside their groups, and a copied link keeps it', async ({
        page,
        browser,
    }) => {
        await balanceOfOneTeam(page, 'BalSort')
        await expect(detailLabels(page)).toHaveText(['Ann', 'Bea', 'Cy', 'Dee'])
        const sort = page.getByTestId('report-sort-amount')
        await expect(sort).toHaveAttribute('role', 'button')
        await expect(sort).toHaveAccessibleName('Sort by Amount')
        // A running value is not a sort key.
        await expect(page.getByTestId('report-heading-balance')).toBeVisible()
        await expect(page.getByTestId('report-sort-balance')).toHaveCount(0)

        await sort.click()
        await expect.poll(() => sortParam(page)).toBe('amount:asc')
        await expect(detailLabels(page)).toHaveText(['Dee', 'Ann', 'Bea', 'Cy'])
        // One line keeps tap-to-sort and shows no Sort chip.
        await expect(page.getByTestId('report-sort-menu')).toHaveCount(0)
        await sort.click()
        await expect.poll(() => sortParam(page)).toBe('amount:desc')
        await expect(detailLabels(page)).toHaveText(['Cy', 'Bea', 'Ann', 'Dee'])
        // The running balance keeps the report's own order.
        await expect(await cellOf(page, 'balance', 'Cy')).toHaveText('19.00')

        await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
        await pressToolbar(page, 'report-copy-link', 'Copy link')
        await expect(page.getByText('Link copied', { exact: true })).toBeVisible()
        const link = await page.evaluate(() => navigator.clipboard.readText())
        expect(new URL(link).searchParams.get('s')).toBe('amount:desc')
        const other = await browser.newContext()
        try {
            const q = await other.newPage()
            await loginAsShippedOwner(q)
            // Opening a pasted link is a full load by nature.
            await q.goto(link)
            await expect(detailLabels(q)).toHaveText(['Cy', 'Bea', 'Ann', 'Dee'])
        } finally {
            await other.close()
        }

        await sort.click()
        await expect.poll(() => sortParam(page)).toBeNull()
        await expect(detailLabels(page)).toHaveText(['Ann', 'Bea', 'Cy', 'Dee'])
    })

    // The run refuses a sort by an unknown column with 422 at options.sort.0.col,
    // which the view draws as a report that names something missing.
    test('a link that sorts by a column the report lacks offers Clear sort', async ({
        page,
        browser,
    }) => {
        await balanceOfOneTeam(page, 'BalBadSort')
        const link = new URL(page.url())
        link.searchParams.set('s', 'nosuch:asc')
        const other = await browser.newContext()
        try {
            const q = await other.newPage()
            await loginAsShippedOwner(q)
            // Opening a pasted link is a full load by nature.
            await q.goto(link.toString())
            await expect(q.getByTestId('report-missing')).toContainText('nosuch')
            await q.getByTestId('report-clear-sort').click()
            await expect.poll(() => sortParam(q)).toBeNull()
            await expect(detailLabels(q)).toHaveText(['Ann', 'Bea', 'Cy', 'Dee'])
        } finally {
            await other.close()
        }
    })

    test('All dates shows every sale, says so in the header, and opens at zero', async ({
        page,
    }) => {
        await balanceOfOneTeam(page, 'BalAll')
        await page.getByTestId('report-param-period').click()
        await page.getByTestId('report-period-preset-allDates').click()
        await expect(page.getByTestId('report-param-period')).toHaveText('Period: All dates')
        await expect(page.getByTestId('report-document-period')).toHaveText('All dates')
        await expect(rowOf(page, 'Old')).toBeVisible()
        await expect(await cellOf(page, 'balance', OPENING)).toHaveText(/^\$?0\.00$/)
        await expect(await cellOf(page, 'balance', 'Old')).toHaveText('10.00')
        await expect(await cellOf(page, 'balance', 'Dee')).toHaveText('20.00')
    })

    // A report too wide for one line at 820 wraps (by an arrangement or by itself);
    // every heading stays inside the table and nothing scrolls sideways.
    test('on a view too narrow for one line a report wraps inside the table and never scrolls sideways', async ({
        page,
    }) => {
        await balanceOfOneTeam(page, 'BalFit')
        await page.setViewportSize({ width: 820, height: 720 })
        const measure = () =>
            page.getByTestId('report-table').evaluate(table => {
                const right = table.getBoundingClientRect().right
                const cells = [
                    ...table.querySelectorAll('[data-testid^="report-heading-"]'),
                ].filter(el => el.getAttribute('data-testid') !== 'report-heading-row')
                const tops = new Set(cells.map(el => Math.round(el.getBoundingClientRect().top)))
                return {
                    lines: tops.size,
                    outside: cells.filter(el => el.getBoundingClientRect().right > right + 1)
                        .length,
                }
            })
        await expect(page.getByTestId('report-table')).toHaveCSS('opacity', '1')
        // Polled: the lines are read once the 820 window's re-layout has landed.
        await expect.poll(async () => (await measure()).lines).toBeGreaterThanOrEqual(2)
        const fit = await measure()
        expect(fit.lines).toBeGreaterThanOrEqual(2)
        expect(fit.outside).toBe(0)
        const scroller = await listScroller(rowOf(page, 'Dee'))
        expect(await scroller.evaluate(el => el.scrollWidth === el.clientWidth)).toBe(true)
        expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
            820
        )
    })

    test('on a phone an opening row shows its whole label with its figures right after it, and the Sort chip sorts', async ({
        page,
    }) => {
        await balanceOfOneTeam(page, 'BalPhone')
        await page.setViewportSize(PHONE)
        const opening = rowOf(page, OPENING)
        await expect(opening).toBeVisible()
        const id = (await opening.getAttribute('data-testid'))?.replace('report-row-', '') ?? ''
        const balance = page.getByTestId(`report-cell-balance-${id}`)
        await expect(balance).toHaveText(/^\$?10\.00$/)
        // Blank cells are not drawn: no empty line between the label and the figures.
        for (const col of ['team', 'day', 'status', 'amount', 'cost', 'quantity', 'sameDay']) {
            await expect(page.getByTestId(`report-cell-${col}-${id}`)).toHaveCount(0)
        }
        // One snapshot holds both bounds of the gap and the page's width, so a
        // re-layout between two reads cannot pass one bound and miss the other.
        await expect
            .poll(async () => {
                const label = await opening.boundingBox()
                const figures = await balance.boundingBox()
                const width = await page.evaluate(() => document.documentElement.scrollWidth)
                if (!label || !figures) return null
                const labelWhole = await opening
                    .getByText(OPENING, { exact: true })
                    .evaluate(el => el.scrollWidth <= el.clientWidth)
                return {
                    figuresFollow:
                        figures.y >= label.y - 1 && figures.y <= label.y + label.height + 8,
                    labelWhole,
                    fitsPhone: width <= PHONE.width,
                }
            })
            .toEqual({ figuresFollow: true, labelWhole: true, fitsPhone: true })

        await page.getByTestId('report-sort-menu').click()
        await page.getByTestId('report-sort-choice-amount-asc').click()
        await expect.poll(() => sortParam(page)).toBe('amount:asc')
        await expect(detailLabels(page)).toHaveText(['Dee', 'Ann', 'Bea', 'Cy'])
        await expect(page.getByTestId('report-sort-amount')).toHaveCount(0)
        await expect(page.getByTestId('report-sort-menu')).toContainText('Sort: Amount ↑')
    })

    // The builder's v3 controls, in the order the tasks add them: This row's field,
    // Count and Range start or end (Tasks 16, 16a), a formula on group and total rows
    // and a row that starts each group (Task 18). The asked period (this fiscal year)
    // keeps every sale, all dated this January.
    test('the builder counts a lookup on this row, shows a formula on group and total rows, and starts each group with a row', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptv3build')
        try {
            const team = uniqueName('Built')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, { team, customer: 'Ada', amount: 1000, cost: 0, day: `${YEAR}-01-05` })
            await addSale(p, { team, customer: 'Abe', amount: 500, cost: 0, day: `${YEAR}-01-05` })
            await addSale(p, { team, customer: 'Cy', amount: 200, cost: 0, day: `${YEAR}-01-09` })
            await openReportsHub(p)
            // Columns customer (0), team (1) and the amount summed (2).
            await startTeamSales(p)

            // How many sales share this row's day: Count, with This row's field.
            await p.getByTestId('report-builder-add-formula').click()
            const f = 'report-builder-column-3-formula'
            await chooseOption(p, `${f}-root-part`, 'lookup')
            await chooseOption(p, `${f}-root-source`, SALES_SOURCE)
            await chooseOption(p, `${f}-0-part`, 'eq')
            await chooseOption(p, `${f}-0-0-part`, 'path')
            await pickIn(p, `${f}-0-0-path`, 'report-builder-pick-field-day')
            await chooseOption(p, `${f}-0-1-part`, 'outer')
            await pickIn(p, `${f}-0-1-outer`, 'report-builder-pick-field-day')
            await chooseOption(p, `${f}-1-part`, 'path')
            await pickIn(p, `${f}-1-path`, 'report-builder-pick-field-customer')
            await chooseOption(p, `${f}-root-pick`, 'count')
            await expect(p.getByTestId(`${f}-summary`)).toHaveText(
                "= count Customer in Stub sales where Day = this row's Day"
            )

            // Range start or end: an asked period offers its first day (column 4).
            await p.getByTestId('report-builder-section-filter').click()
            await pickIn(p, 'report-builder-filter-add-condition', 'report-builder-pick-field-day')
            await chooseOption(p, 'report-builder-filter-0-op', 'between')
            await p.getByTestId('report-builder-filter-0-ask').click()
            await chooseOption(p, 'report-builder-filter-0-default', 'thisFiscalYear')
            await p.getByTestId('report-builder-add-formula').click()
            const b = 'report-builder-column-4-formula'
            await chooseOption(p, `${b}-root-part`, 'bound')
            await expect(p.getByTestId(`${b}-root-bound`)).toContainText('Day start')

            // On group and total rows the amount shows its largest sale, not the sum.
            const r = 'report-builder-column-2-rollup'
            await chooseOption(p, r, 'custom')
            await chooseOption(p, `${r}-formula-root-part`, 'largest')
            await chooseOption(p, `${r}-formula-0-part`, 'path')
            await pickIn(p, `${r}-formula-0-path`, 'report-builder-pick-field-amount')

            // Open teams, each starting with a row that adds up the team's amounts.
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'team')
            await p.getByTestId('report-builder-group-0-open').click()
            const o = 'report-builder-group-0-opening'
            await p.getByTestId(o).click()
            await p.getByTestId(`${o}-label`).fill('Brought forward')
            await p.getByTestId(`${o}-cell-amount-add`).click()
            const cell = `${o}-cell-amount-formula`
            await chooseOption(p, `${cell}-root-part`, 'sum')
            await chooseOption(p, `${cell}-0-part`, 'path')
            await pickIn(p, `${cell}-0-path`, 'report-builder-pick-field-amount')
            await saveNewReport(p, uniqueName('Built v3'))

            const ids = await rowIds(p)
            const group = ids.findIndex(id => id.startsWith('g:'))
            expect(ids[group + 1]).toMatch(/^opening:/)
            await expect(await cellOf(p, 'amount', 'Brought forward')).toHaveText(/^\$?17\.00$/)
            await expect(await cellOf(p, 'amount', 'Ada')).toHaveText('10.00')
            await expect(await cellOf(p, 'amount', `Total ${team}`)).toHaveText(/^\$?10\.00$/)
            await expect(await cellOf(p, 'formula', 'Ada')).toHaveText('2')
            await expect(await cellOf(p, 'formula', 'Abe')).toHaveText('2')
            await expect(await cellOf(p, 'formula', 'Cy')).toHaveText('1')
        } finally {
            await close()
        }
    })
})
