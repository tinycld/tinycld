#!/usr/bin/env -S pnpm exec tsx
/**
 * Semi-automated live smoke test for the `tinycld` CLI.
 *
 * WHY THIS EXISTS: every CLI test in the tree runs against a fake HTTP server
 * that has NO SCOPE LAYER AT ALL. The first manual smoke run found four real
 * bugs and THREE of them were scope plumbing — a collection or route missing
 * from a hand-maintained list — while the predicted failure (mirrored-struct
 * field drift between cli/ and the server types) has never once appeared. A
 * fake server cannot see any of that by construction. So this script drives
 * the real binary against a real PocketBase with a real OAuth grant.
 *
 * It replaces the hand-run checklist in HANDOFF-cli-smoke-test.md "Round 2".
 *
 * WHAT IT DOES
 *   1. Boots a throwaway server (own port + data dir), seeded fresh.
 *   2. Mints OAuth grants for BOTH seeded users, non-interactively.
 *   3. Runs every CLI command group, asserting the things only a real server
 *      can prove and reporting the rest.
 *   4. Prints a coverage table naming everything it did NOT run, and exits
 *      non-zero if any assertion failed.
 *
 * USAGE
 *   pnpm exec tsx scripts/cli-smoke.ts                 # hermetic, full run
 *   pnpm exec tsx scripts/cli-smoke.ts --only contacts # one group
 *   pnpm exec tsx scripts/cli-smoke.ts --server localhost:7101   # attach
 *   pnpm exec tsx scripts/cli-smoke.ts --keep          # leave the server up
 *
 * The hermetic default matters: re-run state accumulation has manufactured
 * false failures in this repo before (public-board.spec.ts could only pass
 * once per database). A dedicated data dir also means this never touches
 * server/pb_data.
 */

import { type ChildProcess, spawn, spawnSync } from 'node:child_process'
import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'

const ROOT = path.resolve(import.meta.dirname, '..')

// Seeded fixture credentials. These are literals in scripts/seed-db.ts and
// tests/e2e/helpers.ts for the same reason they are literals here: nothing in
// this path loads .env, so the seed and every consumer must agree on a shared
// constant or authentication fails with no obvious cause.
const OWNER = { email: 'user@tinycld.org', password: 'TestUser1234!' }
const COLLAB = { email: 'collaborator@tinycld.org', password: 'TestUser1234!' }

// The seeded first-party client (pb_migrations/1985000001). Its `scopes` field
// is a hard CEILING enforced by ValidateClientScopes, so a scope missing there
// fails login outright rather than 403ing later.
const CLIENT_ID = 'tinycld-cli'

interface Options {
    port: number
    server: string | null
    keep: boolean
    only: string | null
    verbose: boolean
}

function parseArgs(): Options {
    const argv = process.argv.slice(2)
    const opts: Options = { port: 7301, server: null, keep: false, only: null, verbose: false }

    const value = (flag: string, i: number): string => {
        const v = argv[i + 1]
        if (v === undefined || v.startsWith('-')) {
            throw new Error(`cli-smoke: ${flag} requires a value`)
        }
        return v
    }

    for (let i = 0; i < argv.length; i++) {
        switch (argv[i]) {
            case '--port':
                opts.port = Number.parseInt(value('--port', i), 10)
                i++
                break
            case '--server':
                opts.server = value('--server', i)
                i++
                break
            case '--only':
                opts.only = value('--only', i)
                i++
                break
            case '--keep':
                opts.keep = true
                break
            case '--verbose':
            case '-v':
                opts.verbose = true
                break
            case '--help':
            case '-h':
                printUsage()
                process.exit(0)
                break
            default:
                throw new Error(`cli-smoke: unknown flag ${argv[i]}`)
        }
    }
    return opts
}

function printUsage(): void {
    process.stdout.write(`Live smoke test for the tinycld CLI.

Usage: pnpm exec tsx scripts/cli-smoke.ts [options]

  --port <n>        port for the throwaway server (default 7301)
  --server <origin> attach to an already-running server instead of booting one
  --only <group>    run one group: search|drive|mail|cards|contacts|calendar|text|calc
  --keep            leave the throwaway server running for inspection
  --verbose, -v     echo every command's full output
  --help, -h        this message
`)
}

const OPTS = parseArgs()

// ---------------------------------------------------------------------------
// Output
// ---------------------------------------------------------------------------

const COLOR = process.stdout.isTTY && !process.env.NO_COLOR
const c = {
    dim: (s: string) => (COLOR ? `\x1b[2m${s}\x1b[0m` : s),
    red: (s: string) => (COLOR ? `\x1b[31m${s}\x1b[0m` : s),
    green: (s: string) => (COLOR ? `\x1b[32m${s}\x1b[0m` : s),
    yellow: (s: string) => (COLOR ? `\x1b[33m${s}\x1b[0m` : s),
    bold: (s: string) => (COLOR ? `\x1b[1m${s}\x1b[0m` : s),
}

function log(msg: string): void {
    process.stdout.write(`[cli-smoke] ${msg}\n`)
}

function section(title: string): void {
    process.stdout.write(
        `\n${c.bold(`── ${title} ${'─'.repeat(Math.max(0, 60 - title.length))}`)}\n`
    )
}

// ---------------------------------------------------------------------------
// Results
// ---------------------------------------------------------------------------

type Status = 'pass' | 'fail' | 'skip'

interface Result {
    group: string
    name: string
    status: Status
    detail: string
}

const results: Result[] = []

function record(group: string, name: string, status: Status, detail = ''): void {
    results.push({ group, name, status, detail })
    const mark = status === 'pass' ? c.green('✓') : status === 'fail' ? c.red('✗') : c.yellow('–')
    const suffix = detail ? ` ${c.dim(detail)}` : ''
    process.stdout.write(`  ${mark} ${name}${suffix}\n`)
}

/**
 * A hard assertion — these are the claims only a real server can settle, so a
 * failure here is the whole point of the run.
 */
function assert(group: string, name: string, ok: boolean, detail = ''): boolean {
    record(group, name, ok ? 'pass' : 'fail', detail)
    return ok
}

function skip(group: string, name: string, reason: string): void {
    record(group, name, 'skip', reason)
}

