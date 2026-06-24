/**
 * Ambient declaration for the app-sibling-provided `app-updater` native module.
 *
 * `app-updater` is a local Expo native module that lives in the runnable app
 * shell (`tinycld/modules/app-updater/`) and is autolinked into the binary at
 * build time. Core consumes it by name (`import AppUpdater from 'app-updater'`)
 * but the module's implementation is only present in the app shell — so this
 * minimal shape lets core typecheck standalone without the module on disk.
 *
 * Keep this in sync with `tinycld/modules/app-updater/src/AppUpdater.types.ts`.
 */
declare module 'app-updater' {
    interface AppUpdaterModuleType {
        /** The bundle id baked into the binary at native build time. */
        getEmbeddedId(): string
        /** The currently active bundle id (embedded, or a promoted OTA bundle). */
        getCurrentBundleId(): string
        /**
         * Hex SHA-256 of the bundle the app is currently running (staged hash for
         * a promoted OTA bundle, else the embedded bundle's hash). Lets the server
         * recognize an already-current bundle across the embedded→OTA boundary.
         * May be "" if the embedded bundle can't be hashed.
         */
        getCurrentBundleHash(): string
        /** The runtime version (app version) baked into the binary. */
        getRuntimeVersion(): string
        /**
         * Stage a downloaded bundle dir as pending; promoted on next reload.
         * `hash` is the bundle's hex SHA-256, recorded for getCurrentBundleHash.
         */
        stageBundle(localDir: string, id: string, hash: string): Promise<void>
        /** Mark the active OTA bundle healthy so rollback won't revert it. */
        markBundleHealthy(): void
        /**
         * Persist a crash-detail string (e.g. the regex pattern that aborted
         * Hermes + the error message) for the active bundle, surfaced via
         * takeRevertedBundle and uploaded on the next (rolled-back) launch.
         * Best-effort; cleared once the bundle is marked healthy.
         */
        recordBundleError(detail: string): void
        /**
         * Mark the active OTA bundle bad so the next launch/reload reverts to the
         * previous bundle. Paired with reload() by the global fatal handler to
         * recover a not-yet-healthy crashing bundle in-session.
         */
        markBundleBad(): void
        /**
         * Read-once: { id, hash, error } of a bundle rolled back since the last
         * call, or null. `error` carries recordBundleError's detail (the crashing
         * regex pattern + message), or "". Reported to the server's report-bad
         * endpoint on boot so the bad bundle stops being advertised fleetwide.
         */
        takeRevertedBundle(): { id: string; hash: string; error: string } | null
        /** Reload the JS runtime, promoting any pending bundle. */
        reload(): Promise<void>
    }
    const AppUpdater: AppUpdaterModuleType
    export default AppUpdater
}
