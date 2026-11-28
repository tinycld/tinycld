import { notify } from '@tinycld/core/lib/notify/dispatcher'
import type { FieldValues, Path, UseFormSetError } from 'react-hook-form'
import { log } from './logger'

export const isRecord = (value: unknown): value is Record<string, unknown> =>
    typeof value === 'object' && value !== null

function getBaseMessage(error: Record<string, unknown>): string {
    let msg = ''
    if (typeof error.message === 'string') msg = error.message

    // A failed atomic write carries the failed operation's message itself;
    // its cause is the whole-batch error, which only says the batch failed.
    if (error.name === 'BatchMutationError') return msg

    const nested = error.originalError ?? error.cause
    if (isRecord(nested) && typeof nested.message === 'string' && nested.message) {
        msg = nested.message
    }
    return msg
}

function formatResponseFieldErrors(data: Record<string, unknown>): string[] {
    const errors: string[] = []
    for (const [field, fieldError] of Object.entries(data)) {
        if (field === 'message' || field === 'status') continue
        if (!isRecord(fieldError)) continue

        if (typeof fieldError.message === 'string') {
            errors.push(`${field}: ${fieldError.message}`)
        } else if (typeof fieldError.code === 'string') {
            errors.push(`${field}: ${fieldError.code.replace(/_/g, ' ')}`)
        }
    }
    return errors
}

export function errorToString(error: unknown, fallback = 'An unexpected error occurred'): string {
    if (typeof error === 'string') return error
    if (!isRecord(error)) return fallback

    const msg = getBaseMessage(error)
    const response = error.response

    if (!isRecord(response)) return msg || fallback

    if (isRecord(response.data)) {
        const fieldErrors = formatResponseFieldErrors(response.data)
        const prefix = msg ? `${msg} ` : ''

        if (fieldErrors.length > 0) return prefix + fieldErrors.join(', ')
        if (typeof response.data.message === 'string') return prefix + response.data.message
        if (typeof response.data.error === 'string') return prefix + response.data.error
    }

    if (typeof error.error === 'string') return error.error
    if (isRecord(error.data) && typeof error.data.message === 'string') return error.data.message

    return msg || fallback
}

function extractFieldErrors(data: Record<string, unknown>): Record<string, string> | null {
    const errors: Record<string, string> = {}

    for (const [field, fieldError] of Object.entries(data)) {
        if (isRecord(fieldError)) {
            if (typeof fieldError.message === 'string') {
                errors[field] = fieldError.message
            } else if (typeof fieldError.code === 'string') {
                errors[field] = `Validation failed: ${fieldError.code}`
            }
        } else if (typeof fieldError === 'string') {
            errors[field] = fieldError
        }
    }

    return Object.keys(errors).length > 0 ? errors : null
}

function getBatchRequestData(data: Record<string, unknown>): Record<string, unknown> | null {
    if (!isRecord(data.requests)) return null
    const firstRequest = data.requests['0']
    if (!isRecord(firstRequest)) return null
    if (!isRecord(firstRequest.response) || !isRecord(firstRequest.response.data)) return null
    return firstRequest.response.data
}

export function extractValidationErrors(error: unknown): Record<string, string> | null {
    if (!isRecord(error)) return null

    if (isRecord(error.response) && isRecord(error.response.data)) {
        const data = error.response.data

        const batchData = getBatchRequestData(data)
        if (batchData) {
            const result = extractFieldErrors(batchData)
            if (result) return result
        }

        if (isRecord(data.data)) {
            const result = extractFieldErrors(data.data)
            if (result) return result
        }

        // The SDK's ClientResponseError.response IS the parsed PocketBase
        // error body, so `error.response.data` is already the field-error map
        // for a direct record create/update — the shape every pbtsdb mutation
        // rejects with. Without this branch those errors mapped to nothing and
        // surfaced as a generic toast instead of form field errors.
        const result = extractFieldErrors(data)
        if (result) return result
    }

    if (isRecord(error.errors)) {
        return Object.fromEntries(
            Object.entries(error.errors).filter(([, value]) => typeof value === 'string') as [
                string,
                string,
            ][]
        )
    }

    return null
}