// ---------------------------------------------------------------------------
// Running the CLI
// ---------------------------------------------------------------------------

let CLI_BIN = ''
let CONFIG_DIR = ''

interface RunResult {
    code: number
    stdout: string
    stderr: string
    ok: boolean
}

/**
 * Invoke the CLI. stdin is /dev/null throughout: `mail send --body-file -`
 * reads stdin to EOF and would hang, and ui.Confirm refuses rather than
 * blocking when stdin is not a terminal — so a missing --yes fails loudly
 * instead of deadlocking the run.
 */
function cli(args: string[], ctxName?: string): RunResult {
    const full = ctxName ? ['--context', ctxName, ...args] : args
    const r = spawnSync(CLI_BIN, full, {
        encoding: 'utf-8',
        stdio: ['ignore', 'pipe', 'pipe'],
        env: { ...process.env, TINYCLD_CONFIG_DIR: CONFIG_DIR, NO_COLOR: '1' },
        timeout: 60_000,
    })
    const res: RunResult = {
        code: r.status ?? -1,
        stdout: r.stdout ?? '',
        stderr: r.stderr ?? '',
        ok: r.status === 0,
    }
    if (OPTS.verbose) {
        process.stdout.write(c.dim(`    $ tinycld ${full.join(' ')}\n`))
        for (const line of (res.stdout + res.stderr).split('\n')) {
            if (line.trim()) process.stdout.write(c.dim(`      ${line}\n`))
        }
    }
    return res
}

/** Run a read-only command and record pass/fail on its exit code alone. */
function report(group: string, name: string, args: string[], ctxName?: string): RunResult {
    const r = cli(args, ctxName)
    record(group, name, r.ok ? 'pass' : 'fail', r.ok ? '' : firstLine(r.stderr || r.stdout))
    return r
}

function firstLine(s: string): string {
    return s.trim().split('\n')[0]?.slice(0, 160) ?? ''
}

function parseJSON(r: RunResult): unknown {
    try {
        return JSON.parse(r.stdout)
    } catch {
        return null
    }
}

// ---------------------------------------------------------------------------
// Server lifecycle
// ---------------------------------------------------------------------------

let pbProcess: ChildProcess | null = null

// Set when reset-dev-db reports the seed finished. An attached server is
// assumed already seeded — we did not run its seed and cannot observe it.
let seedFinished = false

async function sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms))
}

async function waitForHealth(origin: string, timeoutMs: number): Promise<void> {
    const deadline = Date.now() + timeoutMs
    while (Date.now() < deadline) {
        try {
            const res = await fetch(`${origin}/api/health`)
            if (res.ok) return
        } catch {
            // not up yet
        }
        await sleep(500)
    }
    throw new Error(`cli-smoke: ${origin}/api/health did not go green within ${timeoutMs}ms`)
}

/**
 * Wait for the SEED, not merely for the server.
 *
 * /api/health goes green the moment PocketBase binds its listener, but
 * reset-dev-db seeds AFTER that — it has to, because it seeds over HTTP. So a
 * run gated only on health races the seed and authenticates before the fixture
 * user exists, which surfaces as a bare "Failed to authenticate" that looks
 * like a wrong password rather than a timing problem.
 */
async function waitForFixtureUser(origin: string, timeoutMs: number): Promise<void> {
    const deadline = Date.now() + timeoutMs
    let lastStatus = 0
    while (Date.now() < deadline) {
        try {
            const res = await fetch(`${origin}/api/collections/users/auth-with-password`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ identity: OWNER.email, password: OWNER.password }),
            })
            if (res.ok) return
            lastStatus = res.status
        } catch {
            // server still coming up
        }
        await sleep(1000)
    }
    throw new Error(
        `cli-smoke: ${OWNER.email} was not authenticable within ${timeoutMs}ms ` +
            `(last HTTP ${lastStatus}) — did the seed finish?`
    )
}

/**
 * Boot a throwaway, freshly-seeded server via reset-dev-db.ts --keep-running.
 * That script already builds PB, wipes the data dir, runs migrations, creates
 * the superuser, and seeds both fixture users — reimplementing any of it here
 * would be a second copy to keep in step.
 */
async function bootServer(origin: string): Promise<void> {
    log(`booting a throwaway server on ${origin} (data: server/pb_smoke_data)`)
    log('this builds PocketBase and reseeds from scratch — expect ~30-60s')

    pbProcess = spawn(
        'pnpm',
        [
            'exec',
            'tsx',
            'scripts/reset-dev-db.ts',
            '--url',
            origin,
            '--data-dir',
            'server/pb_smoke_data',
            '--keep-running',
        ],
        {
            cwd: ROOT,
            // stdout is PIPED, not inherited: it carries the seed's completion
            // marker, which is the only reliable signal that package data has
            // landed. PocketBase's SQL logging goes down the same pipe, so it
            // is echoed only under --verbose.
            stdio: ['ignore', 'pipe', 'inherit'],
            env: {
                ...process.env,
                // Without a supplied admin password reset-dev-db generates a
                // random one per run. Pinning it keeps the run reproducible and
                // stops the seed echoing a fresh secret into the log each time.
                ADMIN_USER_PW: process.env.ADMIN_USER_PW || 'AdminPass1234!',
            },
        }
    )

    pbProcess.stdout?.on('data', (chunk: Buffer) => {
        const text = chunk.toString()
        if (OPTS.verbose) process.stdout.write(text)
        if (text.includes('Seeding complete!')) seedFinished = true
        if (/\[seed\] Failed:/.test(text)) {
            process.stderr.write(`[cli-smoke] the seed reported a failure:\n${text}\n`)
        }
    })

    pbProcess.on('exit', code => {
        if (code !== 0 && code !== null) {
            process.stderr.write(`[cli-smoke] server exited unexpectedly (${code})\n`)
        }
    })

    await waitForHealth(origin, 300_000)
    log('server is up; waiting for the seed to finish')
    await waitForFixtureUser(origin, 300_000)
    await waitForSeedComplete(300_000)
    log('server is ready and seeded')
}

/**
 * Wait for reset-dev-db to say the seed FINISHED.
 *
 * The fixture user is created early in the seed, so authenticating as them
 * proves the run started, not that it completed — package data (boards,
 * calendars, mail) lands afterwards. Acting too early gives a half-populated
 * database, which shows up as arbitrary "no boards seeded" style skips.
 */
