import { expect, type Page, test } from '@playwright/test'
import { login, navigateToPackage, skipWithoutFormsStub } from './helpers'

/**
 * A record form with a table built-in, end to end, against the forms-host
 * stub's "Order" host (tests/scripts/scaffold-forms-host-stub.ts): lines with
 * an Item cell the host draws (picking an item sets the rate), Qty, Rate and a
 * computed Amount, and a computed Subtotal. Everything is driven through the
 * UI. The reload at the end proves the rows reached the server, not only the
 * local store; it is not in-app navigation.
 */

async function rowIds(page: Page): Promise<string[]> {
    const ids = await page
        .locator('[data-testid^="form-table-lines-row-"]')
        .evaluateAll(els => els.map(el => el.getAttribute('data-testid') ?? ''))
    return ids.map(id => id.replace('form-table-lines-row-', ''))
}

async function openOrder(page: Page, name: string) {
    await page.getByTestId('forms-stub-order-list').getByText(name, { exact: true }).click()
    await expect(page.getByTestId('form-input-lines')).toBeVisible()
}

test.describe('forms table', () => {
    test.skip(skipWithoutFormsStub(), 'forms-host-stub not scaffolded in this workspace')

    test('adds, fills, reorders by keyboard, removes and saves rows', async ({ page }) => {
        const name = `Order ${Date.now()}`
        await login(page)
        await navigateToPackage(page, 'forms-stub')

        await page.getByTestId('forms-stub-order-new').click()
        await page.getByTestId('form-input-name').fill(name)
        await page.getByTestId('form-table-lines-add').click()
        await page.getByTestId('form-table-lines-add').click()
        const [first, second] = await rowIds(page)

        await page.getByTestId(`forms-stub-item-${first}-widget`).click()
        await page.getByTestId(`form-table-lines-${first}-qty`).fill('2')
        await page.getByTestId(`forms-stub-item-${second}-gadget`).click()
        await page.getByTestId(`form-table-lines-${second}-qty`).fill('1')
        await expect(page.getByTestId(`form-table-lines-${first}-amount`)).toHaveText('10.00')
        await expect(page.getByTestId(`form-table-lines-${second}-amount`)).toHaveText('7.50')
        await expect(page.getByTestId('form-input-subtotal')).toHaveText('17.50')

        // The row menu button is the row's one Tab stop: open it from the keyboard.
        await page.getByTestId(`form-table-lines-menu-${second}`).focus()
        await page.keyboard.press('Enter')
        await page.getByTestId('form-table-lines-menu-up').click()
        await expect.poll(() => rowIds(page)).toEqual([second, first])
        // Focus stays on the moved row's menu button, so a keyboard user can move it again.
        await expect(page.getByTestId(`form-table-lines-menu-${second}`)).toBeFocused()

        await page.getByTestId('forms-stub-order-create').click()
        await openOrder(page, name)
        await expect.poll(() => rowIds(page)).toEqual([second, first])
        await expect(page.getByTestId('form-input-subtotal')).toHaveText('17.50')

        await page.getByTestId(`form-table-lines-menu-${first}`).click()
        await page.getByTestId('form-table-lines-menu-remove').click()
        await expect(page.getByTestId('form-input-subtotal')).toHaveText('7.50')
        await page.getByTestId('forms-stub-order-save').click()
        await expect(page.getByTestId('forms-stub-order-saved')).toBeVisible()

        await page.reload()
        await navigateToPackage(page, 'forms-stub')
        await openOrder(page, name)
        await expect.poll(() => rowIds(page)).toEqual([second])
        await expect(page.getByTestId('form-input-subtotal')).toHaveText('7.50')
        await expect(page.getByTestId(`form-table-lines-${second}-rate`)).toHaveValue('7.5')
    })

    test('a second save right after adding a row updates that row, not a copy', async ({
        page,
    }) => {
        const name = `Order ${Date.now()}`
        await login(page)
        await navigateToPackage(page, 'forms-stub')
        await page.getByTestId('forms-stub-order-new').click()
        await page.getByTestId('form-input-name').fill(name)
        await page.getByTestId('forms-stub-order-create').click()
        await openOrder(page, name)

        await page.getByTestId('form-table-lines-add').click()
        const [row] = await rowIds(page)
        await page.getByTestId(`forms-stub-item-${row}-widget`).click()
        await page.getByTestId(`form-table-lines-${row}-qty`).fill('1')
        await page.getByTestId('forms-stub-order-save').click()
        await expect(page.getByTestId('forms-stub-order-saved')).toBeVisible()

        // The second write must be an update of the row the first one inserted.
        const updated = page.waitForResponse(
            response =>
                response.url().includes(`/api/collections/forms_stub_order_lines/records/${row}`) &&
                response.request().method() === 'PATCH'
        )
        await page.getByTestId(`form-table-lines-${row}-qty`).fill('3')
        await page.getByTestId('forms-stub-order-save').click()
        expect((await updated).ok()).toBe(true)

        await page.reload()
        await navigateToPackage(page, 'forms-stub')
        await openOrder(page, name)
        await expect.poll(() => rowIds(page)).toEqual([row])
        await expect(page.getByTestId(`form-table-lines-${row}-amount`)).toHaveText('15.00')
        await expect(page.getByTestId('form-input-subtotal')).toHaveText('15.00')
    })

    test('a required cell left empty blocks Create and names the column', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'forms-stub')
        await page.getByTestId('forms-stub-order-new').click()
        await page.getByTestId('form-input-name').fill(`Order ${Date.now()}`)
        await page.getByTestId('form-table-lines-add').click()
        await page.getByTestId('forms-stub-order-create').click()
        // Once in the error summary and once under the host-drawn Item cell.
        await expect(page.getByText('Item is required')).toHaveCount(2)
        await expect(page.getByTestId('forms-stub-order-create')).toBeVisible()
    })
})

