import { execFileSync } from 'node:child_process'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { describe, expect, it } from 'vitest'
import { AuditInputError, gate, parseAuditJson, parseIgnores } from '../security-gate'

const fixture = (name: string) =>
    fs.readFileSync(path.join(__dirname, '..', '__fixtures__', name), 'utf8')

const TODAY = new Date('2026-10-06T00:00:00Z')

describe('parseIgnores', () => {
    it('accepts an entry with id, reason and a future expiry', () => {
        const { valid, errors } = parseIgnores(
            'ignores:\n  - id: GHSA-aaaa-aaaa-aaaa\n    reason: Not reachable from the server build.\n    expires: 2026-12-01\n',
            TODAY
        )
        expect(errors).toEqual([])
        expect(valid).toHaveLength(1)
        expect(valid[0].id).toBe('GHSA-aaaa-aaaa-aaaa')
    })

    it('rejects an expired entry', () => {
        const { valid, errors } = parseIgnores(
            'ignores:\n  - id: GHSA-aaaa-aaaa-aaaa\n    reason: Stale.\n    expires: 2026-10-05\n',
            TODAY
        )
        expect(valid).toEqual([])
        expect(errors.join(' ')).toMatch(/expired/i)
    })

    it('treats the expiry date itself as still valid', () => {
        const { valid, errors } = parseIgnores(
            'ignores:\n  - id: GHSA-aaaa-aaaa-aaaa\n    reason: Last day.\n    expires: 2026-10-06\n',
            TODAY
        )
        expect(errors).toEqual([])
        expect(valid).toHaveLength(1)
    })

    it('rejects an entry with no expires', () => {
        const { errors } = parseIgnores(
            'ignores:\n  - id: GHSA-aaaa-aaaa-aaaa\n    reason: Forever.\n',
            TODAY
        )
        expect(errors.join(' ')).toMatch(/expires/i)
    })

    it('rejects an entry with no reason', () => {
        const { errors } = parseIgnores(
            'ignores:\n  - id: GHSA-aaaa-aaaa-aaaa\n    expires: 2026-12-01\n',
            TODAY
        )
        expect(errors.join(' ')).toMatch(/reason/i)
    })

    it('rejects an entry whose reason is only whitespace', () => {
        const { errors } = parseIgnores(
            'ignores:\n  - id: GHSA-aaaa-aaaa-aaaa\n    reason: "   "\n    expires: 2026-12-01\n',
            TODAY
        )
        expect(errors.join(' ')).toMatch(/reason/i)
    })

    it('rejects a malformed expiry date', () => {
        const { errors } = parseIgnores(
            'ignores:\n  - id: GHSA-aaaa-aaaa-aaaa\n    reason: Bad date.\n    expires: not-a-date\n',
            TODAY
        )
        expect(errors.join(' ')).toMatch(/expires/i)
    })

    it('returns nothing for an empty or comment-only file', () => {
        const { valid, errors } = parseIgnores('# nothing suppressed yet\n', TODAY)
        expect(valid).toEqual([])
        expect(errors).toEqual([])
    })

    it('reports an error instead of throwing on malformed YAML', () => {
        const { valid, errors } = parseIgnores('ignores: [a: b: c\n', TODAY)
        expect(valid).toEqual([])
        expect(errors.length).toBeGreaterThan(0)
    })

    it('rejects a top-level list instead of silently finding nothing', () => {
        const { valid, errors } = parseIgnores(
            '- id: GHSA-aaaa-aaaa-aaaa\n  reason: Oops.\n  expires: 2026-12-01\n',
            TODAY
        )
        expect(valid).toEqual([])
        expect(errors.join(' ')).toMatch(/mapping/i)
    })

    it('rejects an unknown top-level key instead of silently finding nothing', () => {
        const { valid, errors } = parseIgnores(
            'ignorse:\n  - id: GHSA-aaaa-aaaa-aaaa\n    reason: Typo.\n    expires: 2026-12-01\n',
            TODAY
        )
        expect(valid).toEqual([])
        expect(errors.join(' ')).toMatch(/ignorse/)
    })

    it('rejects a bare string top level', () => {
        const { valid, errors } = parseIgnores('just a string\n', TODAY)
        expect(valid).toEqual([])
        expect(errors.length).toBeGreaterThan(0)
    })

    it('returns nothing for an empty string', () => {
        const { valid, errors } = parseIgnores('', TODAY)
        expect(valid).toEqual([])
        expect(errors).toEqual([])
    })

    it('accepts a file with both ignores and forkReviews at the top level', () => {
        const { valid, errors } = parseIgnores(
            'ignores:\n  - id: GHSA-aaaa-aaaa-aaaa\n    reason: Fine.\n    expires: 2026-12-01\n' +
                'forkReviews:\n  - fork: example/fork\n    reviewed: 2026-10-01\n    days: 30\n',
            TODAY
        )
        expect(errors).toEqual([])
        expect(valid).toHaveLength(1)
        expect(valid[0].id).toBe('GHSA-aaaa-aaaa-aaaa')
    })
})

