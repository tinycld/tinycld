declare const __DEV__: boolean

import * as Sentry from '@sentry/react-native'
import { Platform } from 'react-native'
import { getCoreConfigOptional } from './core-config'
import { scrubPII } from './sentry-scrub'

let initialized = false

// Sentry `release` + `dist` overrides — set ONLY when a promoted OTA bundle is
// active, and then they MUST match what the server uploaded its sourcemaps under
// (coreserver app_native_export.go: sentryReleaseFor / uploadBundleSourcemaps) or
// symbolication silently fails:
//
//   release = "tinycld@<app-version>"  (both sides derive it the same way)
//   dist    = the OTA bundle id        (the per-bundle key)
//
// CRITICAL — for the EMBEDDED bundle we return {} and let the SDK use its
// auto-derived release/dist (bundleId@version+buildNumber / buildNumber). That's
// exactly what the EAS @sentry/react-native build uploads the embedded
// sourcemaps under. Overriding them here would make App Store (embedded) crashes
// stop symbolicating — a regression for the common case — to fix the OTA case.
// We distinguish by id: getCurrentBundleId() is "embedded-<version>" for the
// embedded bundle vs "build-<ts>-<platform>" for a promoted OTA bundle.
function sentryReleaseAndDist(): { release?: string; dist?: string } {
    if (Platform.OS === 'web') return {}
    try {
        // Lazy require + a minimal local type, NOT a top-level
        // `import ... from 'app-updater'`: that module is native-only and lives
        // in the runnable app shell. A static import (value OR type) pulls its
        // app-shell-resident ambient declaration into the tsconfig program of
        // every feature that imports captureException (errors.ts → sentry.ts) —
        // and features can't resolve it standalone, so their typecheck fails with
        // "Cannot find module 'app-updater'". Typing the require against an inline
        // interface keeps this file feature-resolvable; the helper only ever runs
        // on native, where the real module is present.
        interface BundleInfo {
            getCurrentBundleId(): string
            getRuntimeVersion(): string
        }
        const AppUpdater = (require('app-updater') as { default: BundleInfo }).default
        const id = AppUpdater.getCurrentBundleId()
        // Embedded (or unknown) → no override; the EAS-uploaded sourcemaps own it.
        if (!id || id.startsWith('embedded')) return {}
        const version = AppUpdater.getRuntimeVersion()
        return {
            release: version ? `tinycld@${version}` : undefined,
            dist: id,
        }
    } catch {
        return {}
    }
}

export function initSentry(): void {
    if (initialized) return
    const config = getCoreConfigOptional()
    const dsn = config?.sentryDsn
    if (__DEV__) {
        console.info('[sentry] init skipped — __DEV__ build')
        return
    }
    if (!dsn) {
        console.warn(
            '[sentry] init skipped — no DSN. The DSN is set in lib/app-config.ts (appConfig.sentryDsn); a missing one means configureCore() did not run before initSentry().'
        )
        return
    }

    // release/dist are paired with the server's sourcemap upload so OTA-bundle
    // crashes symbolicate; an explicit release also overrides Sentry's auto-derived
    // one (which appConfig.release didn't set, leaving it unattributable before).
    const { release, dist } = sentryReleaseAndDist()

    Sentry.init({
        dsn,
        environment: config?.environment ?? 'production',
        release: release ?? config?.release,
        // `dist` distinguishes WHICH JS bundle a crash came from. The OTA updater
        // swaps the bundle out from under a fixed native binary/release, so without
        // a per-bundle dist every OTA build collapses onto the same release and its
        // stack frames can't be mapped to the right uploaded sourcemap (events
        // arrive unsymbolicated or get dropped — which is why an OTA crash can look
        // "missing" in Sentry). Tag it with the active bundle id.
        dist,
        // Capture native crashes (the SIGABRT a fatal JS error escalates to via
        // RCTFatal). The native handler persists the crash to disk and uploads it
        // on the NEXT launch — the only path that survives a process abort, since a
        // JS-layer captureException can't flush before the process dies. Explicit
        // (not relying on the SDK default) so it can't be silently turned off.
        enableNativeCrashHandling: true,
        attachStacktrace: true,
        beforeSend(event) {
            return scrubPII(event) as typeof event
        },
        beforeBreadcrumb(breadcrumb) {
            return scrubPII(breadcrumb) as typeof breadcrumb
        },
        tracesSampleRate: 0.1,
        replaysSessionSampleRate: 0,
        replaysOnErrorSampleRate: 0,
    })
    initialized = true
    console.info(
        `[sentry] initialized (env=${config?.environment ?? 'production'}, release=${release ?? config?.release ?? 'unknown'}, dist=${dist ?? 'none'})`
    )
}

/**
 * Lightweight breadcrumb-style log line that goes to Sentry as a "info" message
 * AND to the browser console. Use sparingly — for tracing intermittent prod
 * issues where you need to see a sequence of events. Remove the call sites once
 * the bug is found.
 */
export function captureMessageToSentry(
    context: string,
    message: string,
    extra?: Record<string, unknown>
): void {
    const scrubbedExtra = extra ? scrubPII(extra) : undefined
    console.info(`[trace:${context}] ${message}`, scrubbedExtra ?? '')
    if (!initialized) return
    Sentry.withScope(scope => {
        scope.setTag('context', context)
        scope.setLevel('info')
        if (scrubbedExtra) scope.setExtras(scrubbedExtra)
        Sentry.captureMessage(message)
    })
}

export function captureExceptionToSentry(
    context: string,
    error: unknown,
    extra?: Record<string, unknown>
): void {
    if (!initialized) {
        console.error(`[sentry:not-initialized] ${context}`, error, extra)
        return
    }
    Sentry.withScope(scope => {
        scope.setTag('context', context)
        if (extra) scope.setExtras(scrubPII(extra))
        Sentry.captureException(error)
    })
}

const SENTRY_BREADCRUMB_LEVEL: Record<string, Sentry.SeverityLevel> = {
    debug: 'debug',
    info: 'info',
    warn: 'warning',
    error: 'error',
}

/**
 * Record a log line as a Sentry breadcrumb. Breadcrumbs are an in-memory ring
 * buffer — no network — and are attached automatically to whatever event fires
 * next, which is what gives an error its preceding context.
 *
 * PII scrubbing is handled by the `beforeBreadcrumb` hook registered in
 * `initSentry`; do not scrub here or the data gets filtered twice.
 */
export function addBreadcrumbToSentry(
    context: string,
    level: string,
    message: string,
    extra?: Record<string, unknown>
): void {
    if (!initialized) return
    Sentry.addBreadcrumb({
        category: context,
        message,
        level: SENTRY_BREADCRUMB_LEVEL[level] ?? 'info',
        data: extra,
    })
}
