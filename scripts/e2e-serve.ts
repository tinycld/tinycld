// E2E launcher that serves the web app the way PRODUCTION does — as a
// pre-built static bundle off a single PocketBase HTTP listener — instead of
// running the Metro dev server behind a proxy (what scripts/dev.ts does).
//
// WHY: in dev/e2e, dev.ts spawns Expo (`expo start`) and proxies non-API
// paths to it. Metro compiles JS chunks LAZILY on first request, so on a
// constrained CI runner the cold compile of a package's screen/sidebar chunk
// stalls or wedges mid-request under concurrent Playwright-worker load — the
// root cause of the flaky/hung e2e CI. Production never runs Metro: the
// Dockerfile runs `expo export` once, stages the dist/ as a release, and the
// Go server serves it statically (coreserver.registerStaticServe, gated on
// --releasesDir). This script replicates that recipe so e2e matches prod and
// the lazy-compile failure class disappears entirely — the bundle is fully
// built on disk before the webServer's /api/health gate goes green.
//
// FLOW (reset → export → promote → serve):
//   1. Reset: remove the data dir and create the superuser. Both write straight
//      to the dir (`superuser upsert --dir`), so NO server runs and NO port is
//      opened here; PocketBase migrates the schema itself when it starts. The
//      fixture seed needs an API and therefore a running server, so it happens
//      after this one is up — see tests/playwright-global-setup.ts.
//   2. Export: `expo export --platform web` → dist/ (one deterministic compile).
//   3. Promote: stage dist/ into the prod-shaped releases layout the Go server
//      reads — a TypeScript port of entrypoint.sh's promote_release().
//   4. Serve: launch the PB binary with --releasesDir on the user-facing port.
//      No Expo, no proxy: ONE listener serves /api/* AND the SPA, exactly like
//      prod (web resolves PB at window.location.origin, so single-origin works).
//
// Local fast iteration: pass --skip-export (or TINYCLD_E2E_SKIP_EXPORT=1) to
// reuse an existing dist/ and skip the multi-minute export. CI never sets it.
//
// Used by playwright.config.ts as the `webServer` command (via the `e2e:serve`
// package.json script). dev.ts is intentionally left untouched — dev keeps
// Metro + HMR; only e2e switches to static serving.

import { type ChildProcess, spawn, spawnSync } from 'node:child_process'
import * as fs from 'node:fs'
import * as net from 'node:net'
import * as path from 'node:path'
import { exportWeb } from './export-web'
import { promoteRelease } from './promote-release'

const ROOT = path.resolve(import.meta.dirname, '..')
const PB_BINARY = path.join(ROOT, 'server', 'app')

function log(msg: string) {
    process.stdout.write(`[e2e-serve] ${msg}\n`)
}

// Pull a `--name value` flag out of process.argv. Returns null when absent.
function flagValue(name: string): string | null {
    const i = process.argv.indexOf(name)
    if (i === -1) return null
    const v = process.argv[i + 1]
    if (v === undefined || v.startsWith('-')) {
        throw new Error(`e2e-serve: flag ${name} requires a value`)
    }
    return v
}

function resolvePort(): number {
    const raw = flagValue('--port') ?? process.env.E2E_PORT ?? '7200'
    const n = Number.parseInt(raw, 10)
    if (!Number.isFinite(n) || n <= 0 || n > 65_533) {
        throw new Error(`e2e-serve: --port must be a port number (got ${raw})`)
    }
    return n
}

// Resolve a dir flag to an absolute path under ROOT.
function resolveDir(flag: string, fallback: string): string {
    const raw = flagValue(flag) ?? fallback
    return path.isAbsolute(raw) ? raw : path.join(ROOT, raw)
}

const skipExport =
    process.argv.includes('--skip-export') || process.env.TINYCLD_E2E_SKIP_EXPORT === '1'

// A second instance started CONCURRENTLY with the one that owns the export
// (Playwright starts every webServer entry at once) must not run `expo export`
// itself: the export CLEANS dist/ before writing it, so two of them racing
// leaves the loser staring at a half-deleted tree.
//
// The value is the producer's releases dir. Waiting on its `current` symlink
// (written last by promoteRelease) means the producer is done — and copying
// THAT rather than dist/ is what makes this safe: a promoted release is
// immutable and complete, while dist/ is scratch space the next export wipes.
const mirrorReleasesFrom = flagValue('--mirror-releases-from')

