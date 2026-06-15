// Dev launcher: spawns the Go PB server + Expo, and runs an HTTP proxy on
// the user-facing port that routes /api and /_ to PB and everything else to
// Expo.
//
// SSL: enabled by default if assets/localhost*.pem are present, disabled
// if absent. Override with --ssl (force on, errors if certs missing) or
// --no-ssl (force off). The proxy listens with TLS when enabled.
//
// Defaults: proxy 7100, PB 7101, Expo 7102. If any port in the block is
// taken we shift the whole block by +10 and re-probe so they stay grouped.
// Override with --port <n> to pin the user-facing port (PB = n+1, Expo =
// n+2); fail fast instead of probing alternatives. --pb-data-dir <path>
// points PB at a non-default data directory (used by the Playwright test
// mode to share state with the seed scripts).
//
// On web, the app always resolves PB at window.location.origin, so the
// dev proxy at the user-facing port handles /api and /_ same-origin. No
// EXPO_PUBLIC_ENV plumbing or build-time URL injection — everything works
// off the page's own origin.
//
// PB Go sources are watched via the same Watchman daemon Metro uses. On
// any .go change under core/server/ or <sibling>/server/ we rebuild the
// PB binary and restart the child. A failed build keeps the running PB
// alive (so you can fix the typo and save again without a broken state).
// Disable with --no-pb-watch.

import { type ChildProcess, spawn, spawnSync } from 'node:child_process'
import * as fs from 'node:fs'
import * as http from 'node:http'
import * as https from 'node:https'
import * as net from 'node:net'
import * as path from 'node:path'
// fb-watchman has no published @types package; declare the surface we use.
import watchman from 'fb-watchman'

const ROOT = path.resolve(import.meta.dirname, '..')
const CERT_PATH = path.join(ROOT, 'assets', 'localhost.pem')
const KEY_PATH = path.join(ROOT, 'assets', 'localhost-key.pem')

const DEFAULT_PROXY_PORT = 7100
const BLOCK_SHIFT = 10
const MAX_BLOCK_ATTEMPTS = 5

// Pull a flag value (`--name value`) out of process.argv. Returns null if
// the flag isn't present; throws if it's the last token (no value follows).
function flagValue(name: string): string | null {
    const i = process.argv.indexOf(name)
    if (i === -1) return null
    const v = process.argv[i + 1]
    if (v === undefined || v.startsWith('-')) {
        throw new Error(`dev: flag ${name} requires a value`)
    }
    return v
}

function resolveExplicitPort(): number | null {
    const raw = flagValue('--port')
    if (raw === null) return null
    const n = Number.parseInt(raw, 10)
    if (!Number.isFinite(n) || n <= 0 || n > 65_533) {
        throw new Error(`dev: --port must be a port number (got ${raw})`)
    }
    return n
}

function resolvePbDataDir(): string | null {
    const raw = flagValue('--pb-data-dir')
    if (raw === null) return null
    return path.isAbsolute(raw) ? raw : path.join(ROOT, raw)
}

// SSL is on when both cert files exist, unless explicitly disabled with
// --no-ssl. --ssl forces it on (and fails if certs are missing).
function resolveUseSsl(): boolean {
    if (process.argv.includes('--no-ssl')) return false
    if (process.argv.includes('--ssl')) {
        if (!fs.existsSync(CERT_PATH) || !fs.existsSync(KEY_PATH)) {
            throw new Error(`--ssl requires ${CERT_PATH} and ${KEY_PATH}; generate with mkcert`)
        }
        return true
    }
    return fs.existsSync(CERT_PATH) && fs.existsSync(KEY_PATH)
}

// --no-expo skips spawning Expo so the dev script runs only PB + proxy.
// Use this to launch Expo yourself in a separate terminal (`npx expo
// start --port <expo>`) when you need Expo's interactive shortcuts
// (`i`, `j`, `r`) or its full log stream — both get muted when Expo is
// spawned with piped stdio because the Expo CLI flips to a quieter
// non-TTY mode. PB and proxy stay on their usual ports; the proxy
// returns 503 for any request targeted at Expo until you start one
// yourself, so the app keeps working as soon as you do.
const skipExpo = process.argv.includes('--no-expo')

