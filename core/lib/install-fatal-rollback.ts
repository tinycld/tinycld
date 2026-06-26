import * as Sentry from '@sentry/react-native'
import AppUpdater from 'app-updater'
import { Platform } from 'react-native'
import { captureException } from './errors'
import { isBundleMarkedHealthy } from './mark-bundle-healthy'

declare const __DEV__: boolean

// Diagnostic globals installed by app/lib/diagnose-regexp.ts (the RegExp shim).
// Present only on Hermes; undefined otherwise. Used to attribute an OTA-bundle
// fatal to the exact regex pattern that aborted Hermes — the native crash report
// only shows `regExpConstructor` with no source.
declare global {
    // eslint-disable-next-line no-var
    var __TINYCLD_LAST_REGEXP: { pattern: string; flags: string; at: string } | undefined
    // eslint-disable-next-line no-var
    var __TINYCLD_REGEXP_ERROR:
        | { pattern: string; flags: string; message: string; at: string }
        | undefined
}

// app-updater's native module MAY expose recordBundleError(detail) to persist a
// crash detail string next to the rollback markers, so the rolled-back next
// launch can upload it (see use-app-updates.reportRevertedBundle). Older binaries
// don't have it; call through a soft type so JS and native can ship independently.
type MaybeErrorRecorder = { recordBundleError?: (detail: string) => void }

// Builds the single triage string persisted + uploaded for an OTA fatal. Leads
// with the last/failed regex (the suspected culprit) when the RegExp shim caught
// one, then the error message — so the report-bad row names the exact pattern.
function buildFatalDetail(error: unknown): string {
    const parts: string[] = []
    const reErr = globalThis.__TINYCLD_REGEXP_ERROR
    const reLast = globalThis.__TINYCLD_LAST_REGEXP
    if (reErr) {
        parts.push(`regexp-throw: /${reErr.pattern}/${reErr.flags} -> ${reErr.message}`)
    } else if (reLast) {
        // No catchable throw recorded (Hermes aborted mid-compile) — the last
        // pattern attempted is the prime suspect.
        parts.push(`regexp-last: /${reLast.pattern}/${reLast.flags}`)
    }
    const msg = error instanceof Error ? `${error.name}: ${error.message}` : String(error)
    parts.push(`fatal: ${msg}`)
    const bundle = (() => {
        try {
            return AppUpdater.getCurrentBundleId()
        } catch {
            return 'unknown'
        }
    })()
    parts.push(`bundle: ${bundle}`)
    return parts.join(' | ').slice(0, 1800) // keep within the report-bad error field
}

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
            // Attribute the fatal to the exact regex pattern when the RegExp shim
            // caught one — the single most useful field for the OTA-crash triage.
            const detail = buildFatalDetail(error)
            captureException('app.globalFatal', error, {
                isFatal: !!isFatal,
                detail,
                lastRegexp: globalThis.__TINYCLD_LAST_REGEXP,
                regexpError: globalThis.__TINYCLD_REGEXP_ERROR,
            })

            // Only a fatal in a not-yet-healthy bundle implicates the bundle
            // itself. A non-fatal, or a crash after the app already proved healthy,
            // is a normal runtime error — don't revert a good bundle for it.
            if (isFatal && !isBundleMarkedHealthy()) {
                // Persist the crash detail next to the rollback markers so the
                // rolled-back NEXT launch can upload it via report-bad (the JS
                // Sentry event below often can't flush before the process dies).
                // Soft-typed: a binary without this native method just skips it.
                try {
                    ;(AppUpdater as unknown as MaybeErrorRecorder).recordBundleError?.(detail)
                } catch {
                    // best-effort persistence — never block rollback on it
                }

                // Give Sentry's transport a brief window to flush the JS event
                // BEFORE we abort/reload, then mark-bad + reload regardless of the
                // flush outcome. Without this the captureException above is enqueued
                // and lost when reload()/RCTFatal kills the process — the reason OTA
                // fatals were invisible in Sentry.
                // @sentry/react-native's flush() takes no timeout (the native SDK
                // bounds it itself) and resolves when the queue drains or times out.
                void Sentry.flush()
                    .catch(() => {})
                    .finally(() => {
                        try {
                            AppUpdater.markBundleBad()
                            // reload() re-runs bundleURL(), which honors the bad
                            // flag and reverts to the previous bundle.
                            void AppUpdater.reload()
                        } catch {
                            // fall through to the chained handler below
                        }
                    })
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
