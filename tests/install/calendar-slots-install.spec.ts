import { expect, type Page, test } from '@playwright/test'

// Minimal, parameterized install spec for @tinycld/calendar-slots, driven through
// the real in-app installer UI against an already-running container (the
// run-ota-crash-rollback.sh driver builds the image + boots the container, then
// invokes this spec). Its ONLY job is to install calendar-slots so the install
// mints a `build-<ts>-ios` OTA bundle — the crash-rollback assertion (the TS
// runner) then boots the Release sim and classifies the outcome. It deliberately
// asserts nothing calendar-slots-feature-specific; the booking-table guard lives
// in the TS runner.
//
// This mirrors the helper set + UI flow of todo-install.spec.ts but is
// self-contained: the helpers there are private to that file, and the human chose
// a separate minimal spec over generalizing the battle-tested todo spec.
//
// Runner-only: every test is skipped unless RUN_CALSLOTS_INSTALL_TEST=1, so the
// spec can't fire in a normal CI glob even though its directory is matched.

const RUN_INSTALL_TEST = process.env.RUN_CALSLOTS_INSTALL_TEST === '1'

const SETUP_TOKEN = process.env.PW_CALSLOTS_SETUP_TOKEN

// The superuser this spec bootstraps + logs in as. Read from the workspace .env
// (ADMIN_USER_LOGIN / ADMIN_USER_PW, forwarded by the driver), falling back to
// fixed smoke values when unset. `||` (not `??`) so a forwarded empty-string also
// falls back. Mirrors todo-install.spec.ts.
const SUPERUSER_EMAIL = process.env.ADMIN_USER_LOGIN || 'calslots-smoke@example.com'
const SUPERUSER_PASSWORD = process.env.ADMIN_USER_PW || 'CalSlotsSmoke1234!'

// The package to install. Overridable so the same spec can target a fork/tag; the
// default is corroborated by core/server/coreserver/pkg_seed_test.go.
const PKG_SPEC = process.env.PW_CALSLOTS_SPEC ?? 'github:stefnnn/tinycld-calendar-slots'

async function loginAsSuperuser(page: Page, timeoutMs?: number) {
    await page.goto('/admin', timeoutMs ? { timeout: timeoutMs } : undefined)
    await expect(page.getByText('Superuser Login')).toBeVisible(
        timeoutMs ? { timeout: timeoutMs } : undefined
    )
    await page.getByRole('textbox', { name: 'Email', exact: true }).fill(SUPERUSER_EMAIL)
    await page.getByRole('textbox', { name: 'Password', exact: true }).fill(SUPERUSER_PASSWORD)
    await page.getByRole('button', { name: 'Sign in' }).click()
    // 'Organizations' appears in both the nav rail and (when that tab is open) the
    // page title, so scope to the first match.
    await expect(page.getByText('Organizations', { exact: true }).first()).toBeVisible(
        timeoutMs ? { timeout: timeoutMs } : undefined
    )
}

// After an install-class restart the server may briefly refuse connections; retry
// the login a few times with a short per-attempt timeout so a still-restarting
// server fails fast and the loop iterates.
async function loginAsSuperuserWithRetry(page: Page, attempts = 20) {
    let lastErr: unknown
    for (let i = 0; i < attempts; i++) {
        try {
            await loginAsSuperuser(page, 8_000)
            return
        } catch (err) {
            lastErr = err
            await page.waitForTimeout(3_000)
        }
    }
    throw new Error(`superuser login failed after restart (${attempts} attempts): ${lastErr}`)
}

// Mint a fresh superuser token via the API (the same call the login form makes).
// The setup PB instance has only an in-memory auth store, so we can't read a token
// from localStorage. Transient 5xx (DB busy during a VACUUM) and connection
// resets/refused (exit-75 restart window) are retried; a real 4xx auth error
// throws immediately.
async function superuserToken(page: Page): Promise<string> {
    let lastStatus = 0
    let lastBody = ''
    for (let attempt = 0; attempt < 8; attempt++) {
        let res: Awaited<ReturnType<typeof page.request.post>>
        try {
            res = await page.request.post('/api/collections/_superusers/auth-with-password', {
                data: { identity: SUPERUSER_EMAIL, password: SUPERUSER_PASSWORD },
                failOnStatusCode: false,
            })
        } catch (err) {
            lastBody = String(err) // connection reset/refused during the restart window
            await page.waitForTimeout(750)
            continue
        }
        if (res.ok()) {
            const body = (await res.json()) as { token?: string }
            if (!body.token) throw new Error('superuser auth returned no token')
            return body.token
        }
        lastStatus = res.status()
        lastBody = await res.text()
        if (lastStatus < 500) {
            throw new Error(`superuser auth failed: ${lastStatus} ${lastBody}`)
        }
        await page.waitForTimeout(750) // transient 5xx (DB busy) — back off and retry
    }
    throw new Error(`superuser auth failed after retries: ${lastStatus} ${lastBody}`)
}

// The SSE progress bar must actually advance — proof the events stream
// authenticates and reaches the browser (a frozen 0% bar is the signature of the
// events-endpoint 403 regression).
async function waitForProgressAdvance(page: Page, minPct: number, timeoutMs: number) {
    const fill = page.getByTestId('install-progress-fill')
    await expect(fill).toBeVisible({ timeout: 30_000 })

    const deadline = Date.now() + timeoutMs
    let lastSeen = -1
    while (Date.now() < deadline) {
        const raw = await fill.getAttribute('aria-valuenow').catch(() => null)
        const pct = raw ? Number(raw) : Number.NaN
        if (Number.isFinite(pct)) {
            lastSeen = pct
            if (pct >= minPct) return
        }
        await page.waitForTimeout(1_000)
    }
    throw new Error(
        `install progress bar did not advance to ${minPct}% within ${Math.round(timeoutMs / 1000)}s ` +
            `(highest observed: ${lastSeen}%). The SSE progress stream likely never reached the browser ` +
            `— check /api/admin/packages/events auth (a 403 freezes the bar at 0%).`
    )
}

