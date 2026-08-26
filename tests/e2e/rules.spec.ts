import { expect, type Page, test } from '@playwright/test'
import { login } from './helpers'

// Element-gated settings navigation, mirroring mail's
// navigateToMailboxSettings (mail/tests/helpers.ts): click the rail's
// settings button, then click into the "Rules" link by its visible text —
// never page.goto(), which would tear down the SPA mid-navigation.
async function navigateToRulesSettings(page: Page) {
    await page.getByTestId('nav-settings').click()
    await page.getByText('Rules', { exact: true }).first().click()
    await expect(page.getByText('My rules', { exact: true })).toBeVisible()
}

// The builder's trigger/action pickers are the house Menu component (a plain
// Pressable trigger + text-labeled items, not native <select>/role=menu) —
// click the trigger, then click the option by its exact label.
async function selectFromMenu(
    page: Page,
    trigger: import('@playwright/test').Locator,
    optionLabel: string
) {
    await trigger.click()
    await page.getByText(optionLabel, { exact: true }).click()
}

// RuleRow's DOM nesting isn't stable enough to hop a fixed number of parents
// from the name Text up to the row (RuleRowMain wraps it in a couple of extra
// Views for the badges row). Instead find the nearest ancestor that also
// contains the row's "More actions" trigger — true regardless of exactly how
// deep the name Text sits.
function ruleRow(page: Page, ruleName: string) {
    return page
        .locator('div')
        .filter({ has: page.getByText(ruleName, { exact: true }) })
        .filter({ has: page.getByLabel('More actions') })
        .last()
}

async function openOverflowMenu(page: Page, ruleName: string) {
    await ruleRow(page, ruleName).getByLabel('More actions').click()
}

