import type { ExpoConfig } from 'expo/config'
import { version } from './package.json'

// Dynamic Expo config. All static fields live in app.json; this only injects the
// single source of truth for the user-facing version (CFBundleShortVersionString
// / versionName) from package.json, so an iOS/Android release version is bumped
// in exactly one place (`npm version` / release tooling) and never drifts.
//
// Note: runtimeVersion uses the appVersion policy, so this also drives the OTA
// runtime version — a version bump intentionally starts a new OTA lane.
//
// Expo passes the resolved app.json in as `config` when this default export is a
// function, so we spread it and override only `version`.
export default ({ config }: { config: ExpoConfig }): ExpoConfig => ({
    ...config,
    version,
})
