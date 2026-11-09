import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub, waitForTableFit } from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    cellOf,
    chooseOption,
    hubRow,
    openReportStub,
    openReportsHub,
    pressToolbar,
    rowOf,
    SALES_SOURCE,
    saveNewReport,
    startReport,
    uniqueName,
} from './reports-helpers'

/**
 * The statement parts (G1, G3, G4, G5, G7, G8, G9) on the shipped "Stub statement"
 * (tests/scripts/reports-stub/server/register.go.txt), and the builder controls that
 * write them. A fresh invited member owns every sale it reads, so the figures are exact.
 */

const STATEMENT = 'Stub statement'
const OLD_DAY = '2024-03-01'
const SHORT_MONTHS = [
    'Jan',
    'Feb',
    'Mar',
    'Apr',
    'May',
    'Jun',
    'Jul',
    'Aug',
    'Sep',
    'Oct',
    'Nov',
    'Dec',
]

// The reader's today and the same day a year earlier (Feb 29 → Feb 28), as the
// browser sees them.
async function days(page: Page): Promise<{ today: string; prior: string }> {
    return page.evaluate(() => {
        const iso = (d: Date) =>
            `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
        const now = new Date()
        const last = new Date(now.getFullYear() - 1, now.getMonth() + 1, 0).getDate()
        const prior = new Date(now.getFullYear() - 1, now.getMonth(), Math.min(now.getDate(), last))
        return { today: iso(now), prior: iso(prior) }
    })
}

// Every row with this label (rowOf keeps only the first, so it cannot count).
const rowsLabelled = (page: Page, label: string) =>
    page
        .getByTestId('report-table')
        .locator('[data-testid^="report-row-"]')
        .filter({ has: page.getByText(label, { exact: true }) })

// One snapshot of the table's rows in order: each label and its depth, read from the
// label's indent (core ReportRow labelStyle: INDENT 16 + depth × 16).
async function rowDepths(page: Page) {
    const rows = await page
        .getByTestId('report-table')
        .locator('[data-testid^="report-row-"]')
        .evaluateAll(labels =>
            labels.map(label => ({
                text: (label.textContent ?? '').trim(),
                depth: (Number.parseFloat(getComputedStyle(label).paddingLeft) - 16) / 16,
            }))
        )
    const at = (text: string) => rows.findIndex(row => row.text === text)
    const depthOf = (text: string) => rows[at(text)]?.depth ?? Number.NaN
    return { rows, at, depthOf }
}

const shortDay = (iso: string) =>
    `${SHORT_MONTHS[Number(iso.slice(5, 7)) - 1]} ${Number(iso.slice(8, 10))}, ${iso.slice(0, 4)}`

// `large`: one more paid sale of this many cents today (the phone run's wide figures).
async function seedStatement(page: Page, large?: number) {
    const invited = await createInvitedUser(page, 'rptstatement')
    const p = invited.inviteePage
    const { today } = await days(p)
    const north = uniqueName('North team')
    const south = uniqueName('South team')
    const empty = uniqueName('Empty team')
    const east = uniqueName('East team')
    await openReportStub(p)
    await addTeam(p, north, 'north')
    await addTeam(p, south, 'south')
    await addTeam(p, empty, 'south')
    await addTeam(p, east, 'east')
    await addSale(p, { team: north, customer: 'N', amount: 3000, cost: 100, day: OLD_DAY })
    await addSale(p, { team: empty, customer: 'E', amount: 0, cost: 0, day: OLD_DAY })
    await addSale(p, {
        team: south,
        customer: 'S',
        amount: 5000,
        cost: 50,
        day: today,
        status: 'paid',
    })
    await addSale(p, {
        team: east,
        customer: 'X',
        amount: 700,
        cost: 0,
        day: OLD_DAY,
        status: 'paid',
    })
    if (large !== undefined) {
        await addSale(p, {
            team: south,
            customer: 'L',
            amount: large,
            cost: 0,
            day: today,
            status: 'paid',
        })
    }
    await openReportsHub(p)
    await p.getByTestId('reports-hub-tab-published').click()
    await hubRow(p, STATEMENT).click()
    await expect(rowOf(p, 'All sales')).toBeVisible()
    return { p, close: invited.close, teams: { north, south, empty, east } }
}

test.describe('reports: statements', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a statement on a desktop: ragged levels, dated headings, a row inside a section', async ({
        page,
    }) => {
        const { p, close, teams } = await seedStatement(page)
        try {
            const { today, prior } = await days(p)
            // G3: the two figure headings name the days; Customize keeps the names.
            await expect(p.getByTestId('report-heading-current')).toHaveText(shortDay(today))
            await expect(p.getByTestId('report-heading-prior')).toHaveText(shortDay(prior))
            // G7: no hidden key column in the table.
            for (const id of ['side', 'cls', 'knd']) {
                await expect(p.getByTestId(`report-heading-${id}`)).toHaveCount(0)
            }
            // G1: the east team (no region) sits directly under its section; no (none).
            await expect(rowOf(p, teams.east)).toBeVisible()
            await expect(rowsLabelled(p, '(none)')).toHaveCount(0)
            // G5: the empty south team is hidden, and with it its desk and region in Open sales.
            await expect(rowsLabelled(p, teams.empty)).toHaveCount(0)
            // The emptied desk and region groups of Open sales are gone: only Paid
            // sales' South › Southern desks (the south team's paid sale) remains.
            for (const label of [
                'Southern desks',
                'Total Southern desks',
                'South',
                'Total South',
            ]) {
                await expect(rowsLabelled(p, label)).toHaveCount(1)
            }
            // G4 + G8: Adjustments has its own cell per column and sits before the paid total.
            await expect(await cellOf(p, 'current', 'Adjustments')).toHaveText('$1.50')
            await expect(await cellOf(p, 'prior', 'Adjustments')).toHaveText('$1.00')
            await expect(await cellOf(p, 'current', 'Total Paid sales')).toHaveText('$58.50')
            await expect(await cellOf(p, 'prior', 'Total Paid sales')).toHaveText('$8.00')
            await expect(await cellOf(p, 'change', 'Total Paid sales')).toHaveText('$50.50')
            const { at, depthOf } = await rowDepths(p)
            const paid = at('Paid sales')
            const paidTotal = at('Total Paid sales')
            const east = at(teams.east)
            // G1: the east team (no region, no desk) sits directly under Paid sales, one
            // depth below the section and two above the south team, which sits under
            // its region and desk groups; it stays inside the section, before its total.
            expect(paid).toBeGreaterThan(-1)
            expect(east).toBeGreaterThan(paid)
            expect(east).toBeLessThan(paidTotal)
            expect(depthOf(teams.east)).toBe(depthOf('Paid sales') + 1)
            expect(depthOf(teams.south)).toBe(depthOf(teams.east) + 2)
            const adjust = at('Adjustments')
            expect(adjust).toBeGreaterThan(paid)
            expect(adjust).toBeLessThan(paidTotal)
            // Customize lists the plain labels and no hidden column.
            await pressToolbar(p, 'report-customize-open', 'Customize')
            const panel = p.getByTestId('report-customize')
            await panel.getByTestId('report-customize-section-columns').click()
            for (const label of ['Current', 'Prior year', 'Change', '% change']) {
                await expect(panel.getByText(label, { exact: true })).toBeVisible()
            }
            for (const hidden of ['side', 'cls', 'knd']) {
                await expect(panel.getByText(hidden, { exact: true })).toHaveCount(0)
                await expect(panel.getByTestId(`report-option-column-${hidden}`)).toHaveCount(0)
            }
        } finally {
            await close()
        }
    })

    test('the statement on a phone draws its authored mobile layout', async ({ page }) => {
        // A $1,000,000.00 sale today widens Current and Change past the
        // -$99,999.99 worst case, so Current · Prior year · Change no longer fits
        // beside the full indent of four group levels at 390 px, but does at a
        // smaller one (G9). A figure column takes no minWidth (a figure is never
        // cut), so the figures themselves must be wide.
        const { p, close } = await seedStatement(page, 100_000_000)
        try {
            await p.setViewportSize({ width: 390, height: 844 })
            expect(await waitForTableFit(p)).toBe('mobile')
            // One snapshot: the layout, the least left offset of a Current cell (on
            // line 2) from the table's left edge, the left-out % change, no sideways scroll.
            const drawn = await p.getByTestId('report-table').evaluate(table => {
                const left = table.getBoundingClientRect().left
                const offsets = [
                    ...table.querySelectorAll('[data-testid^="report-cell-current-"]'),
                ].map(cell => cell.getBoundingClientRect().left - left)
                return {
                    layout: (table as HTMLElement).dataset.layout,
                    offset: Math.min(...offsets),
                    cells: offsets.length,
                    pct: table.querySelectorAll('[data-testid^="report-cell-pct-"]').length,
                    fits: table.scrollWidth === table.clientWidth,
                }
            })
            // INDENT 16 + 4 levels × 16 − CELL_PADDING / 2 (core continuationIndent).
            const fullIndent = 16 + 4 * 16 - 12
            expect(drawn.layout).toBe('mobile')
            expect(drawn.cells).toBeGreaterThan(0)
            expect(drawn.offset).toBeLessThan(fullIndent)
            expect(drawn.pct).toBe(0)
            expect(drawn.fits).toBe(true)
        } finally {
            await close()
        }
    })

    test('a statement built in the builder: heading, hidden key, skipped and hidden groups, a row inside a section', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptstmtbuild')
        try {
            const team = uniqueName('Builder team')
            await openReportStub(p)
            await addTeam(p, team)
            // Tag = "Big" where amount ≥ cost, else blank. Zero is tagged but its
            // customer group is hidden by the amount > 0 condition.
            await addSale(p, { team, customer: 'Big1', amount: 5000, cost: 100, day: OLD_DAY })
            await addSale(p, { team, customer: 'Small1', amount: 100, cost: 900, day: OLD_DAY })
            await addSale(p, {
                team,
                customer: 'Zero',
                amount: 0,
                cost: 0,
                day: OLD_DAY,
                status: 'paid',
            })
            await addSale(p, {
                team,
                customer: 'Small2',
                amount: 100,
                cost: 900,
                day: OLD_DAY,
                status: 'paid',
            })
            const { today } = await days(p)
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            // Columns: customer (0), amount summed (1), status (2), the Tag formula (3).
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-1-aggregate', 'sum')
            await addColumn(p, ['status'])
            await p.getByTestId('report-builder-add-formula').click()
            await p.getByTestId('report-builder-column-3-label').fill('Tag')
            const f = 'report-builder-column-3-formula'
            await chooseOption(p, `${f}-root-part`, 'if')
            await chooseOption(p, `${f}-0-part`, 'gte')
            for (const [i, field] of ['amount', 'cost'].entries()) {
                await chooseOption(p, `${f}-0-${i}-part`, 'path')
                await p.getByTestId(`${f}-0-${i}-path`).click()
                await p
                    .getByTestId('report-builder-picker')
                    .getByTestId(`report-builder-pick-field-${field}`)
                    .click()
            }
            await chooseOption(p, `${f}-1-part`, 'text')
            await p.getByTestId(`${f}-1-text`).fill('Big')

            // An as-of day the reader sets (default today), for the heading.
            await p.getByTestId('report-builder-section-filter').click()
            await p.getByTestId('report-builder-filter-add-condition').click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-day')
                .click()
            await chooseOption(p, 'report-builder-filter-0-op', 'lte')
            await p.getByTestId('report-builder-filter-0-ask').click()
            await chooseOption(p, 'report-builder-filter-0-default', 'today')
            // "Heading: from a parameter" on Amount; "Use only to group or sort" on status.
            await p.getByTestId('report-builder-column-1-heading').click()
            await p.getByTestId('report-builder-column-2-hidden').click()

            // Status › Tag (rows with no Tag skip it; a Tag with nothing shown hides) ›
            // customer, shown only where its amount is above 0.
            await p.getByTestId('report-builder-section-group').click()
            for (const [level, col] of ['status', 'formula', 'customer'].entries()) {
                await p.getByTestId('report-builder-group-add').click()
                await chooseOption(p, `report-builder-group-${level}-col`, col)
            }
            await p.getByTestId('report-builder-group-0-open').click()
            await p.getByTestId('report-builder-group-1-open').click()
            await chooseOption(p, 'report-builder-group-1-blank', 'skip')
            await p.getByTestId('report-builder-group-1-hide-empty').click()
            const h = 'report-builder-group-2-having'
            await chooseOption(p, `${h}-add-column`, 'amount')
            await chooseOption(p, `${h}-0-op`, 'gt')
            await p.getByTestId(`${h}-0-value`).fill('0')

            // Open and Paid sections, and Adjust (= Open sales) inside Paid sales.
            await p.getByTestId('report-builder-section-layout').click()
            await p.getByTestId('report-builder-layout-add-section').click()
            await p.getByTestId('report-builder-layout-0-title').fill('Open sales')
            await chooseOption(p, 'report-builder-layout-0-key', 'open')
            await p.getByTestId('report-builder-layout-add-section').click()
            await p.getByTestId('report-builder-layout-1-title').fill('Paid sales')
            await chooseOption(p, 'report-builder-layout-1-key', 'paid')
            await p.getByTestId('report-builder-layout-add-row').click()
            await p.getByTestId('report-builder-layout-2-label').fill('Adjust')
            await chooseOption(p, 'report-builder-layout-2-formula-root-part', 'field')
            await chooseOption(p, 'report-builder-layout-2-formula-root-field', 'section')
            await chooseOption(p, 'report-builder-layout-2-within', 'section2')
            await saveNewReport(p, uniqueName('Built statement'))

            // Heading: the amount column's header names the as-of day.
            await expect(p.getByTestId('report-heading-amount')).toHaveText(shortDay(today))
            // Hidden: no status column.
            await expect(p.getByTestId('report-heading-status')).toHaveCount(0)
            // Within: Adjust adds Open sales ($51.00) to Paid sales' own $1.00.
            await expect(await cellOf(p, 'amount', 'Total Paid sales')).toHaveText('$52.00')
            const { at, depthOf, rows } = await rowDepths(p)
            // Skip: Small1 (no Tag) sits directly under Open sales, before its total.
            expect(at('Small1')).toBeGreaterThan(at('Open sales'))
            expect(at('Small1')).toBeLessThan(at('Total Open sales'))
            expect(depthOf('Small1')).toBe(depthOf('Open sales') + 1)
            expect(depthOf('Big1')).toBe(depthOf('Small1') + 1)
            expect(rows.filter(row => row.text === '(none)')).toEqual([])
            // Hide empty: Paid sales' Big group (only Zero, hidden) is gone with it.
            expect(rows.filter(row => row.text === 'Big')).toHaveLength(1)
            expect(at('Big')).toBeLessThan(at('Total Open sales'))
            expect(at('Zero')).toBe(-1)
            // Within: Adjust sits inside Paid sales, before its total.
            expect(at('Adjust')).toBeGreaterThan(at('Paid sales'))
            expect(at('Adjust')).toBeLessThan(at('Total Paid sales'))
            // Customize lists Amount by its label and no status column.
            await pressToolbar(p, 'report-customize-open', 'Customize')
            const panel = p.getByTestId('report-customize')
            await panel.getByTestId('report-customize-section-columns').click()
            await expect(panel.getByTestId('report-option-column-amount')).toBeVisible()
            await expect(panel.getByText('Amount', { exact: true })).toBeVisible()
            await expect(panel.getByTestId('report-option-column-status')).toHaveCount(0)
        } finally {
            await close()
        }
    })
})
