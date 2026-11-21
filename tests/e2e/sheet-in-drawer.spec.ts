import { expect, type Locator, type Page, test } from '@playwright/test'
import {
    login,
    navigateToPackage,
    skipWithoutReportsStub,
    TEST_COLLABORATOR_EMAIL,
    TEST_COLLABORATOR_NAME,
} from './helpers'
import { openReportStub } from './reports-helpers'

/**
 * A Sheet opened from inside a core Drawer at phone width. On a phone a Menu,
 * a Popover and a Dialog all present as a Sheet, which renders into the
 * `sheet` overlay host inside the mobile content. An overlay Drawer is a
 * gluestack portal that draws after the app tree, so a Sheet in that host is
 * covered by the Drawer it was opened from. The Group drawer's remove-member
 * confirm is such a Sheet. No core screen has an overlay Drawer that holds an
 * `auto` Menu, so the Menu case runs on the salesstub test package's landing
 * screen (tests/scripts/reports-stub/screens/index.tsx.txt, `DrawerMenuDemo`).
 */

const PHONE = { width: 390, height: 844 }
const WIDE = { width: 1280, height: 800 }

/**
 * Whether the element is painted topmost at its centre point (nothing covers it).
 *
 * The open sheet is modal, so the overlay module marks the rest of the
 * document `inert`, and hit testing skips inert nodes: a plain elementFromPoint
 * (and a Playwright click) passes straight through a drawer that is painted
 * over the sheet. The probe lifts `inert` and puts it back in one synchronous
 * task (no frame is painted and React does not run in between), so the hit
 * test reads the paint order alone.
 */
function isTopmostAtCentre(target: Locator) {
    return target.evaluate(el => {
        const inert = Array.from(document.querySelectorAll('[inert]'))
        for (const node of inert) node.removeAttribute('inert')
        const r = el.getBoundingClientRect()
        const hit = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2)
        for (const node of inert) node.setAttribute('inert', '')
        return !!hit && el.contains(hit)
    })
}

test.describe('sheet-in-drawer: a sheet opened inside a drawer draws above it', () => {
    test('removes a group member through the confirm sheet on a phone, and the drawer stays open', async ({
        page,
    }) => {
        const groupName = `Sheet drawer ${Date.now().toString(36)}`

        await login(page)
        await navigateToPackage(page, 'settings')
        await page.getByTestId('settings-index').getByText('Groups', { exact: true }).click()
        await expect(page.getByTestId('groups-new-button')).toBeVisible()
        await page.setViewportSize(PHONE)
        await expect(page.getByTestId('nav-more')).toBeVisible()

        await page.getByTestId('groups-new-button').click()
        await page.getByTestId('name').filter({ visible: true }).fill(groupName)
        await page.getByTestId('group-create-submit').click()
        await expect(page.getByTestId('group-delete')).toBeVisible()

        await page.getByTestId('group-add-member-search').fill('collaborator')
        await page
            .getByRole('button', { name: `Add ${TEST_COLLABORATOR_NAME}`, exact: true })
            .click()
        const memberRow = page.getByTestId(`group-member-row-${TEST_COLLABORATOR_EMAIL}`)
        await expect(memberRow).toBeVisible()

        await memberRow
            .getByRole('button', { name: `Remove ${TEST_COLLABORATOR_NAME} from group` })
            .click()
        const title = page.getByText(`Remove ${TEST_COLLABORATOR_NAME} from "${groupName}"?`, {
            exact: true,
        })
        await expect(title).toBeVisible()
        const confirm = page.getByRole('button', { name: 'Remove', exact: true })
        await expect(confirm).toBeVisible()

        // The confirm sheet is drawn above the drawer it was opened from.
        await expect.poll(() => isTopmostAtCentre(title)).toBe(true)
        await expect.poll(() => isTopmostAtCentre(confirm)).toBe(true)

        await confirm.click()
        await expect(title).toBeHidden()
        await expect(memberRow).toBeHidden()
        // The choice closed only the sheet: the drawer is still open on the group.
        await expect(page.getByText('No members yet.', { exact: true })).toBeVisible()
        await expect(page.getByTestId('group-delete')).toBeVisible()
    })
})

test.describe('sheet-in-drawer: a menu opened inside a drawer', () => {
    test.skip(() => skipWithoutReportsStub(), 'needs the salesstub package')

    async function openDrawer(page: Page, viewport: { width: number; height: number }) {
        await login(page)
        await openReportStub(page)
        await page.setViewportSize(viewport)
        await page.getByTestId('salesstub-drawer-open').click()
        const drawer = page.getByTestId('salesstub-drawer')
        await expect(drawer).toBeVisible()
        await drawer.getByTestId('salesstub-drawer-menu-trigger').click()
        return drawer
    }

    test('chooses a menu item from the sheet above the drawer on a phone', async ({ page }) => {
        const drawer = await openDrawer(page, PHONE)
        const menu = page.getByTestId('salesstub-drawer-menu')
        await expect(menu.getByTestId('sheet-handle')).toBeVisible()
        const south = page.getByTestId('salesstub-drawer-menu-south')
        await expect(south).toBeVisible()

        // The menu's sheet is drawn above the drawer it was opened from.
        await expect.poll(() => isTopmostAtCentre(south)).toBe(true)

        await south.click()
        await expect(menu).toBeHidden()
        await expect(drawer.getByTestId('salesstub-drawer-chosen')).toHaveText('South')
        // The choice closed only the menu: the drawer is still open.
        await expect(drawer).toBeVisible()
    })

    test('chooses a menu item from the popover inside the drawer on a wide window', async ({
        page,
    }) => {
        const drawer = await openDrawer(page, WIDE)
        const menu = page.getByTestId('salesstub-drawer-menu')
        await expect(menu).toBeVisible()
        // A popover, not a sheet.
        await expect(menu.getByTestId('sheet-handle')).toHaveCount(0)
        const east = page.getByTestId('salesstub-drawer-menu-east')
        await expect.poll(() => isTopmostAtCentre(east)).toBe(true)

        await east.click()
        await expect(menu).toBeHidden()
        await expect(drawer.getByTestId('salesstub-drawer-chosen')).toHaveText('East')
        await expect(drawer).toBeVisible()
    })
})
