'use strict'

// Stub for the `app-updater` native module in unit tests. It is Metro-resolved
// (no node_modules entry), so Vite's import-analysis can't resolve the bare
// specifier — which fails at transform time under some environments (e.g.
// happy-dom). Aliasing it to this stub (like react-native, async-storage, etc.)
// lets any module that imports `app-updater` load in unit tests regardless of
// env. Mirrors the web stub (modules/app-updater/index.web.ts): a never-OTA'd,
// embedded surface. Tests that need specific values still `vi.mock('app-updater')`.
module.exports = {
    default: {
        getCurrentBundleId: () => 'web',
        getCurrentBundleHash: () => '',
        getRuntimeVersion: () => '',
        markBundleHealthy: () => {},
        takeRevertedBundle: () => null,
    },
}
