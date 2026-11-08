import { type Browser, expect, type Locator, type Page, test } from '@playwright/test'
import { createInvitedUser, loginAs, skipWithoutReportsStub } from './helpers'
import {
    addColumn,
    addSale,
    addSales,
    addTeam,
    chooseOption,
    holdRuns,
    listScroller,
    openReportStub,
    openReportsHub,
    rowOf,
    SALES_SOURCE,
    saveNewReport,
    scrollRowTo,
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

            // Money is asked too, as an amount (F18).
            await p.getByTestId('report-builder-filter-add-condition').click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-amount')
                .click()
            await expect(p.getByTestId('report-builder-filter-1-op')).toBeVisible()
            await expect(p.getByTestId('report-builder-filter-1-ask')).toBeVisible()
        } finally {
            await close()
        }
    })

    test('a range of numbers asked when run asks both ends, and the report runs on them', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptrange')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            const sales = [
                { customer: 'Few', quantity: 1 },
                { customer: 'Some', quantity: 5 },
                { customer: 'Many', quantity: 9 },
            ]
            for (const sale of sales) {
                await addSale(p, { team, amount: 100, cost: 0, day: '2026-01-10', ...sale })
            }

            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await p.getByTestId('report-builder-section-filter').click()
            await p.getByTestId('report-builder-filter-add-condition').click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-quantity')
                .click()
            await chooseOption(p, 'report-builder-filter-0-op', 'between')
            await p.getByTestId('report-builder-filter-0-ask').click()
            await expect(p.getByTestId('report-builder-filter-0-value')).toBeHidden()
            await expect(p.getByTestId('report-builder-filter-0-high')).toBeHidden()
            await saveNewReport(p, uniqueName('Range'))

            const setNumber = async (id: string, value: string) => {
                await p.getByTestId('report-add-filter').click()
                await p.getByTestId(`report-add-filter-${id}`).click()
                await p.getByTestId('report-filter-value').fill(value)
                await p.getByTestId('report-filter-apply').click()
                await expect(p.getByTestId(`report-param-${id}`)).toBeVisible()
            }
            await setNumber('quantity', '3')
            await setNumber('quantity_to', '7')
            const table = p.getByTestId('report-table')
            await expect(table.getByText('Some', { exact: true })).toBeVisible()
            await expect(table.getByText('Few', { exact: true })).toHaveCount(0)
            await expect(table.getByText('Many', { exact: true })).toHaveCount(0)
        } finally {
            await close()
        }
    })

    test('none of a group, and a new operator stops asking', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptnone')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, {
                team,
                customer: 'Open one',
                amount: 100,
                cost: 0,
                day: '2026-01-10',
            })
            await addSale(p, {
                team,
                customer: 'Paid one',
                amount: 200,
                cost: 0,
                day: '2026-01-20',
                status: 'paid',
            })

            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await p.getByTestId('report-builder-section-filter').click()
            // None of: status is Paid, inside an added group.
            await p.getByTestId('report-builder-filter-add-group').click()
            await p.getByTestId('report-builder-filter-0-match').click()
            await p.getByTestId('report-builder-filter-0-match-none').click()
            await p.getByTestId('report-builder-filter-0-add-condition').click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-status')
                .click()
            await p.getByTestId('report-builder-filter-0-0-value').click()
            await p.getByTestId('report-builder-filter-0-0-value-paid').click()
            const table = p.getByTestId('report-table')
            await expect(table.getByText('Open one', { exact: true })).toBeVisible()
            await expect(table.getByText('Paid one', { exact: true })).toHaveCount(0)

            // An asked day; a new operator drops the asked value and shows the value again.
            await p.getByTestId('report-builder-filter-add-condition').click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-day')
                .click()
            const ask = p.getByTestId('report-builder-filter-1-ask')
            await ask.click()
            await expect(ask).toHaveAttribute('aria-checked', 'true')
            await expect(p.getByTestId('report-builder-filter-1-value')).toBeHidden()
            await p.getByTestId('report-builder-filter-1-op').click()
            await p.getByTestId('report-builder-filter-1-op-lte').click()
            await expect(ask).toHaveAttribute('aria-checked', 'false')
            await p.getByTestId('report-builder-filter-1-value').fill('2026-01-05')
            await expect(p.getByTestId('report-empty')).toBeVisible()
        } finally {
            await close()
        }
    })

    test('on a phone a condition is added in the builder sheet and shows in Preview', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptphonef')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, {
                team,
                customer: 'Kept sale',
                amount: 100,
                cost: 0,
                day: '2026-01-10',
            })
            await addSale(p, {
                team,
                customer: 'Other sale',
                amount: 200,
                cost: 0,
                day: '2026-01-20',
            })

            await openReportsHub(p)
            await p.setViewportSize({ width: 390, height: 844 })
            await startReport(p, SALES_SOURCE)
            await expect(p.getByTestId('report-builder-mode-design')).toBeVisible()
            await p.getByTestId('report-builder-section-filter').click()
            await p.getByTestId('report-builder-filter-add-condition').click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-customer')
                .click()
            await p.getByTestId('report-builder-filter-0-op').click()
            await p.getByTestId('report-builder-filter-0-op-startsWith').click()
            await p.getByTestId('report-builder-filter-0-value').fill('Kept')

            await p.getByTestId('report-builder-mode-preview').click()
            const table = p.getByTestId('report-table')
            await expect(table.getByText('Kept sale', { exact: true })).toBeVisible()
            await expect(table.getByText('Other sale', { exact: true })).toHaveCount(0)
        } finally {
            await close()
        }
    })
})

