import { BatchMutationError } from 'pbtsdb'
import { ClientResponseError } from 'pocketbase'
import { describe, expect, it } from 'vitest'
import { errorToString, extractValidationErrors } from '../../lib/errors'

// The error a pbtsdb insert/update rejects with is the SDK's raw
// ClientResponseError, whose `.response` is the parsed PocketBase error BODY:
// { code, message, data: { <field>: { code, message } } }. That means
// `error.response.data` IS the field-error map. extractValidationErrors used
// to look only one level deeper (`data.data`), so every direct collection
// validation error — a duplicate mailbox address, a required field — mapped to
// nothing and surfaced as a generic "Something went wrong" toast instead of a
// form field error. Found live by mail's duplicate-mailbox e2e.

function pbError(body: Record<string, unknown>): ClientResponseError {
    return new ClientResponseError({ status: 400, response: body })
}

describe('extractValidationErrors', () => {
    it('maps a direct PocketBase record validation error to field messages', () => {
        const err = pbError({
            code: 400,
            message: 'Failed to create record.',
            data: {
                address: { code: 'validation_not_unique', message: 'Value must be unique.' },
            },
        })
        expect(extractValidationErrors(err)).toEqual({ address: 'Value must be unique.' })
    })

    it('maps multiple field errors and falls back to the code when message is missing', () => {
        const err = pbError({
            code: 400,
            message: 'Failed to create record.',
            data: {
                address: { code: 'validation_required' },
                display_name: { code: 'x', message: 'Too long.' },
            },
        })
        expect(extractValidationErrors(err)).toEqual({
            address: 'Validation failed: validation_required',
            display_name: 'Too long.',
        })
    })

    it('still handles the doubly-nested shape some endpoints return', () => {
        const err = pbError({
            code: 400,
            message: 'Failed.',
            data: { data: { email: { code: 'x', message: 'Invalid email.' } } },
        })
        expect(extractValidationErrors(err)).toEqual({ email: 'Invalid email.' })
    })

    it('returns null for a non-validation error body', () => {
        const err = pbError({ code: 404, message: 'Not found.', data: {} })
        expect(extractValidationErrors(err)).toBeNull()
    })

    it('returns null for a plain Error', () => {
        expect(extractValidationErrors(new Error('boom'))).toBeNull()
    })
})

describe('a failed atomic write', () => {
    // What pbtsdb throws: the failed operation's PocketBase error body in
    // `response`, and the whole-batch ClientResponseError as `cause`.
    const batchError = (response: { message: string; data: Record<string, unknown> }) =>
        new BatchMutationError({
            index: 1,
            collection: 'widgets',
            recordId: 'rec123',
            response: { status: 400, ...response },
            cause: new Error('Batch transaction failed.'),
        })

    it('maps the field errors of the failed operation', () => {
        const error = batchError({
            message: 'Failed to create record.',
            data: { title: { code: 'validation_required', message: 'Cannot be blank.' } },
        })
        expect(extractValidationErrors(error)).toEqual({ title: 'Cannot be blank.' })
    })

    it('reports the message of the failed operation, not of the batch wrapper', () => {
        const error = batchError({ message: 'The period is closed through 2026-06-30.', data: {} })
        expect(errorToString(error)).toBe('The period is closed through 2026-06-30.')
    })
})
