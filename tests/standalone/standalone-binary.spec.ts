import { existsSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test } from '@playwright/test'
import { bootBinary, buildBinary } from './boot-binary'

/**
 * Boots the SHIPPED artifact in an empty directory with no workspace around it.
 *
 * This is the check that the web bundle and the migrations are genuinely INSIDE
 * the binary. A build that reads them from disk passes every unit test and fails
 * here, and so does one whose embed was dropped by the linker — that binary
 * boots and serves, just with no web UI.
 */

let server: Awaited<ReturnType<typeof bootBinary>>

// One server for the whole file. Each boot applies every embedded migration,
// so booting per test doubles the slowest part of the run for no extra coverage.
test.beforeAll(async () => {
    buildBinary()
    server = await bootBinary()
})

test.afterAll(() => server.stop())

test('serves the app and applies migrations from embedded assets in a clean directory', async ({
    page,
}) => {
    const { baseURL, dataDir } = server

    // The SPA shell, served from the embedded bundle.
    const shell = await fetch(baseURL)
    expect(shell.status).toBe(200)
    const html = await shell.text()
    expect(html).toContain('<div id="root">')

    // Every hashed JS chunk must resolve. A shell whose scripts 404 still
    // renders #root, so asserting on the shell alone would miss a blank app —
    // which is exactly what the asset-pool routes once caused here.
    const scripts = [...html.matchAll(/src="([^"]+\.js)"/g)].map(m => m[1])
    expect(scripts.length).toBeGreaterThan(0)
    for (const src of scripts) {
        const chunk = await fetch(new URL(src, baseURL).href)
        expect(chunk.status, `chunk ${src}`).toBe(200)
        expect(Number(chunk.headers.get('content-length') ?? 0), `chunk ${src}`).toBeGreaterThan(0)
    }

    // Migrations ran from the embedded FS — without them the API has no
    // collections and this 404s.
    const collections = await fetch(`${baseURL}/api/collections/users/records`)
    expect(collections.status).not.toBe(404)

    // The app actually renders in a browser.
    await page.goto(baseURL)
    await expect(page.locator('#root')).toBeAttached()

    expect(existsSync(join(dataDir, 'pb_data', 'data.db'))).toBe(true)
})

test('does not expose the package rebuild API', async () => {
    // A single binary cannot rebuild itself, so the endpoint must not be routed.
    // An unauthenticated request to a REGISTERED route would be rejected by its
    // owner guard (401/403) rather than reported missing.
    const res = await fetch(`${server.baseURL}/api/admin/packages/status/mail`)
    expect(res.status).toBe(404)
})
