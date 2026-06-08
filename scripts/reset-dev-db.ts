#!/usr/bin/env -S pnpm exec tsx
/**
 * Reset Database Script
 *
 * Deletes server/pb_data, starts PocketBase to run migrations,
 * then seeds the database with a test user and org.
 *
 * Usage:
 *   pnpm exec tsx scripts/reset-dev-db.ts [options]
 *
 * Options:
 *   --url <url>        PocketBase URL (default: http://127.0.0.1:7100)
 *   --browse-url <url> URL the developer opens in the browser (the dev proxy).
 *                      Defaults to --url with 127.0.0.1 → localhost. Set this
 *                      when PB sits behind a proxy on a different port (e.g.
 *                      the expo:test flow seeds PB on :7299 but browses :7200).
 *   --data-dir <dir>   Data directory (default: server/pb_data)
 *   --skip-build       Skip building PocketBase
 *   --keep-running     Keep server running after seeding (default: false)
 *   --help             Show this help message
 *
 * Environment variables (from .env):
 *   POCKETBASE_EMAIL     - Superuser email (or SEED_ADMIN_EMAIL)
 *   POCKETBASE_PASSWORD  - Superuser password (or SEED_ADMIN_PW)
 */

import { spawn, spawnSync } from 'node:child_process'
import { randomBytes } from 'node:crypto'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { loadEnv } from '@tinycld/core/lib/load-env'

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
                config.keepRunning = args[++i] !== 'false'
                break
            case '--help':
                process.exit(0)
                break
            default:
                if (arg.startsWith('-')) {
                    process.exit(1)
                }
        }
    }

    return config
}

const CONFIG = parseArgs()
const PB_URL = CONFIG.url
const parsedUrl = new URL(PB_URL)
const PB_HOST = parsedUrl.hostname
const PB_PORT = parseInt(parsedUrl.port || '8090', 10)
const PB_DATA_DIR = path.join(process.cwd(), CONFIG.dataDir)
const PB_BINARY = path.join(process.cwd(), 'server/app')

// The URL the developer actually opens. Prefer an explicit --browse-url (the
// dev proxy, which may live on a different port than PB), otherwise fall back to
// the PB URL with 127.0.0.1 → localhost for readability.
const BROWSE_URL = (CONFIG.browseUrl ?? PB_URL.replace('127.0.0.1', 'localhost')).replace(/\/$/, '')

async function sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms))
}

async function waitForPocketBase(maxAttempts = 30): Promise<boolean> {
    for (let i = 0; i < maxAttempts; i++) {
        try {
            const response = await fetch(`${PB_URL}/api/health`)
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

function killExistingPocketBase(): void {
    log('Killing existing PocketBase on port', PB_PORT)
    try {
        const result = spawnSync('lsof', ['-ti', `:${PB_PORT}`], {
            encoding: 'utf-8',
        })
        if (result.stdout.trim()) {
            const pids = result.stdout.trim().split('\n')
            for (const pid of pids) {
                spawnSync('kill', ['-9', pid])
            }
            log('Killed', pids.length, 'process(es)')
        } else {
            log('No existing process found')
        }
    } catch {
        log('No existing process found')
    }
}

function deletePbData(): void {
    log('Deleting', PB_DATA_DIR)
    if (fs.existsSync(PB_DATA_DIR)) {
        fs.rmSync(PB_DATA_DIR, { recursive: true, force: true })
        log('Deleted')
    } else {
        log('No data directory found, skipping')
    }
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

function createSuperuser(): void {
    const { email, password, generated } = getCredentials()
    log('Creating superuser:', email)

    const result = spawnSync(
        PB_BINARY,
        ['superuser', 'upsert', email, password, '--dir', PB_DATA_DIR],
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

async function startPocketBase(): Promise<ReturnType<typeof spawn>> {
    log(`Starting PocketBase at ${PB_HOST}:${PB_PORT}...`)
    const migrationsDir = path.join(process.cwd(), 'server/pb_migrations')
    const pb = spawn(
        PB_BINARY,
        [
            '--dev',
            '--dir',
            PB_DATA_DIR,
            '--migrationsDir',
            migrationsDir,
            '--http',
            `${PB_HOST}:${PB_PORT}`,
            'serve',
        ],
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
// graceful exit — if we proceed before PB is gone, dev.ts opens the same DB
// while seed-PB is still flushing and we get stale/inconsistent reads.
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

async function runSeedScript(): Promise<void> {
    log('Running seed script...')
    const { email, password } = getCredentials()

    return new Promise((resolve, reject) => {
        const seed = spawn(
            'npx',
            [
                'tsx',
                'scripts/seed-db.ts',
                '--url',
                PB_URL,
                '--admin-email',
                email,
                '--admin-pw',
                password,
            ],
            {
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
                    TINYCLD_BROWSE_URL: BROWSE_URL,
                },
            }
        )

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

async function main() {
    let pb: ReturnType<typeof spawn> | null = null

    try {
        killExistingPocketBase()
        deletePbData()
        buildPocketBase()
        createSuperuser()
        pb = await startPocketBase()

        log('Waiting for PocketBase to be ready...')
        const ready = await waitForPocketBase()
        if (!ready) {
            throw new Error('PocketBase failed to start within timeout')
        }
        log('PocketBase is ready')

        // Silence PB's stdout before seeding so the seed's login-summary box
        // isn't interleaved with PB's async SQL logs. The seed prints its own
        // [seed] progress; PB errors still surface via stderr, and a failed
        // seed exits non-zero (thrown below). The box itself (app + superuser
        // creds, the /_/ and /admin URLs) is printed by the seed script.
        relayPbStdout = false

        await runSeedScript()

        if (CONFIG.keepRunning) {
            log('Keeping server running (Ctrl+C to stop)')
            await new Promise<void>(resolve => {
                process.on('SIGINT', () => {
                    resolve()
                })
                process.on('SIGTERM', () => {
                    resolve()
                })
            })
        }

        log('Done!')
    } catch (err) {
        logError('Failed:', err)
        process.exit(1)
    } finally {
        if (pb) {
            const child = pb
            await stopPocketBase(child)
        }
    }
}

main()
