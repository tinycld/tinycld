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
        /** The runtime version (app version) baked into the binary. */
        getRuntimeVersion(): string
        /** Stage a downloaded bundle dir as pending; promoted on next reload. */
        stageBundle(localDir: string, id: string): Promise<void>
        /** Mark the active OTA bundle healthy so rollback won't revert it. */
        markBundleHealthy(): void
        /** Reload the JS runtime, promoting any pending bundle. */
        reload(): Promise<void>
    }
    const AppUpdater: AppUpdaterModuleType
    export default AppUpdater
}
