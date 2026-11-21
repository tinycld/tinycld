import { expect, type Locator, type Page, test } from '@playwright/test'
import { authStorageKey } from './auth-key-helpers'
import { readEmailLog, waitForEmailTo } from './email-log-helpers'
import { openSettingsItem, skipWithoutReportsStub } from './helpers'
import { openExportMenu } from './report-view-helpers'
import {
    loginAsShippedOwner,
    openTeamBalance,
    TEAM_BALANCE_TITLE,
    uniqueName,
} from './reports-helpers'

/**
 * Report exports (email): "Email report…" in the Export menu, its dialog, the send
 * and its refusals, the row's state while the server cannot send, and the phone's
 * "⋯" entry and sheet. Each test signs in as the shipped reports' owner (an admin,
 * so Settings → Audit log opens), opens the stub's shipped "Stub team balance",
 * and sends to addresses no other test uses: the e2e server logs every message
 * to the capture log, which the test reads back by recipient.
 *
 * `page.route` fakes only what the server cannot be made to give: a 429 refusal,
 * a held send, and the email status (false, held, failed).
 */

const DESKTOP = { width: 1600, height: 900 }
const PHONE = { width: 390, height: 844 }
const RATE_LIMIT = 'You have sent 20 report emails in the last hour. Try again later.'
const NOT_SET_UP = 'Email sending is not set up'

/** An email address no other test, worker or run uses. */
function uniqueAddress(prefix: string): string {
    return `${uniqueName(prefix).toLowerCase().replace(/\s+/g, '-')}@example.test`
}

async function openReport(page: Page) {
    await loginAsShippedOwner(page)
    await page.setViewportSize(DESKTOP)
    await openTeamBalance(page)
    await expect(page.getByTestId('report-export-open')).toBeEnabled()
}

/** Opens the dialog with the mouse: the Export menu, then "Email report…". */
async function openEmailDialog(page: Page) {
    await openExportMenu(page)
    await page.getByTestId('report-export-email').click()
    await expect(page.getByTestId('report-email-to')).toBeFocused()
}

function sentToast(page: Page, title: string): Locator {
    return page.getByTestId('toast-card').filter({ hasText: title })
}

/** The signed-in user's own email, read from the stored session. */
async function signedInEmail(page: Page): Promise<string> {
    const key = await authStorageKey(page)
    return page.evaluate(k => JSON.parse(localStorage.getItem(k) ?? '{}').record?.email ?? '', key)
}

/** The Audit log screen's innermost boxes that hold a `report.email` of this report. */
function emailAuditRows(page: Page) {
    const both = (locator: Locator) =>
        locator
            .filter({ has: page.getByText('report.email', { exact: true }) })
            .filter({ has: page.getByText(TEAM_BALANCE_TITLE, { exact: true }) })
    return both(page.locator('div')).filter({ hasNot: both(page.locator('div')) })
}

function isEmailSend(url: string, method: string): boolean {
    return method === 'POST' && new URL(url).pathname.endsWith('/api/reports/email')
}