async function waitForSeedComplete(timeoutMs: number): Promise<void> {
    const deadline = Date.now() + timeoutMs
    while (Date.now() < deadline) {
        if (seedFinished) return
        if (pbProcess?.exitCode !== null && pbProcess?.exitCode !== undefined) {
            throw new Error('cli-smoke: the server exited before the seed completed')
        }
        await sleep(500)
    }
    throw new Error(`cli-smoke: the seed did not complete within ${timeoutMs}ms`)
}

function stopServer(): void {
    if (!pbProcess || pbProcess.exitCode !== null) return
    log('stopping the throwaway server')
    pbProcess.kill('SIGTERM')
}

// ---------------------------------------------------------------------------
// Auth — the RFC 8628 device grant, driven without a browser
// ---------------------------------------------------------------------------

/**
 * `tinycld auth login` blocks on human approval even under --yes, so it can
 * never run in a script. Instead we perform the same four steps the CLI and
 * the web consent screen perform between them, then write the credential to
 * disk in the CLI's own format.
 *
 * This is not a bypass of the auth system — it exercises the real endpoints
 * (/oauth/device, /oauth/authorize/approve, /oauth/token) with a real grant
 * and real scope validation. Only the human click is replaced.
 */
async function mintGrant(
    origin: string,
    user: { email: string; password: string },
    label: string
): Promise<{ access_token: string; refresh_token: string; scope: string; expires_in: number }> {
    // 1. Ask for a device code. The scope string is the CLI's own request set,
    //    read from the server catalog so this script cannot drift from it.
    const scopes = await fetchServerScopes(origin)
    const deviceRes = await fetch(`${origin}/oauth/device`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: new URLSearchParams({ client_id: CLIENT_ID, scope: scopes.join(' ') }),
    })
    if (!deviceRes.ok) {
        throw new Error(`POST /oauth/device → HTTP ${deviceRes.status}: ${await deviceRes.text()}`)
    }
    const device = (await deviceRes.json()) as { device_code: string; user_code: string }

    // 2. Sign in as the fixture user to get a PocketBase SESSION token. The
    //    approve endpoint rejects an OAuth access token (rejectOAuthToken), so
    //    it must be this and not a bearer grant.
    const authRes = await fetch(`${origin}/api/collections/users/auth-with-password`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ identity: user.email, password: user.password }),
    })
    if (!authRes.ok) {
        throw new Error(
            `auth-with-password for ${user.email} → HTTP ${authRes.status}: ${await authRes.text()}`
        )
    }
    const session = (await authRes.json()) as { token: string }

    // 3. Approve. FORM-ENCODED ONLY — a JSON body yields an empty user_code and
    //    a misleading 404 "That code is not valid" for a code that is present
    //    and pending. The grant binds to whoever this token authenticates as.
    const approveRes = await fetch(`${origin}/oauth/authorize/approve`, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
            Authorization: session.token,
        },
        body: new URLSearchParams({ user_code: device.user_code, device_label: label }),
    })
    if (!approveRes.ok) {
        throw new Error(
            `POST /oauth/authorize/approve → HTTP ${approveRes.status}: ${await approveRes.text()}`
        )
    }

    // 4. Exchange the device code for tokens.
    const tokenRes = await fetch(`${origin}/oauth/token`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: new URLSearchParams({
            grant_type: 'urn:ietf:params:oauth:grant-type:device_code',
            device_code: device.device_code,
            client_id: CLIENT_ID,
        }),
    })
    if (!tokenRes.ok) {
        throw new Error(`POST /oauth/token → HTTP ${tokenRes.status}: ${await tokenRes.text()}`)
    }
    return (await tokenRes.json()) as {
        access_token: string
        refresh_token: string
        scope: string
        expires_in: number
    }
}

/**
 * The scopes this deployment actually advertises. Reading them rather than
 * hardcoding a list means the script requests exactly what the server offers —
 * if a package's scopes are missing from the catalog, that surfaces as its
 * commands 403ing, which is the finding, not as a login failure here.
 */
async function fetchServerScopes(origin: string): Promise<string[]> {
    const res = await fetch(`${origin}/.well-known/oauth-authorization-server`)
    if (!res.ok) {
        throw new Error(`OAuth metadata → HTTP ${res.status}`)
    }
    const meta = (await res.json()) as { scopes_supported?: string[] }
    if (!meta.scopes_supported?.length) {
        throw new Error('OAuth metadata carried no scopes_supported')
    }
    return meta.scopes_supported
}

/**
 * Write config.toml + the credential file into a scratch TINYCLD_CONFIG_DIR.
 *
 * The file store is the CLI's own documented fallback when no OS keychain
 * responds, and systemStore.Get falls through to it for a context name the
 * keychain has never seen — so this is a supported path, and it keeps the run
 * out of the developer's real ~/.config/tinycld and login keychain entirely.
 */
function writeCredentials(
    contexts: {
        name: string
        origin: string
        user: string
        token: Awaited<ReturnType<typeof mintGrant>>
    }[]
): void {
    const lines: string[] = []
    lines.push(`current = "${contexts[0].name}"`)
    lines.push('')
    lines.push('[contexts]')
    for (const ctx of contexts) {
        lines.push(`[contexts.${JSON.stringify(ctx.name)}]`)
        lines.push(`origin = "${ctx.origin}"`)
        lines.push(`user = "${ctx.user}"`)
    }
    fs.writeFileSync(path.join(CONFIG_DIR, 'config.toml'), `${lines.join('\n')}\n`, { mode: 0o600 })

    const credDir = path.join(CONFIG_DIR, 'credentials')
    fs.mkdirSync(credDir, { recursive: true, mode: 0o700 })
    for (const ctx of contexts) {
        // The filename is the url-escaped context name (fileStore.path).
        const file = path.join(credDir, `${encodeURIComponent(ctx.name)}.json`)
        const expiresAt = new Date(Date.now() + ctx.token.expires_in * 1000).toISOString()
        fs.writeFileSync(
            file,
            JSON.stringify({
                access_token: ctx.token.access_token,
                refresh_token: ctx.token.refresh_token,
                scope: ctx.token.scope,
                origin: ctx.origin,
                expires_at: expiresAt,
            }),
            { mode: 0o600 }
        )
    }
}

