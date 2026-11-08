import { expect, type Locator, type Page, test } from '@playwright/test'
import { createInvitedUser, login, skipWithoutReportsStub } from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    cellOf,
    chooseOption,
    groupWithTeamRegion,
    listScroller,
    openReportStub,
    openReportsHub,
    rowOf,
    SALES_SOURCE,
    saveNewReport,
    scrollRowTo,
    startReport,
    startTeamSales,
    uniqueName,
} from './reports-helpers'

/**
 * E14: on a phone a row too wide for one line wraps: as many columns as fit on
 * each line, each under its heading (the headings wrap the same way), and nothing
 * scrolls sideways; an open group's row sticks while its rows scroll under it.
 * Desktop keeps one line per row, narrowing text columns before it wraps, never
 * scrolls sideways, and the heading row stays at the top. An author arrangement
 * for phones places columns on chosen lines.
 */

async function seed(p: Page, sales: number) {
    const team = uniqueName('Team')
    await openReportStub(p)
    await addTeam(p, team)
    for (let i = 0; i < sales; i++) {
        await addSale(p, {
            team,
            customer: `Wrapco ${String(i).padStart(2, '0')}`,
            amount: 1000 + i,
            cost: 400,
            quantity: 3,
            day: '2026-01-05',
            status: i % 2 === 0 ? 'open' : 'paid',
        })
    }
}

async function wideReport(p: Page, title: string, grouped: boolean) {
    await openReportsHub(p)
    await startReport(p, SALES_SOURCE)
    await addColumn(p, ['amount'])
    await addColumn(p, ['cost'])
    await addColumn(p, ['quantity'])
    await p.getByTestId('report-builder-section-group').click()
    if (grouped) {
        await addColumn(p, ['status'])
        await p.getByTestId('report-builder-group-add').click()
        await chooseOption(p, 'report-builder-group-0-col', 'status')
        await p.getByTestId('report-builder-group-0-open').click()
    }
    // Sorted, so "Wrapco 00" is the first row (the first of the open Open group
    // when grouped), the one in view at first; unsorted rows come in record order.
    await chooseOption(p, 'report-builder-sort-add-menu', 'customer')
    await saveNewReport(p, title)
}

async function boxes(p: Page, row: string) {
    const label = rowOf(p, row)
    const id = (await label.getAttribute('data-testid'))?.replace('report-row-', '') ?? ''
    // A column the report does not have (status, when not grouped) has no box;
    // boundingBox would wait for it, and stall the poll that reads these.
    const box = async (testId: string) => {
        const locator = p.getByTestId(testId)
        return (await locator.count()) > 0 ? locator.boundingBox() : null
    }
    return {
        label: await label.boundingBox(),
        amount: await box(`report-cell-amount-${id}`),
        cost: await box(`report-cell-cost-${id}`),
        status: await box(`report-cell-status-${id}`),
        headStatus: await box('report-heading-status'),
        headLabel: await box('report-heading-customer'),
        headAmount: await box('report-heading-amount'),
        headCost: await box('report-heading-cost'),
    }
}

async function bottomOf(locator: Locator): Promise<number> {
    const box = await locator.boundingBox()
    return (box?.y ?? 0) + (box?.height ?? 0)
}

// The open status group and the open day group inside it are stuck as one stack
// while the day's rows scroll under them. The next day's row then pushes the
// stack up whole: halfway in, the stack is half gone and its bottom sits on that
// row's top, so it never draws over the row.
async function expectWholeStackPushed(p: Page, stackIds: string[]) {
    const sticky = p.getByTestId('report-sticky-group')
    const next = rowOf(p, '2026-01-06')
    await scrollRowTo(rowOf(p, 'Push 04'))
    // The stack holds both groups' rows (the copies carry their own test ids).
    for (const id of stackIds) {
        await expect(sticky.getByTestId(`report-sticky-row-${id}`)).toBeVisible()
    }
    const headingBottom = await bottomOf(p.getByTestId('report-heading-row'))
    await expect
        .poll(async () => Math.abs(((await sticky.boundingBox())?.y ?? 0) - headingBottom))
        .toBeLessThanOrEqual(2)
    const height = (await sticky.boundingBox())?.height ?? 0
    const dayRow = (await rowOf(p, '2026-01-05').boundingBox())?.height ?? 0
    expect(height).toBeGreaterThan(dayRow + 10)

    await scrollRowTo(next, height / 2)
    await expect
        .poll(async () => {
            const stack = await sticky.boundingBox()
            const row = await next.boundingBox()
            if (!stack || !row) return Number.POSITIVE_INFINITY
            return Math.abs(stack.y + stack.height - row.y)
        })
        .toBeLessThanOrEqual(2)
    const stack = await sticky.boundingBox()
    expect(Math.abs((stack?.y ?? 0) - (headingBottom - height / 2))).toBeLessThanOrEqual(2)
}

