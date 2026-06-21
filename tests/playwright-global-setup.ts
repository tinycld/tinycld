/**
 * Playwright Global Setup
 *
 * Truncates tmp/emails.log so each test run sees a clean mail log.
 *
 * That's all this needs to do now. Under the static-serve webServer
 * (scripts/e2e-serve.ts) the web bundle is built by `expo export` and fully
 * promoted to disk BEFORE the webServer's /api/health gate goes green, so
 * there are no lazy Metro chunks to pre-warm — the old warmWebBundle() /
 * warmPackageChunks() steps (and the TINYCLD_WARM_PACKAGES knob) were
 * Metro-specific and are gone. The DB reset+seed is likewise not done here;
 * it's the first phase of the webServer command (e2e-serve.ts → reset-dev-db.ts).
 */

import * as fs from 'node:fs'
import * as path from 'node:path'

const PROJECT_ROOT = path.resolve(import.meta.dirname, '..')
export const TMP_DIR = path.join(PROJECT_ROOT, 'tmp')
export const EMAIL_LOG_PATH = path.join(TMP_DIR, 'emails.log')

export default async function globalSetup() {
    fs.mkdirSync(TMP_DIR, { recursive: true })
    fs.writeFileSync(EMAIL_LOG_PATH, '')
}
