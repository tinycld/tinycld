// Off CI, playwright.config.ts reuses a server that already answers on an e2e
// port. Several workspaces run e2e on one machine, so that server can belong
// to another workspace: the run then seeds and tests that workspace's database
// while this one's code is under test. This guard finds the process that
// listens on the port and refuses it unless it has this workspace's data dir
// open.
//
// The data dir is read from the process's open files, not its command line:
// the listener is PocketBase itself, which always holds <dir>/data.db open,
// and `lsof -F` prints each path whole on its own line, so a dir with spaces
// survives (a `ps` command line joins its args with spaces and does not).
// lsof ships with macOS and the Linux images the lanes run on.

import { spawnSync } from 'node:child_process'
import * as fs from 'node:fs'
import * as path from 'node:path'

const DATA_DB = 'data.db'

export function parseListenerPids(lsofOutput: string): number[] {
    return lsofOutput
        .split('\n')
        .filter(line => line.startsWith('p'))
        .map(line => Number.parseInt(line.slice(1), 10))
        .filter(pid => Number.isInteger(pid) && pid > 0)
}

export function parseDataDirs(lsofOutput: string): string[] {
    const dirs = lsofOutput
        .split('\n')
        .filter(line => line.startsWith('n') && path.basename(line.slice(1)) === DATA_DB)
        .map(line => path.dirname(line.slice(1)))
    return [...new Set(dirs)]
}

export function dataDirRefusal(opts: {
    port: number
    expectedDir: string
    serverDirs: string[]
}): string | null {
    const { port, expectedDir, serverDirs } = opts
    if (serverDirs.includes(expectedDir)) return null
    const found = serverDirs.length > 0 ? serverDirs.join(', ') : '(none found: no open data.db)'
    return [
        `e2e: the server on :${port} does not serve this workspace's data dir.`,
        `  server's data dir: ${found}`,
        `  this workspace's:  ${expectedDir}`,
        `Stop the server on :${port}, or set E2E_PORT / E2E_PORT_2 to free ports.`,
    ].join('\n')
}

function lsof(args: string[]): string {
    const result = spawnSync('lsof', args, { encoding: 'utf8' })
    if (result.error) throw new Error(`e2e: could not run lsof: ${result.error.message}`)
    // lsof exits 1 when nothing matches; the empty output says that already.
    return result.stdout
}

// lsof reports resolved paths, so the expected dir is resolved the same way.
function realDir(dir: string): string {
    return fs.existsSync(dir) ? fs.realpathSync(dir) : path.resolve(dir)
}

export function assertServerOwnsDataDir(port: number, dataDir: string): void {
    const pids = parseListenerPids(lsof(['-nP', `-iTCP:${port}`, '-sTCP:LISTEN', '-Fp']))
    const serverDirs = pids.flatMap(pid =>
        parseDataDirs(lsof(['-nP', '-a', '-p', String(pid), '-Fn']))
    )
    const refusal = dataDirRefusal({ port, expectedDir: realDir(dataDir), serverDirs })
    if (refusal) throw new Error(refusal)
}
