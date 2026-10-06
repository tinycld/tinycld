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

const REPLACE = /^replace\s+(\S+)\s+=>\s+(.+?)\s*$/

export const findGoForks = (goModContents: Record<string, string>): Fork[] => {
    const byName = new Map<string, Fork>()

    for (const contents of Object.values(goModContents)) {
        for (const line of contents.split('\n')) {
            const match = REPLACE.exec(line.trim())
            if (!match) continue
            const [, name, target] = match
            if (OURS.test(name)) continue
            if (byName.has(name)) continue
            byName.set(name, { name, upstream: name, pinnedAt: target, kind: 'go' })
        }
    }

    return [...byName.values()]
}

export const findNpmForks = (packageVersionsJson: string): Fork[] => {
    const pins: unknown = JSON.parse(packageVersionsJson)
    if (typeof pins !== 'object' || pins === null) return []

    const forks: Fork[] = []
    for (const [name, pin] of Object.entries(pins as Record<string, unknown>)) {
        if (name === '//' || typeof pin !== 'string') continue
        if (!pin.startsWith('github:')) continue
        const [repo, ref = ''] = pin.slice('github:'.length).split('#')
        forks.push({ name, upstream: repo, pinnedAt: ref, kind: 'npm' })
    }
    return forks
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
const GO_MODS = [
    'server/go.mod',
    'cli/go.mod',
    'core/server/go.mod',
    'core/server/backup/format/go.mod',
]

// Every failure here is reported, never swallowed. Returning an empty window
// list would be SAFE (a fork with no window counts as unreviewed and fails the
// job), but silent: the operator would be told the fork is unreviewed rather
// than that their file is malformed. Task 2's gate had the mirror-image bug in
// the fail-open direction; the lesson is the same either way.
const readReviewWindows = (): { windows: ReviewWindow[]; errors: string[] } => {
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
        // A non-numeric `days` must not quietly become the default: that would
        // invent a review window the author never wrote.
        if (record.days !== undefined && typeof record.days !== 'number') {
            errors.push(`${record.fork}: days must be a number`)
            continue
        }
        windows.push({
            fork: record.fork.trim(),
            reviewed: record.reviewed.trim(),
            days: typeof record.days === 'number' ? record.days : 90,
        })
    }

    return { windows, errors }
}

const main = () => {
    const goMods: Record<string, string> = {}
    for (const relative of GO_MODS) {
        const file = path.join(REPO, relative)
        if (fs.existsSync(file)) goMods[relative] = fs.readFileSync(file, 'utf8')
    }

    const pinsFile = path.join(REPO, 'core', 'package-versions.json')
    const forks = [
        ...findGoForks(goMods),
        ...findNpmForks(fs.existsSync(pinsFile) ? fs.readFileSync(pinsFile, 'utf8') : '{}'),
    ]

    const { windows, errors } = readReviewWindows()
    const { stale, unreviewed } = staleForks(forks, windows, new Date())

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
        process.stdout.write(`\nIGNORE FILE       ${error}\n`)
    }

    if (stale.length > 0 || unreviewed.length > 0 || errors.length > 0) process.exit(1)
    process.stdout.write('\nEvery fork has been compared to upstream inside its window.\n')
}

// package.json sets "type": "module", so tsx runs this as real ESM: bare
// __dirname throws, and the CLI guard compares module URLs. Same pattern as
// scripts/write-workspace-root.ts.
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) main()
