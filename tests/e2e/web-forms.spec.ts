import { type Browser, chromium, expect, type Locator, type Page, test } from '@playwright/test'
import { authStorageKey } from './auth-key-helpers'
import { login } from './helpers'

/**
 * Web forms end to end: an admin builds a form under the core "Web form" host,
 * publishes it and reads the embed code from the Publish tab; an anonymous
 * visitor fills the public page (directly, inside an iframe on another
 * origin, and at phone width). Stored submissions are checked with a
 * read-only REST call as the admin; phase 2 adds the Submissions tab.
 */

const HOST_ROW_TEST_ID = 'forms-host-core:web_form'
const FILE = 'tests/e2e/fixtures/resume.pdf'

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

async function addField(page: Page, kind: string, label: string) {
    await page.locator('[data-testid^="form-add-field-"]').first().click()
    await page.getByTestId(`form-kind-${kind}`).click()
    await answerPrompt(page, 'Field label', label)
}

async function saveDefinition(page: Page) {
    const stored = page.waitForResponse(
        r => r.url().includes('/api/collections/forms/records/') && r.request().method() === 'PATCH'
    )
    await page.getByTestId('form-save').click()
    expect((await stored).ok()).toBe(true)
}

async function savePublication(page: Page) {
    const stored = page.waitForResponse(
        r =>
            r.url().includes('/api/collections/form_publications/records') &&
            ['POST', 'PATCH'].includes(r.request().method())
    )
    await page.getByTestId('web-form-save').click()
    expect((await stored).ok()).toBe(true)
}

interface PublishOptions {
    domains?: string
    pow?: boolean
    cvMaxSizeMb?: number
}

