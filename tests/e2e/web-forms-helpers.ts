import * as path from 'node:path'
import { type Browser, expect, type Locator, type Page } from '@playwright/test'
import { authStorageKey } from './auth-key-helpers'
import { login } from './helpers'

/**
 * Shared steps of the web form specs: an admin builds a form under the core
 * "Web form" host and publishes it; an anonymous visitor fills it.
 */

const HOST_ROW_TEST_ID = 'forms-host-core:web_form'
// From this file, not the working directory: package specs run from their own.
const FILE = path.join(import.meta.dirname, 'fixtures', 'resume.pdf')

async function answerPrompt(page: Page, title: string, value: string) {
    const input = page.getByLabel(title, { exact: true })
    await expect(input).toBeVisible()
    await input.fill(value)
    await input.press('Enter')
    await expect(input).toBeHidden()
}

function properties(page: Page): Locator {
    return page.getByTestId('form-properties')
}

async function openWebForms(page: Page) {
    await page.getByTestId('nav-settings').click()
    await page.getByText('Forms', { exact: true }).first().click()
    const hostRow = page.getByTestId(HOST_ROW_TEST_ID)
    const newForm = page.getByTestId('forms-new')
    await expect(hostRow.or(newForm)).toBeVisible()
    if (await hostRow.count()) await hostRow.click()
    await expect(newForm).toBeVisible()
}

async function addRoleSelect(page: Page) {
    await addField(page, 'select', 'Role')
    await addRoleOptions(page)
}

/** Fills the selected select field's options: Developer (dev) and Designer (design). */
async function addRoleOptions(page: Page) {
    const option = (i: number, leaf: string) =>
        properties(page).locator(`[data-testid$=".options.${i}.${leaf}"]`)
    await option(0, 'label').fill('Developer')
    await option(0, 'value').fill('dev')
    await properties(page).getByTestId('form-option-add').click()
    await option(1, 'label').fill('Designer')
    await option(1, 'value').fill('design')
}

async function addField(page: Page, kind: string, label: string) {
    await page.locator('[data-testid^="form-add-field-"]').first().click()
    await page.getByTestId(`form-kind-${kind}`).click()
    await answerPrompt(page, 'Field label', label)
}

/** Adds a field and gives it a width; a select gets the Developer / Designer options. */
async function addSizedField(page: Page, field: SizedField) {
    await addField(page, field.kind, field.label)
    if (field.kind === 'select') await addRoleOptions(page)
    await properties(page).locator(`[data-testid$=".width-${field.width}"]`).click()
    if (field.narrowWidth) {
        await properties(page).locator(`[data-testid$=".narrowWidth-${field.narrowWidth}"]`).click()
    }
}

async function saveDefinition(page: Page) {
    const stored = page.waitForResponse(
        r => r.url().includes('/api/collections/forms/records/') && r.request().method() === 'PATCH'
    )
    await page.getByTestId('form-save').click()
    expect((await stored).ok()).toBe(true)
}

export async function savePublication(page: Page) {
    const stored = page.waitForResponse(
        r =>
            r.url().includes('/api/collections/form_publications/records') &&
            ['POST', 'PATCH'].includes(r.request().method())
    )
    await page.getByTestId('web-form-save').click()
    expect((await stored).ok()).toBe(true)
}

export interface SizedField {
    kind: 'date' | 'number' | 'select' | 'text'
    label: string
    width: FieldWidth
    /** The width below the narrow breakpoint, set with the builder's "On narrow screens" control. */
    narrowWidth?: FieldWidth
}

type FieldWidth = 'full' | 'half' | 'third' | 'quarter' | 'auto'

interface PublishOptions {
    /** More fields after the defaults, each at its own width. */
    fields?: SizedField[]
    domains?: string
    pow?: boolean
    cvMaxSizeMb?: number
    /** Also add a "Role" select: Developer (dev) and Designer (design). */
    withRole?: boolean
}

/** Logs in, then builds and publishes a form; returns its public URL. */
export async function buildAndPublish(page: Page, name: string, opts: PublishOptions = {}) {
    await login(page)
    return await publishForm(page, name, opts)
}

/** Builds and publishes one more form in a session already logged in. */
export async function publishForm(page: Page, name: string, opts: PublishOptions = {}) {
    await openWebForms(page)
    await page.getByTestId('forms-new').click()
    await answerPrompt(page, 'New form', name)
    await expect(page.getByTestId('form-editor')).toBeVisible()

    await addField(page, 'text', 'Name')
    await properties(page).locator('[data-testid$=".required"]').click()
    await addField(page, 'file', 'CV')
    if (opts.cvMaxSizeMb) {
        await properties(page).locator('[data-testid$=".maxSizeMb"]').fill(String(opts.cvMaxSizeMb))
    }
    if (opts.withRole) await addRoleSelect(page)
    for (const field of opts.fields ?? []) await addSizedField(page, field)
    await saveDefinition(page)

    await page.getByTestId('form-tab-publish').click()
    await page.getByTestId('web-form-enabled').click()
    if (opts.domains) await page.getByTestId('web-form-domains').fill(opts.domains)
    if (opts.pow) await page.getByTestId('web-form-pow').click()
    await savePublication(page)
    const code = page.getByTestId('web-form-embed-code')
    await expect(code).toContainText('/p/forms/')
    const text = (await code.textContent()) ?? ''
    const url = /src="([^"]+\/p\/forms\/[^"]+)"/.exec(text)?.[1]
    if (!url) throw new Error(`no URL in embed code: ${text}`)
    return url
}

export async function visitAnonymously(
    browser: Browser,
    url: string,
    viewport?: { width: number; height: number }
) {
    const context = await browser.newContext(viewport ? { viewport } : {})
    const page = await context.newPage()
    // The one legitimate goto: the first load of a new, anonymous context.
    await page.goto(url)
    return page
}

export async function fillAndSend(
    visitor: Page | ReturnType<Page['frameLocator']>,
    chooserPage: Page,
    name: string
) {
    await visitor.getByLabel('Name', { exact: true }).fill(name)
    const chooser = chooserPage.waitForEvent('filechooser')
    await visitor.locator('[data-testid$="-pick"]').click()
    await (await chooser).setFiles(FILE)
    await expect(visitor.getByText('resume.pdf')).toBeVisible()
    await visitor.getByTestId('form-wizard-next').click()
}

export async function submissionsNamed(page: Page, name: string) {
    // Read-only assertion through the admin's own session token.
    const key = await authStorageKey(page)
    const token = await page.evaluate(k => JSON.parse(localStorage.getItem(k) ?? '{}').token, key)
    const res = await page.request.get(
        '/api/collections/form_submissions/records?sort=-created&perPage=100',
        {
            headers: { Authorization: token },
        }
    )
    expect(res.ok()).toBe(true)
    const items = (await res.json()).items as { values: Record<string, unknown> }[]
    return items.filter(i => i.values.name === name)
}