export interface FormErrorHandler<T extends FieldValues = FieldValues> {
    setError: UseFormSetError<T>
    getValues: () => T
    /** Short label describing what the form was doing, surfaced in telemetry. */
    operation?: string
    /**
     * The collection whose fields the form edits. An `atomic()` write needs
     * it: a field error of a failed row goes on the form only when the row is
     * of this collection, and to a toast otherwise.
     */
    collection?: string
    /**
     * For a form that draws refusals in its own `FormErrorSummary`, in place
     * of a toast that leaves the form: the field errors the form knows go
     * inline, and `root` (type `server`) is set only for what no field shows —
     * the server's sentence for a refusal with no field errors, else only the
     * messages of the fields the form does not know (joined by a space, with
     * no field name and no server sentence), or `SERVER_UNREACHED_MESSAGE` for
     * a write that got no answer. A refusal whose every field error is placed
     * sets no root.
     */
    rootMessage?: boolean
}

/** The root text of a write that got no answer from the server (status 0). */
export const SERVER_UNREACHED_MESSAGE = "Your change wasn't saved: the server could not be reached."

/**
 * A write that got no answer: the SDK's status 0 (fetch failed, no response),
 * or `serverFetch`'s ServerUnreachableError. An abort is the caller's own
 * cancel, not a refusal to report as unreachable.
 */
function isUnreached(error: unknown): boolean {
    if (!isRecord(error)) return false
    if (error.name === 'ServerUnreachableError') return true
    return error.status === 0 && error.isAbort !== true && 'response' in error
}

/** Places the errors of the fields the form knows; returns the messages of the rest. */
function placeKnownFieldErrors<T extends FieldValues>(
    form: FormErrorHandler<T>,
    validationErrors: Record<string, string>
): string[] {
    const formFields = Object.keys(form.getValues() as Record<string, unknown>)
    const unplaced: string[] = []
    for (const [field, message] of Object.entries(validationErrors)) {
        if (formFields.includes(field)) {
            form.setError(field as Path<T>, { type: 'manual', message })
        } else {
            unplaced.push(message)
        }
    }
    return unplaced
}

/** The server's own sentence when it sent one, else the full error text. */
function rootMessageOf(error: unknown): string {
    const base = isRecord(error) ? getBaseMessage(error) : ''
    return base || errorToString(error)
}

function setRootRefusal<T extends FieldValues>(
    form: FormErrorHandler<T>,
    error: unknown,
    validationErrors: Record<string, string> | null
) {
    if (isUnreached(error)) {
        form.setError('root', { type: 'server', message: SERVER_UNREACHED_MESSAGE })
        return
    }
    if (!validationErrors) {
        form.setError('root', { type: 'server', message: rootMessageOf(error) })
        return
    }
    const unplaced = placeKnownFieldErrors(form, validationErrors)
    if (unplaced.length === 0) return
    // The server's top sentence ("Failed to update record.") is generic; the
    // field messages are the refusal itself.
    form.setError('root', { type: 'server', message: unplaced.join(' ') })
}

// An atomic write saves rows of several collections, and two of them can have
// a field of the same name, so the field name alone does not say that an
// error belongs to the form.
function belongsToForm(error: unknown, collection: string | undefined): boolean {
    if (!isRecord(error) || error.name !== 'BatchMutationError') return true
    return collection !== undefined && error.collection === collection
}

export function handleMutationErrorsWithForm<T extends FieldValues = FieldValues>(
    form: FormErrorHandler<T>
) {
    return (error: unknown) => {
        const validationErrors = belongsToForm(error, form.collection)
            ? extractValidationErrors(error)
            : null

        if (form.rootMessage) {
            setRootRefusal(form, error, validationErrors)
            return
        }

        if (validationErrors) {
            const formValues = form.getValues()
            const formFields = Object.keys(formValues as Record<string, unknown>)
            const errorFields = Object.keys(validationErrors)
            const unknownFields = errorFields.filter(field => !formFields.includes(field))

            if (unknownFields.length === 0) {
                for (const [field, message] of Object.entries(validationErrors)) {
                    form.setError(field as Path<T>, {
                        type: 'manual',
                        message,
                    })
                }
                return
            }
        }

        const message = errorToString(error)
        notify.emit({
            event: 'mutation.error',
            title: 'Something went wrong',
            body: message,
            data: { operation: form.operation ?? 'unknown', error: message },
        })
    }
}

/**
 * Retained for the many existing call sites; `log.error` is the same thing and
 * is what new code should use.
 */
export function captureException(context: string, error: unknown, extra?: Record<string, unknown>) {
    log.error(context, error, extra)
}