async function buildAndPublish(page: Page, name: string, opts: PublishOptions = {}) {
    await login(page)
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

async function visitAnonymously(
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

async function fillAndSend(
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

async function submissionsNamed(page: Page, name: string) {
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

test('a visitor submits a published form with a file', async ({ page, browser }) => {
    const url = await buildAndPublish(page, `Contact ${Date.now()}`)
    const visitor = await visitAnonymously(browser, url)
    await fillAndSend(visitor, visitor, 'Ada Lovelace')
    await expect(visitor.getByTestId('web-form-confirmation')).toContainText('Thank you')
    const rows = await submissionsNamed(page, 'Ada Lovelace')
    expect(rows).toHaveLength(1)
    expect(typeof rows[0].values.cv).toBe('string')
})

test('a required field shows its error and nothing is sent', async ({ page, browser }) => {
    const url = await buildAndPublish(page, `Required ${Date.now()}`)
    const visitor = await visitAnonymously(browser, url)
    await visitor.getByTestId('form-wizard-next').click()
    await expect(visitor.getByTestId('form-error-summary')).toBeVisible()
    await expect(visitor.getByTestId('web-form-confirmation')).toBeHidden()
})

test('a disabled form says it is not available', async ({ page, browser }) => {
    const url = await buildAndPublish(page, `Off ${Date.now()}`)
    await page.getByTestId('web-form-enabled').click()
    await savePublication(page)
    const visitor = await visitAnonymously(browser, url)
    await expect(visitor.getByTestId('web-form-gone')).toBeVisible()
})

test('proof of work: the form still sends', async ({ page, browser }) => {
    const url = await buildAndPublish(page, `Pow ${Date.now()}`, { pow: true })
    const visitor = await visitAnonymously(browser, url)
    await fillAndSend(visitor, visitor, 'Grace Hopper')
    await expect(visitor.getByTestId('web-form-confirmation')).toBeVisible()
    expect(await submissionsNamed(page, 'Grace Hopper')).toHaveLength(1)
})

/**
 * Frames the form on a page served from hostOrigin (synthesized with
 * page.route, so the frame is really cross-origin) in its own browser with
 * Private Network Access checks off: otherwise Chrome blocks public→localhost
 * before our frame-ancestors header is ever consulted, and the refusal case
 * would pass for the wrong reason.
 */
async function frameFormAt(hostOrigin: string, formUrl: string) {
    const browser = await chromium.launch({
        args: ['--disable-features=LocalNetworkAccessChecks,PrivateNetworkAccessChecks'],
    })
    const page = await browser.newPage()
    let failure: string | null = null
    page.on('requestfailed', r => {
        if (r.url().startsWith(formUrl)) failure = r.failure()?.errorText ?? 'unknown'
    })
    const origin = new URL(formUrl).origin
    await page.route(`${hostOrigin}/**`, route =>
        route.fulfill({
            status: 200,
            contentType: 'text/html',
            body: `<!doctype html><title>Host</title>
                <iframe id="f" src="${formUrl}" data-tinycld-form style="width:100%;border:0"></iframe>
                <script src="${origin}/forms-embed.js"></script>
                <script>document.getElementById('f').addEventListener('tinycld:form-submitted', e => { document.title = 'submitted:' + e.detail.form })</script>`,
        })
    )
    await page.goto(`${hostOrigin}/host.html`)
    return { page, browser, frame: page.frameLocator('#f'), failure: () => failure }
}

// One framing browser for each test: closing a manually launched browser
// flushes its recorded trace, which costs seconds, so each test pays it once.
test('an allowed site frames the form, sizes it and hears the submission', async ({ page }) => {
    const url = await buildAndPublish(page, `Embed ${Date.now()}`, {
        domains: 'https://client.example',
    })
    const allowed = await frameFormAt('https://client.example', url)
    try {
        // The frame grows to show the whole form: Send sits inside it.
        const frameEl = allowed.page.locator('#f')
        const send = allowed.frame.getByTestId('form-wizard-next')
        await expect(send).toBeVisible()
        await expect
            .poll(async () => {
                const frameBox = await frameEl.boundingBox()
                const sendBox = await send.boundingBox()
                if (!frameBox || !sendBox) return false
                return sendBox.y + sendBox.height <= frameBox.y + frameBox.height
            })
            .toBe(true)
        const before = (await frameEl.boundingBox())?.height ?? 0

        await fillAndSend(allowed.frame, allowed.page, 'Embedded Visitor')
        await expect(allowed.frame.getByTestId('web-form-confirmation')).toBeVisible()
        await expect(allowed.page).toHaveTitle(/^submitted:/)
        // And shrinks to the confirmation after sending.
        await expect
            .poll(async () => (await frameEl.boundingBox())?.height ?? 0)
            .toBeLessThan(before)
    } finally {
        await allowed.browser.close()
    }
})

test('a site not on the list cannot frame the form', async ({ page }) => {
    const url = await buildAndPublish(page, `Blocked ${Date.now()}`, {
        domains: 'https://client.example',
    })
    const blocked = await frameFormAt('https://evil.example', url)
    try {
        await expect.poll(blocked.failure).toContain('ERR_BLOCKED_BY_RESPONSE')
    } finally {
        await blocked.browser.close()
    }
})

test('a visitor submits at phone width', async ({ page, browser }) => {
    const url = await buildAndPublish(page, `Phone ${Date.now()}`)
    const visitor = await visitAnonymously(browser, url, { width: 390, height: 844 })
    await fillAndSend(visitor, visitor, 'Phone Visitor')
    await expect(visitor.getByTestId('web-form-confirmation')).toBeVisible()
})

test('Send is off while sending, so a second click sends nothing', async ({ page, browser }) => {
    const url = await buildAndPublish(page, `Double ${Date.now()}`)
    const visitor = await visitAnonymously(browser, url)
    await visitor.getByLabel('Name', { exact: true }).fill('Double Click')
    const send = visitor.getByTestId('form-wizard-next')
    await send.click()
    // While it sends (at least the 3 s minimum fill time), Send is off and says so.
    await expect(send).toBeDisabled()
    await expect(send).toContainText('Sending')
    await send.click({ force: true })
    await expect(visitor.getByTestId('web-form-confirmation')).toBeVisible()
    expect(await submissionsNamed(page, 'Double Click')).toHaveLength(1)
})

test('a file over the field limit is refused before upload', async ({ page, browser }) => {
    const url = await buildAndPublish(page, `Big ${Date.now()}`, { cvMaxSizeMb: 1 })
    const visitor = await visitAnonymously(browser, url)
    let posted = false
    visitor.on('request', r => {
        if (r.url().includes('/submit') && r.method() === 'POST') posted = true
    })
    await visitor.getByLabel('Name', { exact: true }).fill('Too Big')
    const chooser = visitor.waitForEvent('filechooser')
    await visitor.locator('[data-testid$="-pick"]').click()
    await (await chooser).setFiles({
        name: 'big.pdf',
        mimeType: 'application/pdf',
        buffer: Buffer.alloc(2 * 1024 * 1024, 1),
    })
    await visitor.getByTestId('form-wizard-next').click()
    // Named at the field and in the error summary.
    await expect(visitor.getByText(/big\.pdf is larger than 1 MB/)).toHaveCount(2)
    await expect(visitor.getByTestId('web-form-confirmation')).toBeHidden()
    expect(posted).toBe(false)
})