// A finger drag from (x, y) up by `distance` px, sent as raw touch points. The
// browser itself turns them into a pan (hit test, touch-action, touch listeners,
// scroll target), as it does for a touchscreen. Input.synthesizeScrollGesture's
// touch source is not used: in headless Chromium on Linux it scrolls nothing,
// not even a bare overflow-y:auto div, so it cannot tell a blocked list from a
// working one.
async function swipeUp(p: Page, x: number, y: number, distance: number) {
    const cdp = await p.context().newCDPSession(p)
    const steps = 20
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x, y }] })
    for (let i = 1; i <= steps; i++) {
        await cdp.send('Input.dispatchTouchEvent', {
            type: 'touchMove',
            touchPoints: [{ x, y: y - (distance * i) / steps }],
        })
    }
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] })
    await cdp.detach()
}

// A row the list scrolled up under the heading row, or one it no longer holds.
async function isScrolledAway(row: Locator, headingBottom: number): Promise<boolean> {
    if ((await row.count()) === 0) return true
    return (await bottomOf(row)) <= headingBottom + 1
}

test.describe('report rows per width', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('on a phone a wide row wraps under aligned wrapped headings, with no sideways scroll, and an open group sticks', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptwrap')
        try {
            await seed(p, 24)
            await wideReport(p, uniqueName('Wrap'), true)
            await p.setViewportSize({ width: 390, height: 844 })
            // Every layout read in one snapshot: the wrap, the alignment under the
            // headings and the page's width settle together.
            await expect
                .poll(async () => {
                    const b = await boxes(p, 'Wrapco 00')
                    const width = await p.evaluate(() => document.documentElement.scrollWidth)
                    if (!b.label || !b.amount || !b.cost || !b.status) return null
                    return {
                        // The last column cannot share line 1 when one line does not fit.
                        wrapped: b.status.y >= b.label.y + b.label.height - 1,
                        amountUnderHeading: Math.abs((b.headAmount?.x ?? 0) - b.amount.x) <= 1,
                        costUnderHeading: Math.abs((b.headCost?.x ?? 0) - b.cost.x) <= 1,
                        statusUnderHeading: Math.abs((b.headStatus?.x ?? 0) - b.status.x) <= 1,
                        headingsWrapped: (b.headStatus?.y ?? 0) > (b.headLabel?.y ?? 0),
                        fitsPhone: width <= 390,
                    }
                })
                .toEqual({
                    wrapped: true,
                    amountUnderHeading: true,
                    costUnderHeading: true,
                    statusUnderHeading: true,
                    headingsWrapped: true,
                    fitsPhone: true,
                })

            // D6: the wrapped line is shorter than the first (not a press target), and it
            // starts under the label's text (review §7.1-6).
            const id =
                (await rowOf(p, 'Wrapco 00').getAttribute('data-testid'))?.replace(
                    'report-row-',
                    ''
                ) ?? ''
            // The first cell of the wrapped line: quantity, which shares line 2 with status.
            const first = p.getByTestId(`report-cell-quantity-${id}`)
            const labelText = rowOf(p, 'Wrapco 00').getByText('Wrapco 00', { exact: true })
            await expect
                .poll(async () => {
                    const box = await first.boundingBox()
                    const text = await first.locator('div[dir="auto"]').boundingBox()
                    const label = await labelText.boundingBox()
                    if (!box || !text || !label) return null
                    return {
                        compact: box.height <= 30,
                        underLabel: Math.abs(text.x - label.x) <= 1,
                    }
                })
                .toEqual({ compact: true, underLabel: true })
            const headingRow = await p.getByTestId('report-heading-row').boundingBox()
            expect(headingRow?.height ?? 0).toBeLessThan(2 * 44)

            // Scroll the open group's rows: its own row goes up under the heading row,
            // and its stuck copy stays just under the heading row.
            const headingBottom = await bottomOf(p.getByTestId('report-heading-row'))
            await p.getByTestId('report-table').hover()
            // Far enough to scroll the group's own row away, not so far that the next
            // group (the compact lines make the Open group shorter) pushes the copy up.
            await p.mouse.wheel(0, 400)
            const sticky = p.getByTestId('report-sticky-group')
            await expect(sticky).toContainText('Open')
            await expect.poll(() => isScrolledAway(rowOf(p, 'Open'), headingBottom)).toBe(true)
            await expect
                .poll(async () => Math.abs(((await sticky.boundingBox())?.y ?? 0) - headingBottom))
                .toBeLessThanOrEqual(2)
        } finally {
            await close()
        }
    })

    test('on a desktop a row stays one line and the heading row stays at the top', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptline')
        try {
            await seed(p, 24)
            await wideReport(p, uniqueName('Line'), false)
            await expect
                .poll(async () => {
                    const b = await boxes(p, 'Wrapco 00')
                    return b.label && b.amount ? Math.abs(b.label.y - b.amount.y) : 99
                })
                .toBeLessThanOrEqual(1)
            const before = await p.getByTestId('report-heading-row').boundingBox()
            await p.getByTestId('report-table').hover()
            await p.mouse.wheel(0, 900)
            // FlashList keeps a row it scrolled away mounted for reuse, so the row is
            // scrolled out of view rather than gone.
            await expect(rowOf(p, 'Wrapco 00')).not.toBeInViewport()
            await expect
                .poll(async () => (await p.getByTestId('report-heading-row').boundingBox())?.y)
                .toBe(before?.y)
        } finally {
            await close()
        }
    })

    test('an author arrangement for phones puts a cell spanning two lines in the middle of the row', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptarr')
        try {
            await seed(p, 1)
            await wideReport(p, uniqueName('Arrange'), false)
            await p.getByTestId('report-edit-open').click()
            await p.getByTestId('report-builder-section-arrange').click()
            await p.getByTestId('report-builder-arrange-width-mobile').click()
            await p.getByTestId('report-builder-arrange-on').click()
            // A cell that sets no alignment shows the one the table gives it.
            await expect(p.getByTestId('report-builder-arrange-amount-justify')).toContainText(
                'Align end'
            )
            await expect(p.getByTestId('report-builder-arrange-customer-justify')).toContainText(
                'Align start'
            )
            await chooseOption(p, 'report-builder-arrange-tracks', '3')
            // Line 1: customer · quantity · amount; line 2: (empty) · quantity · cost.
            await chooseOption(p, 'report-builder-arrange-customer-span', '1')
            await chooseOption(p, 'report-builder-arrange-amount-line', '1')
            await chooseOption(p, 'report-builder-arrange-amount-start', '3')
            await chooseOption(p, 'report-builder-arrange-cost-line', '2')
            await chooseOption(p, 'report-builder-arrange-cost-start', '3')
            await chooseOption(p, 'report-builder-arrange-quantity-line', '1')
            await chooseOption(p, 'report-builder-arrange-quantity-start', '2')
            await chooseOption(p, 'report-builder-arrange-quantity-lines', '2')
            await p.getByTestId('report-builder-save').click()
            await expect(p.getByTestId('report-builder')).toBeHidden()
            await p.setViewportSize({ width: 390, height: 844 })
            const id = async () =>
                (await rowOf(p, 'Wrapco 00').getAttribute('data-testid'))?.replace(
                    'report-row-',
                    ''
                ) ?? ''
            await expect
                .poll(async () => {
                    const b = await boxes(p, 'Wrapco 00')
                    const quantity = await p
                        .getByTestId(`report-cell-quantity-${await id()}`)
                        .boundingBox()
                    if (!b.label || !b.amount || !b.cost || !quantity) return false
                    const between =
                        quantity.x > b.label.x && quantity.x + quantity.width <= b.amount.x + 1
                    const spans =
                        quantity.y <= b.amount.y + 1 &&
                        quantity.y + quantity.height >= b.cost.y + b.cost.height - 1
                    return (
                        Math.abs(b.label.y - b.amount.y) <= 1 &&
                        b.cost.y > b.amount.y &&
                        between &&
                        spans
                    )
                })
                .toBe(true)
        } finally {
            await close()
        }
    })

    test('an open group stays stuck only through its own section', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptsect')
        try {
            await seed(p, 16)
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-1-aggregate', 'sum')
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'customer')
            // No subtotal rows, so an open group's row keeps its figures.
            await p.getByTestId('report-builder-group-0-subtotals').click()
            await saveNewReport(p, uniqueName('Sections'))
            const first = rowOf(p, 'Wrapco 00')
            await first.click()
            await expect(first).toHaveAttribute('aria-expanded', 'true')

            // A little scroll: the open group's row goes up under the heading row, and
            // its stuck copy, figures included, stays just under the heading row.
            const headingBottom = await bottomOf(p.getByTestId('report-heading-row'))
            const sticky = p.getByTestId('report-sticky-group')
            await p.getByTestId('report-table').hover()
            await p.mouse.wheel(0, 30)
            await expect(sticky).toContainText('Wrapco 00')
            await expect(sticky).toContainText('10.00')
            await expect
                .poll(async () => (await first.boundingBox())?.y ?? 0)
                .toBeLessThan(headingBottom)
            await expect
                .poll(async () => Math.abs(((await sticky.boundingBox())?.y ?? 0) - headingBottom))
                .toBeLessThanOrEqual(2)

            // Into the closed groups after it: no open group is above, so nothing sticks.
            await p.mouse.wheel(0, 200)
            await expect(sticky).toHaveCount(0)
        } finally {
            await close()
        }
    })

    test('the next section pushes the whole stuck stack of open groups, on a desktop and a phone', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptpush')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            // One day with ten sales, then ten days with one each: the rows after
            // the open day leave room to scroll its section away.
            for (let i = 0; i < 20; i++) {
                const day = `2026-01-${String(Math.max(i - 4, 5)).padStart(2, '0')}`
                await addSale(p, {
                    team,
                    customer: `Push ${String(i).padStart(2, '0')}`,
                    amount: 1000 + i,
                    cost: 400,
                    quantity: 3,
                    day,
                })
            }
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-1-aggregate', 'sum')
            await addColumn(p, ['cost'])
            await addColumn(p, ['quantity'])
            await addColumn(p, ['status'])
            await addColumn(p, ['booked'])
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'status')
            await p.getByTestId('report-builder-group-0-subtotals').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-1-col', 'booked')
            await p.getByTestId('report-builder-group-1-subtotals').click()
            // Sorted, so the day's rows come in a known order: Push 04 has five rows
            // under it before the next day's row.
            await chooseOption(p, 'report-builder-sort-add-menu', 'customer')
            await saveNewReport(p, uniqueName('Push'))
            const stackIds: string[] = []
            for (const label of ['Open', '2026-01-05']) {
                const row = rowOf(p, label)
                await row.click()
                await expect(row).toHaveAttribute('aria-expanded', 'true')
                stackIds.push(
                    (await row.getAttribute('data-testid'))?.replace('report-row-', '') ?? ''
                )
            }
            await expect(rowOf(p, 'Push 09')).toBeVisible()

            await expectWholeStackPushed(p, stackIds)

            // On a phone the rows wrap, so the stack is drawn by other code. The last
            // column (booked) cannot share line 1 when one line does not fit.
            await p.setViewportSize({ width: 390, height: 844 })
            const pushRow = rowOf(p, 'Push 09')
            await expect
                .poll(async () => {
                    const id =
                        (await pushRow.getAttribute('data-testid'))?.replace('report-row-', '') ??
                        ''
                    const label = await pushRow.boundingBox()
                    const booked = await p.getByTestId(`report-cell-booked-${id}`).boundingBox()
                    return label && booked ? booked.y >= label.y + label.height - 1 : false
                })
                .toBe(true)
            await expectWholeStackPushed(p, stackIds)
        } finally {
            await close()
        }
    })

    test('on a desktop a wide table fits its columns to their content and never scrolls sideways', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptside')
        try {
            await seed(p, 1)
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            for (const keys of [
                ['amount'],
                ['cost'],
                ['quantity'],
                ['currency'],
                ['day'],
                ['booked'],
                ['status'],
                ['team', 'name'],
            ]) {
                await addColumn(p, keys)
            }
            await saveNewReport(p, uniqueName('Narrowed'))
            // A scroll bar wider than the usual 15-16 px: the line must fit the width
            // the rows really get, whatever the gutter is.
            await p.addStyleTag({ content: '::-webkit-scrollbar { width: 21px }' })
            await p.setViewportSize({ width: 1100, height: 720 })
            const label = rowOf(p, 'Wrapco 00')
            await expect(label).toBeVisible()
            const table = p.getByTestId('report-table')
            // No heading and no row part reaches into its scroller's gutter.
            const intoGutter = () =>
                table.evaluate(element => {
                    const past = (scroller: Element, parts: Element[]) => {
                        const edge =
                            scroller.getBoundingClientRect().left +
                            scroller.clientLeft +
                            scroller.clientWidth
                        return parts.filter(el => el.getBoundingClientRect().right > edge + 0.5)
                            .length
                    }
                    const headingRow = element.querySelector('[data-testid="report-heading-row"]')
                    const firstRow = element.querySelector('[data-testid^="report-row-"]')
                    let list = firstRow?.parentElement ?? null
                    while (list && !['auto', 'scroll'].includes(getComputedStyle(list).overflowY)) {
                        list = list.parentElement
                    }
                    if (!headingRow || !list) return -1
                    return (
                        past(headingRow, [...headingRow.querySelectorAll('*')]) +
                        past(list, [...list.querySelectorAll('[data-testid^="report-row-"] *')])
                    )
                })
            await expect.poll(intoGutter).toBe(0)
            // The table is shown once the rows' width is measured.
            await expect(table).toHaveCSS('opacity', '1')
            // Every heading ends inside the table, on one line.
            const headings = () =>
                table.evaluate(element => {
                    const box = element.getBoundingClientRect()
                    const cells = [
                        ...element.querySelectorAll('[data-testid^="report-heading-"]'),
                    ].filter(el => el.getAttribute('data-testid') !== 'report-heading-row')
                    const tops = new Set(
                        cells.map(el => Math.round(el.getBoundingClientRect().top))
                    )
                    return {
                        count: cells.length,
                        outside: cells.filter(
                            el => el.getBoundingClientRect().right > box.right + 1
                        ).length,
                        lines: tops.size,
                    }
                })
            await expect.poll(headings).toEqual({ count: 9, outside: 0, lines: 1 })
            // Nothing moves sideways: the amount heading's place and every scrollLeft,
            // read together in one poll after the wheel. The place before the wheel is
            // read once the headings have settled (the poll above).
            const x = async (testId: string) => (await p.getByTestId(testId).boundingBox())?.x ?? 0
            const before = await x('report-heading-amount')
            await table.hover()
            await p.mouse.wheel(300, 0)
            await expect
                .poll(async () => ({
                    scrolledSideways: await table.evaluate(element =>
                        [element, ...element.querySelectorAll('*')].some(el => el.scrollLeft !== 0)
                    ),
                    headingMoved: Math.abs((await x('report-heading-amount')) - before) > 1,
                }))
                .toEqual({ scrolledSideways: false, headingMoved: false })
            // Every figure keeps its whole value and heading (R12); the short label is whole.
            const isCut = (testId: string) =>
                p.getByTestId(testId).evaluate(el => {
                    const text = el.querySelector('div[dir="auto"]') ?? el
                    return text.scrollWidth > text.clientWidth
                })
            const id = (await label.getAttribute('data-testid'))?.replace('report-row-', '') ?? ''
            for (const col of ['amount', 'cost', 'quantity', 'day', 'booked']) {
                await expect.poll(() => isCut(`report-cell-${col}-${id}`)).toBe(false)
                await expect.poll(() => isCut(`report-heading-${col}`)).toBe(false)
            }
            const text = label.getByText('Wrapco 00', { exact: true })
            expect(await text.evaluate(el => el.scrollWidth > el.clientWidth)).toBe(false)
        } finally {
            await close()
        }
    })

    // F10: an open group row has no other cell on its line, so its label spans the
    // row, as it does in the phone's blocks; a detail label gets its text's width.
    test('on a desktop line an open group row shows its whole label across the row', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptspan')
        try {
            const team = uniqueName('Northwest Regional Wholesale Accounts Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, {
                team,
                customer: 'Wrapco Longname 00',
                amount: 1000,
                cost: 400,
                quantity: 3,
                day: '2026-01-05',
            })
            await openReportsHub(p)
            await startTeamSales(p)
            for (const keys of [['cost'], ['quantity'], ['currency'], ['day'], ['booked']]) {
                await addColumn(p, keys)
            }
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'team')
            // Subtotals stay on, so the open group's figures are on its subtotal row
            // and its own row has no cell after the label.
            await p.getByTestId('report-builder-group-0-open').click()
            await saveNewReport(p, uniqueName('Spanned'))
            await p.setViewportSize({ width: 1100, height: 720 })
            const table = p.getByTestId('report-table')
            await expect(table).toHaveCSS('opacity', '1')
            const groupRow = rowOf(p, team)
            await expect(groupRow).toHaveAttribute('aria-expanded', 'true')
            const detail = rowOf(p, 'Wrapco Longname 00')
            await expect(detail).toBeVisible()
            const isCut = (text: Locator) => text.evaluate(el => el.scrollWidth > el.clientWidth)
            // The detail label gets the room its text needs.
            await expect
                .poll(() => isCut(detail.getByText('Wrapco Longname 00', { exact: true })))
                .toBe(false)
            const detailWidth = (await detail.boundingBox())?.width ?? 0
            // The group label is whole, in a box wider than the label column.
            await expect.poll(() => isCut(groupRow.getByText(team, { exact: true }))).toBe(false)
            expect((await groupRow.boundingBox())?.width ?? 0).toBeGreaterThan(detailWidth + 200)
            // The detail row's cells keep their columns.
            const amount = await cellOf(p, 'amount', 'Wrapco Longname 00')
            const headAmount = await p.getByTestId('report-heading-amount').boundingBox()
            expect(
                Math.abs(((await amount.boundingBox())?.x ?? 0) - (headAmount?.x ?? 0))
            ).toBeLessThanOrEqual(1)
        } finally {
            await close()
        }
    })

    // F10: a closed group row keeps its figures. Its label spans only the empty
    // cells before its first figure, and each figure stays under its heading.
    test('on a desktop line a closed group row spans its label up to its first figure, and its figures keep their columns', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptspanc')
        try {
            await seed(p, 2)
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['day'])
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-2-aggregate', 'sum')
            await addColumn(p, ['cost'])
            await chooseOption(p, 'report-builder-column-3-aggregate', 'sum')
            // A group level picks one of the report's columns.
            await addColumn(p, ['status'])
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'status')
            await saveNewReport(p, uniqueName('Closed span'))
            const table = p.getByTestId('report-table')
            await expect(table).toHaveCSS('opacity', '1')
            const group = rowOf(p, 'Open')
            await expect(group).toHaveAttribute('aria-expanded', 'false')
            const amount = await cellOf(p, 'amount', 'Open')
            await expect(amount).toHaveText('10.00')
            // The day cell, empty on a group row, is not drawn: the label takes its place.
            await expect(await cellOf(p, 'day', 'Open')).toHaveCount(0)
            const box = async (locator: Locator) =>
                (await locator.boundingBox()) ?? { x: 0, y: 0, width: 0, height: 0 }
            const label = await box(group)
            const headDay = await box(p.getByTestId('report-heading-day'))
            const headAmount = await box(p.getByTestId('report-heading-amount'))
            const headCost = await box(p.getByTestId('report-heading-cost'))
            // The label box ends where the first figure starts, past the day column.
            expect(label.x + label.width).toBeGreaterThanOrEqual(headDay.x + headDay.width - 1)
            expect(Math.abs((await box(amount)).x - headAmount.x)).toBeLessThanOrEqual(1)
            const cost = await cellOf(p, 'cost', 'Open')
            expect(Math.abs((await box(cost)).x - headCost.x)).toBeLessThanOrEqual(1)
            // D1: a fold never re-measures. Once the groups are open the measure holds
            // the details' widths; folding them all hides every detail row, and the
            // columns stay where they were.
            const fold = p.getByTestId('report-fold-all')
            await expect(fold).toHaveText('Expand all')
            await fold.click()
            await expect(fold).toHaveText('Collapse all')
            await expect(group).toHaveAttribute('aria-expanded', 'true')
            await expect(table).toHaveCSS('opacity', '1')
            const opened = await box(p.getByTestId('report-heading-amount'))
            await fold.click()
            await expect(fold).toHaveText('Expand all')
            await expect(group).toHaveAttribute('aria-expanded', 'false')
            await expect
                .poll(async () =>
                    Math.abs((await box(p.getByTestId('report-heading-amount'))).x - opened.x)
                )
                .toBeLessThanOrEqual(1)
        } finally {
            await close()
        }
    })

    test('an arrangement names its problems in the builder, and a pivot that cannot follow it says so', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptpivn')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            for (const status of ['open', 'paid'] as const) {
                const customer = `Pivco ${status}`
                await addSale(p, {
                    team,
                    customer,
                    amount: 100,
                    cost: 40,
                    day: '2026-01-05',
                    status,
                })
            }
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-1-aggregate', 'sum')
            await addColumn(p, ['cost'])
            await chooseOption(p, 'report-builder-column-2-aggregate', 'sum')
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'customer')
            await p.getByTestId('report-builder-section-pivot').click()
            await p.getByTestId('report-builder-pivot-on').click()
            await p.getByTestId('report-builder-pivot-field').click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-status')
                .click()
            await chooseOption(p, 'report-builder-pivot-cell', 'amount')

            await p.getByTestId('report-builder-section-arrange').click()
            await p.getByTestId('report-builder-arrange-width-mobile').click()
            await p.getByTestId('report-builder-arrange-on').click()
            await chooseOption(p, 'report-builder-arrange-tracks', '2')
            await chooseOption(p, 'report-builder-arrange-customer-span', '1')
            await chooseOption(p, 'report-builder-arrange-amount-line', '1')
            // On the customer's track: the cell says why, and Save is refused.
            await chooseOption(p, 'report-builder-arrange-cost-line', '1')
            await chooseOption(p, 'report-builder-arrange-cost-start', '1')
            await expect(p.getByTestId('report-builder-arrange-cost-problem')).toHaveText(
                'the cell overlaps another'
            )
            await p.getByTestId('report-builder-title').fill(uniqueName('Pivot arrange'))
            await p.getByTestId('report-builder-save').click()
            await expect(p.getByTestId('report-builder-arrange-cost-problem')).toBeVisible()
            await expect(p.getByTestId('report-builder')).toBeVisible()

            // Line 2, track 1: the entry is valid, but the second pivot column wraps
            // onto that track.
            await chooseOption(p, 'report-builder-arrange-cost-line', '2')
            await expect(p.getByTestId('report-builder-arrange-cost-problem')).toHaveCount(0)
            await saveNewReport(p, uniqueName('Pivot arrange'))
            await p.setViewportSize({ width: 390, height: 844 })
            await expect(p.getByTestId('report-layout-notice')).toBeVisible()
        } finally {
            await close()
        }
    })

    // E2: in the wrapped phone rows (no arrangement), a column left blank on opened
    // rows is an empty slot under its heading, on its line beside the amounts; the
    // group row still shows the value.
    test('on a phone a column left blank on opened rows keeps its empty slot', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptblankph')
        try {
            const team = uniqueName('BlankPh')
            await openReportStub(p)
            await addTeam(p, team, 'south')
            await addSale(p, { team, customer: 'Ada', amount: 1000, cost: 0, day: '2026-01-05' })
            await openReportsHub(p)
            await startTeamSales(p)
            await groupWithTeamRegion(p)
            // Two more columns: the row must not fit one line at 390, and this test is
            // about the wrapped rows.
            await addColumn(p, ['cost'])
            await addColumn(p, ['quantity'])
            await expect(p.getByTestId('report-builder-column-3-detail')).toBeVisible()
            await chooseOption(p, 'report-builder-column-3-detail', 'blank')
            await saveNewReport(p, uniqueName('Blank phone'))
            await p.setViewportSize({ width: 390, height: 844 })
            await rowOf(p, team).click()
            await expect(rowOf(p, team)).toHaveAttribute('aria-expanded', 'true')

            const amount = await cellOf(p, 'amount', 'Ada')
            const cost = await cellOf(p, 'cost', 'Ada')
            const region = await cellOf(p, 'formula', 'Ada')
            // The blank slot sits under its heading, on a line with a figure of its row.
            await expect
                .poll(async () => {
                    const heading = await p.getByTestId('report-heading-formula').boundingBox()
                    const slot = await region.boundingBox()
                    const figures = [await amount.boundingBox(), await cost.boundingBox()]
                    if (!heading || !slot) return null
                    return {
                        underHeading: Math.abs(heading.x - slot.x) <= 1,
                        besideFigure: figures.some(f => f && Math.abs(f.y - slot.y) <= 1),
                    }
                })
                .toEqual({ underHeading: true, besideFigure: true })
            await expect(amount).toHaveText('10.00')
            await expect(await cellOf(p, 'formula', team)).toHaveText('South')
            await expect(region).toBeVisible()
            await expect(region).toHaveText('')
            expect(
                await p.evaluate(() => document.documentElement.scrollWidth)
            ).toBeLessThanOrEqual(390)
        } finally {
            await close()
        }
    })

    // Review §7.1-2 and H7: a subtotal row's label spans its empty cells on one line,
    // and on a phone it moves beside its figure when the empty tracks hold it.
    test('a subtotal row shows its whole label on a desktop line and beside its figure on a phone', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptsubspan')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, {
                team,
                customer: 'Ada',
                amount: 1000,
                cost: 400,
                quantity: 3,
                day: '2026-01-05',
            })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['quantity'])
            await addColumn(p, ['cost'])
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-3-aggregate', 'sum')
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'customer')
            await p.getByTestId('report-builder-group-0-open').click()
            // Phone: the label across line 1, then quantity · cost · amount.
            await p.getByTestId('report-builder-section-arrange').click()
            await p.getByTestId('report-builder-arrange-width-mobile').click()
            await p.getByTestId('report-builder-arrange-on').click()
            await chooseOption(p, 'report-builder-arrange-tracks', '3')
            await chooseOption(p, 'report-builder-arrange-customer-span', '3')
            await chooseOption(p, 'report-builder-arrange-quantity-line', '2')
            await chooseOption(p, 'report-builder-arrange-cost-line', '2')
            await chooseOption(p, 'report-builder-arrange-amount-line', '2')
            await saveNewReport(p, uniqueName('Subtotal span'))

            const subtotal = rowOf(p, 'Total Ada')
            const amount = await cellOf(p, 'amount', 'Total Ada')
            const isCut = () =>
                subtotal
                    .getByText('Total Ada', { exact: true })
                    .evaluate(el => el.scrollWidth > el.clientWidth)
            await p.setViewportSize({ width: 1280, height: 800 })
            await expect(p.getByTestId('report-table')).toHaveCSS('opacity', '1')
            await expect.poll(isCut).toBe(false)

            await p.setViewportSize({ width: 390, height: 844 })
            await expect
                .poll(async () => {
                    const label = await subtotal.boundingBox()
                    const figure = await amount.boundingBox()
                    return label && figure ? Math.abs(label.y - figure.y) <= 1 : false
                })
                .toBe(true)
            await expect.poll(isCut).toBe(false)
        } finally {
            await close()
        }
    })
})

