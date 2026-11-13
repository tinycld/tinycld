// The mail listeners of an e2e server bind FIXED ports, so two instances on
// one box collide (PB logs "address already in use" and carries on, but the
// noise is misleading, and two workspaces running e2e at once share one IMAP
// listener). Each port comes from the environment, like E2E_PORT does for HTTP:
// E2E_IMAP_PORT, E2E_IMAPS_PORT, E2E_SMTP_PORT and E2E_SMTPS_PORT for the
// primary instance, the same names with a `_2` suffix for the second one.
//
// Defaults: the primary sets only IMAP (:1193, which matches dev.ts and mail's
// tests/imap-helpers.ts) and leaves the rest to the mail server's own
// defaults; the second offsets every listener so it never meets the primary.

const LISTENERS = [
    ['IMAP_ADDR', 'E2E_IMAP_PORT'],
    ['IMAPS_ADDR', 'E2E_IMAPS_PORT'],
    ['SMTP_ADDR', 'E2E_SMTP_PORT'],
    ['SMTPS_ADDR', 'E2E_SMTPS_PORT'],
] as const

type ListenerAddr = (typeof LISTENERS)[number][0]

const PRIMARY_DEFAULTS: Partial<Record<ListenerAddr, number>> = { IMAP_ADDR: 1193 }
const SECOND_DEFAULTS: Partial<Record<ListenerAddr, number>> = {
    IMAP_ADDR: 1293,
    IMAPS_ADDR: 2093,
    SMTP_ADDR: 1687,
    SMTPS_ADDR: 1466,
}

function parsePort(name: string, raw: string): number {
    const n = Number(raw)
    if (!Number.isInteger(n) || n <= 0 || n > 65_535) {
        throw new Error(`e2e-serve: ${name} must be a port number (got ${raw})`)
    }
    return n
}

export function mailListenerEnv(
    env: Record<string, string | undefined>,
    instance: 'primary' | 'second'
): Record<string, string> {
    const defaults = instance === 'second' ? SECOND_DEFAULTS : PRIMARY_DEFAULTS
    const suffix = instance === 'second' ? '_2' : ''
    const result: Record<string, string> = {}
    for (const [addr, portVar] of LISTENERS) {
        const name = `${portVar}${suffix}`
        const raw = env[name]
        const port = raw ? parsePort(name, raw) : defaults[addr]
        if (port !== undefined) result[addr] = `:${port}`
    }
    return result
}