// --clear (alias --reset-cache) wipes Metro's transform cache on startup by
// passing `expo start --clear`. OPT-IN: a normal start reuses the persistent
// Metro cache (in $TMPDIR/metro-cache), so the bundler doesn't rebuild from
// scratch — and you don't pay the "Bundler cache is empty, rebuilding" minute —
// every launch. Reach for this only after a dep/transformer change or when
// chasing a stale-cache bug.
const clearCache = process.argv.includes('--clear') || process.argv.includes('--reset-cache')
const skipPbWatch = process.argv.includes('--no-pb-watch')

const useSsl = resolveUseSsl()

interface PortBlock {
    proxy: number
    pb: number
    expo: number
}

async function probePort(port: number): Promise<boolean> {
    return new Promise(resolve => {
        const server = net.createServer()
        server.once('error', () => resolve(false))
        server.once('listening', () => {
            server.close(() => resolve(true))
        })
        server.listen(port, '127.0.0.1')
    })
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
    throw new Error(`dev: ${label} on :${port} did not accept connections within ${timeoutMs}ms`)
}

async function isBlockFree(block: PortBlock): Promise<boolean> {
    const results = await Promise.all([
        probePort(block.proxy),
        probePort(block.pb),
        probePort(block.expo),
    ])
    return results.every(Boolean)
}

function blockAt(i: number): PortBlock {
    const base = DEFAULT_PROXY_PORT + i * BLOCK_SHIFT
    return { proxy: base, pb: base + 1, expo: base + 2 }
}

interface PortHolder {
    pid: number
    command: string
}

function findPortHolder(port: number): PortHolder | null {
    // lsof on macOS: -t prints just PIDs, -nP skips DNS/port-name lookups,
    // -sTCP:LISTEN restricts to the listening socket so we don't catch
    // ephemeral clients.
    const lsof = spawnSync('lsof', ['-t', '-nP', `-iTCP:${port}`, '-sTCP:LISTEN'], {
        encoding: 'utf8',
    })
    const pid = Number.parseInt(lsof.stdout.trim().split('\n')[0] ?? '', 10)
    if (!Number.isFinite(pid) || pid <= 0) return null
    const ps = spawnSync('ps', ['-o', 'command=', '-p', String(pid)], { encoding: 'utf8' })
    return { pid, command: ps.stdout.trim() || '<unknown>' }
}

function prompt(question: string): Promise<string> {
    process.stdout.write(question)
    return new Promise(resolve => {
        const onData = (chunk: Buffer) => {
            process.stdin.removeListener('data', onData)
            process.stdin.pause()
            resolve(chunk.toString('utf8').trim())
        }
        process.stdin.resume()
        process.stdin.once('data', onData)
    })
}

async function waitForPortFree(port: number, timeoutMs = 5_000): Promise<boolean> {
    const start = Date.now()
    while (Date.now() - start < timeoutMs) {
        if (await probePort(port)) return true
        await new Promise(r => setTimeout(r, 100))
    }
    return false
}

async function findFreeBlock(startIndex: number): Promise<PortBlock | null> {
    for (let i = startIndex; i < MAX_BLOCK_ATTEMPTS; i++) {
        const candidate = blockAt(i)
        if (await isBlockFree(candidate)) return candidate
    }
    return null
}