/** The Order host's header slots, in order (forms-host stub `lines`). */
const SLOTS = ['item', 'qty', 'rate', 'amount'] as const

/** The px floors the stub gives Qty, Rate and Amount: a NumberInput stepper needs ≈130 px plus its digits. */
const FLOORS = { qty: 170, rate: 170, amount: 112 } as const

/** The number cells whose stepper and value must fit their slot. */
const STEPPED = ['qty', 'rate'] as const

interface Box {
    left: number
    right: number
    width: number
}

interface Stepper {
    /** The − button, the input and the + button. */
    parts: Record<'decrease' | 'input' | 'increase', Box>
    value: string
    scrollWidth: number
    clientWidth: number
}

interface RowCells {
    /** The Amount figure's height and its computed line height. */
    amount: { height: number; lineHeight: number }
    steppers: Record<string, Stepper>
}

interface GridLayout {
    /** Header slot boxes by column key. */
    head: Record<string, Box>
    /** Row slot boxes by row id, then column key. */
    rows: Record<string, Record<string, Box>>
    /** Grid row widths by row id. */
    rowWidth: Record<string, number>
    /** The cells inside each row's slots, by row id. */
    cells: Record<string, RowCells>
}

/** Reads the wide grid's header and row slot boxes, and the cells in them, in one pass. */
async function gridLayout(page: Page, ids: string[]): Promise<GridLayout> {
    return page.evaluate(
        ({ ids, slots, stepped }) => {
            const NONE = { left: Number.NaN, right: Number.NaN, width: Number.NaN }
            const rect = (el: Element | null | undefined) => {
                if (!el) return NONE
                const r = el.getBoundingClientRect()
                return { left: r.left, right: r.right, width: r.width }
            }
            const byId = (testId: string) => document.querySelector(`[data-testid="${testId}"]`)
            const head = Object.fromEntries(
                slots.map(k => [k, rect(byId(`form-table-lines-head-${k}`))])
            )
            const rows = Object.fromEntries(
                ids.map(id => [
                    id,
                    Object.fromEntries(
                        slots.map(k => [k, rect(byId(`form-table-lines-slot-${id}-${k}`))])
                    ),
                ])
            )
            const rowWidth = Object.fromEntries(
                ids.map(id => [id, rect(byId(`form-table-lines-row-${id}`)).width])
            )
            const cellsOf = (id: string) => {
                const figure = byId(`form-table-lines-${id}-amount`)
                const style = figure ? getComputedStyle(figure) : null
                const fontSize = style ? Number.parseFloat(style.fontSize) : Number.NaN
                const parsed = style ? Number.parseFloat(style.lineHeight) : Number.NaN
                // A line height of `normal` is about 1.2 × the font size.
                const lineHeight = Number.isNaN(parsed) ? fontSize * 1.2 : parsed
                // The text's own height: the figure's box less its vertical padding and border.
                const inset = style
                    ? [
                          style.paddingTop,
                          style.paddingBottom,
                          style.borderTopWidth,
                          style.borderBottomWidth,
                      ].reduce((sum, v) => sum + (Number.parseFloat(v) || 0), 0)
                    : 0
                const amount = {
                    height: figure ? figure.getBoundingClientRect().height - inset : Number.NaN,
                    lineHeight,
                }
                const stepperOf = (key: string) => {
                    const slot = byId(`form-table-lines-slot-${id}-${key}`)
                    const input = byId(`form-table-lines-${id}-${key}`)
                    const value = input instanceof HTMLInputElement ? input.value : ''
                    return {
                        parts: {
                            decrease: rect(slot?.querySelector('[aria-label^="Decrease"]')),
                            input: rect(input),
                            increase: rect(slot?.querySelector('[aria-label^="Increase"]')),
                        },
                        value,
                        scrollWidth: input?.scrollWidth ?? Number.NaN,
                        clientWidth: input?.clientWidth ?? Number.NaN,
                    }
                }
                const steppers = Object.fromEntries(stepped.map(k => [k, stepperOf(k)]))
                return { amount, steppers }
            }
            const cells = Object.fromEntries(ids.map(id => [id, cellsOf(id)]))
            return { head, rows, rowWidth, cells }
        },
        { ids, slots: [...SLOTS], stepped: [...STEPPED] }
    )
}

