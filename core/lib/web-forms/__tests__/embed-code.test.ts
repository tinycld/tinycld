import { describe, expect, it } from 'vitest'
import { embedCode, publicFormURL } from '../embed-code'
import cases from '../fixtures/embed-cases.json'

describe('embed code', () => {
    it('builds the public URL', () => {
        expect(publicFormURL('https://acme.example', 'tok')).toBe(
            'https://acme.example/p/forms/tok'
        )
    })

    it('builds an iframe and the helper script, escaping the title', () => {
        const code = embedCode({
            origin: 'https://acme.example',
            token: 'tok',
            title: 'Jobs & "Careers"',
        })
        expect(code).toBe(
            '<iframe src="https://acme.example/p/forms/tok" data-tinycld-form title="Jobs &amp; &quot;Careers&quot;" style="width:100%;border:0" loading="lazy"></iframe>\n' +
                '<script src="https://acme.example/forms-embed.js" async></script>'
        )
    })
})
describe('embed code shared fixture', () => {
    it.each(cases)('matches the CLI for $title', c => {
        expect(publicFormURL(c.origin, c.token)).toBe(c.url)
        expect(embedCode({ origin: c.origin, token: c.token, title: c.title })).toBe(c.code)
    })
})
