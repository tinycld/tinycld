declare const __DEV__: boolean

import { getCoreConfigOptional, type LogLevel } from './core-config'
import { addBreadcrumbToSentry, captureExceptionToSentry, captureMessageToSentry } from './sentry'

const SEVERITY: Record<LogLevel, number> = {
    debug: 10,
    info: 20,
    warn: 30,
    error: 40,
}

/**
 * The level at or above which a log call also becomes a Sentry event. Release
 * builds default to 'warn' so a production warning surfaces before it escalates
 * into an exception; __DEV__ defaults to 'debug' (Sentry is inert there anyway,
 * since initSentry early-returns on __DEV__).
 */
export function resolveLogLevel(): LogLevel {
    const configured = getCoreConfigOptional()?.logLevel
    if (configured) return configured
    return __DEV__ ? 'debug' : 'warn'
}

function consoleLine(
    level: LogLevel,
    context: string,
    message: string,
    extra?: Record<string, unknown>
): void {
    if (!__DEV__) return
    const method = level === 'debug' ? 'debug' : level === 'error' ? 'error' : level
    console[method](`[${context}] ${message}`, extra ?? '')
}

function emit(
    level: LogLevel,
    context: string,
    message: string,
    extra?: Record<string, unknown>
): void {
    consoleLine(level, context, message, extra)
    addBreadcrumbToSentry(context, level, message, extra)
    if (SEVERITY[level] < SEVERITY[resolveLogLevel()]) return
    captureMessageToSentry(context, message, extra)
}

/**
 * The one blessed logging API for client code.
 *
 *     log.debug('mail.compose', 'draft saved', { draftId })
 *     log.error('mail.send', err, { messageId })
 *
 * `context` is a short stable dotted string Sentry groups on; `extra` is the
 * variable detail. Every call becomes a breadcrumb; calls at or above the
 * configured level additionally become Sentry events.
 */
export const log = {
    debug(context: string, message: string, extra?: Record<string, unknown>): void {
        emit('debug', context, message, extra)
    },
    info(context: string, message: string, extra?: Record<string, unknown>): void {
        emit('info', context, message, extra)
    },
    warn(context: string, message: string, extra?: Record<string, unknown>): void {
        emit('warn', context, message, extra)
    },
    /**
     * Errors always route through captureException so Sentry gets the real
     * stack, never a stringified message.
     */
    error(context: string, error: unknown, extra?: Record<string, unknown>): void {
        const message = error instanceof Error ? error.message : String(error)
        consoleLine('error', context, message, extra)
        addBreadcrumbToSentry(context, 'error', message, extra)
        captureExceptionToSentry(context, error, extra)
    },
}
