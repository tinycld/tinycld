import AppUpdater from 'app-updater'
import { Platform } from 'react-native'

declare const __DEV__: boolean

// Call once after the app's first stable render so crash-rollback won't revert a
// freshly-applied bundle. No-op on web/dev where there is no native updater.
export function markBundleHealthy(): void {
    if (__DEV__ || Platform.OS === 'web') return
    AppUpdater.markBundleHealthy()
}
