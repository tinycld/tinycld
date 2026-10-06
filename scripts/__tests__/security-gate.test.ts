import * as fs from 'node:fs'
import * as path from 'node:path'
import { describe, expect, it } from 'vitest'
import { gate, parseAuditJson, parseIgnores } from '../security-gate'

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
})