// ---------------------------------------------------------------------------
// Build
// ---------------------------------------------------------------------------

/**
 * Build the CLI with every assembled group linked in. The generated go.work
 * already `use`s each member's cli module, so a plain build picks them all up.
 */
/**
 * Regenerate the package output the build reads.
 *
 * cli/cli_extensions.go is generated and gitignored, so a checkout can carry a
 * stale copy registering a narrower set of groups than the workspace actually
 * assembles — and the binary then reports the missing ones as "not
 * registered", which reads as a package that was never installed rather than
 * as stale generator output. Observed: a run silently skipped calendar and
 * calc for exactly this reason.
 *
 * MUST run before the server boots. The generator rewrites files the server
 * and the seed read, and doing it underneath a running stack kills the seed
 * mid-flight.
 */
function generatePackages(): void {
    log('regenerating package output (cli_extensions.go and friends)')
    const gen = spawnSync('pnpm', ['run', 'packages:generate'], {
        cwd: ROOT,
        stdio: OPTS.verbose ? 'inherit' : 'ignore',
    })
    if (gen.status !== 0) throw new Error('cli-smoke: packages:generate failed')
}

function buildCLI(): void {
    log('building the CLI binary')
    const out = path.join(CONFIG_DIR, 'tinycld')
    const r = spawnSync('go', ['build', '-o', out, '.'], {
        cwd: path.join(ROOT, 'cli'),
        stdio: 'inherit',
    })
    if (r.status !== 0) throw new Error('cli-smoke: go build failed')
    CLI_BIN = out
}

/** Which groups this binary actually registered, so absent ones are skipped honestly. */
function registeredGroups(): Set<string> {
    const help = spawnSync(CLI_BIN, ['--help'], { encoding: 'utf-8' }).stdout ?? ''
    const known = ['search', 'drive', 'mail', 'cards', 'contacts', 'calendar', 'text', 'calc']
    return new Set(known.filter(g => new RegExp(`^\\s+${g}\\s`, 'm').test(help)))
}

// ---------------------------------------------------------------------------
// Group exercises
// ---------------------------------------------------------------------------

const OWNER_CTX = 'smoke-owner'
const COLLAB_CTX = 'smoke-collab'

let scratch = ''

/** A per-run unique suffix so a --server attach run never collides with itself. */
const RUN_ID = `smoke${Date.now().toString(36).slice(-6)}`

function exerciseGlobal(): void {
    section('global')
    const g = 'global'
    report(g, 'version', ['version'])
    report(g, 'context list', ['context', 'list'])

    const status = report(g, 'auth status', ['auth', 'status'], OWNER_CTX)
    assert(
        g,
        'auth status reports the seeded owner',
        status.stdout.includes(OWNER.email),
        status.ok ? '' : firstLine(status.stderr)
    )

    // The grant's scope string is the single best early warning for scope
    // plumbing: a package missing from the catalog never appears here, and
    // every one of its commands will 403 later for that reason.
    const statusJSON = parseJSON(cli(['--json', 'auth', 'status'], OWNER_CTX)) as {
        scopes?: string
    } | null
    const granted = statusJSON?.scopes ?? ''
    for (const scope of [
        'drive:read',
        'mail:read',
        'cards:read',
        'contacts:read',
        'calendar:read',
    ]) {
        assert(
            g,
            `grant carries ${scope}`,
            granted.split(' ').includes(scope),
            granted ? '' : 'no scopes in grant'
        )
    }
    for (const scope of ['text:read', 'calc:read']) {
        assert(
            g,
            `grant carries ${scope}`,
            granted.split(' ').includes(scope),
            granted.includes(scope)
                ? ''
                : 'absent from the server catalog — text/calc commands will 403'
        )
    }
}

function exerciseSearch(): void {
    section('search')
    const g = 'search'
    report(g, 'search (federated)', ['search', 'test'], OWNER_CTX)
    const json = cli(['--json', 'search', 'test'], OWNER_CTX)
    assert(g, 'search --json parses', parseJSON(json) !== null, firstLine(json.stderr))
}

function exerciseDrive(): void {
    section('drive')
    const g = 'drive'
    report(g, 'drive ls', ['drive', 'ls'], OWNER_CTX)
    report(g, 'drive tree', ['drive', 'tree'], OWNER_CTX)
    report(g, 'drive usage', ['drive', 'usage'], OWNER_CTX)
    report(g, 'drive search', ['drive', 'search', 'a'], OWNER_CTX)
    report(g, 'drive trash', ['drive', 'trash'], OWNER_CTX)

    const json = cli(['--json', 'drive', 'ls'], OWNER_CTX)
    assert(g, 'drive ls --json parses', parseJSON(json) !== null, firstLine(json.stderr))

    // The round-trip: bytes in must equal bytes out. This is the one drive
    // assertion a fake server cannot fake — it exercises multipart upload, the
    // file token path, and the download handler together.
    const src = path.join(scratch, `${RUN_ID}-upload.txt`)
    const body = `smoke ${RUN_ID}\nline two\n${'x'.repeat(2048)}\n`
    fs.writeFileSync(src, body)

    const put = cli(['--yes', 'drive', 'put', src, '/'], OWNER_CTX)
    if (!assert(g, 'drive put uploads', put.ok, firstLine(put.stderr || put.stdout))) {
        skip(g, 'drive put → get byte-identical', 'upload failed')
        skip(g, 'drive rm (trash)', 'upload failed')
        return
    }

    const remote = `/${path.basename(src)}`
    const dest = path.join(scratch, `${RUN_ID}-download.txt`)
    const get = cli(['drive', 'get', remote, dest], OWNER_CTX)
    const identical = get.ok && fs.existsSync(dest) && fs.readFileSync(dest, 'utf-8') === body
    assert(g, 'drive put → get byte-identical', identical, get.ok ? '' : firstLine(get.stderr))

    const cat = cli(['drive', 'cat', remote], OWNER_CTX)
    assert(g, 'drive cat matches uploaded bytes', cat.stdout === body, firstLine(cat.stderr))

    // rm is a TRASH by default; --permanent is the hard delete. Both prompt,
    // so both need --yes.
    const rm = cli(['--yes', 'drive', 'rm', remote], OWNER_CTX)
    assert(g, 'drive rm (trash)', rm.ok, firstLine(rm.stderr || rm.stdout))
}

