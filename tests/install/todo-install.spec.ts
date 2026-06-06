import { expect, type Page, test } from '@playwright/test'

// Integration test for installing @tinycld/todo from GitHub through the
// real in-app package installer. Boots against an already-running
// container (the runner script builds the image from the working tree so
// the git-spec validation change is present). Runs serially — every step
// depends on prior container state, and the install restarts the container.
//
// Designed for legible failure: each install stage is asserted by its
// visible modal message, so a hang reports the last stage that appeared
// rather than a generic timeout.
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

// Asserts the install modal advances through an expected stage by its
// visible message text. Both racing waits only RESOLVE — the throw happens
// after the race settles, based on the winner, so the losing wait can't
// reject into an unhandled promise after the race is over. `label` names
// the stage in the failure message; `timeoutMs` lets slow build stages
// (go build, expo export) get more headroom than the fast ones.
async function expectStage(
    page: Page,
    label: string,
    messageSubstring: string,
    timeoutMs = 120_000
) {
    const failed = page.getByText(/^FAILED: /).first()
    const target = page.getByText(messageSubstring, { exact: false }).first()
    const winner = await Promise.race([
        target.waitFor({ state: 'visible', timeout: timeoutMs }).then(() => 'ok' as const),
        failed.waitFor({ state: 'visible', timeout: timeoutMs }).then(() => 'failed' as const),
    ])
    if (winner === 'failed') {
        const msg = (await failed.textContent()) ?? ''
        throw new Error(`install failed before "${label}": ${msg}`)
    }
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

        // The InstallProgressModal renders once the job starts. Walk the
        // stages in order — each assertion names where it got stuck.
        // Per-stage timeouts are the WAIT FOR THAT STAGE'S MESSAGE TO APPEAR,
        // so each must cover the duration of the operation that PRECEDES it.
        // The two long waits: 'pnpm install' follows the cold pnpm install, and
        // 'Running expo export' is reached only after go build (uncached →
        // downloads + CGO compile, the single slowest step). Give those, plus
        // the post-expo restart signal, large headroom; the fast metadata
        // stages stay tight so a genuine hang there still fails fast.
        await expectStage(page, 'Downloading package', 'Running npm pack', 120_000)
        await expectStage(page, 'Manifest parsed', 'Package: Todo (todo)', 120_000)
        await expectStage(page, 'Installing dependencies', 'Running pnpm install', 300_000)
        await expectStage(page, 'Generating wiring', 'Running package generation script', 120_000)
        await expectStage(page, 'Building server', 'Compiling new server binary', 300_000)
        // go build (uncached): downloads + CGO compile — the slowest single step.
        await expectStage(page, 'Building web app', 'Running expo export', 900_000)
        // expo export: the web bundle rebuild.
        await expectStage(page, 'Requesting restart', 'Signaling server restart', 600_000)

        // Confirm the modal didn't end in a failed state.
        await expect(page.getByText('Installation Failed')).not.toBeVisible()
    })

    test('todo is registered, in nav, and reachable after restart', async ({ page }) => {
        test.setTimeout(300_000)

        // 1. Registry: Todo appears on the Packages tab with an installed badge.
        await loginAsSuperuserWithRetry(page)
        await expect(page.getByText('Todo', { exact: true })).toBeVisible()
        await expect(page.getByText('installed', { exact: true }).first()).toBeVisible()

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

            // The Todo nav-rail entry is icon-only; target it by testID.
            const todoNav = orgPage.getByTestId('nav-todo')
            await expect(todoNav).toBeVisible({ timeout: 30_000 })
            await todoNav.click()

            // The Todo screen mounts at /a/<orgSlug>/todo. Its add-todo input is a
            // unique, stable signal that the installed package's screen loaded.
            await expect(orgPage).toHaveURL(/\/a\/[^/]+\/todo/, { timeout: 30_000 })
            await expect(orgPage.getByPlaceholder('Add a todo…')).toBeVisible({ timeout: 30_000 })
        } finally {
            await orgPage.close()
        }
    })
})
