#!/usr/bin/env -S pnpm exec tsx
/**
 * Reset Database Script
 *
 * Builds a fresh database in a temp directory: starts PocketBase on a free
 * port to run migrations, seeds a test user and org, then moves the result
 * over server/pb_data. A running `pnpm run dev` keeps its proxy and Expo up:
 * the script pauses only its PocketBase for the swap (see dev-pb-control.ts).
 *
 * Usage:
 *   pnpm exec tsx scripts/reset-dev-db.ts [options]
 *
 * Options:
 *   --url <url>        URL PocketBase serves on with --keep-running
 *                      (default: http://127.0.0.1:7100)
 *   --browse-url <url> URL the developer opens in the browser (cosmetic — it's
 *                      printed in the seed's login summary). Defaults to the
 *                      running dev proxy, else --url with 127.0.0.1 → localhost.
 *   --data-dir <dir>   Data directory (default: server/pb_data)
 *   --skip-build       Skip building PocketBase
 *   --keep-running     Serve the new data dir on --url after the swap
 *   --help             Show this help message
 *
 * Environment variables (from .env):
 *   POCKETBASE_EMAIL     - Superuser email (or SEED_ADMIN_EMAIL)
 *   POCKETBASE_PASSWORD  - Superuser password (or SEED_ADMIN_PW)
 */

import { spawn, spawnSync } from 'node:child_process'
import { randomBytes } from 'node:crypto'
import * as fs from 'node:fs'
import * as net from 'node:net'
import * as os from 'node:os'
import * as path from 'node:path'
import { loadEnv } from '@tinycld/core/lib/load-env'
import {
    type DevPbState,
    PAUSE_PB_SIGNAL,
    RESUME_PB_SIGNAL,
    readLiveDevPbState,
} from './dev-pb-control'
import { isPortFree } from './port-probe'

function log(...args: unknown[]) {
    process.stdout.write(`[reset-dev-db] ${args.join(' ')}\n`)
}

function logError(...args: unknown[]) {
    process.stderr.write(`[reset-dev-db] ${args.join(' ')}\n`)
}

loadEnv()

interface Config {
    url: string
    browseUrl: string | null
    dataDir: string
    skipBuild: boolean
    keepRunning: boolean
}

function printUsage(): void {
    process.stdout.write(
        `Reset the dev database: seed a fresh one in a temp dir, then swap it over pb_data.\n\n` +
            `Usage:\n` +
            `  pnpm exec tsx scripts/reset-dev-db.ts [options]\n\n` +
            `Options:\n` +
            `  --url <url>        URL PocketBase serves on with --keep-running\n` +
            `                     (default: http://127.0.0.1:7100)\n` +
            `  --browse-url <url> URL the developer opens in the browser (cosmetic; printed in\n` +
            `                     the seed login summary). Defaults to the running dev proxy,\n` +
            `                     else --url with 127.0.0.1 → localhost.\n` +
            `  --data-dir <dir>   Data directory (default: server/pb_data)\n` +
            `  --skip-build       Skip building PocketBase\n` +
            `  --keep-running     Serve the new data dir on --url after the swap (default: exit)\n` +
            `  --help             Show this help message\n`
    )
}

function parseArgs(): Config {
    const args = process.argv.slice(2)
    const config: Config = {
        url: 'http://127.0.0.1:7100',
        browseUrl: null,
        dataDir: 'server/pb_data',
        skipBuild: false,
        keepRunning: false,
    }

    for (let i = 0; i < args.length; i++) {
        const arg = args[i]
        switch (arg) {
            case '--url':
                config.url = args[++i]
                break
            case '--browse-url':
                config.browseUrl = args[++i]
                break
            case '--data-dir':
                config.dataDir = args[++i]
                break
            case '--skip-build':
                config.skipBuild = true
                break
            case '--keep-running':
                // Boolean flag — it consumes no value. (Previously it swallowed
                // the next argv token, so a following flag was silently eaten.)
                config.keepRunning = true
                break
            case '--help':
                printUsage()
                process.exit(0)
                break
            default:
                if (arg.startsWith('-')) {
                    logError(`Unknown flag: ${arg}`)
                    printUsage()
                    process.exit(1)
                }
        }
    }

    return config
}

const CONFIG = parseArgs()
const PB_DATA_DIR = path.join(process.cwd(), CONFIG.dataDir)
const PB_BINARY = path.join(process.cwd(), 'server/app')
const MIGRATIONS_DIR = path.join(process.cwd(), 'server/pb_migrations')