describe('parseAuditJson', () => {
    it('returns no findings for a clean audit', () => {
        expect(parseAuditJson(fixture('pnpm-audit-clean.json'))).toEqual([])
    })

    it('extracts every advisory with its severity', () => {
        const findings = parseAuditJson(fixture('pnpm-audit-findings.json'))
        expect(findings).toHaveLength(3)
        expect(findings.map(f => f.severity).sort()).toEqual(['critical', 'high', 'moderate'])
    })

    it('prefers the GitHub advisory id, because that is what an ignore entry names', () => {
        const findings = parseAuditJson(fixture('pnpm-audit-findings.json'))
        expect(findings.map(f => f.id)).toContain('GHSA-aaaa-aaaa-aaaa')
    })

    it('throws on empty input instead of silently reporting zero findings', () => {
        expect(() => parseAuditJson('')).toThrow(AuditInputError)
        expect(() => parseAuditJson('   \n')).toThrow(AuditInputError)
    })

    it('throws on non-JSON input instead of an uncaught SyntaxError', () => {
        expect(() => parseAuditJson('not json')).toThrow(AuditInputError)
    })

    it('recognises a severity regardless of case', () => {
        const raw = JSON.stringify({
            advisories: {
                1: {
                    id: 1,
                    github_advisory_id: 'GHSA-x',
                    severity: 'Critical',
                    module_name: 'm',
                    title: 't',
                    url: 'u',
                },
            },
        })
        expect(parseAuditJson(raw)[0].severity).toBe('critical')
    })

    it('flags a finding that falls back to the numeric id, which no GHSA ignore can match', () => {
        const raw = JSON.stringify({
            advisories: {
                77: {
                    id: 77,
                    severity: 'high',
                    module_name: 'some-module',
                    title: 't',
                    url: 'u',
                },
            },
        })
        const [finding] = parseAuditJson(raw)
        expect(finding.id).toBe('77')
        expect(finding.numericIdFallback).toBe(true)
    })

    it('does not set numericIdFallback when github_advisory_id is present', () => {
        const [finding] = parseAuditJson(fixture('pnpm-audit-findings.json'))
        expect(finding.numericIdFallback).toBeUndefined()
    })
})

describe('gate', () => {
    const findings = (): ReturnType<typeof parseAuditJson> =>
        parseAuditJson(fixture('pnpm-audit-findings.json'))

    it('blocks high and critical, and leaves moderate below the line', () => {
        const result = gate(findings(), [], 'high')
        expect(result.blocking.map(f => f.severity).sort()).toEqual(['critical', 'high'])
        expect(result.below.map(f => f.severity)).toEqual(['moderate'])
    })

    it('moves a finding with a valid ignore out of blocking', () => {
        const ignores = [
            { id: 'GHSA-aaaa-aaaa-aaaa', reason: 'Not reachable.', expires: '2026-12-01' },
        ]
        const result = gate(findings(), ignores, 'high')
        expect(result.blocking.map(f => f.id)).toEqual(['GHSA-bbbb-bbbb-bbbb'])
        expect(result.ignored.map(f => f.id)).toEqual(['GHSA-aaaa-aaaa-aaaa'])
    })

    it('does not block anything when the audit is clean', () => {
        const result = gate(parseAuditJson(fixture('pnpm-audit-clean.json')), [], 'high')
        expect(result.blocking).toEqual([])
    })

    it('leaves a numeric-id finding blocking even with a GHSA ignore present — a dead end, but still safe', () => {
        const raw = JSON.stringify({
            advisories: {
                77: { id: 77, severity: 'high', module_name: 'm', title: 't', url: 'u' },
            },
        })
        const ignores = [
            { id: 'GHSA-aaaa-aaaa-aaaa', reason: 'Not reachable.', expires: '2026-12-01' },
        ]
        const result = gate(parseAuditJson(raw), ignores, 'high')
        expect(result.blocking.map(f => f.id)).toEqual(['77'])
        expect(result.ignored).toEqual([])
    })

    it('blocks an unrecognised severity rather than treating it as below every threshold', () => {
        const raw = JSON.stringify({
            advisories: {
                1: {
                    id: 1,
                    github_advisory_id: 'GHSA-x',
                    severity: 'catastrophic',
                    module_name: 'm',
                    title: 't',
                    url: 'u',
                },
            },
        })
        const result = gate(parseAuditJson(raw), [], 'critical')
        expect(result.blocking.map(f => f.id)).toEqual(['GHSA-x'])
        expect(result.below).toEqual([])
    })
})

describe('CLI stdout/stderr separation', () => {
    const scriptPath = path.join(__dirname, '..', 'security-gate.ts')
    // tsx is hoisted to the workspace root's node_modules, not this member's.
    const tsxBin = path.join(__dirname, '..', '..', '..', 'node_modules', '.bin', 'tsx')

    const runGate = (input: string): { status: number; stdout: string; stderr: string } => {
        try {
            const stdout = execFileSync(tsxBin, [scriptPath, 'high'], {
                input,
                stdio: ['pipe', 'pipe', 'pipe'],
            }).toString()
            return { status: 0, stdout, stderr: '' }
        } catch (err) {
            const e = err as { status: number; stdout: Buffer; stderr: Buffer }
            return { status: e.status, stdout: e.stdout.toString(), stderr: e.stderr.toString() }
        }
    }

    it('sends BLOCK lines to stderr, not stdout, and exits 1', () => {
        const result = runGate(fixture('pnpm-audit-findings.json'))
        expect(result.status).toBe(1)
        expect(result.stderr).toMatch(/BLOCK/)
        expect(result.stdout).not.toMatch(/BLOCK/)
    })

    it('keeps note/ignored/the summary on stdout', () => {
        const result = runGate(fixture('pnpm-audit-clean.json'))
        expect(result.status).toBe(0)
        expect(result.stdout).toMatch(/No blocking vulnerabilities/)
    })
})
