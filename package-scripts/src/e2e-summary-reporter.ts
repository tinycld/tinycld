import * as fs from 'node:fs'
import type { FullConfig, Reporter, Suite } from '@playwright/test/reporter'
import { type RunSummary, SUMMARY_FILE_ENV } from './e2e-summary'

interface ReportedTest {
    outcome(): string
    results: readonly unknown[]
}

// A test with no result never ran (a `--list` run, or a run cut short), so it
// is not counted as skipped; a run cut short exits non-zero anyway.
export function summarizeTests(tests: readonly ReportedTest[]): RunSummary {
    const skipped = tests.filter(test => test.outcome() === 'skipped' && test.results.length > 0)
    return { selected: tests.length, skipped: skipped.length }
}

// Loaded by Playwright through PW_TEST_REPORTER (see summaryRunEnv), which adds
// it next to the reporters of the config or of `--reporter`. It writes the
// counts of the run for tinycld-pkg, which reads them after Playwright exits.
export default class SummaryReporter implements Reporter {
    private suite: Suite | undefined

    onBegin(_config: FullConfig, suite: Suite) {
        this.suite = suite
    }

    onEnd() {
        const file = process.env[SUMMARY_FILE_ENV]
        if (!file || !this.suite) return
        fs.writeFileSync(file, JSON.stringify(summarizeTests(this.suite.allTests())))
    }

    printsToStdio() {
        return false
    }
}
