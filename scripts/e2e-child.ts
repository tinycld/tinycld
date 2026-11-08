import type { ChildProcess } from 'node:child_process'

function hasExited(child: ChildProcess): boolean {
    return child.exitCode !== null || child.signalCode !== null
}

// A child spawned `detached` leads its own process group; signal the group so
// anything it started goes too.
function stopChild(child: ChildProcess): Promise<void> {
    if (hasExited(child)) return Promise.resolve()
    const exited = new Promise<void>(resolve => child.once('exit', () => resolve()))
    const pid = child.pid
    try {
        if (pid !== undefined && process.platform !== 'win32' && isGroupLeader(pid)) {
            process.kill(-pid, 'SIGTERM')
        } else {
            child.kill('SIGTERM')
        }
    } catch {
        child.kill('SIGTERM')
    }
    return exited
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
 * group) and wait for it to exit before rethrowing the gate's error.
 */
export async function stopChildOnFailure(
    child: ChildProcess,
    wait: () => Promise<void>
): Promise<void> {
    try {
        await wait()
    } catch (err) {
        await stopChild(child)
        throw err
    }
}
