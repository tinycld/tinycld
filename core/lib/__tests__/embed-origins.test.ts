import { describe, expect, it } from 'vitest'
import { embedOriginProblem, embedOriginsSchema, MAX_EMBED_ORIGINS } from '../embed-origins'

describe('embed origins', () => {
    it('accepts an empty list and valid origins', () => {
        expect(embedOriginsSchema.safeParse('').success).toBe(true)
        expect(
            embedOriginsSchema.safeParse('https://a.example http://b.example:8080').success
        ).toBe(true)
    })

    it.each([
        ['a.example', 'must include a scheme'],
        ['ftp://a.example', 'must use http or https'],
        ['https://a.example/x', 'origin only'],
        ['https://*.a.example', 'wildcard'],
        ['https://a.example;x', 'unsupported character'],
    ])('rejects %s', (raw, message) => {
        expect(embedOriginProblem(raw)).toContain(message)
    })

    it('rejects more than ten', () => {
        const many = Array.from({ length: 11 }, (_, i) => `https://h${i}.example`).join(' ')
        expect(embedOriginsSchema.safeParse(many).success).toBe(false)
    })
})

// The cases boards relied on before it moved to this parser.
describe('embedOriginProblem, as boards used it', () => {
    it('accepts an origin, with or without a trailing slash or a port', () => {
        expect(embedOriginProblem('https://example.com')).toBeNull()
        expect(embedOriginProblem('https://example.com/')).toBeNull()
        expect(embedOriginProblem('http://localhost:3000')).toBeNull()
    })

    it('names each refusal the way the server does', () => {
        expect(embedOriginProblem('example.com')).toContain('must include a scheme')
        expect(embedOriginProblem('ftp://example.com')).toContain('must use http or https')
        expect(embedOriginProblem('https://')).toContain('missing a host')
        expect(embedOriginProblem('https://example.com/page')).toContain('origin only')
        expect(embedOriginProblem('https://user@example.com')).toContain('origin only')
        expect(embedOriginProblem('https://example.com?x=1')).toContain('origin only')
        expect(embedOriginProblem('https://*.example.com')).toContain('wildcard')
        expect(embedOriginProblem("https://example.com;'")).toContain('unsupported character')
    })
})

describe('embedOriginsSchema, as boards used it', () => {
    const firstMessage = (raw: string) =>
        embedOriginsSchema.safeParse(raw).error?.issues[0]?.message

    it('allows an empty or blank list', () => {
        expect(embedOriginsSchema.safeParse('').success).toBe(true)
        expect(embedOriginsSchema.safeParse('  ').success).toBe(true)
    })

    it('accepts several origins separated by spaces or newlines', () => {
        expect(
            embedOriginsSchema.safeParse('https://a.example.com\nhttps://b.example.com').success
        ).toBe(true)
    })

    it('reports the first bad origin', () => {
        expect(firstMessage('https://ok.example.com example.com')).toContain(
            '"example.com" must include a scheme'
        )
    })

    it('caps the number of origins', () => {
        const many = Array.from(
            { length: MAX_EMBED_ORIGINS + 1 },
            (_, i) => `https://s${i}.example.com`
        )
        expect(firstMessage(many.join(' '))).toBe(`At most ${MAX_EMBED_ORIGINS} websites`)
    })
})
