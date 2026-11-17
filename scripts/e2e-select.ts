#!/usr/bin/env node
/**
 * e2e-select.ts — chooses the e2e specs a pull request must run, from the files
 * it changed, and how many CI shards to run them on.
 *
 * The whole suite is slow, and most of it is the reports specs. Reports and forms
 * are leaves of the import graph: nothing but their own app routes imports
 * reports, and only reports imports forms. A change inside one of them can
 * therefore break only its own specs (and, for forms, the reports specs), so CI
 * runs only those. A change anywhere else (shared UI, lib, the server, config,
 * this script) runs every spec. A push to main always runs every spec, which
 * catches a dependency this map does not know.
 *
 * KEEP THE MAP TRUE. When a file outside an area starts to import from it, or an
 * area gains a directory, change AREAS. scripts/__tests__/e2e-select.test.ts
 * fails when an area's directory no longer exists.
 *
 * Runs with plain `node` (type stripping) and imports only node builtins, so the
 * CI plan job needs no install:
 *
 *     node scripts/e2e-select.ts --base <sha> --head <sha>     # a pull request
 *     node scripts/e2e-select.ts --all                         # a push to main
 *
 * Prints `mode`, `specs` (paths from the repo root, space separated; empty when
 * mode is `all`), `shards` (a JSON array of shard numbers) and `shard_total` as
 * `key=value` lines, and appends them to $GITHUB_OUTPUT when it is set.
 */
import { execFileSync } from 'node:child_process'
import { appendFileSync, readdirSync, readFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export interface Area {
    name: string
    /** Path prefixes (from the repo root) of the files that belong to the area. */
    sources: string[]
    /** The spec files (names in tests/e2e) that cover the area. */
    specs: RegExp
    /** Areas whose specs must also run when this one changes: they import it. */
    usedBy?: string[]
}

export const AREAS: Area[] = [
    {
        name: 'reports',
        sources: [
            'core/reports/',
            'core/server/reports/',
            'app/a/(app)/reports/',
            'tests/e2e/reports-',
            'tests/e2e/report-view-helpers.ts',
            'tests/scripts/reports-stub/',
            'tests/scripts/scaffold-reports-stub.ts',
        ],
        specs: /^reports-.*\.spec\.ts$/,
    },
    {
        name: 'forms',
        sources: [
            'core/ui/forms/',
            'core/lib/forms/',
            'core/lib/web-forms/',
            'core/lib/automation/',
            'core/components/forms-builder/',
            'core/components/rules/',
            'core/components/web-forms/',
            'core/server/forms/',
            'core/server/webforms/',
            'core/server/automation/',
            'app/p/forms/',
            'app/a/(app)/settings/forms/',
            'app/a/(app)/settings/rules.tsx',
            'tests/e2e/forms-',
            'tests/e2e/web-forms',
            'tests/e2e/rules.spec.ts',
            'tests/scripts/forms-host-stub/',
            'tests/scripts/scaffold-forms-host-stub.ts',
        ],
        specs: /^(forms-|web-forms|rules\.).*spec\.ts$/,
        // The report builder uses the forms components and the Go formula functions.
        usedBy: ['reports'],
    },
]

// Files that reach no running code: a change to only these runs no e2e.
const NO_E2E = [/^docs\//, /^[^/]+\.md$/, /^LICENSE$/, /^\.vscode\//]

const SPEC_DIR = 'tests/e2e'
// A spec file outside every area, changed alone, runs alone.
const LONE_SPEC = /^tests\/e2e\/[^/]+\.spec\.ts$/

// About how many tests one shard runs. With two workers and the suite's mean test
// time, a shard of this size takes about three minutes.
const TESTS_PER_SHARD = 45
const MAX_SHARDS = 6

export interface Selection {
    mode: 'all' | 'some' | 'none'
    /** Spec paths from the repo root; empty unless mode is `some`. */
    specs: string[]
}

function areaOf(file: string): Area | undefined {
    return AREAS.find(area => area.sources.some(prefix => file.startsWith(prefix)))
}

/**
 * `changed` and `allSpecs` are paths from the repo root. Every changed file must
 * be accounted for (an area, a lone spec, or a file that reaches no code), or the
 * whole suite runs.
 */
export function selectSpecs(changed: string[], allSpecs: string[]): Selection {
    const areas = new Set<string>()
    const lone = new Set<string>()
    for (const file of changed) {
        if (NO_E2E.some(pattern => pattern.test(file))) continue
        const area = areaOf(file)
        if (area) {
            areas.add(area.name)
            for (const name of area.usedBy ?? []) areas.add(name)
        } else if (LONE_SPEC.test(file)) {
            lone.add(file)
        } else {
            return { mode: 'all', specs: [] }
        }
    }
    const patterns = AREAS.filter(area => areas.has(area.name)).map(area => area.specs)
    const specs = allSpecs.filter(spec => {
        const name = spec.slice(SPEC_DIR.length + 1)
        return lone.has(spec) || patterns.some(pattern => pattern.test(name))
    })
    if (specs.length === 0) return { mode: 'none', specs: [] }
    return { mode: 'some', specs }
}

/** The shard numbers for a run of `tests` tests: [1], [1, 2], … up to MAX_SHARDS. */
export function shardsFor(tests: number): number[] {
    const count = Math.min(MAX_SHARDS, Math.max(1, Math.ceil(tests / TESTS_PER_SHARD)))
    return Array.from({ length: count }, (_, i) => i + 1)
}

const TEST_CALL = /^\s*test(\.only)?\(/gm

function countTests(root: string, specs: string[]): number {
    return specs.reduce(
        (sum, spec) => sum + (readFileSync(join(root, spec), 'utf8').match(TEST_CALL)?.length ?? 0),
        0
    )
}

function listSpecs(root: string): string[] {
    return readdirSync(join(root, SPEC_DIR))
        .filter(name => name.endsWith('.spec.ts'))
        .sort()
        .map(name => `${SPEC_DIR}/${name}`)
}

function argValue(args: string[], flag: string): string | undefined {
    const at = args.indexOf(flag)
    return at === -1 ? undefined : args[at + 1]
}

function main() {
    const root = resolve(fileURLToPath(import.meta.url), '..', '..')
    const args = process.argv.slice(2)
    const allSpecs = listSpecs(root)
    let selection: Selection = { mode: 'all', specs: [] }
    if (!args.includes('--all')) {
        const base = argValue(args, '--base')
        const head = argValue(args, '--head') ?? 'HEAD'
        if (!base) throw new Error('usage: e2e-select.ts --all | --base <sha> [--head <sha>]')
        const diff = execFileSync('git', ['diff', '--name-only', base, head], {
            cwd: root,
            encoding: 'utf8',
        })
        selection = selectSpecs(diff.split('\n').filter(Boolean), allSpecs)
    }
    const run = selection.mode === 'all' ? allSpecs : selection.specs
    // One shard when nothing runs: a job matrix must not be empty, and the e2e
    // job is skipped by its `if` in that mode.
    const shards = selection.mode === 'none' ? [1] : shardsFor(countTests(root, run))
    const lines = [
        `mode=${selection.mode}`,
        `specs=${selection.specs.join(' ')}`,
        `shards=${JSON.stringify(shards)}`,
        `shard_total=${shards.length}`,
    ]
    process.stdout.write(`${lines.join('\n')}\n`)
    if (process.env.GITHUB_OUTPUT)
        appendFileSync(process.env.GITHUB_OUTPUT, `${lines.join('\n')}\n`)
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main()
