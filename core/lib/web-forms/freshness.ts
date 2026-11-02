/** A proof of work lives 10 minutes on the server; renew it a minute early. */
export const POW_MAX_AGE_MS = 9 * 60 * 1000
/** A stamp lives 2 hours on the server; renew it ten minutes early. */
export const STAMP_MAX_AGE_MS = 110 * 60 * 1000

export function isStale(at: number, now: number, maxAgeMs: number): boolean {
    return now - at > maxAgeMs
}