/** The edges of each header slot that differ from the row slot under it by more than 0.5 px. */
function misaligned(layout: GridLayout): string[] {
    const off: string[] = []
    for (const [id, slots] of Object.entries(layout.rows)) {
        for (const key of SLOTS) {
            const head = layout.head[key]
            const slot = slots[key]
            const ok =
                Math.abs(head.left - slot.left) <= 0.5 && Math.abs(head.right - slot.right) <= 0.5
            if (!ok)
                off.push(
                    `${id}/${key}: head ${head.left}-${head.right}, row ${slot.left}-${slot.right}`
                )
        }
    }
    return off
}

/** Every slot, in every row, narrower than its floor. */
function underFloor(layout: GridLayout): string[] {
    const off: string[] = []
    for (const [id, slots] of Object.entries(layout.rows)) {
        for (const [key, floor] of Object.entries(FLOORS)) {
            if (!(slots[key].width >= floor - 0.5))
                off.push(`${id}/${key} ${slots[key].width} < ${floor}`)
        }
    }
    return off
}

/** The key of the widest slot in a row. */
function widest(layout: GridLayout, id: string): string {
    const slots = layout.rows[id]
    return SLOTS.reduce((a, b) => (slots[b].width > slots[a].width ? b : a))
}

/**
 * What does not fit in each row: an Amount figure on more than one line, a
 * Qty / Rate input that hides part of its value or holds another value than
 * the one typed, a stepper part (−, input, +) outside its own slot.
 */
