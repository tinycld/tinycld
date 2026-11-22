import { expect, type Locator, type Page, test } from '@playwright/test'
import { skipWithoutReportsStub } from './helpers'
import { pressToolbar } from './report-view-helpers'
import { loginAsShippedOwner, openTeamBalance } from './reports-helpers'

/**
 * On a phone the report builder is a full-height sheet over the report. Its title
 * row and Close must fit in the space the sheet is given, on a tall and on a short
 * phone, so the user can see what is open and close it. The shipped "Stub team
 * balance" report is only opened and closed, never changed.
 */

test.skip(() => skipWithoutReportsStub(), 'needs the salesstub package')

const DESKTOP = { width: 1600, height: 900 }
const PHONES = [
    { name: '390 x 844', size: { width: 390, height: 844 } },
    { name: '375 x 667', size: { width: 375, height: 667 } },
]

type Size = { width: number; height: number }

async function fitsViewport(locator: Locator, size: Size) {
    const box = await locator.boundingBox()
    if (!box) return false
    return (
        box.x >= 0 &&
        box.y >= 0 &&
        box.x + box.width <= size.width &&
        box.y + box.height <= size.height
    )
}

// The sheet's box is the space its host gives it (above the phone's tab bar); a
// part drawn past it is cut off or under the tab bar.
async function fitsSheet(locator: Locator, sheet: Locator) {
    const box = await locator.boundingBox()
    const host = await sheet.boundingBox()
    if (!box || !host) return false
    return box.y >= host.y && box.y + box.height <= host.y + host.height
}

async function openPhoneReport(page: Page, size: Size) {
    await loginAsShippedOwner(page)
    await page.setViewportSize(DESKTOP)
    await openTeamBalance(page)
    await page.setViewportSize(size)
    await expect(page.getByTestId('toolbar-more-button')).toBeVisible()
}

test.describe('reports: the phone builder sheet fits its screen', () => {
    for (const phone of PHONES) {
        test(`at ${phone.name} Edit report shows its title row and Close in view, and Close closes it`, async ({
            page,
        }) => {
            await openPhoneReport(page, phone.size)
            await pressToolbar(page, 'report-edit-open', 'Edit report')
            const builder = page.getByTestId('report-builder')
            await expect(builder).toBeVisible()
            const title = builder.getByText('Edit report', { exact: true })
            const close = builder.getByTestId('report-builder-close')
            await expect.poll(() => fitsViewport(title, phone.size)).toBe(true)
            await expect.poll(() => fitsViewport(close, phone.size)).toBe(true)
            // The content takes the sheet's height, not the window's: from the
            // title row down to the footer's Save, all of it is inside the sheet.
            await expect.poll(() => fitsSheet(title, builder)).toBe(true)
            const save = builder.getByTestId('report-builder-save')
            await expect.poll(() => fitsSheet(save, builder)).toBe(true)
            await close.click()
            await expect(page.getByTestId('report-builder')).toBeHidden()
            await expect(page.getByTestId('report-title')).toBeVisible()
        })
    }
})
