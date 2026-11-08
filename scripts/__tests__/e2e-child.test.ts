import { type ChildProcess, spawn } from 'node:child_process'
import { afterEach, describe, expect, it } from 'vitest'
import { stopChildOnFailure } from '../e2e-child'

// A child that runs until it is killed, as PocketBase does.
const IDLE = 'setInterval(() => {}, 1000)'
// A child that starts an idle grandchild in its own process group (it is
// spawned detached) and prints the grandchild's pid.
const PARENT_OF_IDLE = `
const { spawn } = require('node:child_process')
const g = spawn(process.execPath, ['-e', ${JSON.stringify(IDLE)}], { stdio: 'ignore' })
process.stdout.write(String(g.pid) + '\\n')
setInterval(() => {}, 1000)
`

// A child that ignores SIGTERM, as a hung process may; it prints a line once
// the handler is in place.
const IGNORES_SIGTERM = `
process.on('SIGTERM', () => {})
process.stdout.write('ready\\n')
setInterval(() => {}, 1000)
`

const pids: number[] = []

function isAlive(pid: number): boolean {
    try {
        process.kill(pid, 0)
        return true
    } catch {
        return false
    }
}

function hasExited(child: ChildProcess): boolean {
    return child.exitCode !== null || child.signalCode !== null
}

function spawnIdle(): ChildProcess {
    const child = spawn(process.execPath, ['-e', IDLE], { stdio: 'ignore' })
    if (child.pid) pids.push(child.pid)
    return child
}

function readFirstLine(child: ChildProcess): Promise<string> {
    return new Promise((resolve, reject) => {
        let text = ''
        child.stdout?.on('data', chunk => {
            text += String(chunk)
            const end = text.indexOf('\n')
            if (end !== -1) resolve(text.slice(0, end))
        })
        child.once('error', reject)
    })
}

afterEach(() => {
    for (const pid of pids.splice(0)) {
        if (isAlive(pid)) process.kill(pid, 'SIGKILL')
    }
})

describe('stopChildOnFailure', () => {
    it('stops the child before the failed wait rejects, and rethrows its error', async () => {
        const child = spawnIdle()
        const failure = new Error('pb on :7220 did not accept connections within 60000ms')

        await expect(stopChildOnFailure(child, () => Promise.reject(failure))).rejects.toBe(failure)

        expect(hasExited(child)).toBe(true)
        expect(isAlive(child.pid as number)).toBe(false)
    })

    it('kills a child that ignores SIGTERM after the grace time, and rethrows its error', async () => {
        const child = spawn(process.execPath, ['-e', IGNORES_SIGTERM], {
            stdio: ['ignore', 'pipe', 'ignore'],
        })
        if (child.pid) pids.push(child.pid)
        await readFirstLine(child)
        const failure = new Error('start failed')

        await expect(
            stopChildOnFailure(child, () => Promise.reject(failure), { killAfterMs: 200 })
        ).rejects.toBe(failure)

        expect(child.signalCode).toBe('SIGKILL')
        expect(isAlive(child.pid as number)).toBe(false)
    }, 3000)

    it('leaves the child running when the wait succeeds', async () => {
        const child = spawnIdle()

        await expect(stopChildOnFailure(child, () => Promise.resolve())).resolves.toBeUndefined()

        expect(hasExited(child)).toBe(false)
        expect(isAlive(child.pid as number)).toBe(true)
    })

    it('stops the whole process group of a detached child', async () => {
        const child = spawn(process.execPath, ['-e', PARENT_OF_IDLE], {
            detached: true,
            stdio: ['ignore', 'pipe', 'ignore'],
        })
        if (child.pid) pids.push(child.pid)
        const grandchild = Number(await readFirstLine(child))
        pids.push(grandchild)
        expect(isAlive(grandchild)).toBe(true)

        await expect(
            stopChildOnFailure(child, () => Promise.reject(new Error('start failed')))
        ).rejects.toThrow('start failed')

        expect(hasExited(child)).toBe(true)
        // The orphaned grandchild is reaped by init, not by us, so its pid can
        // linger for a moment after the signal.
        await expect.poll(() => isAlive(grandchild), { timeout: 3000 }).toBe(false)
    })
})
