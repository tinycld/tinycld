import path from 'node:path'
import { defineConfig, devices } from '@playwright/test'

// App shell owns the canonical Playwright config: the webServer (the real dev
// stack via `pnpm run expo:test`, which resets the test DB then boots PB+Expo
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
    // Override Playwright's CI default (the `dot` reporter, which prints a bare
    // `·` per completed test — no name, so a run looks frozen during the cold
    // Metro compile each worker pays on its first test, then dumps every dot at
    // once). In non-TTY CI, `list` prints a NAMED line as each test COMPLETES
    // (`✓ 3 mail › opens thread (4.1s)`) — it doesn't stream a per-test "started"
    // line (that's TTY-only), but the accruing named lines + durations show
    // which tests have finished and that the run is progressing. Inherited by
    // every package's playwright.config.ts.
    reporter: 'list',
    // Retry failed specs on CI only. The 2-core ubuntu runner pays a cold
    // Metro lazy-chunk compile on each worker's first navigation into a
    // package route; under parallel load that compile occasionally blows
    // past the per-test budget (e.g. a beforeEach waiting for the package
    // sidebar to mount, or a post-reload route re-mount), failing an
    // otherwise-correct test. Retries re-run only the failed spec — with a
    // now-warm chunk cache — so a one-off cold-compile timeout self-heals,
    // while a genuine regression still fails all attempts. Trace/video are
    // retain-on-failure, so the final failing attempt is always captured.
    // 0 retries locally keeps `reuseExistingServer` runs fast and surfaces
    // real failures immediately. Inherited by every package's config.
    retries: process.env.CI ? 2 : 0,
    // Scoped to tests/e2e/ specifically. The tests/install/ tree has
    // its own playwright.config.ts and is invoked separately by the
    // docker smoke-test workflow — leaving testDir at the playwright
    // default (this file's dir) would pull both into the same run, and
    // the install spec's EXPECTED_BUNDLED assertions would trip when
    // run against the regular expo:test webServer.
    testDir: path.join(import.meta.dirname, 'tests', 'e2e'),
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
        command: 'pnpm run expo:test',
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
