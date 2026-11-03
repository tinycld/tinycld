import { expect, type Locator, type Page, test } from '@playwright/test'
import { login, skipWithoutFormsStub } from './helpers'

/**
 * The forms builder (Settings → Forms) end to end, against the forms-host
 * stub scaffolded by tests/scripts/scaffold-forms-host-stub.ts. App-shell CI
 * installs no feature package, and the builder only appears when a package
 * declares a form host, so the stub is the host.
 *
 * Stub fixtures this file depends on (keep in sync with the scaffolder):
 *   host "Widget" (forms-stub:widget); other packages may declare hosts too, in
 *   which case the host picker shows first and the spec picks this one;
 *   built-in fields `name` (required), `notes`, `due`.
 *
 * The table and formula tests add custom fields `parts` (a table) and
 * `grand_total` (a calculated value); the Widget host has no column of those names.
 */

const HOST_LABEL = 'Widget'
const HOST_ROW_TEST_ID = 'forms-host-forms-stub:widget'

async function openFormsSettings(page: Page) {
    await page.getByTestId('nav-settings').click()
    await page.getByText('Forms', { exact: true }).first().click()
    const hostRow = page.getByTestId(HOST_ROW_TEST_ID)
    const newForm = page.getByTestId('forms-new')
    await expect(hostRow.or(newForm)).toBeVisible()
    if (await hostRow.count()) await hostRow.click()
    await expect(newForm).toBeVisible()
}

function formRow(page: Page, name: string): Locator {
    return page.locator('[data-testid^="forms-row-"]').filter({
        has: page.getByText(name, { exact: true }),
    })
}

async function formIdOf(page: Page, name: string): Promise<string> {
    const testId = await formRow(page, name).getAttribute('data-testid')
    if (!testId) throw new Error(`no row for form ${name}`)
    return testId.replace('forms-row-', '')
}

/** Answers a PromptDialog: its input is labelled by the dialog title. */
async function answerPrompt(page: Page, title: string, value: string) {
    const input = page.getByLabel(title, { exact: true })
    await expect(input).toBeVisible()
    await input.fill(value)
    await input.press('Enter')
    await expect(input).toBeHidden()
}

async function createForm(page: Page, name: string) {
    await page.getByTestId('forms-new').click()
    await answerPrompt(page, 'New form', name)
    await expect(page.getByTestId('form-editor')).toBeVisible()
}

/** The editor's canvas, not the rules tab's read-only picture of the form. */
function canvas(page: Page): Locator {
    return page.getByTestId('form-canvas')
}

async function pageKeys(page: Page): Promise<string[]> {
    const ids = await canvas(page)
        .locator('[data-testid^="form-page-header-"]')
        .evaluateAll(els => els.map(el => el.getAttribute('data-testid') ?? ''))
    return ids.map(id => id.replace('form-page-header-', ''))
}

/** Field keys in canvas order on one page; drop bars and inputs share the prefix, so match whole ids. */
async function fieldOrder(page: Page, pageKey: string): Promise<string[]> {
    const ids = await canvas(page)
        .getByTestId(`form-page-${pageKey}`)
        .locator('[data-testid^="form-field-"]')
        .evaluateAll(els => els.map(el => el.getAttribute('data-testid') ?? ''))
    return ids.filter(id => /^form-field-[a-z0-9_]+$/.test(id)).map(id => id.slice(11))
}

async function addField(page: Page, pageKey: string, kind: string, label: string) {
    await page.getByTestId(`form-add-field-${pageKey}`).click()
    await page.getByTestId(`form-kind-${kind}`).click()
    await answerPrompt(page, 'Field label', label)
}

/** Picks a row from a builder PickMenu: the chip, then the `${testID}-${id}` item. */
async function pick(page: Page, testId: string, id: string) {
    await page.getByTestId(testId).click()
    await page.getByTestId(`${testId}-${id}`).click()
}

/**
 * A real pointer drag: press on the handle, move in steps, release at `at`
 * (relative to the target's box), once the expected drop bar shows.
 */
