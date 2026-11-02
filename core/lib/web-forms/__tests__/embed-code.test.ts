import { describe, expect, it } from 'vitest'
import { embedCode, publicFormURL } from '../embed-code'

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
