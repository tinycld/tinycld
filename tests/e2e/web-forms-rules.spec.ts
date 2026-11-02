import { expect, type Page, test } from '@playwright/test'
import { buildAndPublish, fillAndSend, publishForm, visitAnonymously } from './web-forms-helpers'

/**
 * Web form destinations end to end. A destination is an org rule on
 * "A web form is sent" for one form; the tests use core:notify so the result
 * shows in the admin's notifications without any external service.
 */

function builder(page: Page) {
    return page.getByRole('dialog').filter({ hasText: /^(New rule|Edit rule)/ })
}

async function addAction(page: Page, label: string) {
    await builder(page).getByText('add action', { exact: true }).click()
    await page.getByText(label, { exact: true }).click()
}

async function setActionParam(page: Page, label: string, value: string) {
    const input = builder(page)
        .getByText(label, { exact: true })
        .locator('..')
        .getByRole('textbox')
        .first()
    await input.fill(value)
}

async function saveRule(page: Page) {
    await builder(page).getByText('Save', { exact: true }).click()
}

async function addNotifyDestination(page: Page, title: string) {
    await page.getByTestId('web-form-add-destination').click()
    // The builder opens on the form trigger with this form picked.
    await expect(page.getByTestId('rule-form-picker')).toBeVisible()
    await addAction(page, 'Send me a notification')
    await setActionParam(page, 'Title', title)
    await saveRule(page)
    await expect(builder(page)).toBeHidden()
}

async function editFirstDestinationTitle(page: Page, title: string) {
    await page.getByTestId('web-form-destinations').getByLabel('More actions').first().click()
    await page.getByText('Edit', { exact: true }).click()
    await setActionParam(page, 'Title', title)
    await saveRule(page)
    await expect(builder(page)).toBeHidden()
}

function submissionRows(page: Page) {
    return page.getByTestId('web-form-submissions').locator('[data-testid^="web-form-submission-"]')
}

test('a submission reaches its destination, shows as delivered, and other forms are not affected', async ({
    page,
    browser,
}) => {
    const first = await buildAndPublish(page, `Leads ${Date.now()}`)
    await addNotifyDestination(page, 'New lead {{fields.name}}')
    const second = await publishForm(page, `Other ${Date.now()}`)

    // The other form's visitor sends first: by the time the lead's notice
    // shows, a wrongly delivered one from the other form would show too.
    const other = await visitAnonymously(browser, second)
    await fillAndSend(other, other, 'Not Me')
    await expect(other.getByTestId('web-form-confirmation')).toBeVisible()
    const visitor = await visitAnonymously(browser, first)
    await fillAndSend(visitor, visitor, 'Lead Visitor')
    await expect(visitor.getByTestId('web-form-confirmation')).toBeVisible()

    await page.getByLabel(/Notifications/).click()
    await expect(page.getByText('New lead Lead Visitor', { exact: true })).toBeVisible()
    await expect(page.getByText('New lead Not Me', { exact: true })).toHaveCount(0)
})

test('a failed destination shows as not delivered, and retry delivers it after a fix', async ({
    page,
    browser,
}) => {
    const url = await buildAndPublish(page, `Retry ${Date.now()}`)
    // An empty title makes core:notify fail: the template reads a missing field.
    await addNotifyDestination(page, '{{fields.missing}}')
    const visitor = await visitAnonymously(browser, url)
    await fillAndSend(visitor, visitor, 'Retry Visitor')
    await expect(visitor.getByTestId('web-form-confirmation')).toBeVisible()

    await page.getByTestId('form-tab-submissions').click()
    const row = submissionRows(page).first()
    await expect(row.locator('[data-testid^="web-form-delivery-"]')).toContainText(
        '0 of 1 delivered'
    )

    // Fix the rule from the Publish tab, then retry from the submission.
    await page.getByTestId('form-tab-publish').click()
    await editFirstDestinationTitle(page, 'Fixed {{fields.name}}')
    await page.getByTestId('form-tab-submissions').click()
    await row.click()
    await page.locator('[data-testid^="web-form-retry-"]').first().click()
    await expect(row.locator('[data-testid^="web-form-delivery-"]')).toContainText(
        '1 of 1 delivered'
    )
})