function cellsNotFitting(
    layout: GridLayout,
    typed: Record<string, Record<(typeof STEPPED)[number], string>>
): string[] {
    const off: string[] = []
    for (const [id, cells] of Object.entries(layout.cells)) {
        const { height, lineHeight } = cells.amount
        if (!(height <= lineHeight * 1.5))
            off.push(`${id}/amount ${height}px tall, line ${lineHeight}px`)
        for (const key of STEPPED) {
            const stepper = cells.steppers[key]
            const slot = layout.rows[id][key]
            if (stepper.value !== typed[id][key])
                off.push(`${id}/${key} value "${stepper.value}" != "${typed[id][key]}"`)
            if (!(stepper.scrollWidth <= stepper.clientWidth))
                off.push(
                    `${id}/${key} input scroll ${stepper.scrollWidth} > ${stepper.clientWidth}`
                )
            for (const [part, b] of Object.entries(stepper.parts)) {
                const inside = b.left >= slot.left - 0.5 && b.right <= slot.right + 0.5
                if (!inside)
                    off.push(
                        `${id}/${key} ${part} ${b.left}-${b.right} outside ${slot.left}-${slot.right}`
                    )
            }
        }
    }
    return off
}

test.describe('forms-table: column widths', () => {
    test.skip(skipWithoutFormsStub(), 'forms-host-stub not scaffolded in this workspace')

    test('keeps column floors, gives free width to the weighted column and aligns the header with the rows', async ({
        page,
    }) => {
        await login(page)
        await navigateToPackage(page, 'forms-stub')
        const height = page.viewportSize()?.height ?? 720

        await page.getByTestId('forms-stub-order-new').click()
        await page.getByTestId('form-input-name').fill(`Order ${Date.now()}`)
        await page.getByTestId('form-table-lines-add').click()
        await page.getByTestId('form-table-lines-add').click()
        const ids = await rowIds(page)
        expect(ids).toHaveLength(2)
        const [first, second] = ids
        const typed = {
            [first]: { qty: '1000', rate: '1890' },
            [second]: { qty: '250', rate: '1234' },
        }
        await page.getByTestId(`forms-stub-item-${first}-widget`).click()
        await page.getByTestId(`forms-stub-item-${second}-gadget`).click()
        for (const id of ids) {
            await page.getByTestId(`form-table-lines-${id}-qty`).fill(typed[id].qty)
            await page.getByTestId(`form-table-lines-${id}-rate`).fill(typed[id].rate)
        }
        await expect(page.getByTestId(`form-table-lines-${first}-amount`)).toHaveText('1890000.00')
        await expect(page.getByTestId(`form-table-lines-${second}-amount`)).toHaveText('308500.00')

        const at: Record<number, GridLayout> = {}
        for (const width of [911, 1280]) {
            await page.setViewportSize({ width, height })
            await expect(page.getByTestId('form-table-lines-grid')).toBeVisible()
            await expect.poll(async () => misaligned(await gridLayout(page, ids))).toEqual([])
            await expect
                .poll(async () => cellsNotFitting(await gridLayout(page, ids), typed))
                .toEqual([])
            await expect.poll(async () => underFloor(await gridLayout(page, ids))).toEqual([])
            await expect.poll(async () => widest(await gridLayout(page, ids), first)).toBe('item')
            at[width] = await gridLayout(page, ids)
        }

        // The width the grid row gains from 911 to 1280 goes mostly to the weighted Item column.
        const itemGain = at[1280].rows[first].item.width - at[911].rows[first].item.width
        const rowGain = at[1280].rowWidth[first] - at[911].rowWidth[first]
        expect(rowGain).toBeGreaterThan(0)
        expect(itemGain).toBeGreaterThanOrEqual(rowGain / 2)

        await page.setViewportSize({ width: 390, height: 844 })
        await expect(page.getByTestId('form-table-lines-cards')).toBeVisible()
        await expect.poll(() => rowIds(page)).toEqual(ids)
        await expect
            .poll(() =>
                page.locator('[data-testid^="form-table-lines-"]').evaluateAll(els =>
                    els
                        .map(el => ({
                            id: el.getAttribute('data-testid'),
                            r: el.getBoundingClientRect(),
                        }))
                        .filter(({ r }) => r.width > 0 && (r.left < -0.5 || r.right > 390.5))
                        .map(({ id }) => id)
                )
            )
            .toEqual([])
    })
})
