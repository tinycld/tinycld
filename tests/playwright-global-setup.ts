/**
 * Playwright Global Setup
 *
 * 1. Truncates tmp/emails.log so each test run sees a clean mail log.
 * 2. Warms the Expo web bundle before any test runs.
 *
 * The DB reset+seed is NOT done here — it's part of the webServer command
 * (`npm run expo:test` chains `reset-dev-db.ts` before `dev.ts`). Doing it
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

const PROJECT_ROOT = path.resolve(import.meta.dirname, '..')
export const TMP_DIR = path.join(PROJECT_ROOT, 'tmp')
export const EMAIL_LOG_PATH = path.join(TMP_DIR, 'emails.log')

const PORT = Number(process.env.E2E_PORT ?? 7200)
const BASE_URL = `http://localhost:${PORT}`
// The expo-router web entry. Requesting it forces Metro to compile the web
// bundle and only returns 200 with real JS once that compile finishes — the
// exact "web app is serveable" signal /api/health doesn't give us.
const ENTRY_BUNDLE = `${BASE_URL}/node_modules/expo-router/entry.bundle?platform=web&dev=true&hot=false&lazy=true&transform.engine=hermes&transform.routerRoot=app&unstable_transformProfile=hermes-stable`

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

export default async function globalSetup() {
    fs.mkdirSync(TMP_DIR, { recursive: true })
    fs.writeFileSync(EMAIL_LOG_PATH, '')
    await warmWebBundle()
}
