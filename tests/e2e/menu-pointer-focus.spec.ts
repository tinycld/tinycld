import { expect, type Page, test } from '@playwright/test'
import { login, openSettingsItem } from './helpers'

/**
 * A picker menu whose options arrive after it opens must keep the focus in its
 * search field, even when the pointer rests where the grown menu lands.
 *
 * The picker is core's RelationRecordPicker, reached as the "Apply label"
 * action's Label value in the rule builder (Settings → Rules → New rule,
 * trigger "A user joins"): a Menu with a search field and options from a live
 * query over `labels`. The spec makes its own labels in Settings → Labels so
 * the options exist on a lean shell.
 *
 * The geometry that made the defect: the trigger sits so low that the empty
 * menu (search field and "Searching…") opens ABOVE it, flush with it. When
 * the options arrive, the grown menu was drawn for a frame at its old top, so
 * it reached down over the trigger and the resting pointer; the option under
 * the pointer took `mouseenter` and focused itself, out of the search field.
 */

// Rule-builder height at which the Label picker's trigger, scrolled to the
// bottom of the builder's body, leaves less room below it than the empty menu
// needs (measured at 1280 wide: about 70 px of room, the empty menu is taller).
const VIEWPORT = { width: 1280, height: 600 }
// Long enough for the menu to open and place itself empty before the options come.
const LIST_HOLD_MS = 600

async function createLabel(page: Page, name: string) {
    await page.getByText('New label...', { exact: true }).click()
    const input = page.getByPlaceholder('Label name')
    await expect(input).toBeFocused()
    await input.fill(name)
    await input.press('Enter')
    await expect(page.getByText(name, { exact: true })).toBeVisible()
}

async function openApplyLabelAction(page: Page) {
    await openSettingsItem(page, 'Rules')
    await expect(page.getByText('My rules', { exact: true })).toBeVisible()
    await page.getByText('New rule', { exact: true }).first().click()
    await page.getByText('Select a trigger…', { exact: true }).click()
    await page.getByText('A user joins', { exact: true }).click()
    await page.getByText('add action', { exact: true }).click()
    await page.getByText('Apply label', { exact: true }).click()
}

test.describe('menu-pointer-focus: a picker keeps its search focus while its options arrive', () => {
    test('the options land under a resting pointer and the search field keeps the focus; a moving pointer focuses an option', async ({
        page,
        browserName,
    }) => {
        test.skip(
            browserName !== 'chromium',
            'CPU throttling is a Chrome DevTools Protocol command (Emulation.setCPUThrottlingRate)'
        )
        await login(page)
        const stamp = `${Date.now()}${Math.random().toString(36).slice(2, 6)}`
        const names = [1, 2, 3].map(n => `Focus ${stamp} ${n}`)
        await openSettingsItem(page, 'Labels')
        for (const name of names) await createLabel(page, name)

        await openApplyLabelAction(page)
        await page.setViewportSize(VIEWPORT)
        const trigger = page.getByText('Select…', { exact: true })
        await expect(trigger).toBeVisible()
        await trigger.evaluate(el => el.scrollIntoView({ block: 'end' }))

        // Hold the options' list request, so they arrive after the menu has
        // opened and placed itself empty; then let the real request through.
        await page.route('**/api/collections/labels/records**', async route => {
            if (route.request().method() !== 'GET') return route.continue()
            await new Promise(resolve => setTimeout(resolve, LIST_HOLD_MS))
            await route.continue()
        })
        const cdp = await page.context().newCDPSession(page)
        await cdp.send('Emulation.setCPUThrottlingRate', { rate: 6 })
        try {
            // The click leaves the pointer resting on the trigger.
            await trigger.click()
            const search = page.getByPlaceholder('Search…')
            await expect(search).toBeFocused()
            const firstOption = page.getByRole('menuitem', { name: names[0], exact: true })
            await expect(firstOption).toBeVisible()

            // A steal can come a frame after the options show, while the grown
            // surface re-places: assert only once its box holds still.
            const surface = page.getByRole('menu').filter({ has: search })
            let lastBox = ''
            await expect
                .poll(async () => {
                    const box = JSON.stringify(await surface.boundingBox())
                    const isStable = box === lastBox && box !== 'null'
                    lastBox = box
                    return isStable
                })
                .toBe(true)

            // Names what holds the focus, so a failure says which row took it.
            const focused = () =>
                page.evaluate(() => {
                    const el = document.activeElement
                    if (el?.getAttribute('role') === 'menuitem') return `option ${el.textContent}`
                    return el?.getAttribute('placeholder') ?? el?.tagName ?? 'nothing'
                })
            await expect.poll(focused).toBe('Search…')
            await expect(search).toBeFocused()
            await expect(page.locator('[role="menuitem"]:focus')).toHaveCount(0)

            // A pointer that really moves onto a row still focuses it.
            const target = page.getByRole('menuitem', { name: names[1], exact: true })
            const box = await target.boundingBox()
            if (!box) throw new Error('the option has no box')
            await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2, { steps: 5 })
            await expect(target).toBeFocused()
        } finally {
            await cdp.send('Emulation.setCPUThrottlingRate', { rate: 1 })
            await page.unrouteAll({ behavior: 'wait' })
        }
    })
})
