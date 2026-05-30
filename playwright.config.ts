import path from 'node:path'
import { defineConfig, devices } from '@playwright/test'

// App shell owns the canonical Playwright config: the webServer (the real dev
// stack via `npm run expo:test`, which resets the test DB then boots PB+Expo
// behind a proxy on PORT) and the browser project. Package-scoped configs
// inherit this and override `testDir` to point at one package's tests/e2e
// through the node_modules symlink, so @playwright/test resolves against the
// app shell's install.
const PORT = Number(process.env.E2E_PORT ?? 7200)
// Outbound mail is gated to LogSender during e2e (the PB --dev flag flips
// delivery off). Pointing TINYCLD_EMAIL_LOG at the same tmp/emails.log file
// the globalSetup truncates lets tests assert on emails without scraping
// stdout. The Go LogSender appends one JSONL record per send to this path.
// app/scripts/dev.ts spawns PB inheriting process.env, so PB and the test
// process both see the same path resolved from this file's directory.
const EMAIL_LOG_PATH = path.join(import.meta.dirname, 'tmp', 'emails.log')

export default defineConfig({
    testMatch: '**/*.spec.ts',
    // Per-failure artifacts: trace, screenshot, video. retain-on-failure
    // skips writing for passing tests (saves disk + upload size on green
    // runs) while keeping a complete record for any failure. Traces let
    // us replay the run in Playwright's trace viewer; screenshots +
    // videos surface the final visual state without needing the trace
    // tooling. CI uploads these as artifacts via the workflow's
    // upload-artifact step.
    use: {
        ...devices['Desktop Chrome'],
        baseURL: `http://localhost:${PORT}`,
        trace: 'retain-on-failure',
        screenshot: 'only-on-failure',
        video: 'retain-on-failure',
    },
    webServer: {
        command: 'npm run expo:test',
        cwd: import.meta.dirname,
        // expo:test resets the DB then boots PB+Expo behind the proxy on PORT;
        // /api/health proxies to PB, so wait on that (not the Expo bundle).
        url: `http://localhost:${PORT}/api/health`,
        reuseExistingServer: !process.env.CI,
        timeout: 240_000,
        // Inherits the launching shell (process.env is the default), but
        // we explicitly export the email log path so PB (spawned by dev.ts)
        // writes JSONL records there for tests to assert on. Filter out
        // undefined values from process.env to satisfy Playwright's strict
        // `{[key: string]: string}` env type.
        env: Object.fromEntries(
            Object.entries(process.env)
                .filter(([, v]) => v !== undefined)
                .map(([k, v]) => [k, v as string])
                .concat([
                    ['TINYCLD_EMAIL_LOG', EMAIL_LOG_PATH],
                    // Shrink the @tinycld/text edit-event debounce window
                    // from 60s to 1s for e2e so the Activity tab populates
                    // within a single test budget. Production leaves this
                    // unset and runs at the default. Read by the Go side
                    // in text/server/edit_event_buffer.go:configureWindowFromEnv.
                    ['TINYCLD_EDIT_EVENT_WINDOW_MS', '1000'],
                ])
        ),
    },
    // Absolute path: per-package configs spread this config, and Playwright
    // resolves a relative globalSetup against the INHERITING config's dir —
    // so a relative './tests/...' would break for contacts/etc. Pin it here.
    globalSetup: path.join(import.meta.dirname, 'tests', 'playwright-global-setup.ts'),
})
