/**
 * Playwright Global Setup
 *
 * Runs AFTER the webServer entries are up and BEFORE any test, which is what
 * makes it the right home for the fixture seed: writing fixtures goes through
 * the PocketBase API, so it needs a running server — and this is the first
 * point at which one exists.
 *
 * Everything that does NOT need a server happens earlier, in
 * scripts/e2e-serve.ts: clearing the data dir and creating the superuser both
 * write straight to the data dir (`superuser upsert --dir`), and PocketBase
 * migrates the schema itself on startup. So nothing here starts a second
 * server, and no port is opened for seeding.
 *
 * Also truncates tmp/emails.log so each run sees a clean mail log.
 */

import { spawn } from 'node:child_process'
import * as fs from 'node:fs'
import * as path from 'node:path'

const PROJECT_ROOT = path.resolve(import.meta.dirname, '..')
export const TMP_DIR = path.join(PROJECT_ROOT, 'tmp')
export const EMAIL_LOG_PATH = path.join(TMP_DIR, 'emails.log')

const PORT = Number(process.env.E2E_PORT ?? 7200)
const SECOND_PORT = Number(process.env.E2E_PORT_2 ?? 7201)

function seedFixtures(port: number): Promise<void> {
    const url = `http://127.0.0.1:${port}`
    process.stdout.write(`[global-setup] seeding fixtures → ${url}\n`)
    return new Promise((resolve, reject) => {
        const child = spawn(
            'npx',
            [
                'tsx',
                'scripts/seed-db.ts',
                '--url',
                url,
                '--admin-email',
                process.env.ADMIN_USER_LOGIN || 'admin@tinycld.org',
                '--admin-pw',
                process.env.ADMIN_USER_PW || 'AdminPass1234!',
            ],
            {
                cwd: PROJECT_ROOT,
                stdio: 'inherit',
                // Sibling packages link in via symlinks; without
                // --preserve-symlinks Node resolves the seed to its realpath
                // inside the sibling repo and cannot find peer deps in the app
                // shell's node_modules.
                env: { ...process.env, NODE_OPTIONS: '--preserve-symlinks' },
            }
        )
        child.on('error', reject)
        child.on('close', code =>
            code === 0
                ? resolve()
                : reject(new Error(`global-setup: seeding ${url} failed with code ${code}`))
        )
    })
}

export default async function globalSetup() {
    fs.mkdirSync(TMP_DIR, { recursive: true })
    fs.writeFileSync(EMAIL_LOG_PATH, '')

    // Both deployments the config starts. The saved-server switcher spec signs
    // into the second one, so it needs the same fixture user as the first.
    //
    // In parallel: they are separate servers over separate data dirs, and the
    // seed is minutes of API round-trips, so serializing would double the wait
    // before the first test runs.
    await Promise.all([seedFixtures(PORT), seedFixtures(SECOND_PORT)])
}