function exerciseMail(): void {
    section('mail')
    const g = 'mail'
    report(g, 'mail mailboxes', ['mail', 'mailboxes'], OWNER_CTX)
    report(g, 'mail list', ['mail', 'list'], OWNER_CTX)
    report(g, 'mail status', ['mail', 'status'], OWNER_CTX)
    report(g, 'mail labels', ['mail', 'labels'], OWNER_CTX)
    report(g, 'mail search', ['mail', 'search', 'a'], OWNER_CTX)

    const json = cli(['--json', 'mail', 'list'], OWNER_CTX)
    assert(g, 'mail list --json parses', parseJSON(json) !== null, firstLine(json.stderr))

    // mail_domains was missing from the scope table once and silently broke the
    // entire send path, because a mailbox address is only a local part and every
    // full address joins that row. `mailboxes` is the cheapest command that
    // proves the join is reachable.
    const boxes = cli(['--json', 'mail', 'mailboxes'], OWNER_CTX)
    const hasFullAddress = boxes.ok && /@/.test(boxes.stdout)
    assert(
        g,
        'mail mailboxes resolves full addresses (mail_domains reachable)',
        hasFullAddress,
        hasFullAddress ? '' : boxes.ok ? 'no @ in any address' : firstLine(boxes.stderr)
    )
}

function exerciseCards(): void {
    section('cards')
    const g = 'cards'
    const list = report(g, 'cards board list', ['cards', 'board', 'list'], OWNER_CTX)

    const json = cli(['--json', 'cards', 'board', 'list'], OWNER_CTX)
    // The KEY column is `slug` renamed at render time — the JSON carries no
    // `key` field. Reading `key` here silently yielded undefined and skipped the
    // check below as "no boards seeded" while three boards sat in the list.
    const boards = parseJSON(json) as { slug?: string; name?: string; id?: string }[] | null
    assert(g, 'cards board list --json parses', Array.isArray(boards), firstLine(json.stderr))

    // A board must be resolvable by the KEY its own output prints — that
    // round-trip was broken once (resolveProject matched ids and names only)
    // and the fake server shared the blind spot because its fixture never set
    // a slug.
    const key = boards?.[0]?.slug
    if (key) {
        const view = cli(['cards', 'board', 'view', key], OWNER_CTX)
        assert(g, `cards board view by printed KEY (${key})`, view.ok, firstLine(view.stderr))
    } else {
        skip(
            g,
            'cards board view by printed KEY',
            list.ok ? 'boards listed but none carried a slug' : 'board list failed'
        )
    }
}

function exerciseContacts(): void {
    section('contacts')
    const g = 'contacts'
    report(g, 'contacts list', ['contacts', 'list'], OWNER_CTX)
    report(g, 'contacts search', ['contacts', 'search', 'a'], OWNER_CTX)

    const first = `Ada${RUN_ID}`
    const add = cli(
        [
            '--yes',
            'contacts',
            'add',
            '--first',
            first,
            '--last',
            'Lovelace',
            '--email',
            `${RUN_ID}@example.com`,
        ],
        OWNER_CTX
    )
    if (!assert(g, 'contacts add', add.ok, firstLine(add.stderr || add.stdout))) {
        skip(g, 'contacts export → import reports updated', 'add failed')
        skip(g, 'contacts rm soft-deletes', 'add failed')
        return
    }

    const listed = cli(['--json', 'contacts', 'list'], OWNER_CTX)
    const rows = parseJSON(listed) as { id?: string; first_name?: string }[] | null
    const mine = Array.isArray(rows) ? rows.find(r => r.first_name === first) : undefined
    if (!assert(g, 'added contact appears in list', !!mine?.id)) {
        return
    }
    const id = mine?.id as string

    report(g, 'contacts show', ['contacts', 'show', id], OWNER_CTX)

    // --phone "" must CLEAR the field rather than be ignored: the flag layer
    // keys on Changed(), not emptiness, and an edit that sent the whole struct
    // would blank every unmentioned field.
    const setPhone = cli(['--yes', 'contacts', 'edit', id, '--phone', '555-0100'], OWNER_CTX)
    assert(g, 'contacts edit sets a field', setPhone.ok, firstLine(setPhone.stderr))
    const clearPhone = cli(['--yes', 'contacts', 'edit', id, '--phone', ''], OWNER_CTX)
    assert(g, 'contacts edit --phone "" clears it', clearPhone.ok, firstLine(clearPhone.stderr))

    // The export→import upsert. Re-importing your own export must UPDATE, not
    // duplicate — the defect here (a globally-unique vCard UID index vs. the
    // per-address-book uniqueness RFC 6350 actually specifies) only reproduces
    // against a real database with the real index.
    const vcf = path.join(scratch, `${RUN_ID}.vcf`)
    const exported = cli(['contacts', 'export', '--out', vcf], OWNER_CTX)
    const wrote =
        exported.ok && fs.existsSync(vcf) && fs.readFileSync(vcf, 'utf-8').includes('BEGIN:VCARD')
    assert(g, 'contacts export writes vCard', wrote, exported.ok ? '' : firstLine(exported.stderr))

    if (wrote) {
        const before = countContacts()
        const imported = cli(['--yes', 'contacts', 'import', vcf], OWNER_CTX)
        const after = countContacts()
        assert(
            g,
            'contacts import re-import does not duplicate',
            imported.ok && after === before,
            imported.ok ? `${before} → ${after}` : firstLine(imported.stderr)
        )
        // The summary goes to STDERR by design — stdout stays clean so the
        // command composes in a pipeline. Read both so this cannot silently
        // pass by matching nothing.
        const summary = `${imported.stderr}${imported.stdout}`
        assert(
            g,
            'contacts import reports updated, not created',
            /\b[1-9]\d* updated/i.test(summary) && !/\b[1-9]\d* created/i.test(summary),
            firstLine(summary)
        )
    } else {
        skip(g, 'contacts import re-import does not duplicate', 'export failed')
        skip(g, 'contacts import reports updated, not created', 'export failed')
    }

    // rm is a SOFT delete. The full round trip: trash → list --trashed →
    // restore → permanent delete.
    const rm = cli(['--yes', 'contacts', 'rm', id], OWNER_CTX)
    assert(g, 'contacts rm', rm.ok, firstLine(rm.stderr))
    const trashed = cli(['--json', 'contacts', 'list', '--trashed'], OWNER_CTX)
    const inTrash = (parseJSON(trashed) as { id?: string }[] | null)?.some(r => r.id === id)
    assert(
        g,
        'contacts rm soft-deletes (appears in --trashed)',
        !!inTrash,
        firstLine(trashed.stderr)
    )

    const restore = cli(['--yes', 'contacts', 'edit', id, '--restore'], OWNER_CTX)
    assert(g, 'contacts edit --restore brings it back', restore.ok, firstLine(restore.stderr))

    const purge = cli(['--yes', 'contacts', 'rm', id, '--permanent'], OWNER_CTX)
    assert(g, 'contacts rm --permanent', purge.ok, firstLine(purge.stderr))
    const gone = !(
        parseJSON(cli(['--json', 'contacts', 'list'], OWNER_CTX)) as { id?: string }[] | null
    )?.some(r => r.id === id)
    assert(g, 'permanently deleted contact is gone', gone)
}

