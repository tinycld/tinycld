import { QueryClient } from '@tanstack/react-query'
import { ClientResponseError } from 'pocketbase'
import { describe, expect, it } from 'vitest'
import { forgetSavedRuns, isTransientRunError, RUN_QUERY_KEY, refusalOf } from '../run-client'

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

describe('forgetSavedRuns', () => {
    it('drops the cached runs of one saved report and keeps the others', () => {
        const client = new QueryClient()
        const keyOf = (body: unknown) => [...RUN_QUERY_KEY, body]
        const mine = keyOf({ reportId: 'r1', params: {}, options: {} })
        const mineFiltered = keyOf({ reportId: 'r1', params: { day: 'x' }, options: {} })
        const other = keyOf({ reportId: 'r2', params: {}, options: {} })
        const draft = keyOf({ spec: { source: 'a.b' }, params: {}, options: {} })
        for (const key of [mine, mineFiltered, other, draft]) client.setQueryData(key, 'cached')
        forgetSavedRuns(client, 'r1')
        expect(client.getQueryData(mine)).toBeUndefined()
        expect(client.getQueryData(mineFiltered)).toBeUndefined()
        expect(client.getQueryData(other)).toBe('cached')
        expect(client.getQueryData(draft)).toBe('cached')
    })
})
