import type { ExpoConfig } from 'expo/config'

// Dynamic Expo config. All static fields — including the user-facing store/OTA
// version (CFBundleShortVersionString / versionName) — live in app.json. The
// store version is intentionally DECOUPLED from package.json: a project release
// bump (`pnpm version`) must never silently change what ships to the App Store /
// Play Store. To cut a new app release, bump `expo.version` in app.json.
//
// This function only derives runtimeVersion from that single source of truth.
// runtimeVersion is set to the SAME value as version (not the `appVersion`
// policy, which only applies to expo-updates — we ship our own `app-updater`
// instead). This is the OTA runtime version: with-app-updater.cjs stamps it into
// the native binary (Info.plist TinyCldRuntimeVersion) and the server's
// appVersionFromManifest reads the same app version (it reads app.json's
// expo.version first), so a bundle only OTA-updates devices on the matching
// runtime. A version bump intentionally starts a new OTA lane.
//
// Expo passes the resolved app.json in as `config` when this default export is a
// function, so we spread it and derive runtimeVersion from its version.
export default ({ config }: { config: ExpoConfig }): ExpoConfig => ({
    ...config,
    runtimeVersion: config.version,
})
