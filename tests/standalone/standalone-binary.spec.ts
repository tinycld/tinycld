import { execFileSync, spawn } from 'node:child_process'
import { existsSync, mkdtempSync } from 'node:fs'
import { createServer } from 'node:net'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { expect, test } from '@playwright/test'

/**
 * Boots the SHIPPED artifact in an empty directory with no workspace around it.
 *
 * This is the check that the web bundle and the migrations are genuinely INSIDE
 * the binary. A build that reads them from disk passes every unit test and fails
 * here, and so does one whose embed was dropped by the linker — that binary
 * boots and serves, just with no web UI.
 */
const serverDir = join(import.meta.dirname, '..', '..', 'server')
const binary = join(tmpdir(), 'tinycld-standalone-e2e')

/** An OS-assigned free port; a random one in a fixed range can collide. */
const freePort = () =>
    new Promise<number>((resolve, reject) => {
        const srv = createServer()
        srv.on('error', reject)
        srv.listen(0, '127.0.0.1', () => {
            const addr = srv.address()
            if (addr === null || typeof addr === 'string') {
                reject(new Error('could not resolve a free port'))
                return
            }
            const { port } = addr
            srv.close(() => resolve(port))
        })
    })

let proc: ReturnType<typeof spawn>
let baseURL: string
let dataDir: string
let log = ''

// One server for the whole file. Each boot applies every embedded migration,
// so booting per test doubles the slowest part of the run for no extra coverage.
test.beforeAll(async () => {
    execFileSync(
        'go',
        ['build', '-tags', 'embedassets', '-trimpath', '-ldflags=-s -w', '-o', binary, '.'],
        { cwd: serverDir, env: { ...process.env, CGO_ENABLED: '0' }, stdio: 'inherit' }
    )

    dataDir = mkdtempSync(join(tmpdir(), 'tinycld-standalone-'))
    const port = await freePort()
    baseURL = `http://127.0.0.1:${port}`

    // cwd is deliberately NOT the workspace: no dist/, no pb_migrations/, no
    // public/ anywhere above it, so anything served can only come from inside
    // the binary.
    proc = spawn(
        binary,
        ['serve', '--dir', join(dataDir, 'pb_data'), '--http', `127.0.0.1:${port}`],
        { cwd: dataDir, stdio: ['ignore', 'pipe', 'pipe'] }
    )
    proc.stdout?.on('data', c => {
        log += String(c)
    })
    proc.stderr?.on('data', c => {
        log += String(c)
    })

    const exited = new Promise<never>((_, reject) => {
        proc.on('exit', code => reject(new Error(`binary exited early (${code}):\n${log}`)))
    })

    await Promise.race([
        exited,
        expect
            .poll(
                async () => {
                    try {
                        return (await fetch(`${baseURL}/api/health`)).status
                    } catch {
                        return 0
                    }
                },
                { timeout: 180_000, intervals: [500] }
            )
            .toBe(200),
    ])
})

test.afterAll(() => {
    proc?.kill('SIGTERM')
})

test('serves the app and applies migrations from embedded assets in a clean directory', async ({
    page,
}) => {
    // The SPA shell, served from the embedded bundle.
    const shell = await fetch(baseURL)
    expect(shell.status).toBe(200)
    const html = await shell.text()
    expect(html).toContain('<div id="root">')

    // Every hashed JS chunk must resolve. A shell whose scripts 404 still
    // renders #root, so asserting on the shell alone would miss a blank app —
    // which is exactly what the asset-pool routes once caused here.
    const scripts = [...html.matchAll(/src="([^"]+\.js)"/g)].map(m => m[1])
    expect(scripts.length).toBeGreaterThan(0)
    for (const src of scripts) {
        const chunk = await fetch(new URL(src, baseURL).href)
        expect(chunk.status, `chunk ${src}`).toBe(200)
        expect(Number(chunk.headers.get('content-length') ?? 0), `chunk ${src}`).toBeGreaterThan(0)
    }

    // Migrations ran from the embedded FS — without them the API has no
    // collections and this 404s.
    const collections = await fetch(`${baseURL}/api/collections/users/records`)
    expect(collections.status).not.toBe(404)

    // The app actually renders in a browser.
    await page.goto(baseURL)
    await expect(page.locator('#root')).toBeAttached()

    expect(existsSync(join(dataDir, 'pb_data', 'data.db'))).toBe(true)
})

test('does not expose the package rebuild API', async () => {
    // A single binary cannot rebuild itself, so the endpoint must not be routed.
    // An unauthenticated request to a REGISTERED route would be rejected by its
    // owner guard (401/403) rather than reported missing.
    const res = await fetch(`${baseURL}/api/admin/packages/status/mail`)
    expect(res.status).toBe(404)
})