async function pickBlock(): Promise<PortBlock> {
    const explicit = resolveExplicitPort()
    if (explicit !== null) {
        const block: PortBlock = { proxy: explicit, pb: explicit + 1, expo: explicit + 2 }
        if (await isBlockFree(block)) return block
        // Don't fall back when the user pinned a port — they almost certainly
        // want to know that it's busy. List the holders so the failure is
        // actionable.
        const holders = [block.proxy, block.pb, block.expo]
            .map(p => ({ port: p, holder: findPortHolder(p) }))
            .filter(h => h.holder !== null)
        const detail = holders
            .map(h => `:${h.port} pid ${h.holder?.pid} (${h.holder?.command})`)
            .join(', ')
        throw new Error(
            `dev: --port ${explicit} block (${block.proxy}/${block.pb}/${block.expo}) is in use${detail ? ` — ${detail}` : ''}`
        )
    }

    const preferred = blockAt(0)
    if (await isBlockFree(preferred)) return preferred

    // Default block taken — show the user who's holding it and what to do.
    const holder = findPortHolder(preferred.proxy)
    process.stdout.write(`\ndev: port ${preferred.proxy} is in use`)
    if (holder) {
        process.stdout.write(` by pid ${holder.pid} (${holder.command})`)
    }
    process.stdout.write('\n')

    const interactive = process.stdin.isTTY === true
    if (!interactive) {
        const fallback = await findFreeBlock(1)
        if (!fallback) {
            throw new Error(
                `dev: no free port block in range ${DEFAULT_PROXY_PORT}..${DEFAULT_PROXY_PORT + MAX_BLOCK_ATTEMPTS * BLOCK_SHIFT}`
            )
        }
        process.stdout.write(`dev: non-interactive shell, falling back to :${fallback.proxy}\n`)
        return fallback
    }

    const fallback = await findFreeBlock(1)
    const fallbackHint = fallback ? `:${fallback.proxy}` : 'none free'
    const canKill = holder !== null
    const choices = canKill ? '[k]ill, [u]se alternative, [q]uit' : '[u]se alternative, [q]uit'
    const answer = (
        await prompt(`Choose: ${choices} (alternative=${fallbackHint}): `)
    ).toLowerCase()

    if (answer === 'q' || answer === 'quit') {
        throw new Error('dev: aborted by user')
    }
    if (answer === 'k' || answer === 'kill') {
        if (!canKill) throw new Error('dev: no holder to kill (lsof returned nothing)')
        process.stdout.write(`dev: killing pid ${holder.pid}\n`)
        try {
            process.kill(holder.pid, 'SIGTERM')
        } catch (err) {
            throw new Error(
                `dev: failed to signal pid ${holder.pid}: ${err instanceof Error ? err.message : String(err)}`
            )
        }
        if (!(await waitForPortFree(preferred.proxy))) {
            // Escalate to SIGKILL if SIGTERM didn't free the port in time.
            process.stdout.write(
                `dev: pid ${holder.pid} still holding :${preferred.proxy}, sending SIGKILL\n`
            )
            try {
                process.kill(holder.pid, 'SIGKILL')
            } catch {
                // process may already be gone
            }
            if (!(await waitForPortFree(preferred.proxy))) {
                throw new Error(`dev: port ${preferred.proxy} still in use after SIGKILL`)
            }
        }
        if (await isBlockFree(preferred)) return preferred
        // Killing the proxy holder freed :proxy but :pb or :expo is still
        // taken — fall through to alternative.
        process.stdout.write(
            `dev: port ${preferred.proxy} freed, but the rest of the block isn't — using alternative\n`
        )
    }
    // Default branch (including 'u'/'use'/empty input): alternative block.
    if (!fallback) {
        throw new Error(
            `dev: no free port block in range ${DEFAULT_PROXY_PORT}..${DEFAULT_PROXY_PORT + MAX_BLOCK_ATTEMPTS * BLOCK_SHIFT}`
        )
    }
    return fallback
}

function withPrefix(prefix: string, color: string) {
    return (chunk: Buffer | string) => {
        const text = typeof chunk === 'string' ? chunk : chunk.toString('utf8')
        for (const line of text.split('\n')) {
            if (line.length === 0) continue
            process.stdout.write(`${color}[${prefix}]\x1b[0m ${line}\n`)
        }
    }
}

function buildPbSync() {
    const buildResult = spawn('go', ['build', '-o', 'app', '.'], {
        cwd: path.join(ROOT, 'server'),
        stdio: 'inherit',
    })
    return new Promise<void>((resolve, reject) => {
        buildResult.on('exit', code => {
            if (code === 0) resolve()
            else reject(new Error(`go build exited with code ${code}`))
        })
        buildResult.on('error', reject)
    })
}

// Quiet rebuild used by the file watcher — stdio: 'pipe' so we can prefix
// the lines and resolve(false) on non-zero rather than rejecting. Returns
// true iff the build succeeded; false leaves the old binary in place.
function rebuildPb(): Promise<boolean> {
    const onOut = withPrefix('go', '\x1b[34m') // blue
    const onErr = withPrefix('go', '\x1b[31m')
    const child = spawn('go', ['build', '-o', 'app', '.'], {
        cwd: path.join(ROOT, 'server'),
        stdio: ['ignore', 'pipe', 'pipe'],
    })
    child.stdout?.on('data', onOut)
    child.stderr?.on('data', onErr)
    return new Promise(resolve => {
        child.on('exit', code => resolve(code === 0))
        child.on('error', err => {
            onErr(`spawn error: ${err.message}\n`)
            resolve(false)
        })
    })
}

