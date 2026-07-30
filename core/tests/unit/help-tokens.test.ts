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
})
