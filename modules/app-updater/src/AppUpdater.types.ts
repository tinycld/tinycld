export interface AppUpdaterModuleType {
    /** The bundle id baked into the binary at native build time. */
    getEmbeddedId(): string
    /** The currently active bundle id (embedded, or a promoted OTA bundle). */
    getCurrentBundleId(): string
    /** The runtime version (app version) baked into the binary. */
    getRuntimeVersion(): string
    /** Stage a downloaded bundle dir as pending; promoted on next reload. */
    stageBundle(localDir: string, id: string): Promise<void>
    /** Mark the active OTA bundle healthy so rollback won't revert it. */
    markBundleHealthy(): void
    /** Reload the JS runtime, promoting any pending bundle. */
    reload(): Promise<void>
}
