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

async function loginAsSuperuser(page: Page) {
    await page.goto('/setup')
    await expect(page.getByText('Superuser Login')).toBeVisible()
    await page.getByRole('textbox', { name: 'Email', exact: true }).fill(SUPERUSER_EMAIL)
    await page.getByRole('textbox', { name: 'Password', exact: true }).fill(SUPERUSER_PASSWORD)
    await page.getByRole('button', { name: 'Sign in' }).click()
    await expect(page.getByText('Organizations', { exact: true })).toBeVisible()
}

// Asserts the install modal advances through an expected stage by its
// VISIBLE message text (step.message, not the short step name). If a
// FAILED line appears first, throws with the failing message so the
// output names the stage that broke. `label` is the human stage name
// for the error string.
async function expectStage(page: Page, label: string, messageSubstring: string) {
    const failed = page.getByText(/^FAILED: /)
    const target = page.getByText(messageSubstring, { exact: false }).first()
    await Promise.race([
        target.waitFor({ state: 'visible', timeout: 300_000 }),
        failed
            .first()
            .waitFor({ state: 'visible', timeout: 300_000 })
            .then(async () => {
                const msg = (await failed.first().textContent()) ?? ''
                throw new Error(`install failed before "${label}": ${msg}`)
            }),
    ])
}

test.describe.configure({ mode: 'serial' })

test.describe('todo install', () => {
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
        test.setTimeout(900_000) // go build + expo export are minutes-long

        await loginAsSuperuser(page)

        // Login lands on the Packages tab. Open the install form, then submit.
        await page.getByRole('button', { name: 'Install', exact: true }).click()
        await page
            .getByRole('textbox', { name: 'npm Package Name', exact: true })
            .fill(TODO_SPEC)
        // The form's submit button shares the 'Install' label; click the one
        // inside the install form (the second match, after the toggle).
        await page.getByRole('button', { name: 'Install', exact: true }).last().click()

        // The InstallProgressModal renders once the job starts. Walk the
        // stages in order — each assertion names where it got stuck.
        await expectStage(page, 'Downloading package', 'Running npm pack')
        await expectStage(page, 'Manifest parsed', 'Package: Todo (todo)')
        await expectStage(page, 'Installing dependencies', 'Running pnpm install')
        await expectStage(page, 'Generating wiring', 'Running package generation script')
        await expectStage(page, 'Building server', 'Compiling new server binary')
        await expectStage(page, 'Building web app', 'Running expo export')
        await expectStage(page, 'Requesting restart', 'Signaling server restart')

        // Confirm the modal didn't end in a failed state.
        await expect(page.getByText('Installation Failed')).not.toBeVisible()
    })
})
