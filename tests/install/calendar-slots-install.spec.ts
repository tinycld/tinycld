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

// calendar-slots' bookings collection relates into calendar's collection, so
// calendar-slots declares peerVersions: { calendar }. This image bundles calendar
// (present by default), so the gate-rejection test simulates its absence by
// temporarily removing calendar's pkg_registry row rather than relying on a lean
// assembly.

async function loginAsSuperuser(page: Page, timeoutMs?: number) {
    // /setup, not /admin: the superuser-login console moved there in the
    // single-org migration. /admin is now behind AuthGate and shows the app's
    // LoginModal instead of the 'Superuser Login' form asserted below.
    await page.goto('/setup', timeoutMs ? { timeout: timeoutMs } : undefined)
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

// The four collections calendar-slots' create migration
// (1800000000_create_calendar-slots.js) is supposed to define. The install can
// report `success` while these are MISSING: that migration adds a `bookings`
// relation field targeting the calendar package's `pbc_cal_calendars_01`
// collection, and PocketBase rejects a relation whose target collection doesn't
// exist (`validation_field_relation_missing_collection`). With the `calendar`
// package absent, `app.save(bookings)` throws and the whole migration
// transaction rolls back atomically — so NONE of the four tables get created
// even though calendar-slots declares `migrations` correctly. This guard turns
// that silent gap into a failure.
const BOOKING_COLLECTIONS = [
    'booking_pages',
    'booking_slot_types',
    'booking_availability',
    'bookings',
]

// Returns whether a collection exists, via a superuser read of its records
// endpoint (read-only assertion — never a raw write). PocketBase returns 404 for
// an UNKNOWN COLLECTION but 200 with items:[] for a known-but-empty one, so the
// 200/404 split is a reliable existence check. Any other status (incl. transient
// mid-restart) returns null so callers retry rather than mis-assert. Mirrors the
// helper in todo-install.spec.ts.
async function collectionExists(page: Page, name: string): Promise<boolean | null> {
    let token: string
    try {
        token = await superuserToken(page)
    } catch {
        return null
    }
    let res: Awaited<ReturnType<typeof page.request.get>>
    try {
        res = await page.request.get(`/api/collections/${name}/records?perPage=1`, {
            headers: { Authorization: token },
            failOnStatusCode: false,
        })
    } catch {
        return null // connection reset/refused during the restart window
    }
    if (res.status() === 200) return true
    if (res.status() === 404) return false
    return null
}

// Polls collectionExists until it reports `want`, tolerating transient nulls.
async function waitForCollection(page: Page, name: string, want: boolean, timeoutMs: number) {
    const deadline = Date.now() + timeoutMs
    let last: boolean | null = null
    while (Date.now() < deadline) {
        const exists = await collectionExists(page, name)
        if (exists !== null) {
            last = exists
            if (exists === want) return
        }
        await page.waitForTimeout(2_000)
    }
    throw new Error(`collection ${name} exists=${last}, wanted ${want} within ${timeoutMs}ms`)
}

// Ground-truth wait on the server's pkg_install_log: poll the slug's most-recent
// op until it reaches `wantStatus`, throwing if it ends 'failed'/'rolled_back'.
// The job ends by restarting the server (exit 75), so connection resets and 401s
// mid-poll are EXPECTED and retried; the token is re-minted on auth failure.
// `notId` is the stale prior-operation row to ignore until the new op's row lands
// — required once this spec runs a SECOND op (uninstall after install), so the
// uninstall poll doesn't immediately match the still-present install/success row.
async function waitForOpStatus(
    page: Page,
    slug: string,
    wantStatus: string,
    timeoutMs: number,
    wantAction?: string,
    notId?: string | null
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
            // Ignore the stale prior-operation row until the new row lands.
            const isStale = notId != null && body.id === notId
            const actionMatches = !wantAction || body.action === wantAction
            if (!isStale && actionMatches && body.status === wantStatus) return
            if (
                !isStale &&
                actionMatches &&
                (body.status === 'failed' || body.status === 'rolled_back')
            ) {
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

// Polls the slug's install op-log row until it reaches a terminal FAILURE status
// (`failed`/`rolled_back`), throwing if it reaches `success` instead. Used to
// assert the compat gate rejects calendar-slots when calendar is absent. The gate
// fails in the resolve pre-flight (seconds, no build), so this resolves quickly.
async function pollInstallTerminalFailure(
    page: Page,
    slug: string,
    timeoutMs: number,
    notId?: string | null
) {
    const url = `/api/admin/packages/status/${slug}`
    const deadline = Date.now() + timeoutMs
    let token = await superuserToken(page)
    let last = 'no-response-yet'
    while (Date.now() < deadline) {
        let res: Awaited<ReturnType<typeof page.request.get>>
        try {
            res = await page.request.get(url, {
                headers: { Authorization: token },
                failOnStatusCode: false,
            })
        } catch {
            await page.waitForTimeout(2_000)
            continue
        }
        if (res.status() === 401 || res.status() === 403) {
            token = await superuserToken(page).catch(() => token)
        }
        if (res.ok()) {
            const body = (await res.json().catch(() => null)) as {
                id?: string
                status?: string
                action?: string
            } | null
            if (body && body.action === 'install') {
                last = `${body.action}/${body.status ?? '?'}`
                const isStale = notId != null && body.id === notId
                if (!isStale && (body.status === 'failed' || body.status === 'rolled_back')) return
                if (!isStale && body.status === 'success') {
                    throw new Error(
                        `${slug} install reached 'success' — expected the compat gate to reject it`
                    )
                }
            }
        }
        await page.waitForTimeout(2_000)
    }
    throw new Error(
        `${slug} install did not reach a terminal failure within ${timeoutMs}ms (last=${last})`
    )
}

// Reads the slug's current op-log row id (the install row), so the subsequent
// uninstall poll can skip it as stale via waitForOpStatus's `notId`. Returns null
// if unreadable — the caller treats that as "no prior row to guard against".
async function currentOpId(page: Page, slug: string): Promise<string | null> {
    let token: string
    try {
        token = await superuserToken(page)
    } catch {
        return null
    }
    try {
        const res = await page.request.get(`/api/admin/packages/status/${slug}`, {
            headers: { Authorization: token },
            failOnStatusCode: false,
        })
        if (!res.ok()) return null
        const body = (await res.json()) as { id?: string }
        return body.id ?? null
    } catch {
        return null
    }
}

// POSTs to an admin package endpoint with superuser auth — the same call the
// /admin UI makes (the package-row Trash2 confirm issues POST /uninstall {slug}).
// This is the admin package-management API, NOT a raw PocketBase collection write,
// so it's the legitimate way to drive an uninstall from the test. Mirrors
// todo-install.spec.ts.
async function postAdminPackageOp(page: Page, path: string, payload: Record<string, string>) {
    const token = await superuserToken(page)
    const res = await page.request.post(`/api/admin/packages/${path}`, {
        headers: { Authorization: token },
        data: payload,
        failOnStatusCode: false,
    })
    if (!res.ok()) {
        throw new Error(
            `POST /api/admin/packages/${path} failed: ${res.status()} ${await res.text()}`
        )
    }
}

// Fetches a pkg_registry record by slug (full record, so it can be recreated
// verbatim). Returns null if absent. Superuser auth bypasses the collection's
// super_admin-only write rules. This is test infrastructure — simulating a
// package's ABSENCE to exercise the compat gate — not an app-data write path.
async function getRegistryRecord(
    page: Page,
    slug: string
): Promise<Record<string, unknown> | null> {
    const token = await superuserToken(page)
    const res = await page.request.get(
        `/api/collections/pkg_registry/records?filter=${encodeURIComponent(`slug='${slug}'`)}&perPage=1`,
        { headers: { Authorization: token }, failOnStatusCode: false }
    )
    if (!res.ok()) return null
    const body = (await res.json()) as { items?: Record<string, unknown>[] }
    return body.items?.[0] ?? null
}

// Deletes a pkg_registry row (by record id) so the compat gate resolves the slug
// as not-installed. The package's schema/collections (from bundled migrations)
// are untouched — this isolates the GATE's behavior, which keys off the registry.
async function deleteRegistryRecord(page: Page, id: string) {
    const token = await superuserToken(page)
    const res = await page.request.delete(`/api/collections/pkg_registry/records/${id}`, {
        headers: { Authorization: token },
        failOnStatusCode: false,
    })
    if (!res.ok() && res.status() !== 404) {
        throw new Error(`delete pkg_registry/${id} failed: ${res.status()} ${await res.text()}`)
    }
}

// Recreates a pkg_registry row from a previously-captured record (preserving its
// id), restoring the state a gate-rejection test temporarily removed.
async function restoreRegistryRecord(page: Page, record: Record<string, unknown>) {
    const token = await superuserToken(page)
    const res = await page.request.post('/api/collections/pkg_registry/records', {
        headers: { Authorization: token },
        data: record,
        failOnStatusCode: false,
    })
    if (!res.ok()) {
        throw new Error(`restore pkg_registry row failed: ${res.status()} ${await res.text()}`)
    }
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

        await page.goto(`/setup?token=${SETUP_TOKEN}`)
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
        // Single-org: the dashboard lands on Packages, and the Organizations
        // tab is a static "managed by the router" explainer, not an empty list.
        await expect(page.getByText('Packages', { exact: true }).first()).toBeVisible()
    })

    test('installing calendar-slots WITHOUT calendar is rejected by the compat gate', async ({
        page,
    }) => {
        // The compat gate fails in the resolve pre-flight (npm pack + manifest
        // parse), before any build — seconds, not minutes.
        test.setTimeout(600_000) // 10 min (cold npm pack only)

        await loginAsSuperuserWithRetry(page)

        // This image bundles calendar (its registry row + schema are present), so to
        // exercise the gate's "dependency absent" path we temporarily remove the
        // calendar pkg_registry row — the compat gate resolves presence from the
        // registry. We restore it after, so the prerequisite for the next test holds.
        const calendarRecord = await getRegistryRecord(page, 'calendar')
        expect(calendarRecord, 'calendar must be in the registry to remove it').toBeTruthy()
        await deleteRegistryRecord(page, calendarRecord?.id as string)

        try {
            await page.getByRole('button', { name: 'Install package' }).click()
            await page.getByRole('textbox', { name: 'Package source', exact: true }).fill(PKG_SPEC)
            await page.getByRole('button', { name: 'Install', exact: true }).click()

            // CRUX (Part 2 — compat gate): calendar-slots declares peerVersions on
            // calendar, now absent from the registry, so the install must FAIL up
            // front in the resolve pre-flight rather than build, run the create
            // migration, and roll it back into a silent empty-schema success.
            await pollInstallTerminalFailure(page, 'calendar-slots', 300_000)
        } finally {
            // Restore calendar so the rest of the suite runs against a faithful state.
            await restoreRegistryRecord(page, calendarRecord as Record<string, unknown>)
        }
    })

    test('install calendar-slots succeeds with calendar present', async ({ page }) => {
        // Generous budget: the runtime image has no Go module cache, so the
        // installer's `go build` downloads + compiles for minutes, and `expo
        // export` is another multi-minute web build. 45 min covers a cold run.
        test.setTimeout(2_700_000) // 45 min

        await loginAsSuperuserWithRetry(page)

        // The prior test left a FAILED calendar-slots install row; capture its id so
        // this poll skips it (notId) and waits for the new success row.
        const failedId = await currentOpId(page, 'calendar-slots')

        // Login lands on the Packages tab. 'Install package' opens a modal; fill the
        // package source field, then submit with the modal's 'Install' button.
        await page.getByRole('button', { name: 'Install package' }).click()
        await page.getByRole('textbox', { name: 'Package source', exact: true }).fill(PKG_SPEC)
        await page.getByRole('button', { name: 'Install', exact: true }).click()

        // The SSE bar must advance past 50% within 10 min (live stream proof).
        await waitForProgressAdvance(page, 50, 600_000)

        // Ground truth: the server's own install log reaching status `success`,
        // independent of the SSE modal (the stream dies on the exit-75 restart).
        await waitForOpStatus(page, 'calendar-slots', 'success', 2_400_000, 'install', failedId) // up to 40 min

        // The modal must ALSO resolve (it relies on the durable job-status poll once
        // the SSE stream dies on restart) — the regression guard for the old hang.
        await expect(page.getByText('Installation Complete')).toBeVisible({ timeout: 120_000 })

        // CRUX: a `success` install is necessary but NOT sufficient — the create
        // migration can roll back (taking all four tables with it) while the install
        // job still reports success, if the `bookings` relation into the calendar
        // package's collection fails to validate. Assert every booking collection
        // actually exists. Generous timeout: the post-install exit-75 restart must
        // finish and re-wire the collection routes before these reads land.
        for (const name of BOOKING_COLLECTIONS) {
            await waitForCollection(page, name, true, 180_000)
        }
    })

    test('uninstall @tinycld/calendar-slots drops all booking tables', async ({ page }) => {
        // Same cold-build budget as install: uninstall rebuilds the web bundle
        // without the member and runs the package's DOWN migrations.
        test.setTimeout(2_700_000) // 45 min

        await loginAsSuperuserWithRetry(page)

        // The install row is still the slug's most-recent op; capture its id so the
        // uninstall poll skips it (notId) until the uninstall row lands.
        const installId = await currentOpId(page, 'calendar-slots')

        // POST /uninstall — the same call the package-row Trash2 confirm modal issues.
        await postAdminPackageOp(page, 'uninstall', { slug: 'calendar-slots' })

        // Ground truth: the uninstall op-log row reaching `success`.
        await waitForOpStatus(page, 'calendar-slots', 'success', 2_400_000, 'uninstall', installId)

        // CRUX: the DOWN migration must drop every booking collection. After the
        // uninstall restart re-wires the routes, each records endpoint must 404 —
        // a leftover table here is the "uninstalled but tables persist" symptom.
        for (const name of BOOKING_COLLECTIONS) {
            await waitForCollection(page, name, false, 180_000)
        }
    })

    test('reinstall recreates all booking tables (no stale _migrations rows)', async ({ page }) => {
        // The install/uninstall cycle's regression guard. If uninstall left the
        // package's _migrations rows behind, the reinstall's create migration is
        // skipped as "already applied" and the tables are NEVER recreated — the
        // install still reports success. This test reproduces the reported
        // "repeatedly install/uninstall → tables stop being created" bug and proves
        // the uninstall row-purge fixed it.
        test.setTimeout(2_700_000) // 45 min

        await loginAsSuperuserWithRetry(page)

        const uninstallId = await currentOpId(page, 'calendar-slots')

        await page.getByRole('button', { name: 'Install package' }).click()
        await page.getByRole('textbox', { name: 'Package source', exact: true }).fill(PKG_SPEC)
        await page.getByRole('button', { name: 'Install', exact: true }).click()

        await waitForProgressAdvance(page, 50, 600_000)
        await waitForOpStatus(page, 'calendar-slots', 'success', 2_400_000, 'install', uninstallId)
        await expect(page.getByText('Installation Complete')).toBeVisible({ timeout: 120_000 })

        // CRUX: every booking table must exist AGAIN. A 404 here is the stale-row
        // bug — the create migration was skipped because its _migrations row
        // survived the prior uninstall.
        for (const name of BOOKING_COLLECTIONS) {
            await waitForCollection(page, name, true, 180_000)
        }
    })
})
