export interface UserSession {
    id: string
    name: string
    email: string
    primaryOrgSlug?: string
    /** True for sandboxed demo accounts. Outbound side effects (mail send,
     *  invite email, share email, expo push) are suppressed server-side. The
     *  UI uses this flag to show a persistent "Demo" banner. */
    isDemo: boolean
    /** Mirrors users.metadata.isBetaTester in PocketBase. Retained as user
     *  metadata but no longer drives an update channel — the self-hosted updater
     *  serves one bundle to everyone, so the beta-channel split is deferred
     *  future work. Set the JSON value by hand against an individual user in the
     *  PocketBase admin (no UI yet). */
    isBetaTester: boolean
}