async function dragTo(
    page: Page,
    handle: Locator,
    target: Locator,
    dropBar: Locator,
    at: (box: { x: number; y: number; width: number; height: number }) => [number, number]
) {
    const from = await handle.boundingBox()
    const to = await target.boundingBox()
    if (!from || !to) throw new Error('drag handle or target is not laid out')
    await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2)
    await page.mouse.down()
    const [x, y] = at(to)
    await page.mouse.move(x, y, { steps: 12 })
    await expect(dropBar).toBeVisible()
    await page.mouse.up()
}

/** Releases near the target's top-left corner: before it, whether its neighbours lie beside or below it. */
function dragBefore(page: Page, handle: Locator, target: Locator, dropBar: Locator) {
    return dragTo(page, handle, target, dropBar, box => [
        box.x + Math.min(12, box.width / 4),
        box.y + Math.min(8, box.height / 4),
    ])
}

/** Releases over the target's middle. */
function dragOnto(page: Page, handle: Locator, target: Locator, dropBar: Locator) {
    return dragTo(page, handle, target, dropBar, box => [
        box.x + box.width / 2,
        box.y + box.height / 2,
    ])
}

/** How much of the container's width the cell takes. */
async function widthShare(cell: Locator, container: Locator): Promise<number> {
    const [a, b] = await Promise.all([cell.boundingBox(), container.boundingBox()])
    return a && b ? a.width / b.width : 1
}

type Box = { x: number; y: number; width: number; height: number }

async function boxOf(locator: Locator): Promise<Box> {
    const box = await locator.boundingBox()
    if (!box) throw new Error('element is not laid out')
    return box
}

/** Both side edges of `inner` lie within `outer`: nothing is cut off by the canvas's column. */
function expectInsideSides(inner: Box, outer: Box) {
    expect(inner.x).toBeGreaterThanOrEqual(outer.x)
    expect(inner.x + inner.width).toBeLessThanOrEqual(outer.x + outer.width)
}

async function opacityOf(locator: Locator): Promise<number> {
    return Number(await locator.evaluate(el => getComputedStyle(el).opacity))
}

const TRANSPARENT = 'rgba(0, 0, 0, 0)'

function borderColorOf(locator: Locator): Promise<string> {
    return locator.evaluate(el => getComputedStyle(el).borderLeftColor)
}

function backgroundOf(locator: Locator): Promise<string> {
    return locator.evaluate(el => getComputedStyle(el).backgroundColor)
}

/** Selects a canvas cell by its label and gives it a width from the properties panel. */
async function setWidth(page: Page, key: string, label: string, width: string) {
    await canvas(page).getByTestId(`form-field-${key}`).getByText(label, { exact: true }).click()
    await expect(properties(page).getByTestId('form-prop-key')).toContainText(key)
    await propControl(page, `width-${width}`).click()
}

function properties(page: Page): Locator {
    return page.getByTestId('form-properties')
}

/** The properties panel's control for `fields.<i>.<suffix>`, whichever index the field has. */
function propControl(page: Page, suffix: string): Locator {
    return properties(page).locator(`[data-testid$=".${suffix}"]`)
}

const COLUMN_NOUNS: Record<string, string> = { select: 'choice list', computed: 'calculated' }

/** Adds a column to the selected table: its kind from the menu, then its label. */
async function addColumn(page: Page, kind: string, label: string) {
    await properties(page).getByTestId('form-column-add').click()
    await page.getByTestId(`form-column-kind-${kind}`).click()
    await answerPrompt(page, `Name the ${COLUMN_NOUNS[kind] ?? kind} column`, label)
    await expect(properties(page).getByTestId('form-column-settings')).toBeVisible()
}

/** Column keys of the selected table, in order. */
async function columnOrder(page: Page): Promise<string[]> {
    const ids = await properties(page)
        .locator('[data-testid^="form-column-"][data-testid$="-edit"]')
        .evaluateAll(els => els.map(el => el.getAttribute('data-testid') ?? ''))
    return ids.map(id => id.replace(/^form-column-/, '').replace(/-edit$/, ''))
}

