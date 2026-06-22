import AppUpdater from 'app-updater'
import { Platform } from 'react-native'

declare const __DEV__: boolean

// In-memory "this bundle has reached a stable state this session" flag. The
// global fatal handler (install-fatal-rollback) reads it to decide whether an
// uncaught fatal warrants reverting the bundle: a crash BEFORE the first healthy
// mark means the freshly-applied bundle is broken (revert), whereas a crash
// AFTER it is a normal runtime error the bundle shouldn't be blamed for.
let bundleMarkedHealthy = false

export function isBundleMarkedHealthy(): boolean {
    return bundleMarkedHealthy
}

// Call once after the app's first stable render so crash-rollback won't revert a
// freshly-applied bundle. No-op on web/dev where there is no native updater.
export function markBundleHealthy(): void {
    bundleMarkedHealthy = true
    if (__DEV__ || Platform.OS === 'web') return
    AppUpdater.markBundleHealthy()
}