/**
 * A run with new params starts at the top of the report; a live re-run of the same
 * params keeps the reader's place. The report lists 40 sales (customers Top 00…39,
 * amounts 10.00…10.39, sorted by customer) with an asked "amount at least" filter.
 * The table stays mounted while a new run loads (the earlier figures stay in view),
 * so its list keeps its scroll offset from one run to the next.
 */

const SALES = 40

function topName(i: number): string {
    return `Top ${String(i).padStart(2, '0')}`
}

function dayOf(i: number): string {
    return new Date(Date.UTC(2026, 0, 1 + i)).toISOString().slice(0, 10)
}

async function seedAskedAmount(p: Page) {
    const team = uniqueName('Team')
    await openReportStub(p)
    await addTeam(p, team)
    await addSales(p, {
        team,
        customerPrefix: 'Top',
        count: SALES,
        firstDay: dayOf(0),
        firstAmount: 1000,
        cost: 400,
        quantity: 3,
    })
    await openReportsHub(p)
    await startReport(p, SALES_SOURCE)
    await addColumn(p, ['amount'])
    await addColumn(p, ['cost'])
    await addColumn(p, ['quantity'])
    await p.getByTestId('report-builder-section-filter').click()
    await p.getByTestId('report-builder-filter-add-condition').click()
    await p
        .getByTestId('report-builder-picker')
        .getByTestId('report-builder-pick-field-amount')
        .click()
    await chooseOption(p, 'report-builder-filter-0-op', 'gte')
    await p.getByTestId('report-builder-filter-0-ask').click()
    await p.getByTestId('report-builder-section-group').click()
    await chooseOption(p, 'report-builder-sort-add-menu', 'customer')
    await saveNewReport(p, uniqueName('Top'))
    await expect(rowOf(p, topName(0))).toBeVisible()
    return team
}

/** Any row the list has drawn: the way to its scroller when the row looked for is not drawn. */
function anyRow(p: Page): Locator {
    return p.getByTestId('report-table').locator('[data-testid^="report-row-"]').first()
}

async function scrollOffset(p: Page): Promise<number> {
    return (await listScroller(anyRow(p))).evaluate(scroller => scroller.scrollTop)
}

/** How far the row's top edge is under the list's top edge. */
async function gapToListTop(row: Locator): Promise<number> {
    const list = await listScroller(row)
    return row.evaluate(
        (element, scroller) =>
            element.getBoundingClientRect().top - scroller.getBoundingClientRect().top,
        list
    )
}

// The list draws only the rows near its view (fewer ahead on a phone), so a row
// further down is not in the page yet: scroll down half a view at a time until
// the list draws it.
async function revealRow(p: Page, row: Locator) {
    const list = await listScroller(anyRow(p))
    for (let step = 0; step < 40 && (await row.count()) === 0; step++) {
        await list.evaluate(
            scroller =>
                new Promise(resolve => {
                    scroller.scrollTop += scroller.clientHeight / 2
                    requestAnimationFrame(() => requestAnimationFrame(resolve))
                })
        )
    }
    await expect(row, 'the list drew the row').toHaveCount(1)
}

