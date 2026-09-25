import { execFileSync, spawn } from 'node:child_process'
import { mkdtempSync } from 'node:fs'
import { createServer } from 'node:net'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { expect } from '@playwright/test'

/**
 * Shared boot machinery for the standalone-binary suite: build the SHIPPED
 * artifact once, then spawn it against an empty (or reused) data dir with no
 * workspace around it — same reasoning as standalone-binary.spec.ts, which
 * this was extracted from so the first-run wizard spec can reuse it to boot
 * and restart its own servers.
 */

const serverDir = join(import.meta.dirname, '..', '..', 'server')
const binary = join(tmpdir(), 'tinycld-standalone-e2e')

/** Builds the shipped binary once; call from `beforeAll`. */
export function buildBinary(): void {
    execFileSync(
        'go',
        ['build', '-tags', 'embedassets', '-trimpath', '-ldflags=-s -w', '-o', binary, '.'],
        { cwd: serverDir, env: { ...process.env, CGO_ENABLED: '0' }, stdio: 'inherit' }
    )
}

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

/** The most recently printed setup code, or null if none has printed yet. */
export function setupCodeFromLog(log: string): string | null {
    const matches = [...log.matchAll(/code=([A-Z0-9]{8})/g)]
    return matches.at(-1)?.[1] ?? null
}

export interface BootedServer {
    baseURL: string
    dataDir: string
    logs: () => string
    stop: () => Promise<void>
}

/**
 * Boots the built binary against `opts.dataDir` (a fresh temp dir when
 * omitted) and waits for /api/health. Reusing a dataDir across two boots
 * lets a test observe state that survives a restart.
 */
export async function bootBinary(opts?: { dataDir?: string }): Promise<BootedServer> {
    const dataDir = opts?.dataDir ?? mkdtempSync(join(tmpdir(), 'tinycld-standalone-'))
    const port = await freePort()
    const baseURL = `http://127.0.0.1:${port}`
    let log = ''

    // cwd is deliberately NOT the workspace: no dist/, no pb_migrations/, no
    // public/ anywhere above it, so anything served can only come from inside
    // the binary.
    const proc = spawn(
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

    return {
        baseURL,
        dataDir,
        logs: () => log,
        stop: () =>
            new Promise<void>(resolve => {
                if (proc.exitCode !== null || proc.signalCode !== null) {
                    resolve()
                    return
                }
                proc.on('exit', () => resolve())
                proc.kill('SIGTERM')
            }),
    }
}
