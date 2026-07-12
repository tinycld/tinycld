import { describe, expect, it } from 'vitest'
import { PII_KEY_PATTERN, scrubPII } from './sentry-scrub'

describe('scrubPII', () => {
    it('removes top-level PII keys', () => {
        const input = { email: 'a@b.com', status: 500 }
        expect(scrubPII(input)).toEqual({ email: '[Filtered]', status: 500 })
    })

    it('removes nested PII keys', () => {
        const input = { user: { email: 'a@b.com', id: '1' } }
        expect(scrubPII(input)).toEqual({ user: { email: '[Filtered]', id: '1' } })
    })

    it('scrubs arrays element-wise', () => {
        const input = { contacts: [{ name: 'x', id: '1' }] }
        expect(scrubPII(input)).toEqual({ contacts: [{ name: '[Filtered]', id: '1' }] })
    })

    it('scrubs body, subject, filename, phone, address, content, title', () => {
        const input = {
            body: 'x',
            subject: 'x',
            filename: 'x',
            phone: 'x',
            address: 'x',
            content: 'x',
            title: 'x',
            ok: 'keep',
        }
        const out = scrubPII(input) as Record<string, unknown>
        expect(out.ok).toBe('keep')
        for (const k of ['body', 'subject', 'filename', 'phone', 'address', 'content', 'title']) {
            expect(out[k]).toBe('[Filtered]')
        }
    })

    it('handles null, undefined, primitives, cycles', () => {
        expect(scrubPII(null)).toBe(null)
        expect(scrubPII(undefined)).toBe(undefined)
        expect(scrubPII(42)).toBe(42)
        const a: Record<string, unknown> = {}
        a.self = a
        const out = scrubPII(a) as Record<string, unknown>
        expect(out.self).toBe('[Circular]')
    })

    it('PII_KEY_PATTERN is case-insensitive', () => {
        expect(PII_KEY_PATTERN.test('Email')).toBe(true)
        expect(PII_KEY_PATTERN.test('USER_EMAIL')).toBe(true)
        expect(PII_KEY_PATTERN.test('orgId')).toBe(false)
    })

    it('scrubs credential keys', () => {
        const input = {
            token: 'abc',
            password: 'p',
            secret: 's',
            authorization: 'Bearer x',
            apiKey: 'k',
            api_key: 'k2',
            s3_key: 'k3',
            ok: 'keep',
        }
        const out = scrubPII(input) as Record<string, unknown>
        expect(out.ok).toBe('keep')
        for (const k of [
            'token',
            'password',
            'secret',
            'authorization',
            'apiKey',
            'api_key',
            's3_key',
        ]) {
            expect(out[k]).toBe('[Filtered]')
        }
    })

    it('does not over-scrub benign identifier keys', () => {
        const input = { orgId: '1', monkey: 'ook', keyboard: 'qwerty', id: 'x' }
        expect(scrubPII(input)).toEqual(input)
    })

    it('redacts the token in a [share-session, token] queryKey array', () => {
        expect(scrubPII(['share-session', 'super-secret-token'])).toEqual([
            'share-session',
            '[Filtered]',
        ])
    })

    it('redacts credential query params in URL string values', () => {
        const input = { url: 'https://api.example.com/x?token=abc123&keep=1' }
        const out = scrubPII(input) as Record<string, unknown>
        expect(out.url).toBe('https://api.example.com/x?token=[Filtered]&keep=1')
    })

    it('redacts a bare ?token= URL passed as a top-level string', () => {
        expect(scrubPII('/api/session?token=deadbeef')).toBe('/api/session?token=[Filtered]')
    })
})
