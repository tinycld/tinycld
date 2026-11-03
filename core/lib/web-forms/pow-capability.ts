/** Proof of work hashes with Web Crypto, which browsers give only to https (and localhost). */
export function canSolvePow(env: { isSecureContext?: boolean; subtle?: unknown }): boolean {
    return env.isSecureContext !== false && env.subtle !== undefined
}

/**
 * Whether this browser could solve the proof of work. Outside a browser (the
 * native app) there is no page to judge, so it does not block the setting.
 */
export function browserCanSolvePow(): boolean {
    if (typeof window === 'undefined' || typeof document === 'undefined') return true
    return canSolvePow({
        isSecureContext: window.isSecureContext,
        subtle: globalThis.crypto?.subtle,
    })
}