function countContacts(): number {
    const rows = parseJSON(cli(['--json', 'contacts', 'list'], OWNER_CTX))
    return Array.isArray(rows) ? rows.length : -1
}

function exerciseCalendar(): void {
    section('calendar')
    const g = 'calendar'
    report(g, 'calendar list', ['calendar', 'list'], OWNER_CTX)
    report(g, 'calendar agenda', ['calendar', 'agenda'], OWNER_CTX)
    report(g, 'calendar agenda --days 30', ['calendar', 'agenda', '--days', '30'], OWNER_CTX)
    report(
        g,
        'calendar events',
        ['calendar', 'events', '--from', '2020-01-01', '--to', '2030-01-01'],
        OWNER_CTX
    )

    const json = cli(['--json', 'calendar', 'list'], OWNER_CTX)
    const cals = parseJSON(json) as { id?: string; name?: string; role?: string }[] | null
    assert(g, 'calendar list --json parses', Array.isArray(cals), firstLine(json.stderr))

    // ROLE decides whether a write will be accepted, so it must be readable
    // from --json and not only from the table — otherwise a script has to
    // parse columns to answer "which calendars may I write to", which is what
    // this file used to do.
    assert(
        g,
        'calendar list --json carries the ROLE column',
        !!cals?.length && cals.every(cl => !!cl.role),
        cals?.length ? `roles: ${cals.map(cl => cl.role ?? '-').join(', ')}` : 'no calendars seeded'
    )

    // Write to a calendar the caller can actually write to. Read is membership
    // in ANY role, so the first row may well be one the user only views — the
    // seed's first calendar is exactly that — and picking it blindly turns a
    // correct refusal into a failure that looks like a broken `add`.
    const writable = (cals ?? [])
        .filter(cl => cl.role === 'owner' || cl.role === 'editor')
        .map(cl => [cl.name ?? cl.id ?? ''])
    const target = writable[0]?.[0]
    if (!target) {
        for (const name of [
            'calendar add',
            'calendar show',
            'calendar export → import reports updated',
            'calendar rm',
        ]) {
            skip(g, name, 'no calendar this user may write to')
        }
        return
    }

    const title = `Standup ${RUN_ID}`
    const add = cli(
        [
            '--yes',
            'calendar',
            'add',
            '--calendar',
            target,
            '--title',
            title,
            '--start',
            '2026-09-01 09:30',
        ],
        OWNER_CTX
    )
    if (!assert(g, 'calendar add', add.ok, firstLine(add.stderr || add.stdout))) {
        for (const name of [
            'calendar show',
            'calendar export → import reports updated',
            'calendar rm',
        ]) {
            skip(g, name, 'add failed')
        }
        return
    }

    const events = parseJSON(
        cli(
            ['--json', 'calendar', 'events', '--from', '2026-08-01', '--to', '2026-10-01'],
            OWNER_CTX
        )
    ) as { id?: string; title?: string }[] | null
    const created = Array.isArray(events) ? events.find(e => e.title === title) : undefined
    assert(g, 'added event appears in events', !!created?.id)

    if (created?.id) {
        report(g, 'calendar show', ['calendar', 'show', created.id], OWNER_CTX)
    } else {
        skip(g, 'calendar show', 'created event not found')
    }

    // Same upsert contract as contacts, on the other file format.
    const ics = path.join(scratch, `${RUN_ID}.ics`)
    const exported = cli(['calendar', 'export', '--calendar', target, '--out', ics], OWNER_CTX)
    const wrote =
        exported.ok &&
        fs.existsSync(ics) &&
        fs.readFileSync(ics, 'utf-8').includes('BEGIN:VCALENDAR')
    assert(
        g,
        'calendar export writes iCalendar',
        wrote,
        exported.ok ? '' : firstLine(exported.stderr)
    )

    if (wrote) {
        const before = countEvents()
        const imported = cli(['--yes', 'calendar', 'import', '--calendar', target, ics], OWNER_CTX)
        const after = countEvents()
        assert(
            g,
            'calendar import re-import does not duplicate',
            imported.ok && after === before,
            imported.ok ? `${before} → ${after}` : firstLine(imported.stderr)
        )
        const summary = `${imported.stderr}${imported.stdout}`
        assert(
            g,
            'calendar import reports updated, not created',
            /\b[1-9]\d* updated/i.test(summary) && !/\b[1-9]\d* created/i.test(summary),
            firstLine(summary)
        )
    } else {
        skip(g, 'calendar import re-import does not duplicate', 'export failed')
        skip(g, 'calendar import reports updated, not created', 'export failed')
    }

    if (created?.id) {
        const rm = cli(['--yes', 'calendar', 'rm', created.id], OWNER_CTX)
        assert(g, 'calendar rm', rm.ok, firstLine(rm.stderr))
    } else {
        skip(g, 'calendar rm', 'created event not found')
    }
}

