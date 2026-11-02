import { describe, expect, it } from 'vitest'
import { embedOriginProblem, embedOriginsSchema } from '../embed-origins'

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
