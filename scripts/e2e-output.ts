// Where the output of the processes an e2e run starts goes: the server build,
// the web export, PocketBase, and the fixture seed.
//
// Playwright prints what its web servers and its global setup write, between
// the test results. PocketBase in --dev mode logs every SQL statement, which
// was 98% of a local run's output (86% SQL alone) and buried the results. So
// off CI that output goes to a log file under tmp/, and the run prints only
// the file's path, and the end of the file when a step fails.
// TINYCLD_E2E_VERBOSE=1 puts the output back inline. CI keeps it inline: a job
// log is read after the run, when the log file is gone.

import type { StdioOptions } from 'node:child_process'
import * as fs from 'node:fs'
import * as path from 'node:path'

const ROOT = path.resolve(import.meta.dirname, '..')
const FAILURE_TAIL_LINES = 60

export const verbose = Boolean(process.env.CI) || process.env.TINYCLD_E2E_VERBOSE === '1'

export interface E2eOutput {
    stdio: StdioOptions
    // null when the output is inline.
    logPath: string | null
    // Writes a line of the caller's own into the same place as its children's
    // output: the log file, or stdout when the output is inline.
    note: (line: string) => void
}

// Opens tmp/<name>.log, emptied, for a child process's stdout and stderr.
export function openE2eOutput(name: string): E2eOutput {
    if (verbose) {
        return { stdio: 'inherit', logPath: null, note: line => process.stdout.write(`${line}\n`) }
    }
    const logPath = path.join(ROOT, 'tmp', `${name}.log`)
    fs.mkdirSync(path.dirname(logPath), { recursive: true })
    const fd = fs.openSync(logPath, 'w')
    return { stdio: ['ignore', fd, fd], logPath, note: line => fs.writeSync(fd, `${line}\n`) }
}

// The end of the log, for a failure: without it a quiet run would say only
// that a step failed.
export function printLogTail(logPath: string | null): void {
    if (!logPath || !fs.existsSync(logPath)) return
    const lines = fs.readFileSync(logPath, 'utf8').trimEnd().split('\n')
    const tail = lines.slice(-FAILURE_TAIL_LINES).join('\n')
    process.stderr.write(
        `[e2e] last ${Math.min(lines.length, FAILURE_TAIL_LINES)} lines of ${logPath}:\n${tail}\n`
    )
}
