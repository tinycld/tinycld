import { afterEach, describe, expect, it } from 'vitest'

import { substituteHelpTokens } from '../../lib/help/tokens'
import { setResolvedAddress } from '../../lib/server-address'

// Help topics are static markdown, but connection settings (IMAP/SMTP hosts)
// are per-deployment: the mail server answers on the org's own web hostname.
// {{server-host}} lets a topic show the reader their real hostname instead of
// a made-up example they'd copy verbatim.
describe('substituteHelpTokens', () => {
    afterEach(() => setResolvedAddress(null))

    it('replaces every {{server-host}} with the deployment hostname', () => {
        setResolvedAddress('https://acme.tinycld.org')
        const body = 'Server: `{{server-host}}`\n\n    imaps://{{server-host}}:993'
        expect(substituteHelpTokens(body)).toBe(
            'Server: `acme.tinycld.org`\n\n    imaps://acme.tinycld.org:993'
        )
    })

    it('strips the port from the resolved address', () => {
        setResolvedAddress('http://localhost:8090')
        expect(substituteHelpTokens('{{server-host}}')).toBe('localhost')
    })

    it('falls back to a readable placeholder when unresolved', () => {
        setResolvedAddress(null)
        expect(substituteHelpTokens('{{server-host}}')).toBe('your-tinycld-hostname')
    })

    it('leaves bodies without tokens untouched', () => {
        const body = 'No tokens here, not even {{other}}.'
        expect(substituteHelpTokens(body)).toBe(body)
    })

    it('replaces {{mail-host}} with the given mail host', () => {
        setResolvedAddress('https://acme.tinycld.org')
        expect(
            substituteHelpTokens('imaps://{{mail-host}}:993', { mailHost: 'mx.acme.example' })
        ).toBe('imaps://mx.acme.example:993')
    })

    it('uses the web host for {{mail-host}} when no mail host is given', () => {
        setResolvedAddress('https://acme.tinycld.org')
        expect(substituteHelpTokens('{{mail-host}} {{server-host}}')).toBe(
            'acme.tinycld.org acme.tinycld.org'
        )
        expect(substituteHelpTokens('{{mail-host}}', { mailHost: null })).toBe('acme.tinycld.org')
    })

    it('falls back to the placeholder for {{mail-host}} when nothing is known', () => {
        setResolvedAddress(null)
        expect(substituteHelpTokens('{{mail-host}}')).toBe('your-tinycld-hostname')
    })
})
