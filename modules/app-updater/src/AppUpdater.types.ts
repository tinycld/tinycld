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
     * Stage a downloaded bundle dir as pending; promoted on next reload. `hash`
     * is the bundle's hex SHA-256 (from the manifest), recorded so
     * getCurrentBundleHash can report it once this bundle is the active one.
     */
    stageBundle(localDir: string, id: string, hash: string): Promise<void>
    /** Mark the active OTA bundle healthy so rollback won't revert it. */
    markBundleHealthy(): void
    /**
     * Mark the active OTA bundle bad so the next launch/reload reverts to the
     * previous bundle. Paired with reload() by the global fatal handler to recover
     * a not-yet-healthy crashing bundle in-session.
     */
    markBundleBad(): void
    /**
     * Read-once: returns the { id, hash } of a bundle that was rolled back since
     * the last call, or null. The recovered bundle reports it to the server's
     * report-bad endpoint on boot so the bad bundle stops being advertised fleetwide.
     */
    takeRevertedBundle(): { id: string; hash: string } | null
    /** Reload the JS runtime, promoting any pending bundle. */
    reload(): Promise<void>
}
