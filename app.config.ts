import type { ExpoConfig } from 'expo/config'
import { version } from './package.json'

// Dynamic Expo config. All static fields live in app.json; this injects the
// single source of truth for the user-facing version (CFBundleShortVersionString
// / versionName) from package.json, so an iOS/Android release version is bumped
// in exactly one place (`npm version` / release tooling) and never drifts.
//
// runtimeVersion is set to the SAME value (not the `appVersion` policy, which
// only applies to expo-updates — we ship our own `app-updater` instead). This is
// the OTA runtime version: with-app-updater.cjs stamps it into the native binary
// (Info.plist TinyCldRuntimeVersion) and the server's appVersionFromManifest reads
// the same app version, so a bundle only OTA-updates devices on the matching
// runtime. A version bump intentionally starts a new OTA lane. Setting it here
// (vs a static string in app.json) keeps it tied to package.json — no drift.
//
// Expo passes the resolved app.json in as `config` when this default export is a
// function, so we spread it and override version + runtimeVersion.
export default ({ config }: { config: ExpoConfig }): ExpoConfig => ({
    ...config,
    version,
    runtimeVersion: version,
})
