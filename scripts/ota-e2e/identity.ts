// The OTA bundle id shapes, established by the native config plugin and the
// server export pipeline. They never collide, so an observed flip from one
// class to the other is an unambiguous "the app reloaded into a new bundle".
//   embedded-<appVersion>      — baked into the binary (plugins/with-app-updater.cjs)
//   build-<unixMilli>-<plat>   — minted by the single-tenant installer
//   recipe-<hash12>-<plat>     — minted by the multi-org builder, content-addressed
//                                on the recipe hash, so two orgs with the same
//                                package set advertise the SAME bundle id

export type BundleIdClass = 'embedded' | 'server' | 'unknown'

export function embeddedIdForVersion(appVersion: string): string {
    return `embedded-${appVersion}`
}

export function classifyBundleId(id: string): BundleIdClass {
    if (/^embedded-.+/.test(id)) return 'embedded'
    if (/^(build-\d+|recipe-[a-f0-9]{12})-(ios|android)$/.test(id)) return 'server'
    return 'unknown'
}
