import { Platform } from 'react-native'

declare const __DEV__: boolean

// Thrown when no mechanism exists to restart the JS context. Callers MUST treat
// this as a refused operation, not a soft failure — see the module comment.
export class ReloadUnavailableError extends Error {
    constructor(reason: string) {
        super(`Cannot restart the JS context: ${reason}`)
        this.name = 'ReloadUnavailableError'
    }
}

// reloadJsContext restarts the JS context, and is the ONLY sanctioned way to do
// so. Switching servers depends on it, and depends on it either working or
// throwing — never on it quietly doing nothing.
//
// Why that distinction is load-bearing: a server switch points `pb` at the new
// host via the PB_SERVER_ADDR proxy, but the pbtsdb collections, the QueryClient
// and PBTSDBProvider are module-eval singletons bound to the OLD server, and
// nothing re-creates them (clearStores() is terminal — see the 'cleaned-up'
// skip in pocketbase.ts). Only a fresh module graph rebinds them. So a reload
// that no-ops leaves `pb` on server B while every collection still reads server
// A — a half-switch that presents as success.
//
// That state is easy to reach by accident: `modules/app-updater/index.web.ts`
// stubs reload() as an inert `async () => {}` that resolves happily, and
// `modules/app-updater/index.ts` throws at IMPORT time when the native module is
// absent. Hence: require the real native module at call time, and throw
// otherwise.
//
// Deliberately no dev fallback. `expo-updates` is not a dependency of this app
// (the OTA path is our own native module), and we do not want update machinery
// in dev builds. In dev the switch is refused and the caller tells the user to
// restart — the saved active-server pointer is already persisted by then, so a
// manual restart completes the switch through the normal launch path.
export async function reloadJsContext(): Promise<void> {
    if (Platform.OS === 'web') {
        throw new ReloadUnavailableError('web has no JS context to restart')
    }
    if (__DEV__) {
        throw new ReloadUnavailableError('the dev bundler owns the JS context')
    }

    // Required lazily: a top-level import of 'app-updater' throws at module
    // scope wherever the native module is absent (web bundle, tests), which
    // would take down every importer of this file rather than just this call.
    let AppUpdater: { reload?: () => Promise<void> }
    try {
        AppUpdater = require('app-updater').default
    } catch {
        throw new ReloadUnavailableError('the app-updater native module is unavailable')
    }
    if (typeof AppUpdater?.reload !== 'function') {
        throw new ReloadUnavailableError('this binary has no reload support')
    }

    await AppUpdater.reload()
}

// isReloadAvailable reports whether reloadJsContext() can succeed, so UI can say
// "restart to finish" up front rather than after a failed switch. Kept in sync
// with the guards above by construction: same platform/__DEV__ conditions.
export function isReloadAvailable(): boolean {
    if (Platform.OS === 'web' || __DEV__) return false
    try {
        return typeof require('app-updater').default?.reload === 'function'
    } catch {
        return false
    }
}
