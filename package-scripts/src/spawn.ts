import { spawn } from 'node:child_process'
import type { Command } from './runners'

// Run a Command via the workspace .bin (resolved from the PATH npm sets for
// member scripts). Inherit stdio. Resolve with the exit code.
export function runCommand(cmd: Command): Promise<number> {
    return new Promise(resolve => {
        const child = spawn(cmd.bin, cmd.args, { cwd: cmd.cwd, stdio: 'inherit', shell: false })
        child.on('exit', code => resolve(code ?? 1))
        // Say WHY the spawn failed. A bare `resolve(1)` here exits the whole
        // CLI non-zero with no output at all — the runner never started, so it
        // printed nothing either — which reads as a mysteriously failing check.
        // The usual cause is the runner missing from PATH: these are bare bin
        // names resolved from the PATH a package-manager script sets up, so
        // invoking the CLI directly (not via `pnpm exec`) finds no biome/tsc.
        child.on('error', (err: NodeJS.ErrnoException) => {
            const reason =
                err.code === 'ENOENT'
                    ? `'${cmd.bin}' not found on PATH — run via \`pnpm exec tinycld-pkg\`, which puts the workspace node_modules/.bin on it.`
                    : err.message
            console.error(`[tinycld-pkg] could not run ${cmd.bin}: ${reason}`)
            resolve(1)
        })
    })
}
