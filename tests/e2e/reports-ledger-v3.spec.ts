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
// Q2: a line at a 1024 window is 944 wide. The narrowed line that Q2 asks about
// (a one-level grouped label floor of 98, + 5 × 80 + 3 × 128) fits in it; this
// report's does not (98 + 2 × 80 + 6 × 128).
const Q2_NARROWED_LINE = 882
const TEAM_BALANCE_NARROWED_LINE = 1026

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
        await loginAsShippedOwner(page)
        const stem = uniqueName('BalPage')
        const teams = [`${stem} A`, `${stem} B`]
        await openReportStub(page)
        for (const [i, team] of teams.entries()) {
            await addTeam(page, team)
            await addSales(page, {
                team,
                customerPrefix: `${stem}${'AB'[i]}`,
                count: 6,
                firstDay: `${YEAR}-02-01`,
                firstAmount: 100 * (i + 1),
                cost: 0,
                quantity: 1,
            })
        }
        // Pages of 5 detail rows: 12 sales page across the first team's end.
        const cursors: string[] = []
        await page.route('**/api/reports/run', async route => {
            const body = route.request().postDataJSON()
            if (body.options?.cursor) cursors.push(body.options.cursor)
            await route.continue({
                postData: JSON.stringify({ ...body, options: { ...body.options, limit: 5 } }),
            })
        })
        // Tall enough that the list draws every row of both teams at once, so one
        // snapshot holds the whole merged list (the list draws only rows near the view).
        await page.setViewportSize({ width: 1280, height: 1600 })
        await openTeamBalance(page)
        // The first run has no Teams filter, so it shows these sales too: no row
        // tells the two runs apart. The new run's params change the list's key, so
        // the list mounts again; once the first run's scroller has left the page,
        // every row on it is the new run's.
        await expect(detailLabels(page).first()).toBeVisible()
        const firstRunScroller = await listScroller(detailLabels(page).first())
        await showTeams(page, stem, teams)
        await expect.poll(() => firstRunScroller.evaluate(el => el.isConnected)).toBe(false)
        const firstSale = rowOf(page, `${stem}A 00`)
        await expect(firstSale).toBeVisible()
        const scroller = await listScroller(firstSale)
        const lastSection = `Total ${teams[1]}`
        // Scroll until the last team's Total row is drawn: every page is merged then.
        await expect
            .poll(async () => {
                await scroller.evaluate(el => {
                    el.scrollTop = el.scrollHeight
                })
                return rowOf(page, lastSection).count()
            })
            .toBe(1)
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

    // Q2, recommended answer (narrow first, then wrap): at a 1024 window the line is
    // 944 wide, so Q2's narrowed line (882) fits and this report's does
    // not. This report wraps to its tablet arrangement: eight headings (Team is left
    // out) on two lines, all inside the table, and nothing scrolls sideways.
    test('on a desktop too narrow for its narrowed line a report wraps to its tablet arrangement inside the table', async ({
        page,
    }) => {
        await balanceOfOneTeam(page, 'BalFit')
        await page.setViewportSize({ width: 1024, height: 720 })
        const measure = () =>
            page.getByTestId('report-table').evaluate(table => {
                const right = table.getBoundingClientRect().right
                const heading = table.querySelector('[data-testid="report-heading-row"]')
                const cells = [
                    ...table.querySelectorAll('[data-testid^="report-heading-"]'),
                ].filter(el => el.getAttribute('data-testid') !== 'report-heading-row')
                const tops = new Set(cells.map(el => Math.round(el.getBoundingClientRect().top)))
                return {
                    // The heading row keeps the list's scroll bar gutter, so its client
                    // width is the width a line's tracks get.
                    line: heading?.clientWidth ?? 0,
                    headings: cells.length,
                    lines: tops.size,
                    outside: cells.filter(el => el.getBoundingClientRect().right > right + 1)
                        .length,
                }
            })
        // At the first window (1280) the line is 1200; wait for the 1024 one.
        await expect
            .poll(async () => (await measure()).line)
            .toBeLessThan(TEAM_BALANCE_NARROWED_LINE)
        await expect.poll(async () => (await measure()).headings).toBe(8)
        const fit = await measure()
        expect(fit.line).toBeGreaterThanOrEqual(Q2_NARROWED_LINE)
        expect(fit).toMatchObject({ lines: 2, outside: 0 })
        await expect(page.getByTestId('report-heading-team')).toHaveCount(0)
        const scroller = await listScroller(rowOf(page, 'Dee'))
        expect(await scroller.evaluate(el => el.scrollWidth === el.clientWidth)).toBe(true)
        expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
            1024
        )
    })

    test('on a phone an opening row is its label and then its figures, and a heading still sorts', async ({
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
        const gap = async () => {
            const label = await opening.boundingBox()
            const figures = await balance.boundingBox()
            return label && figures ? figures.y - (label.y + label.height) : null
        }
        await expect.poll(gap).toBeGreaterThanOrEqual(-1)
        expect(await gap()).toBeLessThanOrEqual(8)
        await expect
            .poll(() => page.evaluate(() => document.documentElement.scrollWidth))
            .toBeLessThanOrEqual(PHONE.width)

        await page.getByTestId('report-sort-amount').click()
        await expect.poll(() => sortParam(page)).toBe('amount:asc')
        await expect(detailLabels(page)).toHaveText(['Dee', 'Ann', 'Bea', 'Cy'])
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
