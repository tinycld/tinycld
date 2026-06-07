declare const __DEV__: boolean

import * as Sentry from '@sentry/react-native'
import { getCoreConfigOptional } from './core-config'
import { scrubPII } from './sentry-scrub'

let initialized = false

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
        tracesSampleRate: 0.1,
        replaysSessionSampleRate: 0,
        replaysOnErrorSampleRate: 0,
        beforeSend(event) {
            return scrubPII(event) as typeof event
        },
        beforeBreadcrumb(breadcrumb) {
            return scrubPII(breadcrumb) as typeof breadcrumb
        },
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