// The URL the developer actually opens. Prefer an explicit --browse-url, then
// the proxy of a dev.ts that serves this data dir, otherwise --url with
// 127.0.0.1 → localhost for readability.
function browseUrl(dev: DevPbState | null): string {
    const url = CONFIG.browseUrl ?? dev?.publicUrl ?? CONFIG.url.replace('127.0.0.1', 'localhost')
    return url.replace(/\/$/, '')
}

async function sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms))
}

async function waitForPocketBase(url: string, maxAttempts = 30): Promise<boolean> {
    for (let i = 0; i < maxAttempts; i++) {
        try {
            const response = await fetch(`${url}/api/health`)
            if (response.ok) {
                return true
            }
        } catch {
            // Server not ready yet
        }
        await sleep(1000)
    }
    return false
}

function freePort(): Promise<number> {
    return new Promise((resolve, reject) => {
        const server = net.createServer()
        server.once('error', reject)
        server.listen(0, '127.0.0.1', () => {
            const { port } = server.address() as net.AddressInfo
            server.close(() => resolve(port))
        })
    })
}

async function waitForPortFree(port: number, timeoutMs: number): Promise<boolean> {
    const start = Date.now()
    while (Date.now() - start < timeoutMs) {
        if (await isPortFree(port)) return true
        await sleep(100)
    }
    return false
}

// The PIDs that hold the SQLite database of dataDir open. A PocketBase that
// still has it open would keep serving the deleted files, or write its WAL
// back into the new database.
function databaseHolders(dataDir: string): string[] {
    const dbFile = path.join(dataDir, 'data.db')
    if (!fs.existsSync(dbFile)) return []
    const result = spawnSync('lsof', ['-t', dbFile], { encoding: 'utf-8' })
    return result.stdout.trim().split('\n').filter(Boolean)
}

async function pauseDevPocketBase(dev: DevPbState): Promise<void> {
    log(`Pausing the PocketBase of dev server pid ${dev.pid}`)
    process.kill(dev.pid, PAUSE_PB_SIGNAL)
    if (!(await waitForPortFree(dev.pbPort, 15_000))) {
        throw new Error(`dev PocketBase did not release :${dev.pbPort}`)
    }
}

async function resumeDevPocketBase(dev: DevPbState): Promise<void> {
    log(`Resuming the PocketBase of dev server pid ${dev.pid}`)
    process.kill(dev.pid, RESUME_PB_SIGNAL)
    if (!(await waitForPocketBase(`http://127.0.0.1:${dev.pbPort}`))) {
        throw new Error(`dev PocketBase did not come back on :${dev.pbPort}`)
    }
}

// A rename fails across volumes (the temp dir can be on another one), so copy
// in that case.
function moveDir(from: string, to: string): void {
    try {
        fs.renameSync(from, to)
    } catch (err) {
        if ((err as NodeJS.ErrnoException).code !== 'EXDEV') throw err
        fs.cpSync(from, to, { recursive: true })
        fs.rmSync(from, { recursive: true, force: true })
    }
}

function replacePbData(seededDir: string): void {
    const holders = databaseHolders(PB_DATA_DIR)
    if (holders.length > 0) {
        throw new Error(
            `${PB_DATA_DIR} is open in pid ${holders.join(', ')}; stop that PocketBase first`
        )
    }
    log('Replacing', PB_DATA_DIR)
    fs.rmSync(PB_DATA_DIR, { recursive: true, force: true })
    moveDir(seededDir, PB_DATA_DIR)
}

function buildPocketBase(): void {
    if (CONFIG.skipBuild) {
        log('Skipping build (--skip-build)')
        return
    }

    log('Building PocketBase...')
    const result = spawnSync('go', ['build', '-o', 'app', '.'], {
        cwd: path.join(process.cwd(), 'server'),
        stdio: 'inherit',
    })
    if (result.status !== 0) {
        throw new Error('Failed to build PocketBase')
    }
    log('Build complete')
}

// A reasonably strong random password for a freshly-created superuser when the
// operator didn't supply one. Not hardcoded — a baked-in default would ship a
// known admin password to anyone who runs the reset without setting the env.
function generateAdminPassword(): string {
    const bytes = randomBytes(18).toString('base64url')
    return `Tc!${bytes}`
}

