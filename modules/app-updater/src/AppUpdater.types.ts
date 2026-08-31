export interface AppUpdaterModuleType {
    /** The bundle id baked into the binary at native build time. */
    getEmbeddedId(): string
    /** The currently active bundle id (embedded, or a promoted OTA bundle). */
    getCurrentBundleId(): string
    /**
     * Hex SHA-256 of the bundle the app is currently running — the staged hash
     * for a promoted OTA bundle, else the embedded bundle's hash. Lets the server
     * recognize an already-current bundle even when ids differ across the
     * embedded→OTA boundary. May be "" if the embedded bundle can't be hashed.
     */
    getCurrentBundleHash(): string
    /** The runtime version (app version) baked into the binary. */
    getRuntimeVersion(): string
    /**
     * Point the store at a server, so every call below reads and writes THAT
     * server's bundle state. `serverKey` is a hex SHA-256 of the server's
     * normalized origin (see serverKeyFor).
     *
     * Bundles are per-server because each org in a hosting deployment runs a
     * different build with a different package set. Call this as soon as an
     * address resolves: the native loader runs before the JS bridge exists, so
     * it can only learn the active server from what was persisted earlier.
     * Idempotent — re-asserting the same key is a no-op.
     */
    setActiveServer(serverKey: string): void
    /** The active server key, or "" when none has been set yet. */
    getActiveServer(): string
    /**
     * Stage a downloaded bundle dir as pending; promoted on next reload. `hash`
     * is the bundle's hex SHA-256 (from the manifest), recorded so
     * getCurrentBundleHash can report it once this bundle is the active one.
     */
    stageBundle(localDir: string, id: string, hash: string): Promise<void>
    /** Mark the active OTA bundle healthy so rollback won't revert it. */
    markBundleHealthy(): void
    /**
     * Persist a crash-detail string (e.g. the regex pattern that aborted Hermes
     * plus the error message) for the active bundle, surfaced via takeRevertedBundle
     * and uploaded on the next (rolled-back) launch. Best-effort; overwrites any
     * prior record; cleared once the bundle is marked healthy.
     */
    recordBundleError(detail: string): void
    /**
     * Mark the active OTA bundle bad so the next launch/reload reverts to the
     * previous bundle. Paired with reload() by the global fatal handler to recover
     * a not-yet-healthy crashing bundle in-session.
     */
    markBundleBad(): void
    /**
     * Read-once: returns the { id, hash, error, serverKey } of a bundle that was
     * rolled back since the last call, or null. `error` carries the detail recorded
     * by recordBundleError (the crashing regex pattern + message), or "" if none. The
     * recovered bundle reports it to the server's report-bad endpoint on boot so the
     * bad bundle stops being advertised fleetwide.
     *
     * `serverKey` names the server the bad bundle CAME FROM, which after a server
     * switch is not the active one — reporting it anywhere else would blocklist an
     * id on a server that never served it. "" when it predates per-server state.
     */
    takeRevertedBundle(): {
        id: string
        hash: string
        error: string
        serverKey: string
    } | null
    /** Reload the JS runtime, promoting any pending bundle. */
    reload(): Promise<void>
}