test.describe('report rows on a touch phone', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')
    test.use({ hasTouch: true })

    test('the table keeps the browser touch gestures, and a swipe scrolls its rows', async ({
        page,
    }) => {
        await login(page)
        await seed(page, 16)
        await wideReport(page, uniqueName('Touch'), false)
        await page.setViewportSize({ width: 390, height: 844 })
        // The swipe goes to the phone layout only: the table is the phone's width and
        // its headings have wrapped, the last column (quantity) on a line under the
        // label's, as it cannot share line 1 when one line does not fit.
        const table = page.getByTestId('report-table')
        await expect
            .poll(async () => Math.abs(((await table.boundingBox())?.width ?? 0) - 390))
            .toBeLessThanOrEqual(1)
        await expect
            .poll(async () => {
                const label = await page.getByTestId('report-heading-customer').boundingBox()
                const last = await page.getByTestId('report-heading-quantity').boundingBox()
                return label && last ? last.y >= label.y + label.height - 1 : false
            })
            .toBe(true)
        const headingBottom = await bottomOf(page.getByTestId('report-heading-row'))

        // The shared sign-in also owns other specs' sales, which sort before or after
        // these, so the row to swipe away is the table's first, not a named one. It is
        // held by its own test id: a later lookup of "the first row" finds another.
        const firstId =
            (await table
                .locator('[data-testid^="report-row-"]')
                .first()
                .getAttribute('data-testid')) ?? ''
        const first = page.getByTestId(firstId)
        await expect
            .poll(async () => Math.abs(((await first.boundingBox())?.y ?? 0) - headingBottom))
            .toBeLessThanOrEqual(1)
        const scroller = await listScroller(first)
        expect(await scroller.evaluate(list => list.scrollHeight > list.clientHeight)).toBe(true)

        // Nothing from a row up to the table turns touch gestures (zoom, pan) off.
        const blocked = await first.evaluate(element => {
            const found: string[] = []
            for (let node: Element | null = element; node; node = node.parentElement) {
                const testId = node.getAttribute('data-testid')
                if (getComputedStyle(node).touchAction === 'none')
                    found.push(testId ?? node.tagName)
                if (testId === 'report-table') break
            }
            return found
        })
        expect(blocked).toEqual([])

        const bounds = await table.boundingBox()
        await swipeUp(page, (bounds?.x ?? 0) + 100, (bounds?.y ?? 0) + 500, 400)
        await expect
            .poll(() => scroller.evaluate(list => list.scrollTop), {
                message: 'the swipe scrolls the list',
            })
            .toBeGreaterThan(0)
        await expect.poll(() => isScrolledAway(first, headingBottom)).toBe(true)
    })
})
