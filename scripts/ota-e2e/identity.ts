// The OTA bundle id shapes, established by the native config plugin and the
// server export pipeline. They never collide, so an observed flip from one
// class to the other is an unambiguous "the app reloaded into a new bundle".
//   embedded-<appVersion>   — baked into the binary (plugins/with-app-updater.cjs)
//   build-<unixMilli>-<plat> — minted by the server (app_native_export.go)

export type BundleIdClass = 'embedded' | 'server' | 'unknown'

export function embeddedIdForVersion(appVersion: string): string {
    return `embedded-${appVersion}`
}

export function classifyBundleId(id: string): BundleIdClass {
    if (/^embedded-.+/.test(id)) return 'embedded'
    if (/^build-\d+-(ios|android)$/.test(id)) return 'server'
    return 'unknown'
}
