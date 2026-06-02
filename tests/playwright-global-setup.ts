/**
 * Playwright Global Setup
 *
 * 1. Truncates tmp/emails.log so each test run sees a clean mail log.
 * 2. Warms the Expo web bundle before any test runs.
 * 3. (Opt-in via TINYCLD_WARM_PACKAGES env var) warms per-package lazy
 *    chunks for each named package by actually navigating a real
 *    browser into them. Each chunk is compiled on first dynamic import;
 *    the per-package E2E workflows pay that cost once here upfront so
 *    the per-test budget doesn't race the compile.
 *
 * The DB reset+seed is NOT done here — it's part of the webServer command
 * (`pnpm run expo:test` chains `reset-dev-db.ts` before `dev.ts`). Doing it
 * in globalSetup raced with webServer startup: Playwright spawns webServer
 * in parallel with globalSetup, so dev.ts's PB would open server/pb_test_data
 * while reset-dev-db.ts was still deleting and reseeding it — yielding a
 * PB that returned auth records pointing at IDs that no longer existed
 * in the on-disk DB. Chaining inside expo:test serializes the two steps.
 *
 * Bundle warming, by contrast, is safe here: it only reads (GETs the web
 * entry bundle) and tolerates the server not being up yet by polling. Metro
 * builds the web bundle lazily on the first request and the dev banner /
 * webServer `/api/health` gate both go green while that ~24 MB compile is
 * still running. On a fast machine the compile finishes before the first
 * test interacts; on a slow CI runner (2 cores, whole stack booting at once)
 * it doesn't, so the first test in each worker raced the cold bundle and
 * timed out clicking a control that hadn't rendered. Forcing + awaiting the
 * compile here makes that cost a deterministic one-time step instead of a
 * race inside a per-test 30s timeout.
 */

import * as fs from 'node:fs'
import * as path from 'node:path'
import { chromium } from '@playwright/test'

const PROJECT_ROOT = path.resolve(import.meta.dirname, '..')
export const TMP_DIR = path.join(PROJECT_ROOT, 'tmp')
export const EMAIL_LOG_PATH = path.join(TMP_DIR, 'emails.log')

const PORT = Number(process.env.E2E_PORT ?? 7200)
const BASE_URL = `http://localhost:${PORT}`
// The expo-router web entry. Requesting it forces Metro to compile the web
// bundle and only returns 200 with real JS once that compile finishes — the
// exact "web app is serveable" signal /api/health doesn't give us.
const ENTRY_BUNDLE = `${BASE_URL}/node_modules/expo-router/entry.bundle?platform=web&dev=true&hot=false&lazy=true&transform.engine=hermes&transform.routerRoot=app&unstable_transformProfile=hermes-stable`

// Comma-separated list of package slugs whose lazy screen chunks should be
// pre-warmed before tests start. Set in CI for per-package workflows;
// unset for app's own E2E (which doesn't navigate into feature packages).
// Example: TINYCLD_WARM_PACKAGES=mail,calendar,drive
const WARM_PACKAGES = (process.env.TINYCLD_WARM_PACKAGES ?? '')
    .split(',')
    .map(s => s.trim())
    .filter(Boolean)

const ORG_SLUG = 'test-org'
const TEST_USER_EMAIL = process.env.TEST_USER_LOGIN || 'user@tinycld.org'
const TEST_USER_PASSWORD = process.env.TEST_USER_PW || 'TestUser1234!'

const sleep = (ms: number) => new Promise(r => setTimeout(r, ms))

async function warmWebBundle() {
    // The webServer (PB + proxy + Expo) starts in parallel with this setup, so
    // poll until the proxy answers before asking for the bundle. Generous
    // overall budget: a cold Metro web compile can take a couple of minutes on
    // a constrained CI runner.
    const deadline = Date.now() + 240_000
    let lastErr: unknown
    while (Date.now() < deadline) {
        try {
            const res = await fetch(ENTRY_BUNDLE, {
                signal: AbortSignal.timeout(180_000),
            })
            // Drain the body so Metro finishes serving the whole bundle (and so
            // the connection isn't left half-read).
            const body = await res.text()
            if (res.ok && body.length > 0) {
                console.log(`[global-setup] web bundle warm (${res.status}, ${body.length} bytes)`)
                return
            }
            lastErr = new Error(`entry.bundle returned ${res.status}, ${body.length} bytes`)
        } catch (err) {
            // Connection refused / timeout while the server is still coming up.
            lastErr = err
        }
        await sleep(1000)
    }
    throw new Error(
        `[global-setup] web bundle did not become ready within 240s: ${String(lastErr)}`
    )
}

// Pre-warm per-package lazy chunks by actually navigating a real browser
// to /a/<org>/<pkg>. Metro compiles each chunk on first dynamic import;
// once it's cached in the dev server, subsequent test workers hit a warm
// cache and the package-sidebar-mounted testID appears in <5s instead of
// >60s — turning a per-spec race into a deterministic one-time cost.
//
// Opt-in via TINYCLD_WARM_PACKAGES env var. App's own E2E workflow
// doesn't set it (app's tests never navigate into feature packages);
// per-package E2E workflows set it to their own slug, e.g.
// TINYCLD_WARM_PACKAGES=mail.
async function warmPackageChunks() {
    if (WARM_PACKAGES.length === 0) return
    console.log(`[global-setup] warming package chunks: ${WARM_PACKAGES.join(', ')}`)
    const browser = await chromium.launch()
    try {
        const context = await browser.newContext({ baseURL: BASE_URL })
        const page = await context.newPage()
        // Log in once so the navigation actually mounts the org layout
        // (the redirect-to-login path doesn't load any package screens).
        await page.goto('/')
        await page.getByTestId('identifier').fill(TEST_USER_EMAIL)
        await page.getByPlaceholder('Password').fill(TEST_USER_PASSWORD)
        await page.getByText('Sign in', { exact: true }).last().click()
        await page.waitForURL(/\/a\//, { timeout: 30_000 })
        for (const pkg of WARM_PACKAGES) {
            const t0 = Date.now()
            await page.goto(`/a/${ORG_SLUG}/${pkg}`)
            // Wait for the package's screen to actually mount. Long
            // timeout — the first chunk compile is the slow path and
            // we're explicitly absorbing it here. Skip the wait
            // silently on a timeout; the per-test budget will still
            // catch any genuinely-broken package.
            try {
                await page.getByTestId('package-sidebar-mounted').waitFor({
                    state: 'visible',
                    timeout: 120_000,
                })
            } catch (err) {
                console.warn(
                    `[global-setup] ${pkg} chunk warm: sidebar testID didn't appear in 120s (${String(err)}); continuing`
                )
                continue
            }
            const t1 = Date.now()
            console.log(`[global-setup] ${pkg} chunk warm (${t1 - t0}ms)`)
        }
    } finally {
        await browser.close()
    }
}

export default async function globalSetup() {
    fs.mkdirSync(TMP_DIR, { recursive: true })
    fs.writeFileSync(EMAIL_LOG_PATH, '')
    await warmWebBundle()
    await warmPackageChunks()
}