// Watchman subscription. Watches the workspace root (one up from app/) for
// any .go change under <pkg>/server/. The .watchmanconfig at the workspace
// root already excludes node_modules, .git, pb_data, etc., so the subscription
// only fires on real source edits. Returns a disposer.
function startPbWatcher(opts: {
    onChange: (changedFiles: string[]) => void
    onLog: (msg: string) => void
}): () => void {
    const workspaceRoot = path.resolve(ROOT, '..')
    const client = new watchman.Client()
    const subName = `tinycld-dev-pb-${process.pid}`
    let disposed = false

    const dispose = () => {
        if (disposed) return
        disposed = true
        try {
            client.command(['unsubscribe', workspaceRoot, subName], () => {
                client.end()
            })
        } catch {
            client.end()
        }
    }

    client.capabilityCheck({ optional: [], required: ['relative_root'] }, capErr => {
        if (disposed) return
        if (capErr) {
            opts.onLog(`watchman capability check failed: ${capErr.message}`)
            client.end()
            return
        }
        client.command(['watch-project', workspaceRoot], (watchErr, watchResp) => {
            if (disposed) return
            if (watchErr) {
                opts.onLog(`watch-project failed: ${watchErr.message}`)
                client.end()
                return
            }
            const resp = watchResp as { watch: string; relative_path?: string }
            const sub = {
                expression: [
                    'allof',
                    ['type', 'f'],
                    ['suffix', 'go'],
                    // Any package-owned server source: core/server/**.go and
                    // <sibling>/server/**.go. Exclude app/server/** because
                    // those .go files (package_extensions.go, go.work) are
                    // generator output — rebuilding on them would loop.
                    ['match', '**/server/**', 'wholename'],
                    ['not', ['match', 'app/server/**', 'wholename']],
                ],
                fields: ['name', 'exists'],
                ...(resp.relative_path ? { relative_root: resp.relative_path } : {}),
            }
            client.command(['subscribe', resp.watch, subName, sub], subErr => {
                if (disposed) return
                if (subErr) {
                    opts.onLog(`subscribe failed: ${subErr.message}`)
                    client.end()
                }
            })
        })
    })

    client.on('subscription', resp => {
        if (resp.subscription !== subName) return
        // is_fresh_instance is the initial snapshot — not a real change.
        if (resp.is_fresh_instance) return
        const files = (resp.files ?? []).map(f => f.name)
        if (files.length === 0) return
        opts.onChange(files)
    })

    client.on('error', err => {
        opts.onLog(`watchman error: ${err.message}`)
    })

    return dispose
}

function spawnPbBinary(pbPort: number, publicUrl: string, dataDir: string | null): ChildProcess {
    const onPbOut = withPrefix('pb', '\x1b[36m') // cyan
    const onPbErr = withPrefix('pb', '\x1b[31m') // red
    const args = ['--dev', '--http', `127.0.0.1:${pbPort}`]
    if (dataDir) args.push('--dir', dataDir)
    args.push('--typesDir', path.join(ROOT, '..', 'core', 'types'), 'serve')
    // The mail package's IMAP server defaults to :1143 in dev. The Playwright
    // IMAP suite (app/tests/e2e/imap-helpers.ts) connects on :1193 — a port
    // distinct from the normal dev one so an e2e run never collides with a
    // developer's running dev IMAP listener. Only the test invocation passes a
    // dedicated --pb-data-dir, so key the override off that: point the test
    // server's IMAP listener at :1193 where the helper expects it. Without this
    // the mail-imap specs fail with ECONNREFUSED 127.0.0.1:1193.
    const testEnv = dataDir ? { IMAP_ADDR: ':1193' } : {}
    const child = spawn(path.join(ROOT, 'server', 'app'), args, {
        cwd: ROOT,
        stdio: ['inherit', 'pipe', 'pipe'],
        // PB's first-run installer prints a /admin URL using the address
        // it bound to. Override with the public proxy URL so the printed
        // URL points where the user actually browses, not at PB's
        // internal port.
        env: { ...process.env, TINYCLD_PUBLIC_URL: publicUrl, ...testEnv },
    })
    child.stdout?.on('data', onPbOut)
    child.stderr?.on('data', onPbErr)
    return child
}

