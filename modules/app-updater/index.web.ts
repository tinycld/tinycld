import type { AppUpdaterModuleType } from './src/AppUpdater.types'

// Web stub for the `app-updater` native module. The real index.ts calls
// requireNativeModule('AppUpdaterModule'), which throws on web (the module is
// iOS/Android only — see expo-module.config.json `platforms`). Metro resolves
// THIS file for web bundles (the `.web` extension wins over `.ts`), so the web
// bundle never reaches that throw.
//
// Every method is an inert no-op: web has no OTA bundle to stage, hash, or
// reload, and the callers (use-app-updates, mark-bundle-healthy) already guard
// on Platform.OS === 'web' and never invoke these. The stub exists purely so the
// top-level `import AppUpdater from 'app-updater'` resolves when bundling for web.
const AppUpdater: AppUpdaterModuleType = {
    getEmbeddedId: () => 'web',
    getCurrentBundleId: () => 'web',
    getCurrentBundleHash: () => '',
    getRuntimeVersion: () => '',
    stageBundle: async () => {},
    markBundleHealthy: () => {},
    reload: async () => {},
}

export default AppUpdater
