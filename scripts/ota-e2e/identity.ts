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

// The server logs each /api/app/update request via slog as a key=value line
// containing `msg="app-update: request"` and `q.currentId=<id>` (optionally
// quoted). Return the currentId, or null if this isn't such a line / has no id.
const APP_UPDATE_MARKER = 'app-update: request'
const CURRENT_ID_RE = /\bq\.currentId=("([^"]*)"|(\S+))/

export function parseCurrentIdFromLogLine(line: string): string | null {
    if (!line.includes(APP_UPDATE_MARKER)) return null
    const m = line.match(CURRENT_ID_RE)
    if (!m) return null
    const value = m[2] ?? m[3] ?? ''
    return value === '' ? null : value
}
