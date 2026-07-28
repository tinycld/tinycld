import { ClientResponseError } from 'pocketbase'
import { describe, expect, it } from 'vitest'
import { extractValidationErrors } from '../../lib/errors'

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
