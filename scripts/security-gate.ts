import * as fs from 'node:fs'
import * as path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { parse as parseYaml } from 'yaml'

// This repo's package.json sets "type": "module", so tsx runs this file as
// ESM — no __dirname/require.main, hence the import.meta.url derivation
// below (the same import.meta.url-based approach scripts/write-workspace-root.ts
// uses for its own invoked-as-script check).
const scriptDir = path.dirname(fileURLToPath(import.meta.url))

// The gate that decides whether a vulnerability scan fails CI.
//
// All policy lives here rather than in workflow YAML so that it can be
// unit-tested: a threshold or an expiry rule encoded in a shell pipeline is
// only ever verified by pushing a commit and watching CI.
//
// An ignore entry needs a reason and an expiry date, and an expired entry
// fails the job. A suppression is therefore a decision with a justification
// and a deadline, reviewed in a pull request, instead of a permanent silence
// nobody revisits.

export type Severity = 'info' | 'low' | 'moderate' | 'high' | 'critical'

const SEVERITY_ORDER: Severity[] = ['info', 'low', 'moderate', 'high', 'critical']

// A severity this gate does not recognise (a new tier the audit tool adds
// above critical, or any other unexpected string) must rank ABOVE every
// known severity, so it blocks at every threshold instead of silently
// falling to the bottom. Fail closed on what the gate cannot understand.
const rank = (s: Severity) => {
    const index = SEVERITY_ORDER.indexOf(s)
    return index === -1 ? SEVERITY_ORDER.length : index
}

export interface IgnoreEntry {
    id: string
    reason: string
    expires: string
}

export interface Finding {
    id: string
    severity: Severity
    module: string
    title: string
    url: string
}

const ISO_DATE = /^\d{4}-\d{2}-\d{2}$/

// Compared as plain ISO strings at day granularity: an expiry is a date a human
// wrote, not an instant, so a timezone must not decide whether CI passes.
const isExpired = (expires: string, today: Date) => expires < today.toISOString().slice(0, 10)

export const parseIgnores = (
    raw: string,
    today: Date
): { valid: IgnoreEntry[]; errors: string[] } => {
    // The file is a mapping, not a list: check-fork-drift.ts owns a second
    // top-level key (forkReviews) in this same file.
    let parsed: unknown
    try {
        parsed = parseYaml(raw)
    } catch (err) {
        // Malformed YAML must surface as a gate error, not an uncaught throw
        // that kills the process before it can report anything useful.
        const message = err instanceof Error ? err.message : String(err)
        return { valid: [], errors: [`security-ignores.yml: could not parse YAML — ${message}`] }
    }
    if (typeof parsed !== 'object' || parsed === null) return { valid: [], errors: [] }
    const entries = (parsed as { ignores?: unknown }).ignores
    if (entries === null || entries === undefined) return { valid: [], errors: [] }
    if (!Array.isArray(entries)) {
        return { valid: [], errors: ['security-ignores.yml: `ignores` must be a list of entries'] }
    }

    const valid: IgnoreEntry[] = []
    const errors: string[] = []

    for (const [index, entry] of entries.entries()) {
        const where = `entry ${index + 1}`
        if (typeof entry !== 'object' || entry === null) {
            errors.push(`${where}: must be a mapping with id, reason and expires`)
            continue
        }
        const record = entry as Record<string, unknown>
        const id = typeof record.id === 'string' ? record.id.trim() : ''
        const reason = typeof record.reason === 'string' ? record.reason.trim() : ''
        const expires = typeof record.expires === 'string' ? record.expires.trim() : ''

        if (!id) {
            errors.push(`${where}: needs an id (the GHSA advisory identifier)`)
            continue
        }
        if (!reason) {
            errors.push(`${id}: needs a reason explaining why this may wait`)
            continue
        }
        if (!ISO_DATE.test(expires)) {
            errors.push(`${id}: needs an expires date in YYYY-MM-DD form`)
            continue
        }
        if (isExpired(expires, today)) {
            errors.push(
                `${id}: the ignore expired on ${expires} — fix it or renew with a new reason`
            )
            continue
        }
        valid.push({ id, reason, expires })
    }

    return { valid, errors }
}

interface RawAdvisory {
    github_advisory_id?: unknown
    id?: unknown
    severity?: unknown
    module_name?: unknown
    title?: unknown
    url?: unknown
}

// Thrown by parseAuditJson instead of returning [] for input the gate cannot
// treat as "an audit ran and found nothing" — an empty file or invalid JSON
// most often means the audit step crashed, OOMed, or never reached the
// registry (Task 4's workflow runs `pnpm audit --json ... > file || true`,
// so a failed audit still leaves an empty file behind). Swallowing that into
// an empty finding list would make CI go green without a scan ever having
// run, so the caller must fail closed instead.
export class AuditInputError extends Error {}

// A severity string this gate does not recognise still needs a Severity to
// flow through gate()'s ranking; the raw string is preserved verbatim (after
// lower-casing) so the caller's message can name exactly what showed up.
// rank() treats anything outside SEVERITY_ORDER as above 'critical', so this
// is safe even though the value isn't one of the five known literals.
const normalizeSeverity = (value: unknown): Severity => {
    if (typeof value !== 'string') return 'unrecognised' as Severity
    const lower = value.toLowerCase()
    return ((SEVERITY_ORDER as string[]).includes(lower) ? lower : value) as Severity
}

