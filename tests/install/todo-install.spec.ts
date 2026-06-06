import { expect, type Page, test } from '@playwright/test'

// Integration test for installing @tinycld/todo from GitHub through the
// real in-app package installer. Boots against an already-running
// container (the runner script builds the image from the working tree so
// the git-spec validation change is present). Runs serially — every step
// depends on prior container state, and the install restarts the container.
//
// Install/revert progress streams to a modal over SSE, but the test judges
// success by polling the server's pkg_install_log status (ground truth),
// not the modal — an EventSource that connects a hair late can miss the
// fast early stages, making modal-text assertions inherently racy here.
//
// NOT a normal-CI test. It needs a purpose-built docker image (the runner
// `tests/install/run-todo-install.sh` builds it) and drives a real,
// minutes-long install (npm pack → pnpm → go build → expo export → restart).
// It is excluded from `tinycld-pkg test:e2e` by living outside tests/e2e/, and
// from the docker smoke workflow (which copies only setup-and-packages.spec.ts).
// As belt-and-suspenders we ALSO hard-skip the whole suite unless the runner
// opts in via RUN_TODO_INSTALL_TEST=1, so it can never run accidentally if some
// future config globs tests/install/ into a CI suite. The skip is asserted in a
// beforeAll inside the describe (below) so every test in the file is skipped as
// a group when the opt-in is absent.
const RUN_INSTALL_TEST = process.env.RUN_TODO_INSTALL_TEST === '1'

const SETUP_TOKEN = process.env.PW_TODO_SETUP_TOKEN

const SUPERUSER_EMAIL = 'todo-smoke@example.com'
const SUPERUSER_PASSWORD = 'TodoSmoke1234!'

const TODO_SPEC = 'github:tinycld/todo'

const TEST_ORG_NAME = 'Todo Org'
const TEST_ORG_SLUG = 'todo-org'
const TEST_ORG_OWNER_NAME = 'Todo Owner'
const TEST_ORG_OWNER_EMAIL = 'owner@todo.example'
const TEST_ORG_OWNER_PASSWORD = 'OwnerPass1234!'
const TEST_ORG_MAIL_DOMAIN = 'todo.example'

async function loginAsSuperuser(page: Page, timeoutMs?: number) {
    await page.goto('/setup', timeoutMs ? { timeout: timeoutMs } : undefined)
    await expect(page.getByText('Superuser Login')).toBeVisible(
        timeoutMs ? { timeout: timeoutMs } : undefined
    )
    await page.getByRole('textbox', { name: 'Email', exact: true }).fill(SUPERUSER_EMAIL)
    await page.getByRole('textbox', { name: 'Password', exact: true }).fill(SUPERUSER_PASSWORD)
    await page.getByRole('button', { name: 'Sign in' }).click()
    await expect(page.getByText('Organizations', { exact: true })).toBeVisible(
        timeoutMs ? { timeout: timeoutMs } : undefined
    )
}

// After the install-triggered restart the server may briefly refuse
// connections. Retry the superuser login a few times before giving up.
async function loginAsSuperuserWithRetry(page: Page, attempts = 20) {
    let lastErr: unknown
    for (let i = 0; i < attempts; i++) {
        try {
            // Short per-attempt timeout so a still-restarting server fails
            // fast and the loop actually iterates, instead of one attempt
            // eating the whole budget under the default ~30s timeouts.
            await loginAsSuperuser(page, 8_000)
            return
        } catch (err) {
            lastErr = err
            await page.waitForTimeout(3_000)
        }
    }
    throw new Error(`superuser login failed after restart (${attempts} attempts): ${lastErr}`)
}

// Mints a fresh superuser token via the API. The setup page's PocketBase
// instance (useSuperUserPB) has no persistent auth store — its token lives only
// in memory — so we can't read it from localStorage. Instead authenticate
// directly against the _superusers collection, the same call the login form makes.
async function superuserToken(page: Page): Promise<string> {
    const res = await page.request.post('/api/collections/_superusers/auth-with-password', {
        data: { identity: SUPERUSER_EMAIL, password: SUPERUSER_PASSWORD },
        failOnStatusCode: false,
    })
    if (!res.ok()) {
        throw new Error(`superuser auth failed: ${res.status()} ${await res.text()}`)
    }
    const body = (await res.json()) as { token?: string }
    if (!body.token) throw new Error('superuser auth returned no token')
    return body.token
}