function spawnExpo(expoPort: number, onReady: () => void): ChildProcess {
    const onOut = withPrefix('expo', '\x1b[35m') // magenta
    const onErr = withPrefix('expo', '\x1b[31m')
    // Stdio is piped so we can prefix Expo's output with [expo]. That
    // already makes Expo treat the run as non-interactive (isTTY=false),
    // which turns off the TUI and gives us plain log lines on stdout.
    //
    // Do NOT set CI=1 here. @expo/cli's instantiateMetro.js gates Metro's
    // file watcher on `env.CI` — CI=true makes Metro log
    // "Metro is running in CI mode, reloads are disabled" and skips
    // watching entirely, which breaks HMR.
    // Give Metro a larger V8 heap. The full ecosystem bundle (every member's
    // screens + sidebars) can push the transform/serializer past Node's default
    // ~4GB old-space ceiling on a cold `--clear` start — it OOM'd mid-bundle
    // ("Reached heap limit Allocation failed") while transforming a sidebar
    // chunk. The EAS production build already runs with 8192; match it here so
    // dev doesn't crash on machines where the default limit is lower than the
    // bundle needs. Preserve any caller-set NODE_OPTIONS by appending.
    const existingNodeOptions = process.env.NODE_OPTIONS ?? ''
    const heapFlag = '--max-old-space-size=8192'
    const nodeOptions = existingNodeOptions.includes('--max-old-space-size')
        ? existingNodeOptions
        : `${existingNodeOptions} ${heapFlag}`.trim()
    // --clear is opt-in (see `clearCache` above): omit it so Metro reuses its
    // persistent cache across restarts. Pass --clear/--reset-cache to the dev
    // script to force a cold rebuild.
    const expoArgs = ['expo', 'start', '--port', String(expoPort)]
    if (clearCache) expoArgs.splice(2, 0, '--clear')
    const child = spawn('npx', expoArgs, {
        cwd: ROOT,
        stdio: ['inherit', 'pipe', 'pipe'],
        env: { ...process.env, NODE_OPTIONS: nodeOptions },
    })
    // Expo prints "Logs for your project will appear …" once the dev server
    // is fully up. We use that as the signal to print the banner so it lands
    // at the bottom of the noisy startup output instead of being buried.
    let readyFired = false
    const watchForReady = (chunk: Buffer | string) => {
        const text = typeof chunk === 'string' ? chunk : chunk.toString('utf8')
        if (!readyFired && text.includes('Logs for your project will appear')) {
            readyFired = true
            onReady()
        }
    }
    child.stdout?.on('data', chunk => {
        watchForReady(chunk)
        onOut(chunk)
    })
    child.stderr?.on('data', onErr)
    return child
}

// Path segments that belong to PocketBase, not Expo. Each entry matches
// the segment exactly or as a path prefix (i.e. followed by '/'), so
// '/api' catches '/api' and '/api/foo' but not '/apiv2'. Mirrors the
// Go-side list in coreserver/server.go::isDavPath plus PB's own /api and
// /_ surfaces so the same set bypasses Expo when tests hit them via the
// proxy.
//   /api                  REST + SSE realtime + custom routes
//   /_                    admin UI
//   /caldav               calendar package's CalDAV handler
//   /carddav              contacts package's CardDAV handler
//   /drive                drive package's WebDAV handler (the in-app
//                         /a/.../drive routes are caught by /a, which
//                         Expo owns)
//   /.well-known/caldav   service discovery (302s)
//   /.well-known/carddav
//   /.well-known/webdav
const PB_PREFIXES = [
    '/api',
    '/_',
    '/caldav',
    '/carddav',
    '/drive',
    '/.well-known/caldav',
    '/.well-known/carddav',
    '/.well-known/webdav',
]

// Metro serves lazily-imported package chunks (React.lazy / dynamic import)
// from a URL derived from the module's path, e.g. a lazy
// `import('@tinycld/drive/sidebar')` is fetched as
// `/drive/tinycld/drive/sidebar.bundle?platform=web&...`. That path starts with
// `/drive`, which would otherwise route to PB's WebDAV handler (→ "Authentication
// required") and crash the lazy load. Metro bundle/source-map requests are
// unambiguously Expo's regardless of prefix, so they must never go to PB. A
// real WebDAV/CalDAV/CardDAV request never targets a `.bundle`/`.map` path.
function isMetroAssetPath(pathname: string): boolean {
    return pathname.endsWith('.bundle') || pathname.endsWith('.map')
}

function isPbPath(url: string): boolean {
    const pathname = url.split('?', 1)[0]
    if (isMetroAssetPath(pathname)) return false
    return PB_PREFIXES.some(prefix => pathname === prefix || pathname.startsWith(`${prefix}/`))
}

