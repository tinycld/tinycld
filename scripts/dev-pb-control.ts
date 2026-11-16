import * as fs from 'node:fs'

// dev.ts records which PocketBase it runs against a data dir, so reset-dev-db
// can pause that PocketBase while it replaces the data dir, instead of killing
// the whole dev stack. The pause/resume requests are POSIX signals to dev.ts;
// the PocketBase port going quiet or healthy is the acknowledgement.
export const PAUSE_PB_SIGNAL = 'SIGUSR1'
export const RESUME_PB_SIGNAL = 'SIGUSR2'

export interface DevPbState {
    pid: number
    pbPort: number
    publicUrl: string
}

function stateFile(dataDir: string): string {
    return `${dataDir.replace(/\/$/, '')}.dev.json`
}

export function writeDevPbState(dataDir: string, state: DevPbState): void {
    fs.writeFileSync(stateFile(dataDir), JSON.stringify(state))
}

export function removeDevPbState(dataDir: string): void {
    fs.rmSync(stateFile(dataDir), { force: true })
}

function isAlive(pid: number): boolean {
    try {
        process.kill(pid, 0)
        return true
    } catch {
        return false
    }
}

// Returns the dev.ts that serves dataDir, or null when none runs. A state file
// left by a dev.ts that crashed is removed.
export function readLiveDevPbState(dataDir: string): DevPbState | null {
    const file = stateFile(dataDir)
    if (!fs.existsSync(file)) return null
    const state = JSON.parse(fs.readFileSync(file, 'utf-8')) as DevPbState
    if (isAlive(state.pid)) return state
    fs.rmSync(file, { force: true })
    return null
}
