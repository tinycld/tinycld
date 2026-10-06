import * as fs from 'node:fs'
import * as path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { parse as parseYaml } from 'yaml'

// Forked dependencies are invisible to every vulnerability scanner.
//
// A Go `replace` removes the upstream version from the module graph, so no
// advisory can match it. An npm dependency pinned to a bare git SHA resolves
// to no version, so `pnpm audit` omits it from the report entirely. Neither is
// reported safe — neither is reported at all. If upstream PocketBase ships a
// security fix, nothing else in CI says so, in the dependency that serves HTTP
// and owns authentication.
//
// This check fails on a stale review window, not on a vulnerability count. An
// upstream advisory cannot be mapped automatically onto a diverged fork: the
// affected code may be absent, already patched locally, or renamed. A tool
// claiming otherwise would be wrong. "Somebody compared this fork to upstream
// within the last N days" is a claim CI can actually check, and it puts the
// judgement where only a human can make it.
//
// The governing rule for every parser below: input we cannot classify must
// fail CLOSED (an errors[] entry that fails the job) and say why. Silently
// skipping an unrecognised line is how a fork goes unreported — which, for
// the one dependency this script exists to cover, is strictly worse than a
// false alarm.

export interface Fork {
    name: string
    upstream: string
    pinnedAt: string
    kind: 'go' | 'npm'
}

export interface ReviewWindow {
    fork: string
    reviewed: string
    days: number
}

// Our own modules are replaced for local workspace wiring, not because they are
// forks of anything upstream.
const OURS = /^tinycld\.org\//

// A line that opens a block form: `replace (` with nothing after the paren.
const REPLACE_BLOCK_START = /^replace\s*\($/
const BLOCK_END = /^\)\s*$/

// Single-line form, both directions Go allows for the left side:
//   replace a => b v1.0.0
//   replace a v1.0.0 => b v2.0.0
// and the same shape for a line inside a `replace ( ... )` block, which omits
// the leading `replace` keyword.
const REPLACE_SINGLE = /^replace\s+(\S+)(?:\s+\S+)?\s+=>\s+(.+)$/
const BLOCK_ENTRY = /^(\S+)(?:\s+\S+)?\s+=>\s+(.+)$/

// Strip a trailing `// comment` from a replace target without truncating a
// local path or version that legitimately contains a slash.
const stripComment = (s: string) => s.replace(/\s*\/\/.*$/, '').trim()

const isReplaceDirective = (trimmed: string) =>
    trimmed.startsWith('replace ') || trimmed === 'replace'

export const findGoForks = (
    goModContents: Record<string, string>
): { forks: Fork[]; errors: string[] } => {
    const byName = new Map<string, Fork>()
    const errors: string[] = []

    for (const [file, contents] of Object.entries(goModContents)) {
        let inBlock = false
        const lines = contents.split('\n')

        for (const [index, rawLine] of lines.entries()) {
            const line = stripComment(rawLine).trim()
            const lineNo = index + 1
            if (!line) continue

            if (inBlock) {
                if (BLOCK_END.test(line)) {
                    inBlock = false
                    continue
                }
                const match = BLOCK_ENTRY.exec(line)
                if (!match) {
                    errors.push(`${file}:${lineNo}: unparseable replace entry: ${rawLine.trim()}`)
                    continue
                }
                addFork(byName, match[1], stripComment(match[2]))
                continue
            }

            if (REPLACE_BLOCK_START.test(line)) {
                inBlock = true
                continue
            }

            if (!isReplaceDirective(line)) continue

            const match = REPLACE_SINGLE.exec(line)
            if (!match) {
                errors.push(`${file}:${lineNo}: unparseable replace directive: ${rawLine.trim()}`)
                continue
            }
            addFork(byName, match[1], stripComment(match[2]))
        }
    }

    return { forks: [...byName.values()], errors }
}

const addFork = (byName: Map<string, Fork>, name: string, target: string) => {
    if (OURS.test(name)) return
    if (byName.has(name)) return
    byName.set(name, { name, upstream: name, pinnedAt: target, kind: 'go' })
}

const SEMVER_RANGE = /^[\^~]?\d|^>=|^<=|^>|^<|^\*$/

