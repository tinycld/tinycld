import path from 'node:path'
import { defineConfig, devices } from '@playwright/test'

// App shell owns the canonical Playwright config: the webServer (the static
// serve stack via `pnpm run e2e:serve`, which resets the test DB, runs
// `expo export`, promotes the bundle into a prod-shaped releases dir, then
// serves it off a single PocketBase listener on PORT — no Metro, no proxy,
// exactly like production) and the browser project. Package-scoped configs
// inherit this and override `testDir` to point at one package's tests/e2e
// through the node_modules symlink, so @playwright/test resolves against the
// app shell's install.
const PORT = Number(process.env.E2E_PORT ?? 7200)
// Outbound mail is gated to LogSender during e2e (the PB --dev flag flips
// delivery off). Pointing TINYCLD_EMAIL_LOG at the same tmp/emails.log file
// the globalSetup truncates lets tests assert on emails without scraping
// stdout. The Go LogSender appends one JSONL record per send to this path.
// scripts/e2e-serve.ts spawns PB inheriting process.env, so PB and the test
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
    // No retries. A test must pass on its first attempt; a flake is a bug in the
    // test (or the code) to fix at the source, not to paper over by re-running
    // until it's green. Trace/video are retain-on-failure, so the failing
    // attempt is always captured for diagnosis. Inherited by every package's
    // config.
    retries: 0,
    // Two workers, overriding Playwright's default of 50% OF CPUS.
    //
    // That default is not a CI special case, whatever the folklore says — see
    // resolveWorkers in playwright's common/config.js, which reads
    // `os.cpus().length` and takes the percentage. A standard 2-core GitHub
    // runner therefore resolves to ONE worker, and the boards e2e log said so
    // exactly: "Running 140 tests using 1 worker", 16.7 minutes for a suite
    // that takes 4.7 locally in parallel.
    //
    // Measured on boards before generalizing here: two workers on the same
    // 2-core runner ran the same 140 tests in 11.9 minutes (139 passed, 1
    // skipped — the same split as the serial run), a ~29% saving with no new
    // failures.
    //
    // WHY A FIXED 2 RATHER THAN A PERCENTAGE. The runner has 2 cores, so any
    // percentage at or below 100% resolves to 1 there, while a developer
    // machine with 14 cores would take 7 and pay seven cold Metro compiles
    // (see the reporter note above — each worker pays one on its first test).
    // A fixed 2 is the number that helps CI, which is where the wall clock
    // actually hurts, and it caps that startup cost on a laptop too.
    //
    // A package that needs different parallelism overrides this in its own
    // config; a suite whose specs are not independent should be FIXED rather
    // than serialized, for the reason `retries: 0` above gives.
    workers: 2,
    // Scoped to tests/e2e/ specifically. The tests/install/ tree has
    // its own playwright.config.ts and is invoked separately by the
    // docker smoke-test workflow — leaving testDir at the playwright
    // default (this file's dir) would pull both into the same run, and
    // the install spec's EXPECTED_BUNDLED assertions would trip when
    // run against the regular e2e:serve webServer.
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
        command: 'pnpm run e2e:serve',
        cwd: import.meta.dirname,
        // e2e:serve resets the DB, exports + promotes the web bundle, then
        // serves it (and /api/*) off one PocketBase listener on PORT. The
        // health gate only goes green AFTER the bundle is fully built and
        // promoted on disk, so tests never race a cold bundle.
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
    // outputDir — where Playwright writes its per-test scratch
    // (.playwright-artifacts-N/, live video/screenshot buffers) AND the
    // retain-on-failure artifacts.
    //
    // LOCAL (macOS): write OUTSIDE the watched workspace tree. Playwright's
    // transient artifact churn during every run (even passing tests buffer
    // video/screenshots live, then discard) floods macOS FSEvents; the kernel
    // drops events (UserDropped), forcing Watchman to recrawl the ~100k-file
    // monorepo — which is what makes `expo query` stall for 60s. Relocating the
    // scratch off the watched filesystem removes the flood at its source.
    // ignore_dirs in .watchmanconfig can't fix this: it stops Watchman INDEXING
    // those paths, not the kernel FSEvents stream that overflows.
    //
    // CI: leave Playwright's default (<inheriting-config-dir>/test-results) so
    // the per-package workflows' upload-artifact steps (which read
    // ws/<pkg>/test-results/ + playwright-report/) keep working with NO changes.
    // CI runs on Linux (inotify, and Watchman typically absent on the runners),
    // so it does not hit the macOS FSEvents recrawl this guards against.
    //
    // Per-RUN subdirectory: Playwright wipes outputDir at the start of every
    // run, so a fixed path discards the previous run's retain-on-failure traces
    // — losing exactly the artifacts needed to debug an intermittent failure.
    // Bucketing each run under its own dir (PW_RUN_ID, else a timestamp) keeps
    // prior runs' traces intact for post-mortem. Still outside the watched tree,
    // so the FSEvents rationale above holds. Cleared by hand or on reboot (/tmp).
    ...(process.env.CI
        ? {}
        : {
              outputDir: path.join(
                  '/tmp',
                  'tinycld-pw-artifacts',
                  process.env.PW_RUN_ID ?? `run-${new Date().toISOString().replace(/[:.]/g, '-')}`
              ),
          }),
})
