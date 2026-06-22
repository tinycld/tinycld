declare const __DEV__: boolean

import * as Sentry from '@sentry/react-native'
import AppUpdater from 'app-updater'
import { Platform } from 'react-native'
import { getCoreConfigOptional } from './core-config'
import { scrubPII } from './sentry-scrub'

let initialized = false

// The active JS bundle id, used as Sentry's `dist` so an OTA bundle's crashes map
// to the right uploaded sourcemap (see the dist comment in initSentry). Undefined
// on web (no native bundle) and best-effort: any failure leaves dist unset rather
// than breaking Sentry init.
function currentBundleDist(): string | undefined {
    if (Platform.OS === 'web') return undefined
    try {
        return AppUpdater.getCurrentBundleId() || undefined
    } catch {
        return undefined
    }
}

export function initSentry(): void {
    if (initialized) return
    const config = getCoreConfigOptional()
    const dsn = config?.sentryDsn
    if (__DEV__) {
        // biome-ignore lint/suspicious/noConsole: visible diagnostic for "where are my errors?"
        console.info('[sentry] init skipped — __DEV__ build')
        return
    }
    if (!dsn) {
        // biome-ignore lint/suspicious/noConsole: visible diagnostic for "where are my errors?"
        console.warn(
            '[sentry] init skipped — no DSN. Set EXPO_PUBLIC_SENTRY_DSN at BUILD time (Dokku: docker-options:add build "--build-arg EXPO_PUBLIC_SENTRY_DSN" and reference the ARG before Metro runs).'
        )
        return
    }

    Sentry.init({
        dsn,
        environment: config?.environment ?? 'production',
        release: config?.release,
        // `dist` distinguishes WHICH JS bundle a crash came from. The OTA updater
        // swaps the bundle out from under a fixed native binary/release, so without
        // a per-bundle dist every OTA build collapses onto the same release and its
        // stack frames can't be mapped to the right uploaded sourcemap (events
        // arrive unsymbolicated or get dropped — which is why an OTA crash can look
        // "missing" in Sentry). Tag it with the active bundle id.
        dist: currentBundleDist(),
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
    // biome-ignore lint/suspicious/noConsole: one-line confirmation that capture will actually work
    console.info(
        `[sentry] initialized (env=${config?.environment ?? 'production'}, release=${config?.release ?? 'unknown'})`
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
    // biome-ignore lint/suspicious/noConsole: tracing aid; always visible in browser
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
        // biome-ignore lint/suspicious/noConsole: don't silently swallow when Sentry isn't wired up
        console.error(`[sentry:not-initialized] ${context}`, error, extra)
        return
    }
    Sentry.withScope(scope => {
        scope.setTag('context', context)
        if (extra) scope.setExtras(scrubPII(extra))
        Sentry.captureException(error)
    })
}