// Pin forms pnpm accepts that point at a non-registry source — every one of
// these is as invisible to `pnpm audit` as a bare git SHA, because none of
// them resolves to a published, advisory-matchable version.
const GITHUB_SHORTHAND = /^github:([^#]+)(?:#(.*))?$/
const GIT_URL = /^git(?:\+https|\+ssh)?:\/\/.+$/
const BARE_SHORTHAND = /^[\w.-]+\/[\w.-]+(?:#.*)?$/
const LOCAL_SOURCE = /^(file|link):(.+)$/
const NPM_ALIAS = /^npm:(.+)$/

export const findNpmForks = (packageVersionsJson: string): { forks: Fork[]; errors: string[] } => {
    const pins: unknown = JSON.parse(packageVersionsJson)
    if (typeof pins !== 'object' || pins === null) return { forks: [], errors: [] }

    const forks: Fork[] = []
    const errors: string[] = []

    for (const [name, pin] of Object.entries(pins as Record<string, unknown>)) {
        if (name === '//') continue
        if (typeof pin !== 'string') {
            errors.push(`package-versions.json: ${name}: pin must be a string`)
            continue
        }

        if (SEMVER_RANGE.test(pin)) continue // ordinary registry pin, not a fork

        const githubMatch = GITHUB_SHORTHAND.exec(pin)
        if (githubMatch) {
            forks.push({
                name,
                upstream: githubMatch[1],
                pinnedAt: githubMatch[2] ?? '',
                kind: 'npm',
            })
            continue
        }

        if (GIT_URL.test(pin) || pin.startsWith('git:')) {
            const [repo, ref = ''] = pin.split('#')
            forks.push({ name, upstream: repo, pinnedAt: ref, kind: 'npm' })
            continue
        }

        const localMatch = LOCAL_SOURCE.exec(pin)
        if (localMatch) {
            // A local path is as invisible to a registry scanner as a git SHA:
            // there is no published version to match an advisory against.
            forks.push({
                name,
                upstream: localMatch[1] === 'file' ? 'local file:' : 'local link:',
                pinnedAt: localMatch[2],
                kind: 'npm',
            })
            continue
        }

        if (NPM_ALIAS.test(pin)) {
            // npm:alias@version still resolves to a real published version on
            // the registry, so it is advisory-matchable — not a fork.
            continue
        }

        if (BARE_SHORTHAND.test(pin)) {
            const [repo, ref = ''] = pin.split('#')
            forks.push({ name, upstream: repo, pinnedAt: ref, kind: 'npm' })
            continue
        }

        errors.push(
            `package-versions.json: ${name}: unrecognised pin "${pin}" is neither a semver range nor a known fork form`
        )
    }

    return { forks, errors }
}

const ISO_DATE = /^\d{4}-\d{2}-\d{2}$/

const daysBetween = (from: string, today: Date) =>
    Math.floor((today.getTime() - new Date(`${from}T00:00:00Z`).getTime()) / 86_400_000)

export const staleForks = (
    forks: Fork[],
    windows: ReviewWindow[],
    today: Date
): { stale: string[]; unreviewed: string[] } => {
    const byFork = new Map(windows.map(w => [w.fork, w]))
    const stale: string[] = []
    const unreviewed: string[] = []

    for (const fork of forks) {
        const window = byFork.get(fork.name)
        if (!window) {
            unreviewed.push(fork.name)
            continue
        }
        if (daysBetween(window.reviewed, today) > window.days) stale.push(fork.name)
    }

    return { stale, unreviewed }
}

const REPO = path.join(path.dirname(fileURLToPath(import.meta.url)), '..')

// Discovered rather than hardcoded, so a fifth Go module is scanned
// automatically instead of silently skipped. `third_party/` is excluded
// because it IS the vendored fork, not a consumer declaring a replace.
const EXCLUDED_DIRS = new Set(['node_modules', '.worktrees', 'third_party', '.git'])

const discoverGoMods = (dir: string, out: string[] = []): string[] => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
        if (entry.isDirectory()) {
            if (EXCLUDED_DIRS.has(entry.name)) continue
            discoverGoMods(path.join(dir, entry.name), out)
        } else if (entry.name === 'go.mod') {
            out.push(path.relative(REPO, path.join(dir, entry.name)))
        }
    }
    return out
}

// Every failure here is reported, never swallowed. Returning an empty window
// list would be SAFE (a fork with no window counts as unreviewed and fails the
// job), but silent: the operator would be told the fork is unreviewed rather
// than that their file is malformed. Task 2's gate had the mirror-image bug in
// the fail-open direction; the lesson is the same either way.
const readReviewWindows = (today: Date): { windows: ReviewWindow[]; errors: string[] } => {
    const file = path.join(REPO, '.github', 'security-ignores.yml')
    if (!fs.existsSync(file)) {
        return { windows: [], errors: [`${file} does not exist`] }
    }

    let parsed: unknown
    try {
        parsed = parseYaml(fs.readFileSync(file, 'utf8'))
    } catch (cause) {
        const message = cause instanceof Error ? cause.message : String(cause)
        return { windows: [], errors: [`security-ignores.yml is not valid YAML: ${message}`] }
    }

    if (typeof parsed !== 'object' || parsed === null) {
        return { windows: [], errors: ['security-ignores.yml must be a mapping'] }
    }
    const raw = (parsed as { forkReviews?: unknown }).forkReviews
    if (raw === null || raw === undefined) return { windows: [], errors: [] }
    if (!Array.isArray(raw)) {
        return { windows: [], errors: ['`forkReviews` must be a list of entries'] }
    }

    const windows: ReviewWindow[] = []
    const errors: string[] = []

    for (const [index, entry] of raw.entries()) {
        const where = `forkReviews entry ${index + 1}`
        if (typeof entry !== 'object' || entry === null) {
            errors.push(`${where}: must be a mapping with fork, reviewed and days`)
            continue
        }
        const record = entry as Record<string, unknown>
        if (typeof record.fork !== 'string' || !record.fork.trim()) {
            errors.push(`${where}: needs a fork name`)
            continue
        }
        if (typeof record.reviewed !== 'string' || !ISO_DATE.test(record.reviewed.trim())) {
            errors.push(`${record.fork}: needs a reviewed date in YYYY-MM-DD form`)
            continue
        }
        const reviewed = record.reviewed.trim()
        // A future date makes daysBetween negative, which never exceeds a
        // window and so never goes stale — a typo or a deliberate future date
        // would silence the check permanently.
        if (daysBetween(reviewed, today) < 0) {
            errors.push(`${record.fork}: reviewed date ${reviewed} is in the future`)
            continue
        }
        // A non-numeric `days` must not quietly become the default: that would
        // invent a review window the author never wrote.
        if (record.days !== undefined && typeof record.days !== 'number') {
            errors.push(`${record.fork}: days must be a number`)
            continue
        }
        windows.push({
            fork: record.fork.trim(),
            reviewed,
            days: typeof record.days === 'number' ? record.days : 90,
        })
    }

    return { windows, errors }
}

const main = () => {
    const goMods: Record<string, string> = {}
    for (const relative of discoverGoMods(REPO).sort()) {
        goMods[relative] = fs.readFileSync(path.join(REPO, relative), 'utf8')
    }

    const pinsFile = path.join(REPO, 'core', 'package-versions.json')
    const goResult = findGoForks(goMods)
    const npmResult = findNpmForks(
        fs.existsSync(pinsFile) ? fs.readFileSync(pinsFile, 'utf8') : '{}'
    )
    const forks = [...goResult.forks, ...npmResult.forks]

    const today = new Date()
    const { windows, errors: windowErrors } = readReviewWindows(today)
    const { stale, unreviewed } = staleForks(forks, windows, today)
    const errors = [...goResult.errors, ...npmResult.errors, ...windowErrors]

    process.stdout.write(`${forks.length} forked dependenc(ies) no scanner can see:\n`)
    for (const fork of forks) {
        process.stdout.write(`  ${fork.kind.padEnd(4)} ${fork.name} -> ${fork.pinnedAt}\n`)
    }

    for (const name of unreviewed) {
        process.stdout.write(`\nNO REVIEW WINDOW  ${name}\n`)
        process.stdout.write('  Add a forkReviews entry to .github/security-ignores.yml.\n')
    }
    for (const name of stale) {
        process.stdout.write(`\nREVIEW OVERDUE    ${name}\n`)
        process.stdout.write(
            '  Compare the fork against upstream, then update its reviewed date.\n'
        )
    }

    for (const error of errors) {
        process.stdout.write(`\nUNPARSEABLE       ${error}\n`)
    }

    if (stale.length > 0 || unreviewed.length > 0 || errors.length > 0) process.exit(1)
    process.stdout.write('\nEvery fork has been compared to upstream inside its window.\n')
}

// package.json sets "type": "module", so tsx runs this as real ESM: bare
// __dirname throws, and the CLI guard compares module URLs. Same pattern as
// scripts/write-workspace-root.ts.
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) main()