// Resolve once per process: getCredentials() is called several times (create,
// seed, summary) and a fresh random each call would diverge — we'd create the
// superuser with one password and then fail to auth/seed with another.
let resolvedCredentials: { email: string; password: string; generated: boolean } | null = null

function getCredentials(): { email: string; password: string; generated: boolean } {
    if (resolvedCredentials) return resolvedCredentials
    const email =
        process.env.POCKETBASE_EMAIL ||
        process.env.SEED_ADMIN_EMAIL ||
        process.env.ADMIN_USER_LOGIN ||
        'admin@tinycld.org'
    // ADMIN_USER_PW is the var CI sets and the e2e superuser helpers read (via
    // POCKETBASE_PASSWORD || 'AdminPass1234!'), so honor it here too — otherwise
    // a fresh random would be created that those helpers can't authenticate.
    const supplied =
        process.env.POCKETBASE_PASSWORD ||
        process.env.SEED_ADMIN_PW ||
        process.env.ADMIN_USER_PW ||
        ''
    const password = supplied || generateAdminPassword()
    resolvedCredentials = { email, password, generated: supplied === '' }
    return resolvedCredentials
}

function createSuperuser(dataDir: string): void {
    const { email, password, generated } = getCredentials()
    log('Creating superuser:', email)

    const result = spawnSync(
        PB_BINARY,
        ['superuser', 'upsert', email, password, '--dir', dataDir],
        {
            stdio: 'inherit',
        }
    )
    if (result.status !== 0) {
        throw new Error('Failed to create superuser')
    }

    // Surface a generated password once so the operator can actually log in.
    // Supplied passwords (env/CI) are never echoed.
    if (generated) {
        log('Generated superuser password (set POCKETBASE_PASSWORD to override):')
        log(`  ${email} / ${password}`)
    }
}

// When false, PB's stdout is consumed but not echoed — see the relay comment in
// startPocketBase. Flipped off before printing the credential boxes so PB's
// async logs don't land between the box borders. stderr is always echoed.
let relayPbStdout = true

function startPocketBase(url: string, dataDir: string): ReturnType<typeof spawn> {
    const { host } = new URL(url)
    log(`Starting PocketBase at ${host}...`)
    const pb = spawn(
        PB_BINARY,
        ['--dev', '--dir', dataDir, '--migrationsDir', MIGRATIONS_DIR, '--http', host, 'serve'],
        {
            stdio: ['ignore', 'pipe', 'pipe'],
            detached: false,
            env: {
                ...process.env,
                SKIP_SENDING_MAIL: 'true',
            },
        }
    )

    // Surface PB's stdout — PocketBase writes its bind banner and a lot of
    // useful startup diagnostics there, and previously we threw it away.
    // When the readiness probe times out, having these lines in the CI log
    // is the difference between a one-line "failed to start" and a real
    // diagnosis.
    // Relay PB's (chatty) stdout through a mutable `relay` flag rather than a
    // detachable listener. When we silence PB (before printing the boxed
    // summaries) we flip relay=false but KEEP consuming the pipe — detaching
    // the reader instead would let PB's stdout buffer fill and make it emit
    // "dropping unclosed output" warnings mid-box. stderr is always relayed so
    // a genuine error is never swallowed.
    pb.stdout?.on('data', data => {
        if (relayPbStdout) log('[pocketbase]', data.toString().trimEnd())
    })

    pb.stderr?.on('data', data => {
        logError('[pocketbase]', data.toString().trim())
    })

    pb.on('error', err => {
        logError('[pocketbase] spawn error:', err)
    })

    return pb
}

// SIGTERM gets PocketBase to start shutting down (background goroutines like
// the push scheduler and drive watcher get notified), but exit can take more
// than 5s on a busy machine. Critically, the SQLite WAL only checkpoints on
// graceful exit — if we move the data dir before PB is gone, the copy can miss
// writes that are still in the WAL.
// So: SIGTERM, wait up to 15s, escalate to SIGKILL, then wait for the
// process to actually be reaped before returning.
async function stopPocketBase(child: ReturnType<typeof spawn>): Promise<void> {
    if (child.exitCode !== null || child.signalCode !== null) return

    const waitForExit = (timeoutMs: number) =>
        new Promise<boolean>(resolve => {
            const timer = setTimeout(() => resolve(false), timeoutMs)
            child.once('exit', () => {
                clearTimeout(timer)
                resolve(true)
            })
        })

    child.kill('SIGTERM')
    if (await waitForExit(15_000)) return

    log('PocketBase did not exit after SIGTERM, sending SIGKILL...')
    try {
        child.kill('SIGKILL')
    } catch {
        // process may already be gone between the SIGTERM check and here
    }
    await waitForExit(5_000)
}