/**
 * Saves and waits for the server to store the definition. A disabled Save
 * button alone does not show this: the button is also disabled while the
 * write is pending, and a reload then would cancel it.
 */
async function saveForm(page: Page) {
    const stored = page.waitForResponse(
        response =>
            response.url().includes('/api/collections/forms/records/') &&
            response.request().method() === 'PATCH'
    )
    const save = page.getByTestId('form-save')
    await save.click()
    expect((await stored).ok()).toBe(true)
    await expect(save).toBeDisabled()
}

const BACK_GUARD_VIEWPORTS = [
    { name: 'desktop', size: { width: 1280, height: 800 } },
    { name: 'phone', size: { width: 390, height: 844 } },
] as const

/** The forms list's own URL: settings/forms, with at most a query, never a form id. */
const FORMS_LIST_URL = /\/settings\/forms\/?(\?[^/]*)?$/

function formsListShown(page: Page): Locator {
    return page.getByTestId(HOST_ROW_TEST_ID).or(page.getByTestId('forms-new'))
}

/**
 * Adds a text field so the editor is dirty. On a phone the new field's
 * properties open in a sheet over the editor; Escape closes it so the
 * navigation under test reaches the editor, not the sheet.
 */
async function addBackNote(page: Page, viewport: 'desktop' | 'phone') {
    const [firstPage] = await pageKeys(page)
    await addField(page, firstPage, 'text', 'Back note')
    await expect(canvas(page).getByTestId('form-field-back_note')).toBeVisible()
    if (viewport !== 'phone') return
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('form-panel-sheet')).toBeHidden()
}

/** The URL of the history entry `offset` steps from the current one (Chromium's Navigation API). */
async function entryUrl(page: Page, offset: number): Promise<string> {
    const url = await page.evaluate(step => {
        const index = (window.navigation.currentEntry?.index ?? 0) + step
        return window.navigation.entries()[index]?.url ?? null
    }, offset)
    if (!url) throw new Error(`no history entry ${offset} steps from here`)
    return url
}

