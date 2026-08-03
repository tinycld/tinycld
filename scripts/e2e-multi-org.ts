// Launcher for the MULTI-ORG e2e stack: two independent tenant backends behind
// the real front router, reachable as two subdomains of `localhost`.
//
// WHY a second launcher rather than a flag on e2e-serve.ts: the switcher under
// test is about moving BETWEEN origins, and the single-origin stack cannot
// produce a second one. Two orgs on the hosted router are two subdomains, two
// processes and two DBs — so the harness reproduces exactly that, minus the
// parts irrelevant to the UI (provisioning, artifact builds, process
// supervision), which the Go-side hosted e2e already covers.
//
// FLOW:
//   1. Seed two separate data dirs (reset-dev-db.ts, once per org).
//   2. Reuse the web bundle e2e-serve.ts already exported into
//      server/pb_test_releases — both orgs serve the SAME bundle, which is the
//      honest shape for this test: the app is identical, only the origin differs.
//   3. Launch one PB per org on its own private port.
//   4. Launch multi-org's cmd/e2e-router in front, dispatching
//      acme.localhost / globex.localhost to those backends.
//
// Chrome resolves *.localhost to loopback natively, so no DNS or TLS setup and
// no /etc/hosts entries are needed.
//
// Used as the webServer of playwright.multi-org.config.ts.

import { type ChildProcess, spawn } from 'node:child_process'
import * as fs from 'node:fs'
import * as net from 'node:net'
import * as path from 'node:path'

const ROOT = path.resolve(import.meta.dirname, '..')
const MULTI_ORG_DIR = path.resolve(ROOT, '..', 'multi-org')
const PB_BINARY = path.join(ROOT, 'server', 'app')
const RELEASES_DIR = path.join(ROOT, 'server', 'pb_test_releases')

// Kept in sync with playwright.multi-org.config.ts.
const ROUTER_PORT = Number(process.env.E2E_MULTI_ORG_PORT ?? 7300)
const ORGS = [
    { slug: 'acme', port: 7301, seedPort: 7391 },
    { slug: 'globex', port: 7302, seedPort: 7392 },
]

const children: ChildProcess[] = []

function log(msg: string) {
    console.log(`[e2e-multi-org] ${msg}`)
}

function run(cmd: string, args: string[], cwd = ROOT): Promise<void> {
    return new Promise((resolve, reject) => {
        const child = spawn(cmd, args, { cwd, stdio: 'inherit', env: process.env })
        child.on('exit', code =>
            code === 0 ? resolve() : reject(new Error(`${cmd} ${args.join(' ')} exited ${code}`))
        )
        child.on('error', reject)
    })
}

async function waitForPort(port: number, label: string, timeoutMs = 60_000): Promise<void> {
    const deadline = Date.now() + timeoutMs
    while (Date.now() < deadline) {
        const ok = await new Promise<boolean>(resolve => {
            const sock = net.connect({ port, host: '127.0.0.1' }, () => {
                sock.end()
                resolve(true)
            })
            sock.on('error', () => resolve(false))
            sock.setTimeout(1000, () => {
                sock.destroy()
                resolve(false)
            })
        })
        if (ok) return
        await new Promise(r => setTimeout(r, 250))
    }
    throw new Error(`${label} did not accept connections on :${port} within ${timeoutMs}ms`)
}

// Each org gets its own data dir, so the two really are separate databases —
// the property that makes "signed into A but not B" reproducible.
async function seedOrg(slug: string, seedPort: number): Promise<string> {
    const dataDir = path.join('server', `pb_test_data_${slug}`)
    log(`seeding ${slug} → ${dataDir}`)
    await run('npx', [
        'tsx',
        'scripts/reset-dev-db.ts',
        '--url',
        `http://127.0.0.1:${seedPort}`,
        '--data-dir',
        dataDir,
    ])
    return dataDir
}

function serveOrg(slug: string, dataDir: string, port: number): ChildProcess {
    log(`serving ${slug} on :${port}`)
    const child = spawn(
        PB_BINARY,
        [
            '--dev',
            '--http',
            `127.0.0.1:${port}`,
            '--dir',
            dataDir,
            '--releasesDir',
            RELEASES_DIR,
            '--migrationsDir',
            path.join(ROOT, 'server', 'pb_migrations'),
            '--publicDir',
            path.join(ROOT, 'public'),
            '--fallbackFile',
            'app.html',
            'serve',
        ],
        { cwd: ROOT, stdio: 'inherit', env: { ...process.env } }
    )
    children.push(child)
    return child
}

function serveRouter(): ChildProcess {
    const orgArgs = ORGS.flatMap(o => ['--org', `${o.slug}=http://127.0.0.1:${o.port}`])
    log(`routing :${ROUTER_PORT} → ${ORGS.map(o => `${o.slug}.localhost`).join(', ')}`)
    const child = spawn(
        'go',
        ['run', './cmd/e2e-router', '--addr', `127.0.0.1:${ROUTER_PORT}`, ...orgArgs],
        { cwd: MULTI_ORG_DIR, stdio: 'inherit', env: process.env }
    )
    children.push(child)
    return child
}

function shutdown() {
    for (const child of children) child.kill('SIGTERM')
}

async function main() {
    if (!fs.existsSync(PB_BINARY)) {
        throw new Error(
            `PocketBase binary missing at ${PB_BINARY}. Run the single-origin e2e once first ` +
                `(pnpm run e2e:serve) so the binary and the exported bundle exist.`
        )
    }
    // Export + promote unless explicitly told to reuse. Skipping this by default
    // would silently serve whatever bundle happened to be promoted last, so a
    // source change would be tested against stale JS and pass for the wrong
    // reason — which is exactly what happened while developing this harness.
    // --skip-export is for tight local iteration when you KNOW the bundle is
    // current; CI must never set it.
    const skipExport =
        process.argv.includes('--skip-export') || process.env.TINYCLD_E2E_SKIP_EXPORT === '1'
    if (skipExport) {
        if (!fs.existsSync(path.join(RELEASES_DIR, 'current'))) {
            throw new Error(
                `--skip-export given but no promoted bundle at ${RELEASES_DIR}/current.`
            )
        }
        log('skipping export (--skip-export): serving the previously promoted bundle')
    } else {
        log('exporting + promoting the web bundle')
        const { exportWeb } = await import('./export-web')
        const { promoteRelease } = await import('./promote-release')
        exportWeb()
        promoteRelease(path.join(ROOT, 'dist'), RELEASES_DIR, `multi-org-${Date.now()}`, log)
    }

    process.on('SIGINT', () => {
        shutdown()
        process.exit(0)
    })
    process.on('SIGTERM', () => {
        shutdown()
        process.exit(0)
    })

    // Seeded serially: reset-dev-db.ts builds PB on first run, and two
    // concurrent builds race over the same output path.
    for (const org of ORGS) {
        const dataDir = await seedOrg(org.slug, org.seedPort)
        serveOrg(org.slug, dataDir, org.port)
        await waitForPort(org.port, `${org.slug} PB`)
    }

    serveRouter()
    await waitForPort(ROUTER_PORT, 'router')
    log(`ready: http://${ORGS[0].slug}.localhost:${ROUTER_PORT}`)
}

main().catch(err => {
    console.error(err)
    shutdown()
    process.exit(1)
})