async function toListTop(p: Page, row: Locator) {
    await revealRow(p, row)
    await scrollRowTo(row)
    expect(Math.abs(await gapToListTop(row)), 'the row is at the list top').toBeLessThanOrEqual(1)
}

// Holds the run that `change` starts until "Updating…" shows, so the earlier
// figures are still in view while it loads, then lets it land.
async function landNewRun(p: Page, change: () => Promise<void>) {
    const release = await holdRuns(p)
    await change()
    await expect(p.getByTestId('report-updating')).toBeVisible()
    release()
    await p.unrouteAll({ behavior: 'wait' })
    await expect(p.getByTestId('report-updating')).toBeHidden()
}

async function setMinAmount(p: Page, amount: string) {
    const chip = p.getByTestId('report-param-amount')
    if (await chip.isVisible()) {
        await chip.click()
    } else {
        await p.getByTestId('report-add-filter').click()
        await p.getByTestId('report-add-filter-amount').click()
    }
    const picker = p.getByTestId('report-filter-picker')
    await picker.getByTestId('report-filter-value').fill(amount)
    await picker.getByTestId('report-filter-apply').click()
    await expect(picker).toBeHidden()
}

// The list is at its top and the report's first row is the first row in view.
async function expectAtTop(p: Page, first: number) {
    await expect.poll(() => scrollOffset(p), 'the list scroll offset').toBeLessThanOrEqual(1)
    const row = rowOf(p, topName(first))
    await expect(row).toBeInViewport()
    expect(
        Math.abs(await gapToListTop(row)),
        'the first row is at the list top'
    ).toBeLessThanOrEqual(1)
}

// A row lower down is first in view; a narrower filter, a narrower one again and
// then no filter each start the new run at the top. Each row scrolled to has 15
// or more rows under it, so it can reach the list top, and stays in the next
// run, so the earlier run's row is there to hold.
async function expectNewParamsStartAtTop(p: Page) {
    await toListTop(p, rowOf(p, topName(20)))
    expect(await scrollOffset(p)).toBeGreaterThan(0)
    await landNewRun(p, () => setMinAmount(p, '10.05'))
    await expectAtTop(p, 5)

    // Top 15 is row 10 of Top 05…39, and row 5 of the next run.
    await toListTop(p, rowOf(p, topName(15)))
    await landNewRun(p, () => setMinAmount(p, '10.10'))
    await expectAtTop(p, 10)

    // Widened: Top 25 is row 15 of Top 10…39, and row 23 of the next run.
    await toListTop(p, rowOf(p, topName(25)))
    await landNewRun(p, () => setMinAmount(p, '10.02'))
    await expectAtTop(p, 2)

    // Widened back to every sale: Top 20 is row 18 of Top 02…39, and row 20 after.
    // The unfiltered run is the report's first one, still cached, so its rows
    // land at once: no run loads and "Updating…" never shows.
    await toListTop(p, rowOf(p, topName(20)))
    await p.getByTestId('report-filter-remove-amount').click()
    await expect(p.getByTestId('report-param-amount')).toHaveCount(0)
    await expectAtTop(p, 0)
}

// The 40 sales in a new report's builder, sorted by customer, with no asked
// value: every design change below runs the draft with the same (empty) params.
async function seedBuilderDraft(p: Page) {
    const team = uniqueName('Team')
    await openReportStub(p)
    await addTeam(p, team)
    await addSales(p, {
        team,
        customerPrefix: 'Top',
        count: SALES,
        firstDay: dayOf(0),
        firstAmount: 1000,
        cost: 400,
        quantity: 3,
    })
    await openReportsHub(p)
    await startReport(p, SALES_SOURCE)
    await addColumn(p, ['amount'])
    // Amount cells are drawn only once the run with that column has landed, so
    // the sort below has earlier figures to keep in view while it loads. The draft
    // has no sort yet, so rows come in record-id order: wait on any amount cell,
    // not one customer's row, which may sit outside the rows FlashList draws.
    await expect(
        p.getByTestId('report-table').locator('[data-testid^="report-cell-amount-"]').first()
    ).toBeVisible()
    await p.getByTestId('report-builder-section-group').click()
    await landNewRun(p, () => chooseOption(p, 'report-builder-sort-add-menu', 'customer'))
    await expect(rowOf(p, topName(0))).toBeVisible()
}

