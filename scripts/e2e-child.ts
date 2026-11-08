import type { ChildProcess } from 'node:child_process'

function hasExited(child: ChildProcess): boolean {
    return child.exitCode !== null || child.signalCode !== null
}

const DEFAULT_KILL_AFTER_MS = 5000

// A child spawned `detached` leads its own process group; signal the group so
// anything it started goes too.
function signalChild(child: ChildProcess, signal: NodeJS.Signals) {
    const pid = child.pid
    try {
        if (pid !== undefined && process.platform !== 'win32' && isGroupLeader(pid)) {
            process.kill(-pid, signal)
        } else {
            child.kill(signal)
        }
    } catch {
        child.kill(signal)
    }
}

// SIGTERM first; a child that has not exited within `killAfterMs` (it ignores
// the signal or is hung) gets SIGKILL, so the stop always ends.
async function stopChild(child: ChildProcess, killAfterMs: number): Promise<void> {
    if (hasExited(child)) return
    const exited = new Promise<'exited'>(resolve => child.once('exit', () => resolve('exited')))
    signalChild(child, 'SIGTERM')
    let timer: ReturnType<typeof setTimeout> | undefined
    const graceOver = new Promise<'grace-over'>(resolve => {
        timer = setTimeout(() => resolve('grace-over'), killAfterMs)
    })
    const first = await Promise.race([exited, graceOver])
    clearTimeout(timer)
    if (first === 'exited' || hasExited(child)) return
    signalChild(child, 'SIGKILL')
    await exited
}

function isGroupLeader(pid: number): boolean {
    try {
        process.kill(-pid, 0)
        return true
    } catch {
        return false
    }
}

/**
 * Await a start gate for a spawned child. If the gate fails, the child would
 * otherwise outlive the script and hold its port, so stop it (and its process
 * group) and wait for it to exit before rethrowing the gate's error. A child
 * still running `killAfterMs` after SIGTERM is sent SIGKILL.
 */
export async function stopChildOnFailure(
    child: ChildProcess,
    wait: () => Promise<void>,
    { killAfterMs = DEFAULT_KILL_AFTER_MS }: { killAfterMs?: number } = {}
): Promise<void> {
    try {
        await wait()
    } catch (err) {
        await stopChild(child, killAfterMs)
        throw err
    }
}