export const parseAuditJson = (raw: string): Finding[] => {
    const trimmed = raw.trim()
    if (!trimmed) {
        throw new AuditInputError('the audit produced no output (empty input)')
    }

    let report: unknown
    try {
        report = JSON.parse(trimmed)
    } catch (err) {
        const message = err instanceof Error ? err.message : String(err)
        throw new AuditInputError(`the audit output is not valid JSON — ${message}`)
    }

    if (typeof report !== 'object' || report === null) return []
    const advisories = (report as { advisories?: unknown }).advisories
    if (typeof advisories !== 'object' || advisories === null) return []

    return Object.values(advisories as Record<string, RawAdvisory>).map(advisory => ({
        // An ignore entry names the GHSA id, so that is the identity we key on.
        // The numeric id is only a fallback for a report that omits it.
        id:
            typeof advisory.github_advisory_id === 'string'
                ? advisory.github_advisory_id
                : String(advisory.id ?? 'unknown'),
        severity: normalizeSeverity(advisory.severity),
        module: typeof advisory.module_name === 'string' ? advisory.module_name : 'unknown',
        title: typeof advisory.title === 'string' ? advisory.title : '',
        url: typeof advisory.url === 'string' ? advisory.url : '',
    }))
}

export const gate = (
    findings: Finding[],
    ignores: IgnoreEntry[],
    minSeverity: Severity
): { blocking: Finding[]; ignored: Finding[]; below: Finding[] } => {
    const ignoredIds = new Set(ignores.map(i => i.id))
    const blocking: Finding[] = []
    const ignored: Finding[] = []
    const below: Finding[] = []

    for (const finding of findings) {
        if (rank(finding.severity) < rank(minSeverity)) below.push(finding)
        else if (ignoredIds.has(finding.id)) ignored.push(finding)
        else blocking.push(finding)
    }

    return { blocking, ignored, below }
}

const IGNORE_FILE = path.join(scriptDir, '..', '.github', 'security-ignores.yml')

export const readIgnoreFile = (today: Date) => {
    if (!fs.existsSync(IGNORE_FILE)) return { valid: [], errors: [] }
    return parseIgnores(fs.readFileSync(IGNORE_FILE, 'utf8'), today)
}

const isKnownSeverity = (value: string): value is Severity =>
    (SEVERITY_ORDER as string[]).includes(value)

const parseMinSeverity = (arg: string | undefined): Severity => {
    const candidate = arg ?? 'high'
    if (!isKnownSeverity(candidate)) {
        // A workflow typo here (e.g. 'hgih') must not be silently accepted —
        // an unrecognised threshold previously ranked as -1 and blocked
        // everything, which is safe but gives no indication the argument is
        // wrong, so a real typo would go unnoticed until it is fixed by
        // accident. Reject it explicitly instead.
        process.stdout.write(
            `security-gate: unrecognised minimum severity "${candidate}" — expected one of ${SEVERITY_ORDER.join(', ')}\n`
        )
        process.exit(1)
    }
    return candidate
}

const main = () => {
    const minSeverity = parseMinSeverity(process.argv[2])
    const auditJson = fs.readFileSync(0, 'utf8')

    const { valid, errors } = readIgnoreFile(new Date())

    let findings: Finding[]
    try {
        findings = parseAuditJson(auditJson)
    } catch (err) {
        // Input the gate cannot understand must fail CLOSED: an empty or
        // non-JSON audit report most likely means the scan itself never ran,
        // and treating that as "zero findings" would let CI go green without
        // a scan ever having happened.
        const message = err instanceof AuditInputError ? err.message : String(err)
        process.stdout.write(`security-gate: ${message}\n`)
        process.exit(1)
    }

    const { blocking, ignored, below } = gate(findings, valid, minSeverity)

    for (const finding of below) {
        process.stdout.write(`note    ${finding.severity} ${finding.module} — ${finding.title}\n`)
    }
    for (const finding of ignored) {
        process.stdout.write(`ignored ${finding.id} ${finding.module} — ${finding.title}\n`)
    }
    for (const finding of blocking) {
        // An unrecognised severity string (not one of the five known
        // literals) is named explicitly here, rather than just printed as
        // part of the normal line, so it reads as a loud anomaly and not a
        // routine high/critical finding.
        const severityLabel = isKnownSeverity(finding.severity)
            ? finding.severity
            : `UNRECOGNISED SEVERITY "${finding.severity}"`
        process.stdout.write(
            `BLOCK   ${severityLabel} ${finding.module} ${finding.id} — ${finding.title}\n${finding.url}\n`
        )
    }
    for (const error of errors) {
        process.stdout.write(`IGNORE FILE ${error}\n`)
    }

    if (errors.length > 0 || blocking.length > 0) {
        process.stdout.write(
            `\n${blocking.length} blocking finding(s), ${errors.length} ignore-file problem(s)\n`
        )
        process.exit(1)
    }
    process.stdout.write('No blocking vulnerabilities.\n')
}

// Run only as a CLI, so the test import does not read stdin.
const invokedAsScript =
    process.argv[1] !== undefined &&
    import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href
if (invokedAsScript) main()