function startProxy(opts: { proxyPort: number; pbPort: number; expoPort: number; ssl: boolean }) {
    const log = withPrefix('proxy', '\x1b[33m') // yellow

    const handler: http.RequestListener = (req, res) => {
        const target = isPbPath(req.url ?? '/') ? opts.pbPort : opts.expoPort

        // Browsers and Playwright cancel in-flight requests aggressively.
        // Once the client socket goes away, any further write to req/res
        // surfaces as EPIPE/ECONNRESET on the underlying socket — and a bare
        // 'error' event on a Socket crashes Node. Attach handlers to swallow
        // those instead of taking the proxy down.
        req.on('error', err => log(`client req error: ${err.message}\n`))
        res.on('error', err => log(`client res error: ${err.message}\n`))

        const upstream = http.request(
            {
                host: '127.0.0.1',
                port: target,
                method: req.method,
                path: req.url,
                headers: { ...req.headers, host: `127.0.0.1:${target}` },
            },
            upstreamRes => {
                upstreamRes.on('error', err =>
                    log(`upstream res :${target} error: ${err.message}\n`)
                )
                if (!res.writableEnded)
                    res.writeHead(upstreamRes.statusCode ?? 502, upstreamRes.headers)
                upstreamRes.pipe(res)
            }
        )
        upstream.on('error', err => {
            log(`upstream :${target} error: ${err.message}\n`)
            if (!res.headersSent && !res.writableEnded) res.writeHead(502)
            if (!res.writableEnded) res.end('Bad Gateway')
        })
        // If the client disconnects mid-flight, abort the upstream so we
        // don't pile up half-open sockets to PB/Expo.
        res.on('close', () => upstream.destroy())
        req.pipe(upstream)
    }

    // WebSocket upgrades — Expo dev server uses WS for HMR. PB doesn't use WS
    // (its realtime is SSE, plain HTTP), but route by path anyway.
    const onUpgrade = (req: http.IncomingMessage, clientSocket: NodeJS.Socket, head: Buffer) => {
        const target = isPbPath(req.url ?? '/') ? opts.pbPort : opts.expoPort
        // Same EPIPE-class crash protection on the upgrade path.
        clientSocket.on('error', err => log(`client ws error: ${err.message}\n`))
        const upstreamSocket = net.connect(target, '127.0.0.1', () => {
            const headerLines = [
                `${req.method} ${req.url} HTTP/${req.httpVersion}`,
                ...Object.entries(req.headers).map(
                    ([k, v]) => `${k}: ${Array.isArray(v) ? v.join(', ') : v}`
                ),
                '',
                '',
            ]
            upstreamSocket.write(headerLines.join('\r\n'))
            if (head.length > 0) upstreamSocket.write(head)
            upstreamSocket.pipe(clientSocket as unknown as NodeJS.WritableStream)
            ;(clientSocket as unknown as NodeJS.ReadableStream).pipe(upstreamSocket)
        })
        upstreamSocket.on('error', err => {
            log(`upgrade upstream :${target} error: ${err.message}\n`)
            ;(clientSocket as unknown as { destroy: () => void }).destroy()
        })
    }

    let server: http.Server | https.Server
    if (opts.ssl) {
        server = https.createServer(
            { cert: fs.readFileSync(CERT_PATH), key: fs.readFileSync(KEY_PATH) },
            handler
        )
    } else {
        server = http.createServer(handler)
    }
    server.on('upgrade', onUpgrade)
    server.listen(opts.proxyPort, '0.0.0.0')
    return server
}

function printBanner(block: PortBlock, ssl: boolean, suffix?: string) {
    const scheme = ssl ? 'https' : 'http'
    const lines = [
        '',
        `\x1b[1m  TinyCld dev\x1b[0m`,
        `  Open: \x1b[32m${scheme}://localhost:${block.proxy}\x1b[0m`,
        `  PB:    127.0.0.1:${block.pb} (proxied at /api, /_)`,
        `  Expo:  127.0.0.1:${block.expo}`,
    ]
    if (suffix) lines.push(`  \x1b[33m${suffix}\x1b[0m`)
    lines.push('')
    process.stdout.write(`${lines.join('\n')}\n`)
}