test.describe('report-exports: email', () => {
    test.skip(() => skipWithoutReportsStub(), 'needs the salesstub package')

    test('send a report from the keyboard', async ({ page }) => {
        await openReport(page)
        const periodWords = (await page.getByTestId('report-document-period').textContent()) ?? ''
        expect(periodWords).not.toBe('')

        await openExportMenu(page)
        const rows = [
            'report-export-pdf',
            'report-export-xlsx',
            'report-export-csv',
            'report-export-email',
        ]
        const labels = ['Download PDF', 'Download Excel', 'Download CSV', 'Email report…']
        const boxes: Awaited<ReturnType<Locator['boundingBox']>>[] = []
        for (const [i, testId] of rows.entries()) {
            const row = page.getByTestId(testId)
            await expect(row).toHaveAttribute('role', 'menuitem')
            await expect(row).toContainText(labels[i])
            boxes.push(await row.boundingBox())
        }
        const [pdf, xlsx, csv, email] = boxes.map(box => {
            if (!box) throw new Error('a menu row has no box')
            return box
        })
        expect(pdf.y).toBeLessThan(xlsx.y)
        expect(xlsx.y).toBeLessThan(csv.y)
        expect(csv.y).toBeLessThan(email.y)
        // A separator sets "Email report…" apart: its gap is wider than the downloads' gap.
        const downloadGap = csv.y - (xlsx.y + xlsx.height)
        expect(email.y - (csv.y + csv.height)).toBeGreaterThan(downloadGap)
        await page.getByTestId('report-export-email').click()

        // From here on, the keyboard only.
        const dialog = page.getByTestId('report-email-dialog')
        await expect(dialog).toBeVisible()
        await expect(page.getByTestId('report-email-to')).toBeFocused()
        await expect(page.getByTestId('report-email-subject')).toHaveValue(
            `${TEAM_BALANCE_TITLE} — ${periodWords}`
        )
        await expect(page.getByTestId('report-email-message')).toHaveValue(
            new RegExp(`^Hello,[\\s\\S]*${TEAM_BALANCE_TITLE}`)
        )
        const attachment = page.getByTestId('report-email-attachment')
        await expect(attachment).toHaveText(/\.pdf$/)
        const attachmentName = (await attachment.textContent()) ?? ''

        const first = uniqueAddress('email kb to')
        const second = uniqueAddress('email kb to')
        const cc = uniqueAddress('email kb cc')
        await page.keyboard.type(`${first}, ${second}`)
        await page.keyboard.press('Tab')
        await expect(page.getByTestId('report-email-cc')).toBeFocused()
        await page.keyboard.type(cc)
        await page.keyboard.press('ControlOrMeta+Enter')

        await expect(dialog).toHaveCount(0)
        await expect(sentToast(page, `Report sent to ${first} and 2 others`)).toBeVisible()
        const sent = await waitForEmailTo(first)
        expect(sent.to.map(r => r.email)).toEqual([first, second])
        expect(sent.cc?.map(r => r.email)).toEqual([cc])
        expect(sent.subject).toBe(`${TEAM_BALANCE_TITLE} — ${periodWords}`)
        expect(sent.attachments).toBe(1)
        expect(sent.attachment_names?.[0]).toBe(attachmentName)
        expect(sent.reply_to).toBe(await signedInEmail(page))
        expect(sent.from).toContain(' via ')

        // Reopened from the keyboard, the dialog keeps no address.
        await page.getByTestId('report-export-open').focus()
        await page.keyboard.press('Enter')
        const emailRow = page.getByTestId('report-export-email')
        await expect(emailRow).toBeVisible()
        for (let step = 0; step < 6; step++) {
            if (await emailRow.evaluate(el => el === document.activeElement)) break
            await page.keyboard.press('ArrowDown')
        }
        await expect(emailRow).toBeFocused()
        await page.keyboard.press('Enter')
        await expect(page.getByTestId('report-email-to')).toBeFocused()
        await expect(page.getByTestId('report-email-to')).toHaveValue('')
        await expect(page.getByTestId('report-email-cc')).toHaveValue('')
        const cancelled = uniqueAddress('email kb cancelled')
        await page.keyboard.type(cancelled)
        await page.keyboard.press('Escape')
        await expect(dialog).toHaveCount(0)
        expect(readEmailLog().some(e => e.to.some(r => r.email === cancelled))).toBe(false)

        // The send is in the audit log.
        await openSettingsItem(page, 'Audit Log')
        await page.getByTestId('audit-search').fill('report.email')
        await expect(emailAuditRows(page).first()).toBeVisible()
    })

    test('a send that cannot go keeps the form', async ({ page, context }) => {
        await openReport(page)
        let sends = 0
        page.on('request', request => {
            if (isEmailSend(request.url(), request.method())) sends++
        })
        await openEmailDialog(page)
        const dialog = page.getByTestId('report-email-dialog')
        const to = page.getByTestId('report-email-to')
        const send = page.getByTestId('report-email-send')

        // Field errors draw inline, and nothing is sent.
        await to.fill('not-an-address')
        await page.keyboard.press('ControlOrMeta+Enter')
        await expect(
            dialog.getByText('not-an-address is not an email address', { exact: true })
        ).toBeVisible()
        await to.fill(Array.from({ length: 11 }, (_, i) => `p${i}@example.test`).join(', '))
        await page.keyboard.press('ControlOrMeta+Enter')
        await expect(
            dialog.getByText('Use at most 10 addresses in To and Cc', { exact: true })
        ).toBeVisible()
        expect(sends).toBe(0)

        // A server refusal shows in the summary and keeps every typed value.
        const address = uniqueAddress('email refused')
        const subject = uniqueName('Refused subject')
        const message = 'A message typed by hand.'
        await to.fill(address)
        await page.getByTestId('report-email-subject').fill(subject)
        await page.getByTestId('report-email-message').fill(message)
        await page.route('**/api/reports/email', route =>
            route.fulfill({
                status: 429,
                contentType: 'application/json',
                body: JSON.stringify({ status: 429, message: RATE_LIMIT, data: {} }),
            })
        )
        await send.click()
        await expect(page.getByTestId('report-email-refusal')).toContainText(RATE_LIMIT)
        await expect(dialog).toBeVisible()
        await expect(to).toHaveValue(address)
        await expect(page.getByTestId('report-email-subject')).toHaveValue(subject)
        await expect(page.getByTestId('report-email-message')).toHaveValue(message)
        // The button also holds its hidden "⌘↵" hint, so the label is matched at the start.
        await expect(send).toHaveText(/^Send(?!ing)/)
        await expect(send).toBeEnabled()
        await page.unrouteAll({ behavior: 'wait' })

        // While a send is held, the buttons wait and say so.
        let release = () => {}
        const held = new Promise<void>(resolve => {
            release = resolve
        })
        await page.route('**/api/reports/email', async route => {
            await held
            await route.fulfill({
                status: 200,
                contentType: 'application/json',
                body: JSON.stringify({ sent: true, recipients: 1 }),
            })
        })
        await send.click()
        await expect(send).toHaveText(/^Sending…/)
        await expect(send).toBeDisabled()
        await expect(page.getByTestId('report-email-cancel')).toBeDisabled()
        release()
        await expect(dialog).toHaveCount(0)
        await page.unrouteAll({ behavior: 'wait' })
        expect(sends).toBe(2)

        // Offline, Send waits and the typed values stay.
        await openEmailDialog(page)
        const offlineAddress = uniqueAddress('email offline')
        await to.fill(offlineAddress)
        await expect(send).toBeEnabled()
        const offlineNotice = page.getByText('Offline — waiting for a connection', { exact: true })
        await context.setOffline(true)
        await expect(offlineNotice).toBeVisible({ timeout: 3_000 })
        await expect(send).toBeDisabled()
        await expect(to).toHaveValue(offlineAddress)
        await context.setOffline(false)
        await expect(offlineNotice).toBeHidden({ timeout: 2_000 })
        await expect(send).toBeEnabled()
        await expect(to).toHaveValue(offlineAddress)
    })

    test('"Email report…" is offered only when the server can send', async ({ page }) => {
        // The status is read once when the report view mounts; it is held from the start.
        let release = () => {}
        const held = new Promise<void>(resolve => {
            release = resolve
        })
        await page.route('**/api/reports/email-status', async route => {
            await held
            await route.fulfill({
                status: 500,
                contentType: 'application/json',
                body: JSON.stringify({ status: 500, message: 'Something went wrong.', data: {} }),
            })
        })
        await openReport(page)
        const emailRow = page.getByTestId('report-export-email')
        const description = page.getByTestId('report-export-email-description')
        const dialog = page.getByTestId('report-email-dialog')

        // Not answered yet: disabled, with no second line.
        await openExportMenu(page)
        await expect(emailRow).toHaveAttribute('aria-disabled', 'true')
        await expect(description).toHaveCount(0)
        await page.keyboard.press('Escape')
        await expect(emailRow).toHaveCount(0)

        // The read failed: the row is offered, and the send's own refusal covers it.
        release()
        await openExportMenu(page)
        await expect(emailRow).not.toHaveAttribute('aria-disabled', 'true')
        await expect(description).toHaveCount(0)
        await emailRow.click()
        await expect(dialog).toBeVisible()
        await page.keyboard.press('Escape')
        await expect(dialog).toHaveCount(0)
        await page.unrouteAll({ behavior: 'wait' })

        // The server cannot send: disabled, saying why, and a press opens nothing.
        await page.route('**/api/reports/email-status', route =>
            route.fulfill({
                status: 200,
                contentType: 'application/json',
                body: JSON.stringify({ canSend: false }),
            })
        )
        const answered = page.waitForResponse(r => r.url().includes('/api/reports/email-status'))
        await openTeamBalance(page)
        await answered
        await openExportMenu(page)
        await expect(emailRow).toHaveAttribute('aria-disabled', 'true')
        await expect(description).toHaveText(NOT_SET_UP)
        await emailRow.click({ force: true })
        await expect(dialog).toHaveCount(0)
        await expect(page.getByTestId('report-email-to')).toHaveCount(0)
        await page.unrouteAll({ behavior: 'wait' })
    })

    test('on a phone "⋯" lists the four entries and the dialog is a sheet', async ({ page }) => {
        await openReport(page)
        await page.setViewportSize(PHONE)
        // The phone layout: "⋯" holds the toolbar's actions (the report may be empty,
        // so there is no table to wait for).
        const more = page.getByTestId('toolbar-more-button')
        await expect(more).toBeVisible()
        await expect(page.getByTestId('report-export-open')).toHaveCount(0)

        await more.click()
        for (const [testId, label] of [
            ['report-export-pdf', 'Download PDF'],
            ['report-export-xlsx', 'Download Excel'],
            ['report-export-csv', 'Download CSV'],
            ['report-export-email', 'Email report…'],
        ]) {
            const row = page.getByTestId(testId)
            await expect(row).toHaveAttribute('role', 'menuitem')
            await expect(row).toContainText(label)
        }
        await page.getByTestId('report-export-email').click()

        // A core Sheet has no dialog role: it is found by its title.
        await expect(page.getByText('Email report', { exact: true })).toBeVisible()
        await expect(page.getByTestId('report-email-to')).toBeVisible()
        await page.getByText('Excel', { exact: true }).click()
        const attachment = page.getByTestId('report-email-attachment')
        await expect(attachment).toHaveText(/\.xlsx$/)
        const attachmentName = (await attachment.textContent()) ?? ''

        const address = uniqueAddress('email phone')
        await page.getByTestId('report-email-to').fill(address)
        await page.getByTestId('report-email-send').click()
        await expect(sentToast(page, `Report sent to ${address}`)).toBeVisible()
        const sent = await waitForEmailTo(address)
        expect(sent.attachment_names?.[0]).toBe(attachmentName)
        expect(sent.attachment_names?.[0]).toMatch(/\.xlsx$/)
    })
})