async function runSeedScript(url: string, browse: string): Promise<void> {
    log('Running seed script...')
    const { email, password, generated } = getCredentials()

    // Tell the seed whether we RANDOMLY generated this admin password (so its
    // login summary may echo it) vs. it being a supplied secret. The flag must
    // come AFTER --admin-pw, which resets the generated marker to false.
    const seedArgs = [
        'tsx',
        'scripts/seed-db.ts',
        '--url',
        url,
        '--admin-email',
        email,
        '--admin-pw',
        password,
    ]
    if (generated) seedArgs.push('--admin-pw-generated')

    return new Promise((resolve, reject) => {
        const seed = spawn('npx', seedArgs, {
            stdio: 'inherit',
            // Sibling packages link in via symlinks; without
            // --preserve-symlinks Node resolves seed.ts to its
            // realpath inside the sibling repo and can't find peer
            // deps (exceljs, etc.) in the app shell's node_modules.
            // TINYCLD_BROWSE_URL tells the seed's login summary which URL
            // the developer browses (the proxy), not PB's internal port.
            env: {
                ...process.env,
                NODE_OPTIONS: '--preserve-symlinks',
                TINYCLD_BROWSE_URL: browse,
            },
        })

        seed.on('close', code => {
            if (code === 0) {
                resolve()
            } else {
                reject(new Error(`Seed script exited with code ${code}`))
            }
        })

        seed.on('error', reject)
    })
}

async function seedFreshDataDir(dataDir: string, browse: string): Promise<void> {
    createSuperuser(dataDir)
    const url = `http://127.0.0.1:${await freePort()}`
    const pb = startPocketBase(url, dataDir)
    try {
        log('Waiting for PocketBase to be ready...')
        if (!(await waitForPocketBase(url))) {
            throw new Error('PocketBase failed to start within timeout')
        }
        log('PocketBase is ready')

        // Silence PB's stdout before seeding so the seed's login-summary box
        // isn't interleaved with PB's async SQL logs. The seed prints its own
        // [seed] progress; PB errors still surface via stderr, and a failed
        // seed exits non-zero (thrown below). The box itself (app + superuser
        // creds, the /_/ and /admin URLs) is printed by the seed script.
        relayPbStdout = false

        await runSeedScript(url, browse)
    } finally {
        await stopPocketBase(pb)
    }
}

async function serveUntilSignal(): Promise<void> {
    relayPbStdout = true
    const pb = startPocketBase(CONFIG.url, PB_DATA_DIR)
    if (!(await waitForPocketBase(CONFIG.url))) {
        await stopPocketBase(pb)
        throw new Error('PocketBase failed to start within timeout')
    }
    log('Keeping server running (Ctrl+C to stop)')
    await new Promise<void>(resolve => {
        process.on('SIGINT', () => resolve())
        process.on('SIGTERM', () => resolve())
    })
    await stopPocketBase(pb)
}

async function main() {
    const seededDir = fs.mkdtempSync(path.join(os.tmpdir(), 'tinycld-reset-'))
    let pausedDev: DevPbState | null = null

    try {
        const dev = readLiveDevPbState(PB_DATA_DIR)
        if (dev && CONFIG.keepRunning) {
            throw new Error(
                `--keep-running: dev server pid ${dev.pid} already serves ${PB_DATA_DIR}`
            )
        }

        buildPocketBase()
        await seedFreshDataDir(seededDir, browseUrl(dev))

        if (dev) {
            pausedDev = dev
            await pauseDevPocketBase(dev)
        }
        replacePbData(seededDir)
        if (pausedDev) {
            const resumed = pausedDev
            pausedDev = null
            await resumeDevPocketBase(resumed)
        }

        if (CONFIG.keepRunning) await serveUntilSignal()

        log('Done!')
    } catch (err) {
        logError('Failed:', err)
        process.exitCode = 1
    } finally {
        // Never leave the dev server without its PocketBase.
        if (pausedDev) process.kill(pausedDev.pid, RESUME_PB_SIGNAL)
        fs.rmSync(seededDir, { recursive: true, force: true })
    }
}

main()
