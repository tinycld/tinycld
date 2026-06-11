import { expect, type Page, test } from '@playwright/test'

// Integration test for the per-package VERSION-CHANGE flow in /admin, driven
// through the real in-app installer + Versions tab against an already-running
// container (the runner script builds the image from the working tree so the
// pinned-git-tag install change is present). Runs serially — every step depends
// on prior container state, and each install-class operation restarts the
// container.
//
// The scenario validates upgrade, downgrade, rollback AND delete end-to-end:
//   1. install @tinycld/todo pinned to v1.0.0 (no tags feature)
//   2. upgrade to v2.0.0 via the Versions tab (adds the tags/todo_tags schema)
//   3. tag a todo through the v2 UI (exercises the new feature)
//   4. downgrade to v1.0.0 via the Versions tab (runs the v2->v1 DOWN migration)
//   5. prove the down migration ran: tags/todo_tags collections are DROPPED
//      (collections API) and the in-app TAGS editor is gone (v1 binary).
//   6. ROLLBACK: revert to the archived v2.0.0 build (the Build-History revert).
//   7. DELETE: uninstall todo (the package-row Trash2 confirm).
//
// Steps 6 and 7 reproduce the operations an operator reported failing on /admin
// with `registry update: FAILED: database disk image is malformed (11)`. They
// run against a DB three prior install-class ops have already churned, and the
// runner mounts pb_data/builds/releases so the SQLite file sits on the same
// bind-mounted volume the operator uses (a no-mount run never reproduced it).
//
// Install/upgrade/downgrade progress streams to a modal over SSE, but the test
// judges success by polling the server's pkg_install_log status (ground truth),
// not the modal — an EventSource that connects a hair late can miss the fast
// early stages, making modal-text assertions inherently racy here.
//
// NOT a normal-CI test. It needs a purpose-built docker image (the runner
// `tests/install/run-todo-install.sh` builds it) and drives three real,
// minutes-long install-class operations (npm pack → pnpm → go build → expo
// export → restart). It is excluded from `tinycld-pkg test:e2e` by living
// outside tests/e2e/, and from the docker smoke workflow (which copies only
// setup-and-packages.spec.ts). As belt-and-suspenders we ALSO hard-skip the
// whole suite unless the runner opts in via RUN_TODO_INSTALL_TEST=1, so it can
// never run accidentally if some future config globs tests/install/ into a CI
// suite. The skip is asserted in a beforeAll inside the describe (below) so
// every test in the file is skipped as a group when the opt-in is absent.
//
// FIXTURE CONTRACT (repo tinycld/todo): the v1.0.0 tag ships package.json
// version "1.0.0" and only the create_todo migration; the v2.0.0 tag ships
// version "2.0.0" and adds create_tags (tags + todo_tags collections, with a
// down closure that drops both). The Versions feature derives `current` from
// the installed package.json version and `available` from git tags, so the tag
// names MUST match the package.json versions for upgrade/downgrade direction to
// resolve correctly. `main` mirrors v2.0.0.
const RUN_INSTALL_TEST = process.env.RUN_TODO_INSTALL_TEST === '1'

const SETUP_TOKEN = process.env.PW_TODO_SETUP_TOKEN

const SUPERUSER_EMAIL = 'todo-smoke@example.com'
const SUPERUSER_PASSWORD = 'TodoSmoke1234!'

// Install pinned to a git TAG via the #ref suffix. validatePackageSpec accepts
// `<git-spec>#<safe-ref>` and `npm pack` clones the repo at that tag.
const TODO_SPEC_V1 = 'github:tinycld/todo#v1.0.0'

// Core (base) upgrade target + the baked current version, supplied by the
// runner (run-todo-install.sh provisions a local base remote with these tags).
const CORE_CUR = process.env.PW_CORE_CUR ?? '0.0.4'
const CORE_NEXT = process.env.PW_CORE_NEXT ?? '0.0.5'

const TEST_ORG_NAME = 'Todo Org'
const TEST_ORG_SLUG = 'todo-org'
const TEST_ORG_OWNER_NAME = 'Todo Owner'
const TEST_ORG_OWNER_EMAIL = 'owner@todo.example'
const TEST_ORG_OWNER_PASSWORD = 'OwnerPass1234!'
const TEST_ORG_MAIL_DOMAIN = 'todo.example'

const TODO_TEXT = 'Buy milk'
const TAG_TEXT = 'errand'

async function loginAsSuperuser(page: Page, timeoutMs?: number) {
    await page.goto('/admin', timeoutMs ? { timeout: timeoutMs } : undefined)
    await expect(page.getByText('Superuser Login')).toBeVisible(
        timeoutMs ? { timeout: timeoutMs } : undefined
    )
    await page.getByRole('textbox', { name: 'Email', exact: true }).fill(SUPERUSER_EMAIL)
    await page.getByRole('textbox', { name: 'Password', exact: true }).fill(SUPERUSER_PASSWORD)
    await page.getByRole('button', { name: 'Sign in' }).click()
    // 'Organizations' appears in both the nav rail and (when that tab is open)
    // the page title, so scope to the first match.
    await expect(page.getByText('Organizations', { exact: true }).first()).toBeVisible(
        timeoutMs ? { timeout: timeoutMs } : undefined
    )
}