function countEvents(): number {
    const rows = parseJSON(
        cli(
            ['--json', 'calendar', 'events', '--from', '2020-01-01', '--to', '2030-01-01'],
            OWNER_CTX
        )
    )
    return Array.isArray(rows) ? rows.length : -1
}

/**
 * text and calc own only their comment collections — the document itself is a
 * drive_item. So both groups need a real file in Drive first, and a failure
 * here is as likely to be the upload as the comment.
 */
function exerciseComments(group: 'text' | 'calc'): void {
    section(group)
    const g = group
    const ext = group === 'text' ? 'md' : 'xlsx'
    const local = path.join(scratch, `${RUN_ID}-${group}.${ext}`)
    fs.writeFileSync(local, `smoke ${RUN_ID}\n`)

    const put = cli(['--yes', 'drive', 'put', local, '/'], OWNER_CTX)
    if (!put.ok) {
        skip(g, `${group} comments --add`, 'drive put failed')
        skip(g, `${group} comments lists the added thread`, 'drive put failed')
        return
    }
    const remote = `/${path.basename(local)}`

    const body = `Note ${RUN_ID}`
    // A calc comment anchors to one cell on one sheet, and BOTH parts are
    // required by the schema. sheet_id is free-form text (the workbook's own
    // sheet identifier), so a fixed value is fine for a file this run created.
    const SHEET = 'Sheet1'
    const addArgs =
        group === 'calc'
            ? ['--yes', 'calc', 'comments', remote, '--add', body, '--cell', 'B7', '--sheet', SHEET]
            : ['--yes', 'text', 'comments', remote, '--add', body]
    const add = cli(addArgs, OWNER_CTX)
    if (!assert(g, `${group} comments --add`, add.ok, firstLine(add.stderr || add.stdout))) {
        skip(g, `${group} comments lists the added thread`, 'add failed')
        if (group === 'calc') skip(g, 'calc CELL renders A1 notation', 'add failed')
        cli(['--yes', 'drive', 'rm', remote], OWNER_CTX)
        return
    }

    const listed = cli([group, 'comments', remote], OWNER_CTX)
    assert(
        g,
        `${group} comments lists the added thread`,
        listed.ok && listed.stdout.includes(body),
        firstLine(listed.stderr || listed.stdout)
    )

    if (group === 'calc') {
        // A calc comment anchors to a cell, stored as row/col integers. The CLI
        // speaks A1 because that is what the app shows; a raw 6/1 leaking into
        // the column means the conversion is not running at the edge.
        assert(
            g,
            'calc CELL renders A1 notation',
            /\bB7\b/.test(listed.stdout),
            firstLine(listed.stdout)
        )

        // A1 is the first cell of every spreadsheet, so if any reference has to
        // work it is this one. It currently FAILS: parseCell stores zero-based
        // row/col while the schema declares `min: 1` on both, so every cell in
        // row 1 and every cell in column A is rejected by the server. AA1 is
        // included because it additionally covers the two-letter column path.
        const wide = cli(
            [
                '--yes',
                'calc',
                'comments',
                remote,
                '--add',
                `Wide ${RUN_ID}`,
                '--cell',
                'AA1',
                '--sheet',
                SHEET,
            ],
            OWNER_CTX
        )
        const wideListed = cli(['calc', 'comments', remote], OWNER_CTX)
        assert(
            g,
            'calc accepts row 1 / column A (AA1)',
            wide.ok && /\bAA1\b/.test(wideListed.stdout),
            wide.ok ? firstLine(wideListed.stdout) : firstLine(wide.stderr)
        )
    }

    const json = cli(['--json', group, 'comments', remote], OWNER_CTX)
    assert(g, `${group} comments --json parses`, parseJSON(json) !== null, firstLine(json.stderr))

    cli(['--yes', 'drive', 'rm', remote], OWNER_CTX)
}

/**
 * Cross-account coverage. Neither of these has ever been observed: no group had
 * been run against a database with more than one user, so contacts' owner
 * scoping and calendar's viewer/editor split were both fake-tested only.
 */
function exerciseCrossAccount(): void {
    section('cross-account')
    const g = 'cross-account'

    const status = cli(['auth', 'status'], COLLAB_CTX)
    if (
        !assert(
            g,
            'collaborator context authenticates',
            status.stdout.includes(COLLAB.email),
            firstLine(status.stderr)
        )
    ) {
        skip(g, "owner's contact is not visible to the collaborator", 'collaborator auth failed')
        skip(g, 'a non-member is refused on calendar add', 'collaborator auth failed')
        return
    }

    // Contacts are owner-scoped. A contact one user creates must not appear in
    // another's address book.
    const first = `Priv${RUN_ID}`
    const add = cli(['--yes', 'contacts', 'add', '--first', first, '--last', 'Owner'], OWNER_CTX)
    if (add.ok) {
        const theirs = cli(['--json', 'contacts', 'list'], COLLAB_CTX)
        // An empty address book is the EXPECTED result here, and it does not
        // serialize as an array — so treat "no rows" as no leak rather than as
        // an unreadable answer. Only a successful command whose rows contain
        // the owner's contact is a leak; a failed command is inconclusive and
        // must not read as a pass.
        const theirRows = parseJSON(theirs)
        const rows = Array.isArray(theirRows) ? (theirRows as { first_name?: string }[]) : []
        assert(
            g,
            "owner's contact is not visible to the collaborator",
            theirs.ok && !rows.some(r => r.first_name === first),
            theirs.ok ? '' : firstLine(theirs.stderr)
        )

        const mine = parseJSON(cli(['--json', 'contacts', 'list'], OWNER_CTX)) as
            | { id?: string; first_name?: string }[]
            | null
        const row = mine?.find(r => r.first_name === first)
        if (row?.id) cli(['--yes', 'contacts', 'rm', row.id, '--permanent'], OWNER_CTX)
    } else {
        skip(g, "owner's contact is not visible to the collaborator", 'contacts add failed')
    }

    // Calendar read is membership in any role; write is owner-or-editor. What a
    // refused write actually LOOKS like has never been observed — the CLI
    // enforces nothing here by design, so this asserts the server's refusal
    // arrives as something a person can act on.
    const ownerCals = parseJSON(cli(['--json', 'calendar', 'list'], OWNER_CTX)) as
        | { id?: string; name?: string }[]
        | null
    const collabCals = parseJSON(cli(['--json', 'calendar', 'list'], COLLAB_CTX)) as
        | { id?: string; name?: string }[]
        | null
    const collabIds = new Set((collabCals ?? []).map(cl => cl.id))
    const notMine = (ownerCals ?? []).find(cl => cl.id && !collabIds.has(cl.id))

    if (!notMine) {
        skip(
            g,
            'a non-member is refused on calendar add',
            'the collaborator can reach every seeded calendar — no negative case'
        )
        return
    }

    const write = cli(
        [
            '--yes',
            'calendar',
            'add',
            '--calendar',
            notMine.id as string,
            '--title',
            `Nope ${RUN_ID}`,
            '--start',
            '2026-09-02 10:00',
        ],
        COLLAB_CTX
    )
    const message = `${write.stderr}${write.stdout}`.trim()
    assert(
        g,
        'a non-member is refused on calendar add',
        !write.ok,
        message ? '' : 'the write SUCCEEDED'
    )
    assert(
        g,
        'the refusal is comprehensible (not a bare status dump)',
        !write.ok && message.length > 0 && !/^Error:\s*(HTTP\s*)?\d+\s*$/i.test(message),
        firstLine(message)
    )
}

