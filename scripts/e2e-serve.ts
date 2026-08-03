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
// FLOW (seed → export → promote → serve):
//   1. Seed: reuse scripts/reset-dev-db.ts (builds PB, resets + seeds the test
//      DB on a throwaway port :7299, exits after a WAL checkpoint).
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

import { type ChildProcess, spawn } from 'node:child_process'
import * as fs from 'node:fs'
import * as net from 'node:net'
import * as path from 'node:path'
import { exportWeb } from './export-web'
import { promoteRelease } from './promote-release'

const ROOT = path.resolve(import.meta.dirname, '..')
const PB_BINARY = path.join(ROOT, 'server', 'app')

// The throwaway port reset-dev-db.ts seeds on (it exits before we serve, so
// this never collides with the user-facing port). Matches the value the old
// expo:test chain used.
const SEED_PORT = 7299

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

// Run a child to completion, inheriting stdio, rejecting on non-zero exit.
function run(cmd: string, args: string[], env?: NodeJS.ProcessEnv): Promise<void> {
    return new Promise((resolve, reject) => {
        const child = spawn(cmd, args, {
            cwd: ROOT,
            stdio: 'inherit',
            env: env ?? process.env,
        })
        child.on('exit', code =>
            code === 0 ? resolve() : reject(new Error(`${cmd} ${args.join(' ')} exited ${code}`))
        )
        child.on('error', reject)
    })
}

// Phase 1 — seed the test DB. reset-dev-db.ts builds PB, deletes + recreates
// the data dir, runs migrations, seeds, then SIGTERMs its own PB and waits for
// the SQLite WAL to checkpoint before exiting (reset-dev-db.ts), so the
// serving PB below opens a fully-flushed DB. --browse-url points the seed's
// login summary at the user-facing port (cosmetic).
async function seed(dataDir: string, port: number): Promise<void> {
    log('phase 1/3: seeding test DB (reset-dev-db.ts)')
    await run('npx', [
        'tsx',
        'scripts/reset-dev-db.ts',
        '--url',
        `http://127.0.0.1:${SEED_PORT}`,
        '--browse-url',
        `http://localhost:${port}`,
        '--data-dir',
        path.relative(ROOT, dataDir),
    ])
}

// Phase 2 — build the static web bundle (delegates to scripts/export-web.ts so
// the export flags/env live in ONE place, shared with the CI action that
// pre-builds the bundle). --skip-export reuses an existing dist/ — set in CI
// (the action already built it) and for local fast iteration.
function buildBundle(releaseId: string): void {
    if (skipExport) {
        log('phase 2/3: --skip-export set, reusing existing dist/')
        if (!fs.existsSync(path.join(ROOT, 'dist', 'index.html'))) {
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
    return spawn(PB_BINARY, args, {
        cwd: ROOT,
        stdio: 'inherit',
        env: { ...process.env, IMAP_ADDR: ':1193' },
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

    await seed(dataDir, port)
    buildBundle(releaseId)
    promote(distDir, releasesDir, releaseId)

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