// Polls the admin package-status endpoint (backed by pkg_install_log) until the
// slug's install reaches `wantStatus`, throwing on a terminal failure. This is
// the SSE-independent ground truth for "did the background install finish?".
// Uses page.request so it shares the browser's network context (same origin,
// so PB_SERVER_ADDR resolution is irrelevant — we hit the same host as the app).
async function waitForInstallStatus(
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

    // One status read. Returns the parsed body, or null on any transient
    // condition (network error / connection reset / non-ok). The job ends by
    // restarting the server (exit 75), so ECONNRESET and refused connections are
    // EXPECTED mid-poll and must NOT fail the test — they just mean "try again".
    // Re-mints the token on an auth failure (tokens expire; a restart can also
    // invalidate the session).
    async function readStatusOnce(): Promise<{
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
            // Only judge the operation we're waiting on. The status endpoint
            // returns the latest log row; if wantAction is set, ignore rows for a
            // different action (e.g. a stale install row before the revert row
            // lands).
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

// Polls the pkg_build collection until the given build reaches `wantStatus`
// (e.g. base build → `current` after a revert). This is the unambiguous,
// SSE-independent signal a revert finished: a revert logs under the TARGET
// build's slug (for the base build that's "(base image)", not the package
// slug), so the build record's status — not the install log — is the reliable
// ground truth. Resilient to the exit-75 restart the revert triggers.
async function waitForBuildStatus(
    page: Page,
    buildId: string,
    wantStatus: string,
    timeoutMs: number
) {
    const deadline = Date.now() + timeoutMs
    let token = await superuserToken(page)
    let last = 'no-response-yet'
    while (Date.now() < deadline) {
        let body: { items?: Array<{ status?: string }> } | null = null
        try {
            const filter = encodeURIComponent(`build_id='${buildId}'`)
            let res = await page.request.get(
                `/api/collections/pkg_build/records?filter=${filter}`,
                { headers: { Authorization: token }, failOnStatusCode: false }
            )
            if (res.status() === 401 || res.status() === 403) {
                token = await superuserToken(page)
                res = await page.request.get(
                    `/api/collections/pkg_build/records?filter=${filter}`,
                    { headers: { Authorization: token }, failOnStatusCode: false }
                )
            }
            if (res.ok()) body = await res.json()
        } catch {
            // connection reset/refused during the restart window — retry
        }
        const status = body?.items?.[0]?.status
        if (status) {
            last = status
            if (status === wantStatus) return
        }
        await page.waitForTimeout(3_000)
    }
    throw new Error(
        `build ${buildId} did not reach ${wantStatus} within ${timeoutMs}ms (last=${last})`
    )
}

test.describe.configure({ mode: 'serial' })

test.describe('todo install', () => {
    // Hard opt-in gate: this whole suite is runner-only (see the file header).
    // Without RUN_TODO_INSTALL_TEST=1 every test is skipped, so the spec can't
    // run in a normal CI suite even if its directory gets globbed in.
    test.beforeAll(() => {
        test.skip(
            !RUN_INSTALL_TEST,
            'todo-install is runner-only — set RUN_TODO_INSTALL_TEST=1 (run-todo-install.sh does)'
        )
    })

    test('bootstrap superuser via /setup wizard', async ({ page }) => {
        test.skip(
            !SETUP_TOKEN,
            'PW_TODO_SETUP_TOKEN not set — the runner must scrape it from `docker logs`'
        )

        await page.goto(`/setup?token=${SETUP_TOKEN}`)
        await expect(page.getByText('Welcome to TinyCld')).toBeVisible()

        await page
            .getByRole('textbox', { name: 'Application Name', exact: true })
            .fill('Todo TinyCld')
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

    test('install @tinycld/todo from github through the installer UI', async ({ page }) => {
        // Generous overall budget: the runtime image has no Go module cache, so
        // the installer's `go build` downloads hundreds of MB AND compiles
        // (CGO/cgo links mupdf + libde265) — minutes on its own — and `expo
        // export` is another multi-minute web build. 45 min covers a cold run on
        // a slow network without the outer test timeout pre-empting the
        // per-stage, stage-named timeouts below.
        test.setTimeout(2_700_000) // 45 min

        await loginAsSuperuser(page)

        // Login lands on the Packages tab. Open the install form, then submit.
        // PackageManager's Install controls are Pressable+Text with no
        // accessibilityRole, so on RN Web they expose as plain text, not
        // buttons — getByRole('button', { name: 'Install' }) matches nothing.
        // Target by text instead. The field DOES expose a role (TextInput sets
        // accessibilityLabel), so getByRole('textbox', …) is correct there.
        await page.getByText('Install', { exact: true }).click()
        await page.getByRole('textbox', { name: 'npm Package Name', exact: true }).fill(TODO_SPEC)
        // When the form is open the toggle's text flips to 'Cancel', so only the
        // form's submit reads 'Install'; .last() is defensive against future
        // relabeling, not resolving a present collision.
        await page.getByText('Install', { exact: true }).last().click()

        // The install runs server-side as a background job and the modal streams
        // its progress over SSE. We DON'T assert on the SSE-streamed modal stages
        // here: the early stages can blow past in well under a second and an
        // EventSource that connects a hair late never sees them, making
        // stage-by-stage assertions inherently racy in this environment. The
        // authoritative signal that the install succeeded is the server's own
        // pkg_install_log record reaching status `success` — poll that via the
        // admin status endpoint (ground truth, independent of the modal). The
        // install ends by requesting an exit-75 restart; the runner waits for the
        // relaunch and the next test verifies the package is actually live.
        await waitForInstallStatus(page, 'todo', 'success', 2_400_000) // up to 40 min
    })

    test('todo is registered, in nav, and reachable after restart', async ({ page }) => {
        test.setTimeout(300_000)

        // 1. Registry: Todo appears on the Packages tab with an installed badge.
        await loginAsSuperuserWithRetry(page)
        await expect(page.getByText('Todo', { exact: true })).toBeVisible()
        await expect(page.getByText('installed', { exact: true }).first()).toBeVisible()

        // 1b. Build History: the install saved a restorable build. The new tab
        //     lists the todo build as `current` (proves the pkg_build row was
        //     written and builds/<id>/ archived during the install pipeline), and
        //     the base build seeded on first boot as `available` — the target the
        //     later revert test returns to.
        await page.getByText('Build History', { exact: true }).click()
        await expect(page.getByText('todo', { exact: true }).first()).toBeVisible()
        await expect(page.getByText('current', { exact: true }).first()).toBeVisible()
        await expect(page.getByText('(base image)', { exact: true })).toBeVisible()

        // 2. Create an org to log into — the superuser dashboard isn't the app
        //    shell; the nav rail lives in the org-scoped app.
        await page.getByText('Organizations', { exact: true }).first().click()
        await page.getByRole('button', { name: 'New Organization' }).click()
        await page
            .getByRole('textbox', { name: 'Organization Name', exact: true })
            .fill(TEST_ORG_NAME)
        await page.getByRole('textbox', { name: 'Slug', exact: true }).fill(TEST_ORG_SLUG)
        await page
            .getByRole('textbox', { name: 'Owner Name', exact: true })
            .fill(TEST_ORG_OWNER_NAME)
        await page
            .getByRole('textbox', { name: 'Owner Email', exact: true })
            .fill(TEST_ORG_OWNER_EMAIL)
        await page
            .getByRole('textbox', { name: 'Owner Password', exact: true })
            .fill(TEST_ORG_OWNER_PASSWORD)
        await page
            .getByRole('textbox', { name: 'Mail Domain', exact: true })
            .fill(TEST_ORG_MAIL_DOMAIN)
        await page.getByRole('button', { name: 'Create Organization' }).click()
        await expect(page.getByText(TEST_ORG_NAME, { exact: true })).toBeVisible()

        // 3. Nav + reachable screen: log into the org as the owner, confirm the
        //    Todo rail entry renders and its screen mounts.
        // Fresh page for the org-owner session. It shares this context's
        // cookies/storage with `page` (partial isolation), which is fine —
        // we only need a clean tab to drive the org-user login.
        const orgPage = await page.context().newPage()
        try {
            await orgPage.goto('/')
            await orgPage.getByTestId('identifier').fill(TEST_ORG_OWNER_EMAIL)
            await orgPage.getByTestId('login-password').fill(TEST_ORG_OWNER_PASSWORD)
            await orgPage.getByTestId('login-submit').click()
            await orgPage.waitForURL(/\/a\//, { timeout: 30_000 })

            // The Todo nav-rail entry is icon-only; target it by testID. Its
            // presence proves the installed package was wired into the nav.
            const todoNav = orgPage.getByTestId('nav-todo')
            await expect(todoNav).toBeVisible({ timeout: 30_000 })
            await todoNav.click()

            // The Todo screen mounts at /a/<orgSlug>/todo — the route resolving is
            // the signal that the installed package's screen is reachable. On a
            // freshly-installed package the lazy route chunk is compiled by Metro
            // on first navigation (cold), which can take a while on the container,
            // so allow generous headroom. We assert the route, not a specific
            // input inside the third-party Todo screen, whose markup we don't own.
            await expect(orgPage).toHaveURL(/\/a\/[^/]+\/todo/, { timeout: 120_000 })
        } finally {
            await orgPage.close()
        }
    })

    test('revert to the base build through the Build History UI', async ({ page }) => {
        // Reverting swaps in the archived base binary, runs `migrate down N` to
        // reverse todo's migration, re-stages the base web bundle, and triggers
        // another exit-75 relaunch. Generous budget for the relaunch + health
        // check (no go build / expo export this time — the base artifacts are
        // already archived — so it's far quicker than the install).
        test.setTimeout(600_000)

        await loginAsSuperuserWithRetry(page)

        await page.getByText('Build History', { exact: true }).click()
        // Only the base build (status `available`) shows a Revert control; the
        // todo build is `current` and hides it. Click Revert, then confirm.
        await expect(page.getByText('(base image)', { exact: true })).toBeVisible()
        await page.getByText('Revert', { exact: true }).first().click()
        // The confirm dialog appears with a second Revert button; .last() targets
        // the confirm action rather than the row trigger that opened it.
        await page.getByText('Revert', { exact: true }).last().click()

        // Judge the revert by the base build becoming `current` — the
        // unambiguous server-side signal it completed (a revert logs under the
        // target build's slug, "(base image)", so the install-log/status endpoint
        // keyed by package slug isn't the right probe here). The revert ends with
        // the same exit-75 restart, which the runner waits on before the next test.
        await waitForBuildStatus(page, 'build-base', 'current', 300_000)
    })

    test('todo is gone after revert to base', async ({ page }) => {
        test.setTimeout(300_000)

        // 1. Registry: the todo build is now `superseded` and the base build is
        //    `current` again.
        await loginAsSuperuserWithRetry(page)
        await page.getByText('Build History', { exact: true }).click()
        await expect(page.getByText('superseded', { exact: true }).first()).toBeVisible()

        // 2. The package is no longer wired in — its migration was reversed
        //    (collection dropped) and the reverted base binary doesn't register
        //    the package, so the Todo nav-rail entry is gone.
        const orgPage = await page.context().newPage()
        try {
            await orgPage.goto('/')
            await orgPage.getByTestId('identifier').fill(TEST_ORG_OWNER_EMAIL)
            await orgPage.getByTestId('login-password').fill(TEST_ORG_OWNER_PASSWORD)
            await orgPage.getByTestId('login-submit').click()
            await orgPage.waitForURL(/\/a\//, { timeout: 30_000 })

            // The todo rail entry should no longer render after revert.
            await expect(orgPage.getByTestId('nav-todo')).toHaveCount(0, { timeout: 30_000 })
        } finally {
            await orgPage.close()
        }
    })
})