async function sortZtoA(p: Page) {
    await p
        .getByTestId('report-builder-sort-0')
        .getByRole('radio', { name: 'Z to A', exact: true })
        .click()
}

async function filterAmountFrom(p: Page, amount: string) {
    await p.getByTestId('report-builder-section-filter').click()
    await p.getByTestId('report-builder-filter-add-condition').click()
    await p
        .getByTestId('report-builder-picker')
        .getByTestId('report-builder-pick-field-amount')
        .click()
    await chooseOption(p, 'report-builder-filter-0-op', 'gte')
    await p.getByTestId('report-builder-filter-0-value').fill(amount)
}

async function addSaleElsewhere(
    browser: Browser,
    user: { username: string; password: string },
    team: string
) {
    const other = await browser.newContext()
    try {
        const q = await other.newPage()
        await loginAs(q, user.username, user.password)
        await openReportStub(q)
        await addSale(q, { team, customer: 'Earlier', amount: 2000, cost: 0, day: dayOf(0) })
    } finally {
        await other.close()
    }
}

test.describe('where a report run starts', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a run with new params starts at the top of the report', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rpttop')
        try {
            await seedAskedAmount(p)
            await expectNewParamsStartAtTop(p)
        } finally {
            await close()
        }
    })

    test('on a phone a run with new params starts at the top of the report', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rpttopph')
        try {
            await seedAskedAmount(p)
            await p.setViewportSize({ width: 390, height: 844 })
            // The rows wrap on a phone: the last column (quantity) cannot share line 1
            // when one line does not fit, so it sits under the label.
            const first = rowOf(p, topName(0))
            const id = ((await first.getAttribute('data-testid')) ?? '').replace('report-row-', '')
            const quantity = p.getByTestId(`report-cell-quantity-${id}`)
            await expect
                .poll(async () => {
                    const label = await first.boundingBox()
                    const cell = await quantity.boundingBox()
                    return label && cell ? cell.y >= label.y + label.height - 1 : false
                })
                .toBe(true)
            await expectNewParamsStartAtTop(p)
        } finally {
            await close()
        }
    })

    // Top 20 has 19 or more rows under it in both runs below, so it can reach the
    // list top and is there in the next run for the list to hold.
    test('in the builder a new sort starts the preview at the top', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptbsort')
        try {
            await seedBuilderDraft(p)
            await toListTop(p, rowOf(p, topName(20)))
            expect(await scrollOffset(p)).toBeGreaterThan(0)
            await landNewRun(p, () => sortZtoA(p))
            await expectAtTop(p, 39)
        } finally {
            await close()
        }
    })

    test('in the builder a new filter condition starts the preview at the top', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptbfilt')
        try {
            await seedBuilderDraft(p)
            await toListTop(p, rowOf(p, topName(20)))
            expect(await scrollOffset(p)).toBeGreaterThan(0)
            await landNewRun(p, () => filterAmountFrom(p, '10.05'))
            await expectAtTop(p, 5)
            await expect(rowOf(p, topName(4))).toHaveCount(0)
        } finally {
            await close()
        }
    })

    test('a live re-run of the same params keeps the row in view in place', async ({
        page,
        browser,
    }) => {
        const { inviteePage: p, user, close } = await createInvitedUser(page, 'rptkeep')
        try {
            const team = await seedAskedAmount(p)
            await landNewRun(p, () => setMinAmount(p, '10.05'))
            const target = rowOf(p, topName(20))
            await toListTop(p, target)
            const offset = await scrollOffset(p)
            expect(offset).toBeGreaterThan(0)
            const before = (await target.boundingBox())?.y ?? -1
            // "Earlier" sorts before every Top sale and passes the filter, so the
            // re-run adds a row above the one in view.
            const rerun = p.waitForResponse('**/api/reports/run')
            await addSaleElsewhere(browser, user, team)
            await rerun
            await expect
                .poll(() => scrollOffset(p), 'the list moved down by the added row')
                .toBeGreaterThan(offset)
            await expect
                .poll(async () => Math.abs(((await target.boundingBox())?.y ?? -999) - before))
                .toBeLessThanOrEqual(2)
            await expect(target).toBeInViewport()
        } finally {
            await close()
        }
    })
})