// Belt-and-suspenders: if any socket-class error escapes handler-level
// catch blocks, log and continue rather than letting Node tear down the
// whole proxy. EPIPE / ECONNRESET / ECONNABORTED happen routinely when a
// client (browser, Playwright) cancels a request mid-flight; they're
// transient by definition and there's nothing for us to do about them.
const TRANSIENT_SOCKET_ERRORS = new Set(['EPIPE', 'ECONNRESET', 'ECONNABORTED'])
process.on('uncaughtException', err => {
    const code = (err as NodeJS.ErrnoException).code
    if (code && TRANSIENT_SOCKET_ERRORS.has(code)) {
        process.stdout.write(`\x1b[33m[proxy]\x1b[0m transient socket ${code}, ignoring\n`)
        return
    }
    process.stderr.write(`uncaughtException: ${err.stack ?? String(err)}\n`)
    process.exit(1)
})

async function main() {
    const block = await pickBlock()

    // Run packages:generate before launching, mirroring the old `predev`
    // hook. Failing this should fail-fast so the user sees the error.
    // packages:generate also runs each linked package's one-shot build
    // script (e.g. text's webview-editor bundle), so the artifacts are
    // already on disk by the time Expo starts bundling.
    const gen = spawn('npx', ['tsx', 'scripts/generate.ts'], {
        cwd: ROOT,
        stdio: 'inherit',
    })
    await new Promise<void>((resolve, reject) => {
        gen.on('exit', code =>
            code === 0 ? resolve() : reject(new Error(`packages:generate exited ${code}`))
        )
        gen.on('error', reject)
    })

    await buildPbSync()

    // Defer the banner until Expo signals ready (or a 60s fallback fires)
    // so it doesn't get buried under bundling/migration logs.
    let bannerPrinted = false
    const printOnce = (suffix?: string) => {
        if (bannerPrinted) return
        bannerPrinted = true
        printBanner(block, useSsl, suffix)
    }

    const publicUrl = `${useSsl ? 'https' : 'http'}://localhost:${block.proxy}`
    const pbDataDir = resolvePbDataDir()

    // The PB child is swapped out on rebuild; hold a mutable ref so the
    // watcher + shutdown handler always target the current process.
    const pbRef: { current: ChildProcess } = {
        current: spawnPbBinary(block.pb, publicUrl, pbDataDir),
    }
    // With --no-expo we never spawn the Expo child; the user runs it in
    // a separate terminal so they get its TTY-bound logs + shortcuts.
    // The proxy still routes non-PB paths to block.expo, so once their
    // standalone `npx expo start --port <block.expo>` is up, everything
    // works exactly as if dev.ts had spawned Expo itself.
    const expo = skipExpo ? null : spawnExpo(block.expo, () => printOnce())

    // Wait for both upstreams to accept TCP connections before binding the
    // user-facing proxy. Otherwise the browser hits the proxy first, the
    // proxy forwards to a port that's not listening yet, and the user sees
    // 502s + ECONNREFUSED noise during cold start. A few seconds of
    // "connection refused" on :proxy is fine — browsers retry, and the
    // banner only prints once Expo signals ready anyway.
    // PB starts in seconds (no bundling). Expo can take 2-3 minutes on a
    // cold --clear start while it bundles the entire dependency graph, so
    // give it a longer rope.
    const upstreamWaits: Promise<void>[] = [waitForUpstream(block.pb, 'pb', 30_000)]
    if (!skipExpo) {
        upstreamWaits.push(waitForUpstream(block.expo, 'expo', 180_000))
    }
    await Promise.all(upstreamWaits)

    const server = startProxy({
        proxyPort: block.proxy,
        pbPort: block.pb,
        expoPort: block.expo,
        ssl: useSsl,
    })

    if (skipExpo) {
        // No Expo child means no 'ready' signal to gate the banner on —
        // print immediately so the user sees the proxy URL and knows to
        // start Expo themselves in another terminal.
        printOnce(`(--no-expo: run \`npx expo start --port ${block.expo}\` in another terminal)`)
    } else {
        const fallbackTimer: NodeJS.Timeout = setTimeout(() => {
            printOnce('(still bundling — open the URL once Expo is ready)')
        }, 60_000) as unknown as NodeJS.Timeout
        fallbackTimer.unref()
    }

    let shuttingDown = false
    let disposeWatcher: (() => void) | null = null
    // Tags the exact PB child we kill deliberately (planned restart or
    // shutdown) so its asynchronously-delivered 'exit' isn't misread as a
    // crash. See attachPbExitHandler below for why a per-child tag beats
    // gating on the mutable pbRef.
    const intentionallyKilled = new WeakSet<ChildProcess>()
    const shutdown = (signal: NodeJS.Signals) => {
        if (shuttingDown) return
        shuttingDown = true
        process.stdout.write(`\nshutting down (${signal})\n`)
        disposeWatcher?.()
        server.close()
        intentionallyKilled.add(pbRef.current)
        pbRef.current.kill('SIGTERM')
        expo?.kill('SIGTERM')
        // give children a moment, then exit
        ;(
            setTimeout(() => {
                process.exit(0)
            }, 500) as unknown as NodeJS.Timeout
        ).unref()
    }

    process.on('SIGINT', shutdown)
    process.on('SIGTERM', shutdown)

    // If either child dies on its own, tear down the others. With
    // --no-expo we don't own the Expo process, so its lifetime is the
    // user's concern — we only watch pb here.
    //
    // Planned restarts (triggered by the watcher) and shutdown both kill PB
    // deliberately; we must not treat those exits as crashes. We can't gate
    // on pbRef.current because the old child's 'exit' is delivered
    // asynchronously — by the time the OS reports it, the watcher has
    // already reassigned pbRef.current to the freshly-spawned child, so a
    // `child === pbRef.current` guard would misfire. Instead the killer tags
    // the exact child in intentionallyKilled the instant it signals it; this
    // handler (closed over that child) reads the tag with no dependency on
    // later mutations of pbRef.
    const attachPbExitHandler = (child: ChildProcess) => {
        child.on('exit', code => {
            if (intentionallyKilled.has(child)) return
            process.stdout.write(`pb exited (${code}); shutting down\n`)
            shutdown('SIGTERM')
        })
    }
    attachPbExitHandler(pbRef.current)
    expo?.on('exit', code => {
        if (shuttingDown) return
        process.stdout.write(`expo exited (${code}); shutting down\n`)
        shutdown('SIGTERM')
    })

    if (!skipPbWatch) {
        const watcherLog = withPrefix('pb-watch', '\x1b[34m')
        let pending: NodeJS.Timeout | null = null
        let inFlight = false
        let queued = false

        const doRestart = async () => {
            if (inFlight) {
                queued = true
                return
            }
            inFlight = true
            try {
                watcherLog('rebuilding…\n')
                const t0 = Date.now()
                const ok = await rebuildPb()
                const dt = Date.now() - t0
                if (!ok) {
                    watcherLog(
                        `build failed after ${dt}ms — keeping running pb (fix and save again)\n`
                    )
                    return
                }
                watcherLog(`rebuilt in ${dt}ms, restarting pb\n`)
                const old = pbRef.current
                intentionallyKilled.add(old)
                old.kill('SIGTERM')
                // Wait for the port to actually free up before respawning;
                // otherwise the new PB races the old one and EADDRINUSE.
                if (!(await waitForPortFree(block.pb, 5_000))) {
                    watcherLog(`pb did not release :${block.pb}, sending SIGKILL\n`)
                    old.kill('SIGKILL')
                    if (!(await waitForPortFree(block.pb, 2_000))) {
                        watcherLog(`pb still holding :${block.pb}; giving up this restart\n`)
                        return
                    }
                }
                pbRef.current = spawnPbBinary(block.pb, publicUrl, pbDataDir)
                attachPbExitHandler(pbRef.current)
                try {
                    await waitForUpstream(block.pb, 'pb', 30_000)
                    watcherLog('pb back up\n')
                } catch (err) {
                    watcherLog(
                        `pb failed to accept connections: ${err instanceof Error ? err.message : String(err)}\n`
                    )
                }
            } finally {
                inFlight = false
                if (queued) {
                    queued = false
                    void doRestart()
                }
            }
        }

        disposeWatcher = startPbWatcher({
            onChange: files => {
                // Debounce: bursty saves (formatter, git checkout) collapse
                // to one rebuild. 300ms is short enough to feel instant.
                watcherLog(
                    `changed: ${files.slice(0, 3).join(', ')}${files.length > 3 ? ` (+${files.length - 3} more)` : ''}\n`
                )
                if (pending) clearTimeout(pending)
                pending = setTimeout(() => {
                    pending = null
                    void doRestart()
                }, 300)
            },
            onLog: msg => watcherLog(`${msg}\n`),
        })
    }
}

void main().catch(err => {
    process.stderr.write(`dev: ${err instanceof Error ? err.stack : String(err)}\n`)
    process.exit(1)
})