async function waitForFile(target: string, timeoutMs: number): Promise<void> {
    const deadline = Date.now() + timeoutMs
    while (Date.now() < deadline) {
        if (fs.existsSync(target)) return
        await new Promise(r => setTimeout(r, 500))
    }
    throw new Error(`e2e-serve: timed out after ${timeoutMs}ms waiting for ${target}`)
}

async function tryConnect(port: number, host = '127.0.0.1'): Promise<boolean> {
    return new Promise(resolve => {
        const sock = net.connect({ port, host })
        const done = (ok: boolean) => {
            sock.removeAllListeners()
            sock.destroy()
            resolve(ok)
        }
        sock.once('connect', () => done(true))
        sock.once('error', () => done(false))
    })
}

async function waitForUpstream(port: number, label: string, timeoutMs: number): Promise<void> {
    const start = Date.now()
    while (Date.now() - start < timeoutMs) {
        if (await tryConnect(port)) return
        await new Promise(r => setTimeout(r, 200))
    }
    throw new Error(
        `e2e-serve: ${label} on :${port} did not accept connections within ${timeoutMs}ms`
    )
}

// Phase 1 — start from an empty data dir. PocketBase creates and migrates it
// on startup (--migrationsDir + automigrate), so there is nothing to prepare
// and no second server to run: this owns the dir, so it just removes it.
//
// Fixtures are NOT written here. They go through the PocketBase API, which
// needs a running server — so they are seeded by Playwright's globalSetup
// against the one server this script starts. See tests/playwright-global-setup.ts.
function resetDataDir(dataDir: string): void {
    // Build the server first: everything below runs it. This used to happen
    // inside reset-dev-db.ts, which the seed step called — that step is gone
    // (fixtures are seeded by globalSetup against the running server), so the
    // build has to live here or nothing produces the binary on a clean
    // checkout. A developer with one already sees a fast no-op rebuild.
    log('phase 1/3: building the server')
    const built = spawnSync('go', ['build', '-o', 'app', '.'], {
        cwd: path.join(ROOT, 'server'),
        stdio: 'inherit',
    })
    if (built.status !== 0) throw new Error('e2e-serve: failed to build the server binary')

    log('phase 1/3: clearing the test data dir')
    fs.rmSync(dataDir, { recursive: true, force: true })

    // The fixture seed authenticates as a superuser, so one has to exist before
    // the server comes up. `superuser upsert` writes straight to the data dir —
    // no server, no port — and creates the DB if it is missing.
    const email = process.env.ADMIN_USER_LOGIN || 'admin@tinycld.org'
    const password = process.env.ADMIN_USER_PW || 'AdminPass1234!'
    log(`phase 1/3: creating superuser ${email}`)
    const result = spawnSync(
        PB_BINARY,
        ['superuser', 'upsert', email, password, '--dir', dataDir],
        {
            stdio: 'inherit',
        }
    )
    if (result.status !== 0) throw new Error('e2e-serve: failed to create the superuser')
}

// Phase 2 — build the static web bundle (delegates to scripts/export-web.ts so
// the export flags/env live in ONE place, shared with the CI action that
// pre-builds the bundle). --skip-export reuses an existing dist/ — set in CI
// (the action already built it) and for local fast iteration.
function buildBundle(releaseId: string): void {
    const indexHtml = path.join(ROOT, 'dist', 'index.html')

    if (skipExport) {
        log('phase 2/3: --skip-export set, reusing existing dist/')
        if (!fs.existsSync(indexHtml)) {
            throw new Error(
                'e2e-serve: --skip-export but dist/index.html is missing; run once without --skip-export first (or let the CI action build it)'
            )
        }
        return
    }
    log(`phase 2/3: expo export (releaseId=${releaseId})`)
    exportWeb(releaseId)
}

// Phase 3a — promote dist/ into the releases layout the Go server reads. The
// layout itself lives in scripts/promote-release.ts so BOTH e2e launchers use
// one definition of it — see that file for why a second copy is dangerous
// rather than merely redundant.
function promote(distDir: string, releasesDir: string, releaseId: string): void {
    log('phase 3/3: promoting dist/ → releases')
    promoteRelease(distDir, releasesDir, releaseId, log)
}

