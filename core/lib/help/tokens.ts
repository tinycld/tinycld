import { getResolvedAddress } from '../server-address'

// Help topics are static markdown, but some facts are per-deployment. Tokens
// let a topic show the reader their real value instead of a made-up example
// they would copy verbatim — the motivating case is mail client setup, where
// the IMAP/SMTP server is the org's own web hostname, not `mail.<domain>`.
//
// Substitution is plain text replacement over the whole body (before markdown
// parsing) so tokens work inside code spans and fenced blocks, which is where
// connection settings live.

const SERVER_HOST_TOKEN = '{{server-host}}'

// Readable in every context a topic might use the token (prose, `code`,
// imaps:// URLs) when the server address is somehow not resolved yet.
const SERVER_HOST_FALLBACK = 'your-tinycld-hostname'

function serverHost(): string {
    const addr = getResolvedAddress()
    if (!addr) return SERVER_HOST_FALLBACK
    try {
        return new URL(addr).hostname
    } catch {
        return SERVER_HOST_FALLBACK
    }
}

/** Replaces deployment tokens in a help topic body. Currently only
 *  {{server-host}} → the hostname this app talks to. */
export function substituteHelpTokens(body: string): string {
    if (!body.includes(SERVER_HOST_TOKEN)) return body
    return body.split(SERVER_HOST_TOKEN).join(serverHost())
}
