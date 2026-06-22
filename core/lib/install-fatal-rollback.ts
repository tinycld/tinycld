import AppUpdater from 'app-updater'
import { Platform } from 'react-native'
import { captureException } from './errors'
import { isBundleMarkedHealthy } from './mark-bundle-healthy'

declare const __DEV__: boolean

// React Native's global error hook. ErrorUtils is a RN global; type it minimally
// so we don't depend on @types internals.
interface ErrorUtilsLike {
    getGlobalHandler?: () => ((error: unknown, isFatal?: boolean) => void) | undefined
    setGlobalHandler: (handler: (error: unknown, isFatal?: boolean) => void) => void
}
declare const ErrorUtils: ErrorUtilsLike | undefined

let installed = false

// installFatalRollbackHandler wires a global JS error handler that does two
// things the React error boundary and Sentry's own hook do NOT cover on their
// own for an OTA-updated build:
//
//  1. A fatal JS error that escapes React's render tree (e.g. thrown from a
//     TurboModule async callback, a timer, or module-init of a freshly-applied
//     bundle) never reaches <ErrorBoundary> — it goes straight to RCTFatal and
//     aborts the process. We capture it to Sentry here so it's actually
//     reported, then chain RN's existing handler (Sentry installs one too) so
//     the native crash path still runs.
//
//  2. If the fatal happens BEFORE the bundle was ever marked healthy this
//     session, the freshly-promoted bundle is the suspect. We flag it bad
//     (markBundleBad) and reload — bundleURL() then reverts to the previous
//     bundle on the way back up, recovering IN-SESSION instead of waiting for
//     the native crash-launch counter to trip after repeated visible crashes.
//
// Must be installed as early as possible (before any app code that could throw),
// from the embedded/previous good bundle — a bundle can't reliably rescue itself
// if it crashes during its own init, so the value is in the ALREADY-RUNNING
// bundle catching the NEXT one's failure. No-op on web/dev.
export function installFatalRollbackHandler(): void {
    if (installed || __DEV__ || Platform.OS === 'web') return
    if (typeof ErrorUtils === 'undefined') return
    installed = true

    const previous = ErrorUtils.getGlobalHandler?.()

    ErrorUtils.setGlobalHandler((error: unknown, isFatal?: boolean) => {
        try {
            captureException('app.globalFatal', error, { isFatal: !!isFatal })

            // Only a fatal in a not-yet-healthy bundle implicates the bundle
            // itself. A non-fatal, or a crash after the app already proved healthy,
            // is a normal runtime error — don't revert a good bundle for it.
            if (isFatal && !isBundleMarkedHealthy()) {
                AppUpdater.markBundleBad()
                // reload() re-runs bundleURL(), which honors the bad flag and
                // reverts to the previous bundle. Wrapped so a reload failure can't
                // swallow the chained handler below.
                void AppUpdater.reload()
            }
        } catch {
            // Never let recovery bookkeeping mask the original crash — fall through
            // to the previous handler regardless.
        }

        // Chain RN's prior handler (Sentry's native-crash hook lives here) so the
        // platform crash path and any other reporting still run.
        previous?.(error, isFatal)
    })
}
