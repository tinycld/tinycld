import { spawnSync } from 'node:child_process'
import * as fs from 'node:fs'
import { createRequire } from 'node:module'
import * as os from 'node:os'
import * as path from 'node:path'
import { describe, expect, it } from 'vitest'
import { e2eExitCode, readRunSummary, summaryRunEnv } from '../src/e2e-summary'
import { summarizeTests } from '../src/e2e-summary-reporter'

describe('e2eExitCode', () => {
    it('fails a run that selected tests and skipped all of them', () => {
        const result = e2eExitCode(0, { selected: 163, skipped: 163 })
        expect(result.code).toBe(1)
        expect(result.message).toContain('all 163 selected tests skipped')
    })
    it('passes a run with some skipped and some passed tests', () => {
        expect(e2eExitCode(0, { selected: 163, skipped: 4 })).toEqual({ code: 0 })
    })
    it('keeps the exit code of a run that selected no tests', () => {
        expect(e2eExitCode(0, { selected: 0, skipped: 0 })).toEqual({ code: 0 })
    })
    it('keeps the exit code of a run that failed', () => {
        expect(e2eExitCode(1, { selected: 3, skipped: 3 })).toEqual({ code: 1 })
    })
    it('keeps the exit code when the run wrote no summary', () => {
        expect(e2eExitCode(0, undefined)).toEqual({ code: 0 })
    })
})

function testCase(outcome: string, statuses: string[]) {
    return { outcome: () => outcome, results: statuses.map(status => ({ status })) }
}

describe('summarizeTests', () => {
    it('counts each selected test and each skipped one', () => {
        const tests = [
            testCase('expected', ['passed']),
            testCase('skipped', ['skipped']),
            testCase('unexpected', ['failed']),
        ]
        expect(summarizeTests(tests)).toEqual({ selected: 3, skipped: 1 })
    })
    it('does not count a test that has no result (a --list run) as skipped', () => {
        expect(summarizeTests([testCase('skipped', [])])).toEqual({ selected: 1, skipped: 0 })
    })
})

const require = createRequire(import.meta.url)
const playwrightCli = require.resolve('@playwright/test/cli')
const fixtureConfig = path.join(
    import.meta.dirname,
    'fixtures',
    'e2e-summary',
    'playwright.config.ts'
)

function runFixture(args: string[]) {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'tcld-e2e-summary-'))
    const file = path.join(dir, 'summary.json')
    const run = spawnSync(
        process.execPath,
        [playwrightCli, 'test', '--config', fixtureConfig, ...args],
        { env: { ...process.env, ...summaryRunEnv(file) }, encoding: 'utf8' }
    )
    const summary = readRunSummary(file)
    fs.rmSync(dir, { recursive: true, force: true })
    return { status: run.status, summary }
}

describe('the summary reporter in a real Playwright run', () => {
    it('reports a run whose selected tests are all skipped', () => {
        const run = runFixture(['skipped.pw.ts'])
        expect(run.status).toBe(0)
        expect(run.summary).toEqual({ selected: 2, skipped: 2 })
    }, 60_000)
    it('reports a run with passed and skipped tests, also with --reporter on the command line', () => {
        const run = runFixture(['--reporter=list'])
        expect(run.status).toBe(0)
        expect(run.summary).toEqual({ selected: 3, skipped: 2 })
    }, 60_000)
    it('reports no skipped test for a --list run', () => {
        const run = runFixture(['--list'])
        expect(run.status).toBe(0)
        expect(run.summary?.skipped ?? 0).toBe(0)
    }, 60_000)
})
