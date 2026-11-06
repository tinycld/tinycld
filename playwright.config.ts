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
// A SECOND, independent deployment on its own port. The saved-server switcher
// is the one feature whose interesting states need two servers — a second row
// in the list, the active one identified among several, and a switch that
// really lands elsewhere — and serverKeyFor normalizes an address to
// scheme+host+port, so two localhost ports are two distinct servers exactly as
// two self-hosted boxes would be. See tests/e2e/server-switcher.spec.ts.
const SECOND_PORT = Number(process.env.E2E_PORT_2 ?? 7201)
// Outbound mail is gated to LogSender during e2e (the PB --dev flag flips
// delivery off). Pointing TINYCLD_EMAIL_LOG at the same tmp/emails.log file
// the globalSetup truncates lets tests assert on emails without scraping
// stdout. The Go LogSender appends one JSONL record per send to this path.
// scripts/e2e-serve.ts spawns PB inheriting process.env, so PB and the test
// process both see the same path resolved from this file's directory.
const EMAIL_LOG_PATH = path.join(import.meta.dirname, 'tmp', 'emails.log')

// The env EVERY deployment this config starts must carry. It is shared rather
// than repeated because a value present on one server and missing on the other
// is invisible until a test happens to land on the wrong one — which is how the
// @tinycld/text activity spec failed: the second server ran the edit-event
// debounce at its 60s production default and the test timed out at 30s.
//
// Filter out undefined values from process.env to satisfy Playwright's strict
// `{[key: string]: string}` env type.
const SERVER_ENV: Record<string, string> = Object.fromEntries(
    Object.entries(process.env)
        .filter(([, v]) => v !== undefined)
        .map(([k, v]) => [k, v as string])
        .concat([
            // PB (spawned by dev.ts) writes JSONL records here for the tests
            // that assert on outgoing mail.
            ['TINYCLD_EMAIL_LOG', EMAIL_LOG_PATH],
            // Shrink the @tinycld/text edit-event debounce from 60s to 1s so
            // the Activity tab populates within a single test budget.
            // Production leaves this unset. Read by the Go side in
            // text/server/edit_event_buffer.go:configureWindowFromEnv.
            ['TINYCLD_EDIT_EVENT_WINDOW_MS', '1000'],
            // Shrink the automatic-version idle time from 10 minutes to 2s so
            // a spec sees the version the server makes. Read by core
            // realtime's idleFromEnv (edit_checkpoint.go).
            ['TINYCLD_VERSION_IDLE_MS', '2000'],
            // The e2e server is a real build that can rebuild itself; without this a
            // run inside the update window would try to update its own packages.
            ['TINYCLD_AUTOUPGRADE_DISABLED', '1'],
        ])
)

export default defineConfig({
    // `dot` off CI: one character per test, then each failure in full with its
    // error and location. A local run is read for its failures, and a line per
    // passing test is noise there (most of all for a coding agent, which pays
    // for every line it reads). `--reporter=list` on the command line brings
    // the names back.
    //
    // `list` on CI, overriding Playwright's CI default (`dot`, whose bare `·`
    // per completed test names nothing, so a long-running spec makes the run
    // look frozen). In non-TTY CI, `list` prints a NAMED line as each test
    // COMPLETES (`✓ 3 mail › opens thread (4.1s)`) — it doesn't stream a
    // per-test "started" line (that's TTY-only), but the accruing named lines
    // + durations show which tests have finished and that the run is
    // progressing. Inherited by every package's playwright.config.ts.
    reporter: process.env.CI ? 'list' : 'dot',
    // No retries. A test must pass on its first attempt; a flake is a bug in the
    // test (or the code) to fix at the source, not to paper over by re-running
    // until it's green. Trace/video are retain-on-failure, so the failing
    // attempt is always captured for diagnosis. Inherited by every package's
    // config.
    retries: 0,
    // Two workers in CI; Playwright's default (50% of CPUs) locally.
    //
    // The default is not a CI special case, whatever the folklore says — see
    // resolveWorkers in playwright's common/config.js, which reads
    // `os.cpus().length` and takes the percentage. A standard 2-core GitHub
    // runner therefore resolves to ONE worker, and the boards e2e log said so
    // exactly: "Running 140 tests using 1 worker", 16.7 minutes for a suite
    // that takes 4.7 locally in parallel.
    //
    // Measured on boards before generalizing here: two workers on the same
    // 2-core runner ran the same 140 tests in 11.9 minutes (139 passed, 1
    // skipped — the same split as the serial run), a ~29% saving with no new
    // failures. Any percentage at or below 100% still resolves to 1 on that
    // runner, so CI needs the fixed number.
    //
    // Locally the fixed 2 only held a developer machine back: a 14-core
    // machine ran the same two browsers as the runner against a PocketBase
    // that sat mostly idle. Every spec sets up its own data, which is what
    // already lets several workers share one server, so the default applies
    // off CI.
    //
    // A package that needs different parallelism overrides this in its own
    // config; a suite whose specs are not independent should be FIXED rather
    // than serialized, for the reason `retries: 0` above gives.
    workers: process.env.CI ? 2 : undefined,
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
        // Overlays honor the OS reduced-motion setting, so emulating it
        // takes their slides and fades out of the run: a press inside a
        // sliding sheet otherwise waits for the target to be stable.
        reducedMotion: 'reduce',
    },
    webServer: [
        {
            command: 'pnpm run e2e:serve',
            cwd: import.meta.dirname,
            // e2e:serve clears the data dir, exports + promotes the web bundle,
            // then serves it (and /api/*) off one PocketBase listener on PORT.
            // The health gate only goes green AFTER the bundle is fully built
            // and promoted on disk, so tests never race a cold bundle. Fixtures
            // are seeded by globalSetup once this is up.
            url: `http://localhost:${PORT}/api/health`,
            reuseExistingServer: !process.env.CI,
            timeout: 240_000,
            // Off CI the script keeps its children's output in a log file and
            // prints only its own phase lines and that file's path, so show
            // them. On CI the children write inline, and their stdout (the
            // export's file list) stays out of the job log as before.
            stdout: process.env.CI ? 'ignore' : 'pipe',
            env: SERVER_ENV,
        },
        // The second deployment: its own data dir, its own releases dir, its
        // own PocketBase. Both origins serve the SAME app — only the origin
        // differs — which is exactly what the switcher spec needs.
        //
        // Playwright starts every webServer entry CONCURRENTLY, so this one
        // must not run `expo export` itself — the export CLEANS dist/ before
        // writing it, so two racing exports leave the loser with a half-deleted
        // tree. --mirror-releases-from waits for the first instance's `current`
        // symlink (written last by promoteRelease) and then COPIES its promoted
        // release, which is immutable and complete, rather than touching dist/.
        {
            command: 'pnpm run e2e:serve:second',
            cwd: import.meta.dirname,
            url: `http://localhost:${SECOND_PORT}/api/health`,
            reuseExistingServer: !process.env.CI,
            // The mirror waits on the first instance, so this one is idle for
            // however long a cold export takes before it does any work of its
            // own. Give it the export's budget on top of its own.
            timeout: 480_000,
            // Playwright swallows webServer stdout by default; without this a
            // startup failure here surfaces only as ERR_CONNECTION_REFUSED in
            // every test, with no reason attached.
            stdout: 'pipe',
            env: SERVER_ENV,
        },
    ],
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