test.describe('Rules', () => {
    test('create a manual rule, run it now, and see the notification', async ({ page }) => {
        await login(page)
        await navigateToRulesSettings(page)

        const ruleName = `E2E manual rule ${Date.now()}`
        const notifyTitle = `E2E notify ${Date.now()}`

        await page.getByText('New rule', { exact: true }).first().click()
        await expect(page.getByText('New rule', { exact: true }).last()).toBeVisible()

        await page.getByPlaceholder('Rule name').fill(ruleName)

        await selectFromMenu(
            page,
            page.getByText('Select a trigger…', { exact: true }),
            'Run manually'
        )

        await page.getByText('add action', { exact: true }).click()
        await page.getByText('Send me a notification', { exact: true }).click()

        await page.getByText('Title').locator('..').getByRole('textbox').first().fill(notifyTitle)

        await page.getByText('Save', { exact: true }).click()

        // The builder closes on save; the new row renders with its summary line.
        await expect(page.getByText(ruleName, { exact: true })).toBeVisible()
        await expect(
            page.getByText('Run manually · Send me a notification', { exact: false })
        ).toBeVisible()

        await openOverflowMenu(page, ruleName)
        await page.getByText('Run now', { exact: true }).click()

        // "Run now" leaves transient local feedback on the menu item itself
        // (label swaps to "Queued ✓" for 2s) before the menu auto-closes —
        // the real assertion is the notification actually landing.
        const bell = page.getByLabel(/Notifications/)
        await bell.click()
        await expect(page.getByText(notifyTitle, { exact: true })).toBeVisible({ timeout: 15_000 })
        // NotificationDrawer's close button carries accessibilityLabel="Close
        // notifications" — use it directly rather than toggling the bell.
        await page.getByLabel('Close notifications').click()
    })

    test('validation surfaces both missing-name and missing-trigger errors', async ({ page }) => {
        await login(page)
        await navigateToRulesSettings(page)

        await page.getByText('New rule', { exact: true }).first().click()
        await expect(page.getByText('New rule', { exact: true }).last()).toBeVisible()

        await page.getByText('Save', { exact: true }).click()

        await expect(
            page.getByText('Please fix the following errors:', { exact: true })
        ).toBeVisible()
        await expect(page.getByText('Name is required', { exact: true })).toBeVisible()
        await expect(page.getByText('Trigger is required', { exact: true })).toBeVisible()

        await page.getByText('Cancel', { exact: true }).click()
        await expect(
            page.getByText('Please fix the following errors:', { exact: true })
        ).not.toBeVisible()
    })

    test('toggle and delete a rule', async ({ page }) => {
        await login(page)
        await navigateToRulesSettings(page)

        const ruleName = `E2E toggle-delete rule ${Date.now()}`

        await page.getByText('New rule', { exact: true }).first().click()
        await expect(page.getByText('New rule', { exact: true }).last()).toBeVisible()
        await page.getByPlaceholder('Rule name').fill(ruleName)
        await selectFromMenu(
            page,
            page.getByText('Select a trigger…', { exact: true }),
            'Run manually'
        )
        await page.getByText('add action', { exact: true }).click()
        await page.getByText('Send me a notification', { exact: true }).click()
        await page.getByText('Save', { exact: true }).click()
        await expect(page.getByText(ruleName, { exact: true })).toBeVisible()

        const enableSwitch = page.getByLabel(`Disable ${ruleName}`)
        await expect(enableSwitch).toHaveAttribute('aria-checked', 'true')
        await enableSwitch.click()
        await expect(page.getByLabel(`Enable ${ruleName}`)).toHaveAttribute('aria-checked', 'false')

        await openOverflowMenu(page, ruleName)
        await page.getByText('Delete', { exact: true }).click()

        await expect(page.getByText(`Delete "${ruleName}"?`, { exact: true })).toBeVisible()
        // The dialog's own confirm button, not the row's menu item — scope by
        // the dialog to avoid ambiguity with any other "Delete" text on screen.
        await page
            .getByText(`Delete "${ruleName}"?`, { exact: true })
            .locator('..')
            .getByText('Delete', { exact: true })
            .last()
            .click()

        await expect(page.getByText(ruleName, { exact: true })).not.toBeVisible()
    })

    test('run history shows a matched run', async ({ page }) => {
        await login(page)
        await navigateToRulesSettings(page)

        const ruleName = `E2E history rule ${Date.now()}`

        await page.getByText('New rule', { exact: true }).first().click()
        await expect(page.getByText('New rule', { exact: true }).last()).toBeVisible()
        await page.getByPlaceholder('Rule name').fill(ruleName)
        await selectFromMenu(
            page,
            page.getByText('Select a trigger…', { exact: true }),
            'Run manually'
        )
        await page.getByText('add action', { exact: true }).click()
        await page.getByText('Send me a notification', { exact: true }).click()
        await page.getByText('Save', { exact: true }).click()
        await expect(page.getByText(ruleName, { exact: true })).toBeVisible()

        await openOverflowMenu(page, ruleName)
        await page.getByText('Run now', { exact: true }).click()

        await openOverflowMenu(page, ruleName)
        await page.getByText('Run history', { exact: true }).click()

        await expect(page.getByText('Run history', { exact: true })).toBeVisible()
        await expect(page.getByText('Matched', { exact: true }).first()).toBeVisible({
            timeout: 15_000,
        })
    })

    // The trigger menu groups by package. It used to come out alphabetically by
    // slug — a stability hack in use-automation-catalog that matched nothing
    // else in the app — and now follows the user's own app order, with core's
    // package-neutral triggers leading.
    test('the trigger menu groups packages in the sidebar order, core first', async ({ page }) => {
        await login(page)
        await navigateToRulesSettings(page)

        await page.getByText('New rule', { exact: true }).first().click()
        await page.getByText('Select a trigger…', { exact: true }).click()
        // Gate on a known option so the assertions read a painted menu.
        await expect(page.getByTestId('trigger-option-core:manual')).toBeVisible()

        // Menu.Label renders the bare package slug (uppercased in CSS only).
        const groupOrder = await page.evaluate(() => {
            const slugs = ['core', 'mail', 'drive', 'calendar', 'contacts', 'cards', 'calc', 'text']
            const seen: string[] = []
            const walk = (node: Element) => {
                const text = (node.textContent ?? '').trim().toLowerCase()
                if (node.children.length === 0 && slugs.includes(text)) seen.push(text)
                for (const child of Array.from(node.children)) walk(child)
            }
            walk(document.body)
            return seen
        })

        expect(groupOrder.length).toBeGreaterThan(1)
        expect(groupOrder[0]).toBe('core')
        // Not the old alphabetical order — that would have put calc/calendar
        // ahead of core and is exactly what this replaced.
        expect(groupOrder).not.toEqual([...groupOrder].sort())
    })

    // Naming is the first thing a new rule needs, and `autoFocus` alone does
    // not survive GlueStack's focus trap plus ModalContent's enter animation —
    // so this asserts the field is really focused, not merely that the prop is
    // set. Typing without clicking is the behavior users feel.
    test('the name field is focused when a new rule opens', async ({ page }) => {
        await login(page)
        await navigateToRulesSettings(page)

        await page.getByText('New rule', { exact: true }).first().click()
        await expect(page.getByText('New rule', { exact: true }).last()).toBeVisible()

        const nameInput = page.getByPlaceholder('Rule name')
        await expect(nameInput).toBeFocused()
        await page.keyboard.type('typed without clicking')
        await expect(nameInput).toHaveValue('typed without clicking')
    })

    test('editing an existing rule does not steal focus', async ({ page }) => {
        await login(page)
        await navigateToRulesSettings(page)

        const ruleName = `E2E focus rule ${Date.now()}`

        await page.getByText('New rule', { exact: true }).first().click()
        await page.getByPlaceholder('Rule name').fill(ruleName)
        await selectFromMenu(
            page,
            page.getByText('Select a trigger…', { exact: true }),
            'Run manually'
        )
        await page.getByText('add action', { exact: true }).click()
        await page.getByText('Send me a notification', { exact: true }).click()
        await page.getByText('Title').locator('..').getByRole('textbox').first().fill('t')
        await page.getByText('Save', { exact: true }).click()
        await expect(page.getByText(ruleName, { exact: true })).toBeVisible()

        // Reopening is usually to change a condition or action, so grabbing the
        // name field would fight what the user came in to do.
        await openOverflowMenu(page, ruleName)
        await page.getByText('Edit', { exact: true }).click()
        await expect(page.getByText('Edit rule', { exact: true })).toBeVisible()
        await expect(page.getByPlaceholder('Rule name')).not.toBeFocused()
    })

    // The builder's Save/Cancel used to be CLIPPED off-screen: ModalContent is
    // `overflow-hidden` with no height of its own, so header + scroll region +
    // error list + footer could exceed the viewport and the excess simply
    // vanished — and because the modal is centered, what vanished was the
    // footer. toBeVisible() is NOT enough to catch this (a clipped element
    // still reports visible); toBeInViewport is the assertion that fails on the
    // old layout.
    test('the footer stays reachable when the error list is long', async ({ page }) => {
        await login(page)
        // Short viewport so the modal has to cope with real overflow rather
        // than merely fitting by luck on a tall CI screen.
        await page.setViewportSize({ width: 1280, height: 600 })
        await navigateToRulesSettings(page)

        await page.getByText('New rule', { exact: true }).first().click()
        await expect(page.getByText('New rule', { exact: true }).last()).toBeVisible()

        await page.getByText('Save', { exact: true }).click()
        await expect(
            page.getByText('Please fix the following errors:', { exact: true })
        ).toBeVisible()

        await expect(page.getByText('Save', { exact: true })).toBeInViewport()
        await expect(page.getByText('Cancel', { exact: true })).toBeInViewport()
        // Still clickable, not just painted inside the viewport.
        await page.getByText('Cancel', { exact: true }).click()
        await expect(page.getByPlaceholder('Rule name')).not.toBeVisible()
    })

    test('IF and THEN render disabled until a trigger is chosen', async ({ page }) => {
        await login(page)
        await navigateToRulesSettings(page)

        await page.getByText('New rule', { exact: true }).first().click()
        await expect(page.getByText('New rule', { exact: true }).last()).toBeVisible()

        // Both steps stay mounted so the shape of a rule is visible from the
        // start and nothing jumps when the trigger lands.
        await expect(page.getByText('IF', { exact: true })).toBeVisible()
        await expect(page.getByText('THEN', { exact: true })).toBeVisible()
        await expect(page.getByText('Choose a trigger first').first()).toBeVisible()

        // "add action" is inert, not merely faint. pointerEvents:none sets no
        // DOM `disabled`, so assert the click itself cannot land — a trial
        // click fails actionability instead of opening an empty menu.
        await expect(
            page.getByText('add action', { exact: true }).click({ trial: true, timeout: 2000 })
        ).rejects.toThrow()

        // Picking a synthetic trigger keeps IF mounted but explains that it can
        // never apply, rather than silently vanishing.
        await selectFromMenu(
            page,
            page.getByText('Select a trigger…', { exact: true }),
            'Run manually'
        )
        await expect(page.getByText('This trigger has no fields to filter on')).toBeVisible()
        await expect(page.getByText('Choose a trigger first')).not.toBeVisible()
    })

    // "A user joins" is core-owned, so this holds in a lean shell with no
    // feature packages installed — unlike mail/drive triggers.
    test('a record trigger offers a ready condition row that need not be filled', async ({
        page,
    }) => {
        await login(page)
        await navigateToRulesSettings(page)

        const ruleName = `E2E ready-condition rule ${Date.now()}`

        await page.getByText('New rule', { exact: true }).first().click()
        await expect(page.getByText('New rule', { exact: true }).last()).toBeVisible()
        await page.getByPlaceholder('Rule name').fill(ruleName)

        await selectFromMenu(
            page,
            page.getByText('Select a trigger…', { exact: true }),
            'A user joins'
        )

        // The row is there immediately — reaching a first condition used to
        // require clicking "add OR group" first, which means nothing when there
        // is nothing to OR with.
        await expect(page.getByText('Field…', { exact: true })).toBeVisible()
        await expect(page.getByText('add OR group', { exact: true })).toBeVisible()

        // Saving WITHOUT touching that row must work: the offered row is
        // rendered, never seeded into the draft, so the rule saves with no
        // conditions (matching everything) instead of failing validation.
        await page.getByText('add action', { exact: true }).click()
        await page.getByText('Send me a notification', { exact: true }).click()
        await page.getByText('Title').locator('..').getByRole('textbox').first().fill('Welcome')
        await page.getByText('Save', { exact: true }).click()

        await expect(page.getByText(ruleName, { exact: true })).toBeVisible()
    })

    test('the rules help topic is searchable and renders', async ({ page }) => {
        await login(page)

        await page.getByTestId('nav-help').click()
        await expect(page).toHaveURL(/\/help$/)

        await page.getByPlaceholder('Search help topics').fill('rules')
        await expect(page.getByText('Automation rules', { exact: true })).toBeVisible()

        await page.getByText('Automation rules', { exact: true }).click()
        await expect(page).toHaveURL(/\/help\/core\/rules$/)
        await expect(page.getByText('What a rule does', { exact: true })).toBeVisible()
    })
})