test.describe('Forms builder', () => {
    test.skip(skipWithoutFormsStub(), 'forms-host-stub not scaffolded in this workspace')

    test.beforeEach(async ({ page }) => {
        await login(page)
        await openFormsSettings(page)
    })

    test('builds a form with fields, pages, a drag and a rule, saves it and previews it', async ({
        page,
    }) => {
        await expect(formRow(page, HOST_LABEL)).toBeVisible()

        await createForm(page, 'Bug')
        const [firstPage] = await pageKeys(page)
        expect(firstPage).toBeTruthy()

        // A choice list: the new cell is selected and the panel shows its options.
        await addField(page, firstPage, 'select', 'Severity')
        const severity = canvas(page).getByTestId('form-field-severity')
        await expect(severity).toBeVisible()
        await expect(properties(page).getByTestId('form-prop-key')).toContainText('severity')
        await expect(properties(page).getByText('Options', { exact: true })).toBeVisible()

        await properties(page).getByTestId('form-option-add').click()
        await propControl(page, 'options.1.label').fill('High')
        await propControl(page, 'options.1.value').fill('high')
        await propControl(page, 'width-third').click()
        await expect.poll(() => widthShare(severity, canvas(page))).toBeLessThan(0.4)

        // A second page, renamed, with a text field on it.
        await page.getByTestId('form-add-page').click()
        const [, secondPage] = await pageKeys(page)
        expect(secondPage).toBeTruthy()
        const title = page.getByTestId(`form-page-title-${secondPage}`)
        await title.fill('Repro')
        await title.press('Enter')
        await expect(title).toHaveValue('Repro')
        await addField(page, secondPage, 'text', 'Steps')
        await expect(canvas(page).getByTestId('form-field-steps')).toBeVisible()
        expect(await fieldOrder(page, secondPage)).toEqual(['steps'])

        // Drag Steps onto the first page, before Severity.
        const before = await fieldOrder(page, firstPage)
        expect(before.at(-1)).toBe('severity')
        await dragBefore(
            page,
            page.getByTestId('form-handle-steps'),
            severity,
            canvas(page).getByTestId('form-field-severity-drop-before')
        )
        await expect
            .poll(() => fieldOrder(page, firstPage))
            .toEqual([...before.slice(0, -1), 'steps', 'severity'])
        expect(await fieldOrder(page, secondPage)).toEqual([])

        // Let go over the page's title, above every cell: it lands first, not last.
        await dragOnto(
            page,
            page.getByTestId('form-handle-steps'),
            page.getByTestId(`form-page-header-${firstPage}`),
            canvas(page).getByTestId(`form-field-${before[0]}-drop-before`)
        )
        await expect
            .poll(() => fieldOrder(page, firstPage))
            .toEqual(['steps', ...before.slice(0, -1), 'severity'])

        // Back to its page with the panel's page picker, which needs no pointer drag.
        await canvas(page)
            .getByTestId('form-field-steps')
            .getByText('Steps', { exact: true })
            .click()
        await expect(properties(page).getByTestId('form-prop-key')).toContainText('steps')
        await pick(page, 'form-prop-page', secondPage)
        await expect.poll(() => fieldOrder(page, secondPage)).toEqual(['steps'])
        expect(await fieldOrder(page, firstPage)).toEqual(before)

        // Two half-width cells share a row, for the preview's Phone check below.
        const [halfA, halfB] = before
        const halfLabels: Record<string, string> = { name: 'Name', notes: 'Notes', due: 'Due' }
        await setWidth(page, halfA, halfLabels[halfA], 'half')
        await setWidth(page, halfB, halfLabels[halfB], 'half')

        // Rules: when severity is high, require steps.
        await page.getByTestId('form-tab-rules').click()
        await page.getByTestId('form-rule-add').click()
        await page.getByTestId('form-cond-add-0').click()
        await pick(page, 'form-cond-field-0-0', 'severity')
        await page.getByTestId('form-cond-value-0-0-high').click()
        await page.getByTestId('form-action-add-0').click()
        await pick(page, 'form-action-target-0-0', 'steps')
        await pick(page, 'form-action-kind-0-0', 'require')
        const affected = page.getByTestId('form-rule-affected')
        await expect(affected.getByTestId('form-field-severity')).toContainText('when')
        await expect(affected.getByTestId('form-field-steps')).toContainText('Require')

        // Save, then reload: what was saved comes back.
        await saveForm(page)
        await page.reload()
        await expect(page.getByTestId('form-editor')).toBeVisible()
        await expect(canvas(page).getByTestId('form-field-severity')).toBeVisible()
        expect(await fieldOrder(page, secondPage)).toEqual(['steps'])
        await expect(page.getByTestId(`form-page-title-${secondPage}`)).toHaveValue('Repro')
        await page.getByTestId('form-tab-rules').click()
        await expect(page.getByTestId('form-cond-field-0-0')).toContainText('Severity')
        await expect(page.getByTestId('form-action-kind-0-0')).toContainText('Require')
        await expect(page.getByTestId('form-action-target-0-0')).toContainText('Steps')

        // Preview: a two-step wizard where High makes Steps required.
        await page.getByTestId('form-preview').click()
        const dialog = page.getByTestId('form-preview-dialog')
        await expect(dialog.getByTestId('form-wizard-steps')).toBeVisible()
        await expect(dialog.getByTestId('form-wizard-steps')).toContainText('Repro')

        // Desktop keeps the two half cells on one row; Phone stacks them in a 390px frame.
        const cellA = dialog.getByTestId(`form-field-${halfA}`)
        const cellB = dialog.getByTestId(`form-field-${halfB}`)
        await expect.poll(async () => (await boxOf(cellB)).y).toBeCloseTo((await boxOf(cellA)).y, 0)
        await dialog.getByTestId('form-preview-phone').click()
        const frame = dialog.getByTestId('form-preview-viewport-phone')
        expect((await boxOf(frame)).width).toBe(390)
        await expect.poll(async () => (await boxOf(cellB)).y > (await boxOf(cellA)).y).toBe(true)
        await dialog.getByTestId('form-preview-desktop').click()
        await expect(frame).toHaveCount(0)
        await dialog.getByTestId('form-input-name').fill('Crash on save')
        // A select is a dropdown: open it, then pick the option.
        await dialog.getByTestId('form-input-severity').getByLabel('Severity: Choose…').click()
        await page.getByRole('menuitem', { name: 'High', exact: true }).click()
        await dialog.getByTestId('form-wizard-next').click()
        const steps = dialog.getByTestId('form-field-steps')
        await expect(steps).toBeVisible()
        await expect(steps.getByText('*', { exact: true })).toBeVisible()
        await dialog.getByTestId('form-wizard-next').click()
        await expect(steps.getByRole('alert')).toBeVisible()
        await expect(dialog).toBeVisible()
    })

    test('builds a table with a calculated column and a calculated total, saves, reloads and previews it', async ({
        page,
    }) => {
        await createForm(page, 'Quote')
        const [firstPage] = await pageKeys(page)

        // A table: it starts with an Item column; add Qty, Price and a calculated Total.
        await addField(page, firstPage, 'table', 'Parts')
        await expect(properties(page).getByTestId('form-column-item')).toBeVisible()
        await addColumn(page, 'number', 'Qty')
        await addColumn(page, 'number', 'Price')
        await addColumn(page, 'computed', 'Total')

        // Total = Qty × Price, picked from menus; nothing is typed but the decimal places.
        const column = 'form-column-formula'
        await pick(page, `${column}-root-part`, 'mul')
        await pick(page, `${column}-0-part`, 'field')
        await pick(page, `${column}-1-part`, 'field')
        await pick(page, `${column}-1-field`, 'price')
        await expect(page.getByTestId(`${column}-summary`)).toHaveText('= Qty × Price')
        await propControl(page, 'columns.3.decimals').fill('2')
        await expect(canvas(page).getByTestId('form-table-parts-head-total')).toBeVisible()

        // Reorder with the keyboard: Price goes above Qty.
        await page.getByTestId('form-column-price-up').focus()
        await page.keyboard.press('Enter')
        await expect.poll(() => columnOrder(page)).toEqual(['item', 'price', 'qty', 'total'])

        // A calculated value that adds up the Total column.
        await addField(page, firstPage, 'computed', 'Grand total')
        await pick(page, 'form-formula-root-part', 'sumOf')
        await pick(page, 'form-formula-root-column', 'total')
        await expect(page.getByTestId('form-formula-summary')).toHaveText('= sum of Total in Parts')
        await propControl(page, 'decimals').fill('2')

        // Save: the server checks the definition again. Then reload: it comes back.
        await saveForm(page)
        await page.reload()
        await expect(page.getByTestId('form-editor')).toBeVisible()
        await canvas(page)
            .getByTestId('form-field-grand_total')
            .getByText('Grand total', { exact: true })
            .click()
        await expect(page.getByTestId('form-formula-summary')).toHaveText('= sum of Total in Parts')
        await canvas(page)
            .getByTestId('form-field-parts')
            .getByText('Parts', { exact: true })
            .click()
        await expect.poll(() => columnOrder(page)).toEqual(['item', 'price', 'qty', 'total'])

        // Preview: a row's Total and the Grand total follow what is typed.
        await page.getByTestId('form-preview').click()
        const dialog = page.getByTestId('form-preview-dialog')
        await dialog.getByTestId('form-input-name').fill('Bracket')
        await dialog.getByTestId('form-table-parts-add').click()
        const rowTestId = await dialog
            .locator('[data-testid^="form-table-parts-row-"]')
            .first()
            .getAttribute('data-testid')
        const row = (rowTestId ?? '').replace('form-table-parts-row-', '')
        await dialog.getByTestId(`form-table-parts-${row}-qty`).fill('3')
        await dialog.getByTestId(`form-table-parts-${row}-price`).fill('2.5')
        await expect(dialog.getByTestId(`form-table-parts-${row}-total`)).toHaveText('7.50')
        await expect(dialog.getByTestId('form-input-grand_total')).toHaveText('7.50')
    })

    test('names a formula that reads a removed column and does not save', async ({ page }) => {
        await createForm(page, 'Broken quote')
        const [firstPage] = await pageKeys(page)
        await addField(page, firstPage, 'table', 'Parts')
        await addColumn(page, 'number', 'Qty')
        await addColumn(page, 'computed', 'Total')
        await pick(page, 'form-column-formula-root-part', 'field')
        await expect(page.getByTestId('form-column-formula-summary')).toHaveText('= Qty')

        await properties(page).getByTestId('form-column-qty-remove').click()
        await page.getByRole('button', { name: 'Remove', exact: true }).click()
        await expect(page.getByTestId('form-column-formula-summary')).toHaveText('= qty (missing)')
        const save = page.getByTestId('form-save')
        await save.click()
        await expect(page.getByTestId('form-errors')).toContainText(
            "Field 'Parts', column 'Total', formula: unknown column qty"
        )
        await expect(save).toBeEnabled()
    })

    test('shows where a sideways drag lands and keeps the outline inside the canvas', async ({
        page,
    }) => {
        await createForm(page, 'Layout')
        const [firstPage] = await pageKeys(page)
        const [left, right, ...rest] = await fieldOrder(page, firstPage)
        const labels: Record<string, string> = { name: 'Name', notes: 'Notes', due: 'Due' }
        const leftCell = canvas(page).getByTestId(`form-field-${left}`)
        const rightCell = canvas(page).getByTestId(`form-field-${right}`)

        await setWidth(page, left, labels[left], 'half')
        await expect.poll(() => widthShare(leftCell, canvas(page))).toBeLessThan(0.6)
        await setWidth(page, right, labels[right], 'half')
        await expect.poll(() => widthShare(rightCell, canvas(page))).toBeLessThan(0.6)
        await expect
            .poll(async () => (await boxOf(rightCell)).y)
            .toBeCloseTo((await boxOf(leftCell)).y, 0)

        // The right cell is selected; its outline is whole, not clipped by the canvas.
        const rightBox = canvas(page).getByTestId(`form-field-${right}-box`)
        await expect.poll(() => borderColorOf(rightBox)).not.toBe(TRANSPARENT)
        const canvasBox = await boxOf(canvas(page))
        expectInsideSides(await boxOf(rightBox), canvasBox)
        await setWidth(page, left, labels[left], 'half')
        expectInsideSides(
            await boxOf(canvas(page).getByTestId(`form-field-${left}-box`)),
            canvasBox
        )

        // A real pointer drag of the right cell: first over its own right half, then
        // below the row, then over the left half of its neighbour.
        const handle = await boxOf(page.getByTestId(`form-handle-${right}`))
        const target = await boxOf(leftCell)
        const own = await boxOf(rightCell)
        await page.mouse.move(handle.x + handle.width / 2, handle.y + handle.height / 2)
        await page.mouse.down()
        await page.mouse.move(own.x + own.width * 0.75, own.y + own.height / 2, { steps: 8 })
        // A ghost chip with the field's label follows the pointer.
        const ghost = page.getByTestId('form-drag-ghost')
        await expect(ghost).toBeVisible()
        await expect(ghost).toHaveText(labels[right])
        expect((await boxOf(ghost)).x).toBeGreaterThan(own.x + own.width * 0.75)
        // Over its own edge the bar still shows, on the dragged cell itself.
        await expect(canvas(page).getByTestId(`form-field-${right}-drop-after`)).toBeVisible()
        // Below the page's last row its end bar sits above the footer, not under it.
        const last = await boxOf(canvas(page).getByTestId(`form-field-${rest.at(-1) ?? right}`))
        await page.mouse.move(last.x + last.width / 2, last.y + last.height + 4, { steps: 4 })
        const endBar = canvas(page).getByTestId(`form-page-${firstPage}-drop-end`)
        await expect(endBar).toBeVisible()
        const addField = await boxOf(page.getByTestId(`form-add-field-${firstPage}`))
        expect((await boxOf(endBar)).y).toBeLessThan(addField.y)
        await page.mouse.move(target.x + target.width / 4, target.y + target.height / 2, {
            steps: 15,
        })
        await expect(endBar).toBeHidden()
        const bar = canvas(page).getByTestId(`form-field-${left}-drop-before`)
        await expect(bar).toBeVisible()
        const barBox = await boxOf(bar)
        expectInsideSides(barBox, canvasBox)
        expect(barBox.width).toBeGreaterThanOrEqual(2)
        expect(await backgroundOf(canvas(page).getByTestId(`form-field-${left}-box`))).not.toBe(
            TRANSPARENT
        )
        expect(await opacityOf(rightCell)).toBeLessThan(1)
        await page.mouse.up()
        await expect(ghost).toBeHidden()

        await expect.poll(() => fieldOrder(page, firstPage)).toEqual([right, left, ...rest])
        expect(await opacityOf(rightCell)).toBe(1)
        await expect(bar).toBeHidden()
        const [movedBox, otherBox] = await Promise.all([boxOf(rightCell), boxOf(leftCell)])
        expect(movedBox.x).toBeLessThan(otherBox.x)
        expect(movedBox.y).toBeCloseTo(otherBox.y, 0)
    })

    test('a file field outside web forms offers no size limit it cannot enforce', async ({
        page,
    }) => {
        await login(page)
        await openFormsSettings(page)
        await createForm(page, `Files ${Date.now()}`)
        const [firstPage] = await pageKeys(page)
        await addField(page, firstPage, 'file', 'Photo')
        await expect(properties(page).getByText('Accepted types')).toBeVisible()
        await expect(properties(page).locator('[data-testid$=".maxSizeMb"]')).toHaveCount(0)
    })

    test('renames, makes default and deletes forms from the list', async ({ page }) => {
        await expect(formRow(page, HOST_LABEL)).toContainText('Default')

        await createForm(page, 'Task')
        await page.getByTestId('form-back').click()
        await createForm(page, 'Scratch')
        await page.getByTestId('form-back').click()
        await expect(formRow(page, 'Scratch')).toBeVisible()

        const taskId = await formIdOf(page, 'Task')
        await page.getByTestId(`forms-menu-${taskId}`).click()
        await page.getByTestId(`forms-rename-${taskId}`).click()
        await answerPrompt(page, 'Rename form', 'Chore')
        await expect(page.getByTestId(`forms-row-${taskId}`)).toContainText('Chore')

        await page.getByTestId(`forms-menu-${taskId}`).click()
        await page.getByTestId(`forms-default-${taskId}`).click()
        await expect(page.getByTestId(`forms-row-${taskId}`)).toContainText('Default')
        await expect(formRow(page, HOST_LABEL)).not.toContainText('Default')

        // The default form cannot be deleted; the menu item says why.
        await page.getByTestId(`forms-menu-${taskId}`).click()
        const deleteDefault = page.getByTestId(`forms-delete-${taskId}`)
        await expect(deleteDefault).toHaveAttribute('aria-disabled', 'true')
        await expect(deleteDefault).toContainText('default form')
        await page.keyboard.press('Escape')
        await expect(deleteDefault).toBeHidden()

        const scratchId = await formIdOf(page, 'Scratch')
        await page.getByTestId(`forms-menu-${scratchId}`).click()
        await page.getByTestId(`forms-delete-${scratchId}`).click()
        await page.getByText('Delete', { exact: true }).last().click()
        await expect(page.getByTestId(`forms-row-${scratchId}`)).toBeHidden()
        await expect(page.getByTestId(`forms-row-${taskId}`)).toBeVisible()
    })

    for (const viewport of BACK_GUARD_VIEWPORTS) {
        test(`asks before the browser Back button leaves a dirty form (${viewport.name})`, async ({
            page,
        }) => {
            await page.setViewportSize(viewport.size)
            const name = `Back guard ${Date.now()} ${viewport.name}`
            await createForm(page, name)
            await addBackNote(page, viewport.name)
            const editorUrl = page.url()
            const discardTitle = page.getByText('Discard changes?', { exact: true })
            const backNote = canvas(page).getByTestId('form-field-back_note')

            // A Back of two steps (the browser's long-press history menu) asks
            // too, and Cancel puts the URL back on the editor, not one step short.
            await page.evaluate(() => window.history.go(-2))
            await expect(discardTitle).toBeVisible()
            await page.getByRole('button', { name: 'Cancel', exact: true }).click()
            await expect(discardTitle).toBeHidden()
            await expect(page).toHaveURL(editorUrl)
            await expect(canvas(page).getByTestId('form-field-back_note')).toBeVisible()

            // Browser Back on a dirty editor asks first and keeps the editor's URL.
            await page.goBack()
            await expect(discardTitle).toBeVisible()
            await expect(page).toHaveURL(editorUrl)

            // The forms editor's confirm cancels with its own label, "Cancel".
            await page.getByRole('button', { name: 'Cancel', exact: true }).click()
            await expect(discardTitle).toBeHidden()
            await expect(page).toHaveURL(editorUrl)
            await expect(canvas(page).getByTestId('form-field-back_note')).toBeVisible()

            // The guard is not left half way: the next browser Back still asks.
            await page.goBack()
            await expect(discardTitle).toBeVisible()
            await page.getByRole('button', { name: 'Cancel', exact: true }).click()
            await expect(discardTitle).toBeHidden()
            await expect(page).toHaveURL(editorUrl)
            await expect(backNote).toBeVisible()

            // Asked again, Discard lets the browser Back through to the forms list.
            await page.goBack()
            await expect(discardTitle).toBeVisible()
            await page.getByRole('button', { name: 'Discard', exact: true }).click()
            await expect(formsListShown(page)).toBeVisible()
            await expect(page).not.toHaveURL(editorUrl)

            // Discard lets that one move through, not every later one: the
            // editor opened again and made dirty asks on browser Back again.
            const formId = await formIdOf(page, name)
            await page.getByTestId(`forms-open-${formId}`).click()
            await expect(page.getByTestId('form-editor')).toBeVisible()
            await addBackNote(page, viewport.name)
            await page.goBack()
            await expect(discardTitle).toBeVisible()
            await expect(page).toHaveURL(editorUrl)
            await page.getByRole('button', { name: 'Cancel', exact: true }).click()
            await expect(discardTitle).toBeHidden()

            // Discard after a 2-step Back makes the whole move: the URL is
            // the entry two steps back, not the forms list one step back.
            const twoBackUrl = await entryUrl(page, -2)
            expect(twoBackUrl).not.toBe(await entryUrl(page, -1))
            await page.evaluate(() => window.history.go(-2))
            await expect(discardTitle).toBeVisible()
            await page.getByRole('button', { name: 'Discard', exact: true }).click()
            await expect(discardTitle).toBeHidden()
            await expect(page).toHaveURL(twoBackUrl)

            // Forward one step is the forms list again, for the steps below.
            await page.goForward()
            await expect(formsListShown(page)).toBeVisible()

            // A clean editor leaves on browser Back without asking.
            await page.getByTestId(`forms-open-${formId}`).click()
            await expect(page.getByTestId('form-editor')).toBeVisible()
            await expect(canvas(page).getByTestId('form-field-back_note')).toHaveCount(0)
            await page.goBack()
            await expect(formsListShown(page)).toBeVisible()
            await expect(discardTitle).toHaveCount(0)
            await expect(page).not.toHaveURL(editorUrl)

            // The in-app Back still leaves after Discard, to the forms list URL.
            await page.getByTestId(`forms-open-${formId}`).click()
            await expect(page.getByTestId('form-editor')).toBeVisible()
            await addBackNote(page, viewport.name)
            await page.getByTestId('form-back').click()
            await expect(discardTitle).toBeVisible()
            await page.getByRole('button', { name: 'Discard', exact: true }).click()
            await expect(formsListShown(page)).toBeVisible()
            await expect(page).toHaveURL(FORMS_LIST_URL)
            await expect(page).not.toHaveURL(editorUrl)
        })
    }
})