// After an install-class restart the server may briefly refuse connections.
// Retry the superuser login a few times before giving up.
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
    // A package operation's backupDatabase() does `VACUUM INTO`, which briefly
    // contends the SQLite WAL; a superuser auth landing in that window can come
    // back 5xx ("Something went wrong"). It clears in well under a second, so
    // retry transient server errors a few times rather than throwing — the same
    // resilience the polling helpers apply to the mid-restart window. A genuine
    // auth failure (4xx) still throws immediately.
    let lastStatus = 0
    let lastBody = ''
    for (let attempt = 0; attempt < 8; attempt++) {
        const res = await page.request.post('/api/collections/_superusers/auth-with-password', {
            data: { identity: SUPERUSER_EMAIL, password: SUPERUSER_PASSWORD },
            failOnStatusCode: false,
        })
        if (res.ok()) {
            const body = (await res.json()) as { token?: string }
            if (!body.token) throw new Error('superuser auth returned no token')
            return body.token
        }
        lastStatus = res.status()
        lastBody = await res.text()
        if (lastStatus < 500) {
            // A real client/auth error — don't retry.
            throw new Error(`superuser auth failed: ${lastStatus} ${lastBody}`)
        }
        await page.waitForTimeout(750) // transient 5xx (DB busy) — back off and retry
    }
    throw new Error(`superuser auth failed after retries: ${lastStatus} ${lastBody}`)
}

// Reads the id of the slug's most-recent pkg_install_log row (via the status
// endpoint), or null if there's no row yet / the server isn't reachable. Used to
// snapshot the prior operation's row id BEFORE kicking off a new operation, so
// waitForOpStatus can ignore that stale row (see `notId`).
async function latestOpId(page: Page, slug: string): Promise<string | null> {
    let token: string
    try {
        token = await superuserToken(page)
    } catch {
        return null
    }
    let res: Awaited<ReturnType<typeof page.request.get>>
    try {
        res = await page.request.get(`/api/admin/packages/status/${slug}`, {
            headers: { Authorization: token },
            failOnStatusCode: false,
        })
    } catch {
        return null
    }
    if (!res.ok()) return null
    const body = (await res.json()) as { id?: string }
    return body.id ?? null
}

