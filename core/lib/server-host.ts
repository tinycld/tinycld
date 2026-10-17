import { getResolvedAddress } from './server-address'

/** The host name of the server this app talks to, or null before it is known. */
export function serverHostname(): string | null {
    const addr = getResolvedAddress()
    if (!addr) return null
    try {
        return new URL(addr).hostname || null
    } catch {
        return null
    }
}