// ---------------------------------------------------------------------------
// Report
// ---------------------------------------------------------------------------

function printReport(): number {
    section('coverage')

    const groups = [...new Set(results.map(r => r.group))]
    for (const group of groups) {
        const rows = results.filter(r => r.group === group)
        const pass = rows.filter(r => r.status === 'pass').length
        const fail = rows.filter(r => r.status === 'fail').length
        const skipped = rows.filter(r => r.status === 'skip').length
        const parts = [`${pass} passed`]
        if (fail) parts.push(c.red(`${fail} failed`))
        if (skipped) parts.push(c.yellow(`${skipped} skipped`))
        process.stdout.write(`  ${group.padEnd(16)} ${parts.join(', ')}\n`)
    }

    const failures = results.filter(r => r.status === 'fail')
    const skips = results.filter(r => r.status === 'skip')

    // Naming what did not run matters as much as the failures: a silent skip
    // reads as "covered everything" when the run may have covered very little.
    if (skips.length) {
        section('not run')
        for (const s of skips) {
            process.stdout.write(
                `  ${c.yellow('–')} ${s.group}: ${s.name} ${c.dim(`(${s.detail})`)}\n`
            )
        }
    }

    if (failures.length) {
        section('failures')
        for (const f of failures) {
            process.stdout.write(`  ${c.red('✗')} ${f.group}: ${f.name}\n`)
            if (f.detail) process.stdout.write(`      ${c.dim(f.detail)}\n`)
        }
    }

    const total = results.length
    const passed = results.filter(r => r.status === 'pass').length
    process.stdout.write(
        `\n${c.bold(`${passed}/${total} checks passed`)}` +
            (failures.length ? c.red(`, ${failures.length} failed`) : '') +
            (skips.length ? c.yellow(`, ${skips.length} skipped`) : '') +
            '\n'
    )
    return failures.length > 0 ? 1 : 0
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

async function main(): Promise<void> {
    scratch = fs.mkdtempSync(path.join(os.tmpdir(), 'tinycld-smoke-'))
    CONFIG_DIR = path.join(scratch, 'config')
    fs.mkdirSync(CONFIG_DIR, { recursive: true, mode: 0o700 })

    const origin = OPTS.server
        ? OPTS.server.includes('://')
            ? OPTS.server.replace(/\/$/, '')
            : `http://${OPTS.server}`
        : `http://127.0.0.1:${OPTS.port}`

    try {
        // Before anything boots: the generator rewrites files the server and
        // seed read from disk.
        generatePackages()

        if (OPTS.server) {
            log(`attaching to ${origin} (not resetting the database)`)
            await waitForHealth(origin, 10_000)
            // An attached server still has to carry the fixtures, and a short
            // budget here turns "you pointed at a server seeded some other way"
            // into a clear message rather than a cascade of auth failures.
            await waitForFixtureUser(origin, 15_000)
            seedFinished = true
        } else {
            await bootServer(origin)
        }

        buildCLI()

        log('minting OAuth grants for both fixture users')
        const ownerToken = await mintGrant(origin, OWNER, 'cli-smoke (owner)')
        const collabToken = await mintGrant(origin, COLLAB, 'cli-smoke (collaborator)')
        writeCredentials([
            { name: OWNER_CTX, origin, user: OWNER.email, token: ownerToken },
            { name: COLLAB_CTX, origin, user: COLLAB.email, token: collabToken },
        ])
        log(`granted scopes: ${ownerToken.scope}`)

        const available = registeredGroups()
        const wanted = (g: string): boolean => {
            if (OPTS.only && OPTS.only !== g) return false
            if (!available.has(g)) {
                section(g)
                skip(g, `${g} group`, 'not registered in this binary — package not assembled')
                return false
            }
            return true
        }

        if (!OPTS.only) exerciseGlobal()
        if (wanted('search')) exerciseSearch()
        if (wanted('drive')) exerciseDrive()
        if (wanted('mail')) exerciseMail()
        if (wanted('cards')) exerciseCards()
        if (wanted('contacts')) exerciseContacts()
        if (wanted('calendar')) exerciseCalendar()
        if (wanted('text')) exerciseComments('text')
        if (wanted('calc')) exerciseComments('calc')
        if (!OPTS.only) exerciseCrossAccount()

        const code = printReport()

        if (OPTS.keep && !OPTS.server) {
            log(`server still running on ${origin} — Ctrl+C to stop`)
            log(`CLI config: TINYCLD_CONFIG_DIR=${CONFIG_DIR}`)
            await new Promise<void>(resolve => {
                process.on('SIGINT', () => resolve())
                process.on('SIGTERM', () => resolve())
            })
        }
        process.exitCode = code
    } finally {
        stopServer()
        if (!OPTS.keep) fs.rmSync(scratch, { recursive: true, force: true })
    }
}

main().catch(err => {
    process.stderr.write(`[cli-smoke] ${err instanceof Error ? err.stack : String(err)}\n`)
    stopServer()
    process.exit(1)
})
