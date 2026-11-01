import { ClientResponseError } from 'pocketbase'
import { describe, expect, it } from 'vitest'
import { isTransientRunError, refusalOf } from '../run-client'

const refused = (status: number, body: Record<string, unknown> = {}) =>
    new ClientResponseError({ status, response: { status, ...body } })

describe('refusalOf', () => {
    it('reads a 422 as missing parts with each problem by spec path', () => {
        const error = refused(422, {
            message: 'The report names something that does not exist.',
            data: {
                source: { code: 'unknown_source', message: 'the source "zoo.gone" does not exist' },
                'columns.1.path.0.link': { code: 'unknown_link', message: 'no link "keeper"' },
            },
        })
        expect(refusalOf(error)).toEqual({
            kind: 'missing',
            message: 'The report names something that does not exist.',
            problems: [
                {
                    path: 'columns.1.path.0.link',
                    code: 'unknown_link',
                    message: 'no link "keeper"',
                },
                {
                    path: 'source',
                    code: 'unknown_source',
                    message: 'the source "zoo.gone" does not exist',
                },
            ],
        })
    })

    it('reads 400, 403 and 408', () => {
        expect(refusalOf(refused(400, { message: 'The report is not valid.', data: {} }))).toEqual({
            kind: 'invalid',
            message: 'The report is not valid.',
            problems: [],
        })
        expect(
            refusalOf(
                refused(403, {
                    data: {
                        reportId: { code: 'no_access', message: 'You cannot view this report.' },
                    },
                })
            )
        ).toEqual({ kind: 'no-access' })
        expect(refusalOf(refused(408))).toEqual({ kind: 'timeout' })
    })

    it('leaves every other failure to be thrown', () => {
        expect(refusalOf(refused(500))).toBeNull()
        expect(refusalOf(refused(0))).toBeNull()
        expect(refusalOf(new Error('boom'))).toBeNull()
    })
})

describe('isTransientRunError', () => {
    it('retries an unreachable or failing server only', () => {
        expect(isTransientRunError(refused(0))).toBe(true)
        expect(isTransientRunError(refused(503))).toBe(true)
        expect(isTransientRunError(refused(500))).toBe(true)
        expect(isTransientRunError(refused(404))).toBe(false)
        expect(isTransientRunError(new Error('boom'))).toBe(false)
    })
})
