import * as os from 'node:os'
import * as path from 'node:path'
import { defineConfig } from '@playwright/test'

// A Playwright run with no server and no browser, for e2e-summary.test.ts: it
// proves that the summary reporter loads and counts a real run.
export default defineConfig({
    testDir: import.meta.dirname,
    testMatch: '*.pw.ts',
    reporter: 'dot',
    workers: 1,
    outputDir: path.join(os.tmpdir(), 'tinycld-e2e-summary-fixture'),
})
