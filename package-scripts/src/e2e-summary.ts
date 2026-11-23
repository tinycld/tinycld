import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'
import type { Command } from './runners'
import { runCommand } from './spawn'

export interface RunSummary {
    selected: number
    skipped: number
}

export const SUMMARY_FILE_ENV = 'TINYCLD_E2E_SUMMARY_FILE'

// PW_TEST_REPORTER adds one reporter to whatever reporters the config or a
// `--reporter` argument chose, so the summary is written for every run.
export function summaryRunEnv(file: string): Record<string, string> {
    return {
        PW_TEST_REPORTER: path.join(import.meta.dirname, 'e2e-summary-reporter.ts'),
        [SUMMARY_FILE_ENV]: file,
    }
}

export function readRunSummary(file: string): RunSummary | undefined {
    if (!fs.existsSync(file)) return undefined
    return JSON.parse(fs.readFileSync(file, 'utf8')) as RunSummary
}

// Playwright exits 0 when every selected test skipped, which reads as a pass:
// a run whose prerequisite (a stub package, say) was missing skipped all its
// tests and reported success. A run that selected no test keeps its code.
export function e2eExitCode(
    code: number,
    summary: RunSummary | undefined
): { code: number; message?: string } {
    if (code !== 0 || !summary) return { code }
    if (summary.selected === 0 || summary.skipped !== summary.selected) return { code }
    return {
        code: 1,
        message: `[tinycld-pkg] test:e2e failed: all ${summary.selected} selected tests skipped; a prerequisite (such as a stub package) may be missing.`,
    }
}

export async function runE2eCommand(cmd: Command): Promise<number> {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'tinycld-e2e-summary-'))
    const file = path.join(dir, 'summary.json')
    try {
        const code = await runCommand({ ...cmd, env: { ...cmd.env, ...summaryRunEnv(file) } })
        const result = e2eExitCode(code, readRunSummary(file))
        if (result.message) console.error(result.message)
        return result.code
    } finally {
        fs.rmSync(dir, { recursive: true, force: true })
    }
}