// The builder reads the form's answers as condition fields once the form's
// definition loads. Reopened from Settings → Rules, where the form is not
// loaded yet, a destination with such a condition must save unchanged.
test('a destination with a condition on a form answer saves again unchanged', async ({ page }) => {
    const formName = `Condition ${Date.now()}`
    await buildAndPublish(page, formName)
    await page.getByTestId('web-form-add-destination').click()
    await builder(page).getByText('Field…', { exact: true }).click()
    await page.getByTestId('condition-field-option-fields.name').click()
    await builder(page).getByLabel('Condition value').fill('Lead')
    await addAction(page, 'Send me a notification')
    await setActionParam(page, 'Title', 'A lead')
    await saveRule(page)
    await expect(builder(page)).toBeHidden()

    await page.getByTestId('nav-settings').click()
    await page.getByText('Rules', { exact: true }).first().click()
    await page.getByText('Organization', { exact: true }).click()
    const row = page
        .locator('div')
        .filter({ has: page.getByText(`${formName}:`, { exact: true }) })
        .filter({ has: page.getByLabel('More actions') })
        .last()
    await row.getByLabel('More actions').click()
    await page.getByText('Edit', { exact: true }).click()
    await expect(builder(page).getByLabel('Condition value')).toHaveValue('Lead')
    await saveRule(page)
    await expect(builder(page)).toBeHidden()
})

test('an auto-reply to a form address needs proof of work', async ({ page }) => {
    await buildAndPublish(page, `Reply ${Date.now()}`)
    await page.getByTestId('web-form-add-destination').click()
    await addAction(page, 'Send an email')
    await setActionParam(page, 'To', '{{fields.name}}')
    await saveRule(page)
    await expect(
        builder(page).getByText(/needs "Ask the browser to prove it is not a bot"/)
    ).toBeVisible()
})

// One query reads the delivery status of the whole page of submissions, not
// one for each row: a busy form would otherwise open a subscription per row.
test('the submissions list reads delivery status in one request', async ({ page, browser }) => {
    const url = await buildAndPublish(page, `Batch ${Date.now()}`)
    for (const name of ['Batch One', 'Batch Two']) {
        const visitor = await visitAnonymously(browser, url)
        await fillAndSend(visitor, visitor, name)
        await expect(visitor.getByTestId('web-form-confirmation')).toBeVisible()
    }
    const runReads: string[] = []
    page.on('request', r => {
        if (
            r.url().includes('/api/collections/rule_runs/records') &&
            r.url().includes('record_id')
        ) {
            runReads.push(r.url())
        }
    })
    await page.getByTestId('form-tab-submissions').click()
    await expect(submissionRows(page)).toHaveCount(2)
    await expect.poll(() => runReads.length).toBeGreaterThan(0)
    expect(runReads).toHaveLength(1)
})

test('an admin deletes a submission', async ({ page, browser }) => {
    const url = await buildAndPublish(page, `Delete ${Date.now()}`)
    const visitor = await visitAnonymously(browser, url)
    await fillAndSend(visitor, visitor, 'Delete Me')
    await expect(visitor.getByTestId('web-form-confirmation')).toBeVisible()
    await page.getByTestId('form-tab-submissions').click()
    await submissionRows(page).first().click()
    await page.getByTestId('web-form-delete-submission').click()
    await page.getByTestId('web-form-delete-confirm').click()
    await expect(page.getByText('No submissions yet.')).toBeVisible()
})

test.describe('phone width', () => {
    test('the publish and submissions tabs work at 390×844', async ({ page, browser }) => {
        const url = await buildAndPublish(page, `Phone ${Date.now()}`)
        await page.setViewportSize({ width: 390, height: 844 })
        await expect(page.getByTestId('web-form-destinations')).toBeVisible()
        const visitor = await visitAnonymously(browser, url, { width: 390, height: 844 })
        await fillAndSend(visitor, visitor, 'Small Screen')
        await expect(visitor.getByTestId('web-form-confirmation')).toBeVisible()
        await page.getByTestId('form-tab-submissions').click()
        await submissionRows(page).first().click()
        await expect(page.getByTestId('web-form-submission-detail')).toBeVisible()
    })
})
