import { serverHostname } from '../server-host'

// Help topics are static markdown, but some facts are per-deployment. Tokens
// let a topic show the reader their real value instead of a made-up example
// they would copy verbatim — the motivating case is mail client setup, where
// the IMAP/SMTP server is the org's own web hostname, not `mail.<domain>`.
//
// Substitution is plain text replacement over the whole body (before markdown
// parsing) so tokens work inside code spans and fenced blocks, which is where
// connection settings live. {{mail-host}} is the host mail apps connect to; it
// is the web host unless the deployment names another.

const SERVER_HOST_TOKEN = '{{server-host}}'
const MAIL_HOST_TOKEN = '{{mail-host}}'

// Readable in every context a topic might use the token (prose, `code`,
// imaps:// URLs) when the server address is somehow not resolved yet.
const SERVER_HOST_FALLBACK = 'your-tinycld-hostname'

function replaceAll(body: string, token: string, value: string): string {
    if (!body.includes(token)) return body
    return body.split(token).join(value)
}

/** Replaces deployment tokens in a help topic body: {{server-host}} → the host
 *  this app talks to; {{mail-host}} → the host mail apps connect to (the web
 *  host unless the deployment names another). */
export function substituteHelpTokens(
    body: string,
    values: { mailHost?: string | null } = {}
): string {
    const serverHost = serverHostname() ?? SERVER_HOST_FALLBACK
    const withServer = replaceAll(body, SERVER_HOST_TOKEN, serverHost)
    return replaceAll(withServer, MAIL_HOST_TOKEN, values.mailHost || serverHost)
}