// Ground-truth wait on the server's pkg_install_log: poll the slug's most-recent
// op until it reaches `wantStatus`, throwing if it ends 'failed'/'rolled_back'.
// The job ends by restarting the server (exit 75), so connection resets and 401s
// mid-poll are EXPECTED and retried; the token is re-minted on auth failure.
// Note: the todo-install version takes a `notId` stale-row guard for multi-op
// scenarios; this spec does a SINGLE install, so there's no prior row to skip and
// the parameter is intentionally omitted — don't copy this helper into a
// multi-operation spec without restoring it.
async function waitForOpStatus(
    page: Page,
    slug: string,
    wantStatus: string,
    timeoutMs: number,
    wantAction?: string
) {
    const url = `/api/admin/packages/status/${slug}`
    const deadline = Date.now() + timeoutMs
    let token = await superuserToken(page)
    let last = 'no-response-yet'

    async function readStatusOnce(): Promise<{
        id?: string
        status?: string
        error?: string
        action?: string
    } | null> {
        let res: Awaited<ReturnType<typeof page.request.get>>
        try {
            res = await page.request.get(url, {
                headers: { Authorization: token },
                failOnStatusCode: false,
            })
        } catch {
            return null // connection reset/refused during the restart window
        }
        if (res.status() === 401 || res.status() === 403) {
            try {
                token = await superuserToken(page)
                res = await page.request.get(url, {
                    headers: { Authorization: token },
                    failOnStatusCode: false,
                })
            } catch {
                return null
            }
        }
        if (!res.ok()) return null
        try {
            return await res.json()
        } catch {
            return null
        }
    }

    while (Date.now() < deadline) {
        const body = await readStatusOnce()
        if (body) {
            last = `${body.action ?? '?'}/${body.status ?? '?'}`
            const actionMatches = !wantAction || body.action === wantAction
            if (actionMatches && body.status === wantStatus) return
            if (actionMatches && (body.status === 'failed' || body.status === 'rolled_back')) {
                throw new Error(
                    `${slug} ${body.action} ended ${body.status}: ${body.error ?? '(no error)'}`
                )
            }
        }
        await page.waitForTimeout(3_000)
    }
    throw new Error(
        `${slug} did not reach ${wantAction ?? ''}/${wantStatus} within ${timeoutMs}ms (last=${last})`
    )
}

test.describe.configure({ mode: 'serial' })

test.describe('calendar-slots install', () => {
    // Hard opt-in gate: runner-only. Without RUN_CALSLOTS_INSTALL_TEST=1 every test
    // skips, so a normal CI glob can't run it.
    test.beforeAll(() => {
        test.skip(
            !RUN_INSTALL_TEST,
            'calendar-slots-install is runner-only — set RUN_CALSLOTS_INSTALL_TEST=1 (run-ota-crash-rollback.sh does)'
        )
    })

    test('bootstrap superuser via /admin wizard', async ({ page }) => {
        test.skip(
            !SETUP_TOKEN,
            'PW_CALSLOTS_SETUP_TOKEN not set — the runner scrapes it from `docker logs`'
        )

        await page.goto(`/admin?token=${SETUP_TOKEN}`)
        await expect(page.getByText('Welcome to TinyCld')).toBeVisible()

        await page
            .getByRole('textbox', { name: 'Application Name', exact: true })
            .fill('Calendar Slots TinyCld')
        await page.getByRole('textbox', { name: 'Email', exact: true }).fill(SUPERUSER_EMAIL)
        await page.getByRole('textbox', { name: 'Password', exact: true }).fill(SUPERUSER_PASSWORD)
        await page
            .getByRole('textbox', { name: 'Confirm Password', exact: true })
            .fill(SUPERUSER_PASSWORD)
        await page
            .getByRole('textbox', { name: 'App URL', exact: true })
            .fill('http://localhost:7090')

        await page.getByRole('button', { name: 'Create Account & Continue' }).click()
        await expect(page.getByText('No organizations yet.')).toBeVisible()
    })

    test('install @tinycld/calendar-slots through the installer UI', async ({ page }) => {
        // Generous budget: the runtime image has no Go module cache, so the
        // installer's `go build` downloads + compiles for minutes, and `expo
        // export` is another multi-minute web build. 45 min covers a cold run.
        test.setTimeout(2_700_000) // 45 min

        await loginAsSuperuserWithRetry(page)

        // Login lands on the Packages tab. 'Install package' opens a modal; fill the
        // package source field, then submit with the modal's 'Install' button.
        await page.getByRole('button', { name: 'Install package' }).click()
        await page.getByRole('textbox', { name: 'Package source', exact: true }).fill(PKG_SPEC)
        await page.getByRole('button', { name: 'Install', exact: true }).click()

        // The SSE bar must advance past 50% within 10 min (live stream proof).
        await waitForProgressAdvance(page, 50, 600_000)

        // Ground truth: the server's own install log reaching status `success`,
        // independent of the SSE modal (the stream dies on the exit-75 restart).
        await waitForOpStatus(page, 'calendar-slots', 'success', 2_400_000, 'install') // up to 40 min

        // The modal must ALSO resolve (it relies on the durable job-status poll once
        // the SSE stream dies on restart) — the regression guard for the old hang.
        await expect(page.getByText('Installation Complete')).toBeVisible({ timeout: 120_000 })
    })
})
