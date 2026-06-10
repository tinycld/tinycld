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
    /** True when this user is listed in the super_admins collection, granting
     *  the cross-org /admin console. NOT derived from the user record — it lives
     *  in a separate junction. The session builders below default this to false;
     *  the reactive source of truth in UI is useIsSuperAdmin(), which live-queries
     *  the super_admins store. */
    isSuperAdmin: boolean
}