// Polls the admin package-status endpoint (backed by pkg_install_log) until the
// slug's operation reaches `wantStatus`, throwing on a terminal failure. This is
// the SSE-independent ground truth for "did the background operation finish?".
// Uses page.request so it shares the browser's network context (same origin,
// so PB_SERVER_ADDR resolution is irrelevant — we hit the same host as the app).
// `wantAction` (e.g. 'install' / 'version_change') ignores stale rows for a
// different action when more than one operation has run for the slug.
//
// `notId` guards against a stale SAME-action row: the status endpoint returns the
// single most-recent row, and a version-change POST returns 202 and writes its
// new log row ASYNCHRONOUSLY. Between the click and that write, the latest
// `version_change` row is still the PRIOR version-change's `success` row — which
// would falsely satisfy the wait. Pass the prior row's id (snapshot via
// latestOpId before the click) so a matching id is treated as "not yet started".
// waitForProgressAdvance asserts the install progress modal's bar actually moves
// — i.e. SSE progress events are reaching the browser. Reads the numeric value
// off the progress fill (accessibilityValue → aria-valuenow) and waits until it
// climbs to at least `minPct`. This is the end-to-end guard for the events-stream
// auth: a 403 on /api/admin/packages/events (the token-type bug) leaves the bar
// frozen at 0% even though the server-side install runs fine, so a stuck bar here
// catches that regression — distinctly from waitForOpStatus, which reads the
// install log directly and would pass even with a dead stream.
async function waitForProgressAdvance(page: Page, minPct: number, timeoutMs: number) {
    const fill = page.getByTestId('install-progress-fill')
    // The modal mounts as soon as the install POST returns a jobId.
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

    // One status read. Returns the parsed body, or null on any transient
    // condition (network error / connection reset / non-ok). The job ends by
    // restarting the server (exit 75), so ECONNRESET and refused connections are
    // EXPECTED mid-poll and must NOT fail the test — they just mean "try again".
    // Re-mints the token on an auth failure (tokens expire; a restart can also
    // invalidate the session).
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

// Reads pkg_registry.version for a slug via the superuser API — the
// authoritative `current` the Versions feature compares against. Returns null
// when the registry row or server isn't reachable (e.g. mid-restart).
async function registryVersion(page: Page, slug: string): Promise<string | null> {
    let token: string
    try {
        token = await superuserToken(page)
    } catch {
        return null
    }
    const filter = encodeURIComponent(`slug='${slug}'`)
    let res: Awaited<ReturnType<typeof page.request.get>>
    try {
        res = await page.request.get(`/api/collections/pkg_registry/records?filter=${filter}`, {
            headers: { Authorization: token },
            failOnStatusCode: false,
        })
    } catch {
        return null
    }
    if (!res.ok()) return null
    // Tolerate a transient non-JSON body (the SPA HTML shell) the same way
    // collectionExists() tolerates non-200s: during the post-install-restart
    // window the catch-all can briefly answer an /api/ GET with app.html before
    // the collection routes are wired. Return null so the caller retries rather
    // than throwing "Unexpected token '<'" on res.json().
    let body: { items?: Array<{ version?: string }> }
    try {
        body = (await res.json()) as { items?: Array<{ version?: string }> }
    } catch {
        return null
    }
    return body.items?.[0]?.version ?? null
}

// Polls registryVersion until the slug reports `wantVersion`. After a version
// change the registry row is rewritten to the swapped package.json version, so
// this is the SSE-independent confirmation that a version change took effect.
async function waitForRegistryVersion(
    page: Page,
    slug: string,
    wantVersion: string,
    timeoutMs: number
) {
    const deadline = Date.now() + timeoutMs
    let last = 'none'
    while (Date.now() < deadline) {
        const v = await registryVersion(page, slug)
        if (v) {
            last = v
            if (v === wantVersion) return
        }
        await page.waitForTimeout(3_000)
    }
    throw new Error(
        `${slug} registry version did not reach ${wantVersion} within ${timeoutMs}ms (last=${last})`
    )
}

// Returns whether a collection exists, via a superuser read of its records
// endpoint. PocketBase returns 404 for an UNKNOWN COLLECTION (not for an empty
// record set — a known-but-empty collection returns 200 with items: []), so the
// 200/404 split is a reliable existence check. This is the ground truth the
// down-migration assertion rests on: after the v2→v1 downgrade drops `tags`/
// `todo_tags`, their records endpoints must 404. Any other status (incl.
// transient mid-restart) returns null so callers retry rather than mis-assert.
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
        return null
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

// POSTs to an admin package endpoint with superuser auth — the same call the
// /admin UI makes. Used to drive uninstall (`/uninstall` {slug}) and rollback
// (`/revert` {buildId}) from the test the way the Trash2 button / Build-History
// revert do, but without the brittle unlabeled-icon selectors. The server runs
// the operation as a background job; the test judges success via waitForOpStatus
// against pkg_install_log (ground truth), identical to the install/version paths.
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

// The app version (app.json → expo.version) the native bundles are stamped with.
// /api/app/update keys updates by runtimeVersion, so the OTA check must send the
// same value a real device on this build reports. Bump here if app.json changes.
const RUNTIME_VERSION = '1.13.7'

// Polls the public OTA endpoint /api/app/update until it advertises a new bundle
// (HTTP 200 + manifest) for the given platform, or throws. After every package
// modification (install/upgrade/downgrade/rollback) the pipeline runs
// `expo export --platform <p>`, archives the bundle on the new `current`
// pkg_build, and /api/app/update serves it — so a 200 here is the end-to-end
// proof that "a new expo update is available" to mobile clients.
//
// We send `currentId` = a bundle id the server will never have minted (the shape
// a fresh App-Store install reports, `embedded-<ver>`), so the server always
// classifies its current bundle as NEW relative to us and returns 200 with the
// manifest. The returned manifest id is the per-platform bundle id
// (`build-<ts>-<platform>`); the caller asserts it changed across modifications.
// 404/204 mean "no update": 204 specifically is what a toolchain-less image
// returns (no native bundles) — surfaced in the throw so a regression there is
// obvious rather than a silent timeout.
async function waitForExpoUpdate(
    page: Page,
    platform: 'ios' | 'android',
    timeoutMs: number
): Promise<{ id: string; bundleHash: string; bundleUrl: string }> {
    const url =
        `/api/app/update?platform=${platform}` +
        `&runtimeVersion=${RUNTIME_VERSION}` +
        `&currentId=embedded-${RUNTIME_VERSION}&currentHash=none`
    const deadline = Date.now() + timeoutMs
    let last = 'no-response'
    while (Date.now() < deadline) {
        let res: Awaited<ReturnType<typeof page.request.get>> | null = null
        try {
            res = await page.request.get(url, { failOnStatusCode: false })
        } catch {
            res = null // connection refused mid-restart — retry
        }
        if (res) {
            if (res.status() === 200) {
                // Tolerate a transient SPA-HTML 200 during the post-restart
                // window (the catch-all can briefly answer /api/ with app.html
                // before the route is wired); retry rather than throw on json().
                let m: { id?: string; bundleHash?: string; bundleUrl?: string } | null = null
                try {
                    m = (await res.json()) as {
                        id?: string
                        bundleHash?: string
                        bundleUrl?: string
                    }
                } catch {
                    m = null
                }
                if (m?.id)
                    return {
                        id: m.id,
                        bundleHash: m.bundleHash ?? '',
                        bundleUrl: m.bundleUrl ?? '',
                    }
            }
            last = `HTTP ${res.status()}`
        }
        await page.waitForTimeout(2_000)
    }
    throw new Error(
        `/api/app/update never advertised a ${platform} update within ${timeoutMs}ms (last=${last}). ` +
            `A persistent 204 means native bundles weren't produced — the build image is missing the ` +
            `RN toolchain (node_modules/expo), so 'expo export --platform ${platform}' was skipped.`
    )
}

// Asserts a NEW expo update is offered for both native platforms after a package
// modification, and that each platform's bundle id advanced from the previous
// modification's id. `prev` is the per-platform id map from the last call (empty
// on first use). Returns the new id map to thread into the next assertion.
async function assertNewExpoUpdate(
    page: Page,
    prev: { ios?: string; android?: string }
): Promise<{ ios: string; android: string }> {
    const ios = await waitForExpoUpdate(page, 'ios', 120_000)
    const android = await waitForExpoUpdate(page, 'android', 120_000)
    expect(ios.id, 'ios bundle id should be a build-<ts>-ios id').toMatch(/^build-\d+-ios$/)
    expect(android.id, 'android bundle id should be a build-<ts>-android id').toMatch(
        /^build-\d+-android$/
    )
    if (prev.ios) {
        expect(ios.id, 'a modification must produce a NEW ios bundle id').not.toBe(prev.ios)
    }
    if (prev.android) {
        expect(android.id, 'a modification must produce a NEW android bundle id').not.toBe(
            prev.android
        )
    }
    return { ios: ios.id, android: android.id }
}

// Logs in as the org owner on a fresh page and navigates to the Todo screen.
// Returns the page so the caller can interact + close it. The owner session
// shares this context's cookies/storage (partial isolation), which is fine —
// we only need a clean tab to drive the org-user login.
async function openTodoAsOwner(page: Page): Promise<Page> {
    const orgPage = await page.context().newPage()
    await orgPage.goto('/')
    await orgPage.getByTestId('identifier').fill(TEST_ORG_OWNER_EMAIL)
    await orgPage.getByTestId('login-password').fill(TEST_ORG_OWNER_PASSWORD)
    await orgPage.getByTestId('login-submit').click()
    await orgPage.waitForURL(/\/a\//, { timeout: 30_000 })

    const todoNav = orgPage.getByTestId('nav-todo')
    await expect(todoNav).toBeVisible({ timeout: 30_000 })
    await todoNav.click()
    // On a freshly-installed/changed package the lazy route chunk is compiled by
    // Metro on first navigation (cold), which can take a while on the container.
    await expect(orgPage).toHaveURL(/\/a\/[^/]+\/todo/, { timeout: 120_000 })
    return orgPage
}

// Sets a package row's target version on the Packages screen and applies it.
// Versions were merged into Packages: each package row carries its own version
// picker (an anchored Menu / RowVersionSelect) whose options read `v<version>` /
// `v<version> (current)`, plus a staged-changes apply footer. On a downgrade the
// ConfirmChangesModal requires typing the slug to confirm; on an upgrade it's a
// plain Apply. The Packages screen is the default tab after login, so there's no
// tab to switch to first.
async function applyVersionChange(
    page: Page,
    slug: string,
    targetLabel: string,
    opts: { downgrade: boolean }
) {
    // Scope every action to the TARGET package's row. In a full assembly the
    // Packages list shows ~9 rows, each with its own `v… (current)` picker, so a
    // global `.first()` would grab the wrong row. The row renders the bare slug in
    // a monospace SlugTag; identify the row by that slug cell + its `(current)`
    // trigger, then act only within it.
    //
    // The screen fetches /versions on mount, which shells out to `git ls-remote`
    // server-side (seconds, spinner until it resolves) — so gate on the slug cell
    // with a generous budget for a cold ls-remote before touching the picker.
    const slugCell = page.getByText(slug, { exact: true })
    await expect(slugCell.first()).toBeVisible({ timeout: 90_000 })
    // Row container = the nearest ancestor div that holds BOTH this slug cell and a
    // `(current)` version trigger — i.e. the package row. Filtering ancestors by
    // `has:` both descendants pins the right row without depending on the exact
    // nesting depth (which the layout can change).
    const row = page
        .locator('div')
        .filter({ has: page.getByText(slug, { exact: true }) })
        .filter({ has: page.getByText(/^v\d+\.\d+\.\d+ \(current\)$/) })
        .last()

    // Open this row's picker. Its label reads `v<cur> (current)` (the ` (current)`
    // suffix is unique to the trigger; the slug cell has no parenthetical).
    const trigger = row.getByText(/^v\d+\.\d+\.\d+ \(current\)$/)
    await expect(trigger).toBeVisible({ timeout: 30_000 })
    await trigger.click()
    // The opened menu renders in a portal (outside the row), so select the option
    // at page scope by exact label (e.g. `v2.0.0`).
    await page.getByText(targetLabel, { exact: true }).click()

    // Selecting a target kicks off a 300ms-debounced server compatibility check
    // (`/versions/check`); the Apply bar button is disabled while it runs. A RN-Web
    // Pressable's disabled state is opacity-only (no DOM `disabled`), so a click
    // during the check would silently no-op and the confirm modal would never open.
    // Wait for the "Checking compatibility…" indicator to clear before applying.
    await expect(page.getByText('Checking compatibility…')).toBeHidden({ timeout: 30_000 })

    // Apply the pending change, then confirm in the modal that opens.
    await page.getByText(/^Apply \d+ change/).click()

    if (opts.downgrade) {
        // Downgrade modal ("Confirm downgrade") gates on typing the slug; the input
        // placeholder is the comma-joined downgraded slug list.
        await expect(page.getByText('Confirm downgrade')).toBeVisible({ timeout: 15_000 })
        await page.getByPlaceholder(slug).fill(slug)
        await page.getByText('Downgrade', { exact: true }).click()
    } else {
        // Upgrade modal ("Apply version changes") confirms with a plain "Apply".
        await expect(page.getByText('Apply version changes')).toBeVisible({ timeout: 15_000 })
        await page.getByText('Apply', { exact: true }).last().click()
    }
}

test.describe.configure({ mode: 'serial' })

// Tracks the per-platform OTA bundle id seen after the previous modification, so
// each modification can assert its id ADVANCED (a genuinely new update), not just
// that some update exists. Module-scoped because the suite runs serial and these
// verify steps are separate tests sharing the one container's state.
let lastExpoBundleIds: { ios?: string; android?: string } = {}

test.describe('todo version change', () => {
    // Hard opt-in gate: this whole suite is runner-only (see the file header).
    // Without RUN_TODO_INSTALL_TEST=1 every test is skipped, so the spec can't
    // run in a normal CI suite even if its directory gets globbed in.
    test.beforeAll(() => {
        test.skip(
            !RUN_INSTALL_TEST,
            'todo-install is runner-only — set RUN_TODO_INSTALL_TEST=1 (run-todo-install.sh does)'
        )
    })

    test('bootstrap superuser via /admin wizard', async ({ page }) => {
        test.skip(
            !SETUP_TOKEN,
            'PW_TODO_SETUP_TOKEN not set — the runner must scrape it from `docker logs`'
        )

        await page.goto(`/admin?token=${SETUP_TOKEN}`)
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

    test('install @tinycld/todo pinned to v1.0.0 through the installer UI', async ({ page }) => {
        // Generous overall budget: the runtime image has no Go module cache, so
        // the installer's `go build` downloads hundreds of MB AND compiles
        // (CGO/cgo links mupdf + libde265) — minutes on its own — and `expo
        // export` is another multi-minute web build. 45 min covers a cold run on
        // a slow network without the outer test timeout pre-empting the
        // per-stage, stage-named timeouts below.
        test.setTimeout(2_700_000) // 45 min

        await loginAsSuperuser(page)

        // Login lands on the Packages tab. 'Install package' opens a modal; fill
        // the package source field, then submit with the modal's 'Install' button.
        // Buttons are gluestack Button (role=button); the field exposes a role via
        // TextInput's accessibilityLabel.
        await page.getByRole('button', { name: 'Install package' }).click()
        await page.getByRole('textbox', { name: 'Package source', exact: true }).fill(TODO_SPEC_V1)
        await page.getByRole('button', { name: 'Install', exact: true }).click()

        // The SSE progress modal must actually advance — proves the events stream
        // authenticates and reaches the browser. The early stages (validate, npm
        // pack, manifest, copy, pnpm) carry the bar well past 50% before the long
        // go-build/expo-export stages, so requiring ≥50% within 10 min confirms a
        // live stream without coupling to a specific percentage. (A frozen 0% bar
        // here is the signature of the events-endpoint 403 regression.)
        await waitForProgressAdvance(page, 50, 600_000)

        // The install runs server-side as a background job and ends by requesting
        // an exit-75 restart. Judge success by the server's own pkg_install_log
        // reaching status `success` (ground truth, independent of the SSE modal).
        await waitForOpStatus(page, 'todo', 'success', 2_400_000, 'install') // up to 40 min
    })

    test('v1.0.0 is live, has no tags schema, and an org exists', async ({ page }) => {
        test.setTimeout(300_000)

        await loginAsSuperuserWithRetry(page)

        // 1. Registry records the installed version as 1.0.0 (proves the pinned
        //    tag install resolved to the v1 tag, whose package.json is 1.0.0).
        await waitForRegistryVersion(page, 'todo', '1.0.0', 60_000)

        // 2. v1 ships no tagging — the tags/todo_tags collections must NOT exist.
        await waitForCollection(page, 'tags', false, 30_000)
        await waitForCollection(page, 'todo_tags', false, 30_000)

        // 3. Create an org to log into — the superuser dashboard isn't the app
        //    shell; the nav rail lives in the org-scoped app.
        await page.getByText('Organizations', { exact: true }).first().click()
        await page.getByRole('button', { name: 'New organization' }).click()
        await page.getByRole('textbox', { name: 'Name', exact: true }).fill(TEST_ORG_NAME)
        await page.getByRole('textbox', { name: 'Slug', exact: true }).fill(TEST_ORG_SLUG)
        await page
            .getByRole('textbox', { name: 'Full name', exact: true })
            .fill(TEST_ORG_OWNER_NAME)
        await page.getByRole('textbox', { name: 'Email', exact: true }).fill(TEST_ORG_OWNER_EMAIL)
        await page
            .getByRole('textbox', { name: 'Password', exact: true })
            .fill(TEST_ORG_OWNER_PASSWORD)
        await page
            .getByRole('textbox', { name: 'Mail domain', exact: true })
            .fill(TEST_ORG_MAIL_DOMAIN)
        await page.getByRole('button', { name: 'Create organization' }).click()
        await expect(page.getByText(TEST_ORG_NAME, { exact: true })).toBeVisible()

        // 4. Seed a todo as the org owner so the upgrade has data to tag later.
        const orgPage = await openTodoAsOwner(page)
        try {
            await orgPage.getByPlaceholder('Add a todo…').fill(TODO_TEXT)
            await orgPage.getByLabel('Add todo').click()
            await expect(orgPage.getByText(TODO_TEXT, { exact: true })).toBeVisible()
        } finally {
            await orgPage.close()
        }

        // 5. The install produced a new web+native release — mobile clients must now
        //    be offered an OTA update. Record the per-platform bundle ids so later
        //    modifications can assert the id advanced (a genuinely new update).
        lastExpoBundleIds = await assertNewExpoUpdate(page, lastExpoBundleIds)
    })

    test('upgrade todo to v2.0.0 via the Packages version picker', async ({ page }) => {
        // Upgrade fetches v2, runs the create_tags UP migration, rebuilds, and
        // requests an exit-75 relaunch. Same multi-minute build budget as install.
        test.setTimeout(2_700_000) // 45 min

        await loginAsSuperuserWithRetry(page)
        // Snapshot the prior log row (the install's row) so the wait ignores it
        // until the version-change row lands — the POST is async, so the status
        // endpoint briefly still returns the previous operation's row.
        const priorId = await latestOpId(page, 'todo')
        await applyVersionChange(page, 'todo', 'v2.0.0', { downgrade: false })

        // Judge by the version-change log reaching success (ground truth). The
        // restart follows; the runner waits for relaunch before the next test.
        await waitForOpStatus(page, 'todo', 'success', 2_400_000, 'version_change', priorId)
    })

    test('v2.0.0 is live, has the tags schema, and a todo can be tagged', async ({ page }) => {
        test.setTimeout(300_000)

        await loginAsSuperuserWithRetry(page)

        // 1. Registry now reports 2.0.0 and the tags schema exists.
        await waitForRegistryVersion(page, 'todo', '2.0.0', 60_000)
        await waitForCollection(page, 'tags', true, 60_000)
        await waitForCollection(page, 'todo_tags', true, 60_000)

        // 2. Tag the existing todo through the v2 UI — the new feature in action.
        const orgPage = await openTodoAsOwner(page)
        try {
            // Open the todo's detail screen, where the TAGS editor lives.
            await orgPage.getByLabel(`Edit ${TODO_TEXT}`).click()
            await expect(orgPage.getByText('TAGS', { exact: true })).toBeVisible({
                timeout: 30_000,
            })
            await orgPage.getByPlaceholder('Add a tag…').fill(TAG_TEXT)
            await orgPage.getByLabel('Add tag').click()
            // The new tag renders as a chip and can be removed (proves the link
            // row persisted, not just optimistic text).
            await expect(orgPage.getByLabel(`Remove tag ${TAG_TEXT}`)).toBeVisible({
                timeout: 15_000,
            })
        } finally {
            await orgPage.close()
        }

        // 3. Ground truth: a todo_tags link row exists in the DB.
        const token = await superuserToken(page)
        const res = await page.request.get('/api/collections/todo_tags/records?perPage=1', {
            headers: { Authorization: token },
            failOnStatusCode: false,
        })
        expect(res.ok()).toBeTruthy()
        const body = (await res.json()) as { totalItems?: number }
        expect(body.totalItems ?? 0).toBeGreaterThan(0)

        // 4. The upgrade rebuilt the bundles → a new OTA update must be offered,
        //    with bundle ids distinct from the install's.
        lastExpoBundleIds = await assertNewExpoUpdate(page, lastExpoBundleIds)
    })

    test('downgrade todo to v1.0.0 via the Packages version picker', async ({ page }) => {
        // Downgrade fetches v1, runs the create_tags DOWN migration (drops
        // todo_tags then tags), rebuilds, and requests an exit-75 relaunch.
        test.setTimeout(2_700_000) // 45 min

        await loginAsSuperuserWithRetry(page)
        // CRUCIAL: the prior latest row is the UPGRADE's version_change/success
        // row — same action — so without notId the wait would return immediately
        // against it, racing the still-running downgrade. Snapshot it first.
        const priorId = await latestOpId(page, 'todo')
        await applyVersionChange(page, 'todo', 'v1.0.0', { downgrade: true })

        await waitForOpStatus(page, 'todo', 'success', 2_400_000, 'version_change', priorId)
    })

    test('down migration ran: tags schema dropped and TAGS editor gone', async ({ page }) => {
        test.setTimeout(300_000)

        await loginAsSuperuserWithRetry(page)

        // 1. Registry back to 1.0.0.
        await waitForRegistryVersion(page, 'todo', '1.0.0', 60_000)

        // 2. THE CRUX — the v2->v1 down migration dropped both collections.
        await waitForCollection(page, 'todo_tags', false, 60_000)
        await waitForCollection(page, 'tags', false, 60_000)

        // 3. UI confirmation: the todo survived (its row is in todo_items, which
        //    v1 keeps) but the reverted v1 binary no longer ships the tag editor,
        //    so the detail screen has no TAGS section.
        const orgPage = await openTodoAsOwner(page)
        try {
            await expect(orgPage.getByText(TODO_TEXT, { exact: true })).toBeVisible({
                timeout: 30_000,
            })
            await orgPage.getByLabel(`Edit ${TODO_TEXT}`).click()
            // The v1 detail screen renders the DESCRIPTION editor; wait for it so
            // we're asserting against a mounted screen, not a still-loading one.
            await expect(orgPage.getByText('DESCRIPTION', { exact: true })).toBeVisible({
                timeout: 30_000,
            })
            await expect(orgPage.getByText('TAGS', { exact: true })).toHaveCount(0)
        } finally {
            await orgPage.close()
        }

        // A downgrade is a modification too: it reverts files + rebuilds the
        // bundles, so a fresh OTA update must be advertised with new bundle ids.
        lastExpoBundleIds = await assertNewExpoUpdate(page, lastExpoBundleIds)
    })

    // --- The operations the operator reported failing on /admin with
    // --- "registry update: FAILED: database disk image is malformed (11)".
    // These run LAST, against a DB that three prior install-class operations have
    // already churned, which is when the corruption surfaced in the field. The
    // runner mounts pb_data/builds/releases so the SQLite file lives on the same
    // bind-mounted volume the operator uses (the no-mount runner never reproduced
    // this — WAL behaves on the container's own overlayfs).

    test('rollback: revert to the archived v2.0.0 build succeeds (no malformed DB)', async ({
        page,
    }) => {
        // Revert restores the archived build: swaps its binary, runs migrate to
        // reconcile schema, re-stages its bundle, rewrites build/registry rows,
        // then relaunches. Same multi-minute budget as the other install-class ops.
        test.setTimeout(2_700_000) // 45 min

        await loginAsSuperuserWithRetry(page)

        // Target the v2.0.0 build that the upgrade archived. It's now a
        // non-current build in history (the downgrade made the v1 build current),
        // so reverting to it is a valid forward restore that re-applies create_tags.
        const token = await superuserToken(page)
        const filter = encodeURIComponent(
            "pkg_slug='todo' && version='2.0.0' && status!='superseded'"
        )
        const res = await page.request.get(
            `/api/collections/pkg_build/records?filter=${filter}&sort=-created&perPage=1`,
            { headers: { Authorization: token }, failOnStatusCode: false }
        )
        expect(res.ok()).toBeTruthy()
        const builds = (await res.json()) as { items?: Array<{ build_id?: string }> }
        const targetBuild = builds.items?.[0]?.build_id
        expect(targetBuild, 'expected an archived v2.0.0 build to revert to').toBeTruthy()

        const priorId = await latestOpId(page, 'todo')
        // POST /revert — the same call the Build-History "Revert" control issues.
        await postAdminPackageOp(page, 'revert', { buildId: targetBuild as string })

        // THE ASSERTION the operator's failure violates: the revert's
        // pkg_install_log row must reach `success`, NOT `failed` with a malformed
        // DB at "Updating records". waitForOpStatus throws with the server's error
        // string on a failed/rolled_back row, surfacing the exact malformed-DB
        // message if it recurs.
        await waitForOpStatus(page, 'todo', 'success', 2_400_000, 'revert', priorId)
    })

    test('rollback landed: v2.0.0 is current again and the tags schema is back', async ({
        page,
    }) => {
        test.setTimeout(300_000)
        await loginAsSuperuserWithRetry(page)
        // The reverted-to v2 build is current; its schema (create_tags) is reapplied.
        await waitForRegistryVersion(page, 'todo', '2.0.0', 60_000)
        await waitForCollection(page, 'tags', true, 60_000)
        await waitForCollection(page, 'todo_tags', true, 60_000)

        // The revert re-staged the archived v2 build as current, so /api/app/update
        // again advertises that build's bundles — distinct from the downgrade's, so
        // a client that took the downgrade update is offered this one.
        lastExpoBundleIds = await assertNewExpoUpdate(page, lastExpoBundleIds)
    })

    test('delete: uninstalling todo succeeds (no malformed DB at registry update)', async ({
        page,
    }) => {
        // Uninstall rebuilds the web bundle without the member, then flips the
        // registry row to `disabled` and relaunches. No migrate subprocess, but it
        // still ends with the app.Save the operator saw fail at "registry update".
        test.setTimeout(2_700_000) // 45 min

        await loginAsSuperuserWithRetry(page)
        const priorId = await latestOpId(page, 'todo')
        // POST /uninstall — the same call the Trash2 confirm modal issues.
        await postAdminPackageOp(page, 'uninstall', { slug: 'todo' })

        // The uninstall log row must reach `success`. If the malformed-DB bug
        // recurs it lands here as action=uninstall/status=failed and this throws
        // with the server's "registry update: … malformed" error.
        await waitForOpStatus(page, 'todo', 'success', 2_400_000, 'uninstall', priorId)
    })

    test('delete landed: todo registry row is disabled', async ({ page }) => {
        test.setTimeout(300_000)
        await loginAsSuperuserWithRetry(page)
        // The registry keeps the row but marks it disabled (the uninstall pipeline's
        // final state). Poll the row's status via the superuser API.
        const deadline = Date.now() + 90_000
        let last = 'none'
        while (Date.now() < deadline) {
            const token = await superuserToken(page).catch(() => null)
            if (token) {
                const filter = encodeURIComponent("slug='todo'")
                const res = await page.request
                    .get(`/api/collections/pkg_registry/records?filter=${filter}`, {
                        headers: { Authorization: token },
                        failOnStatusCode: false,
                    })
                    .catch(() => null)
                if (res?.ok()) {
                    const body = (await res.json()) as { items?: Array<{ status?: string }> }
                    last = body.items?.[0]?.status ?? 'no-row'
                    if (last === 'disabled') return
                }
            }
            await page.waitForTimeout(3_000)
        }
        throw new Error(`todo registry status did not reach 'disabled' within 90s (last=${last})`)
    })

    test(`upgrade core to v${CORE_NEXT} via the Packages version picker`, async ({ page }) => {
        test.setTimeout(2_700_000) // 45 min — base rebuild is a full image rebuild
        await loginAsSuperuserWithRetry(page)
        const priorId = await latestOpId(page, 'core')
        // The base row uses the same RowVersion picker as any package.
        await applyVersionChange(page, 'core', `v${CORE_NEXT}`, { downgrade: false })
        await waitForOpStatus(page, 'core', 'success', 2_400_000, 'version_change', priorId)
    })

    test(`core upgrade landed: v${CORE_NEXT} live and base_probe schema present`, async ({
        page,
    }) => {
        test.setTimeout(300_000)
        await loginAsSuperuserWithRetry(page)
        await waitForRegistryVersion(page, 'core', CORE_NEXT, 60_000)
        await waitForCollection(page, 'base_probe', true, 60_000)
    })

    test(`downgrade core to v${CORE_CUR} via the Packages version picker`, async ({ page }) => {
        test.setTimeout(2_700_000) // 45 min
        await loginAsSuperuserWithRetry(page)
        const priorId = await latestOpId(page, 'core')
        await applyVersionChange(page, 'core', `v${CORE_CUR}`, { downgrade: true })
        await waitForOpStatus(page, 'core', 'success', 2_400_000, 'version_change', priorId)
    })

    test(`core downgrade landed: v${CORE_CUR} live and base_probe schema dropped`, async ({
        page,
    }) => {
        test.setTimeout(300_000)
        await loginAsSuperuserWithRetry(page)
        await waitForRegistryVersion(page, 'core', CORE_CUR, 60_000)
        await waitForCollection(page, 'base_probe', false, 60_000)
    })
})