// Phase 3b — launch the serving PB on the user-facing port. The flag set
// mirrors production's entrypoint serve (--dir/--releasesDir/--migrationsDir)
// plus --dev (mail → LogSender for the email-log e2e helpers) and --typesDir
// (OnServe runs GenerateSchemas). --releasesDir is THE switch that turns on
// static serving; without it the server would serve only --publicDir and a
// proxy-to-Metro would be required. IMAP_ADDR=:1193 matches dev.ts so the
// IMAP e2e suite finds the listener.
function serve(opts: { port: number; dataDir: string; releasesDir: string }): ChildProcess {
    log(`serving: PB on http://localhost:${opts.port} (static, --releasesDir)`)
    const args = [
        '--dev',
        '--http',
        `127.0.0.1:${opts.port}`,
        '--dir',
        opts.dataDir,
        '--releasesDir',
        opts.releasesDir,
        '--migrationsDir',
        path.join(ROOT, 'server', 'pb_migrations'),
        '--publicDir',
        path.join(ROOT, 'public'),
        '--fallbackFile',
        'app.html',
        '--typesDir',
        path.join(ROOT, 'core', 'types'),
        'serve',
    ]
    // Mail listeners bind FIXED ports, so a second instance on the same box
    // would collide with the first (PB logs "address already in use" and
    // carries on, but the noise is misleading). Offset them alongside the
    // HTTP port; the IMAP e2e suite talks to the primary, whose :1193 matches
    // dev.ts and imap-helpers.ts.
    const mailEnv = mirrorReleasesFrom
        ? {
              IMAP_ADDR: ':1293',
              IMAPS_ADDR: ':2093',
              SMTP_ADDR: ':1687',
              SMTPS_ADDR: ':1466',
          }
        : { IMAP_ADDR: ':1193' }
    return spawn(PB_BINARY, args, {
        cwd: ROOT,
        stdio: 'inherit',
        env: { ...process.env, ...mailEnv },
    })
}

async function main() {
    const port = resolvePort()
    const dataDir = resolveDir('--data-dir', path.join('server', 'pb_test_data'))
    const releasesDir = resolveDir('--releases-dir', path.join('server', 'pb_test_releases'))
    const distDir = path.join(ROOT, 'dist')
    // A monotonic-ish id. Date.now() is fine here (this is a launcher script,
    // not a workflow), and tests never pin the value.
    const releaseId = `e2e-${Date.now()}`

    resetDataDir(dataDir)

    if (mirrorReleasesFrom) {
        // Phases 2+3 collapse into a copy: the producer already exported and
        // promoted, and its releases dir is immutable once `current` exists.
        const sourceDir = path.resolve(ROOT, mirrorReleasesFrom)
        const marker = path.join(sourceDir, 'current')
        log(`phase 2/3: waiting for the exporting instance (${marker})`)
        await waitForFile(marker, 600_000)
        log(`phase 3/3: mirroring ${sourceDir} → ${releasesDir}`)
        fs.rmSync(releasesDir, { recursive: true, force: true })
        fs.cpSync(sourceDir, releasesDir, { recursive: true, verbatimSymlinks: true })
    } else {
        buildBundle(releaseId)
        promote(distDir, releasesDir, releaseId)
    }

    const pb = serve({ port, dataDir, releasesDir })

    // Forward termination to the child and exit. Playwright kills the
    // webServer process group on teardown; relay it so PB shuts down cleanly
    // (checkpoints the WAL) rather than being orphaned.
    let shuttingDown = false
    const shutdown = (signal: NodeJS.Signals) => {
        if (shuttingDown) return
        shuttingDown = true
        log(`shutting down (${signal})`)
        pb.kill('SIGTERM')
        ;(setTimeout(() => process.exit(0), 1000) as unknown as NodeJS.Timeout).unref()
    }
    process.on('SIGINT', shutdown)
    process.on('SIGTERM', shutdown)

    // If PB dies on its own, surface it as a failure so Playwright reports the
    // webServer as down instead of hanging on the health gate.
    pb.on('exit', code => {
        if (shuttingDown) return
        process.stderr.write(`[e2e-serve] PB exited unexpectedly (${code})\n`)
        process.exit(code ?? 1)
    })

    // Readiness log (Playwright's own /api/health gate is the real signal).
    await waitForUpstream(port, 'pb', 60_000)
    log(`ready on http://localhost:${port}`)
}

void main().catch(err => {
    process.stderr.write(`e2e-serve: ${err instanceof Error ? err.stack : String(err)}\n`)
    process.exit(1)
})
