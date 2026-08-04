// Unit test setup for the new workspace.
// react-native and @react-native-async-storage/async-storage are aliased
// to CJS stubs in vitest.config.ts before any module loading occurs.
// Add vi.mock() calls here for modules that cannot be handled by resolve.alias.
import { vi } from 'vitest'

// Mock the generated config to empty. This is REQUIRED to break an import
// cycle: the real tinycld.config.ts imports each package's provider, some of
// which (calc, text) eagerly import @tinycld/core/file-viewer components that
// import @tinycld/core/lib/pocketbase, whose module-eval calls
// buildPackageStores(tinycldConfig) — back into the still-initializing config
// ("entries is not iterable"). Tests that exercise the derivation helpers build
// their own entries inline, so an empty real config is harmless. (Mirrors the
// production core unit-setup.)
vi.mock('@tinycld/app-generated/tinycld-config', () => ({
    tinycldConfig: [],
}))
vi.mock('@tinycld/app-generated/package-help', () => ({
    packageHelp: [],
}))

// Native module; only dynamically imported behind a Platform.OS === 'ios'
// guard (core/lib/stores/orientation-store), so tests never load the real
// thing — this shim keeps any future eager import parseable under vitest.
vi.mock('expo-screen-orientation', () => ({
    Orientation: {
        UNKNOWN: 0,
        PORTRAIT_UP: 1,
        PORTRAIT_DOWN: 2,
        LANDSCAPE_LEFT: 3,
        LANDSCAPE_RIGHT: 4,
    },
    getOrientationAsync: async () => 0,
    addOrientationChangeListener: () => ({ remove: () => {} }),
}))
