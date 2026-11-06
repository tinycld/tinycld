import { QueryClient } from '@tanstack/react-query'
import { ClientResponseError } from 'pocketbase'
import { describe, expect, it, vi } from 'vitest'
import {
    fetchParamOptions,
    forgetSavedRuns,
    isTransientRunError,
    PARAM_OPTIONS_PATH,
    RUN_QUERY_KEY,
    refusalOf,
    rowsKeyOf,
} from '../run-client'
import type { ReportSpec } from '../spec'

const send = vi.fn()
vi.mock('@tinycld/core/lib/pocketbase', () => ({
    pb: { send: (...args: unknown[]) => send(...args) },
}))

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

describe('fetchParamOptions', () => {
    it('posts the request to the param-options route and returns the page', async () => {
        const page = {
            options: [{ value: 'a', label: 'Bank', group: 'cash' }],
            groups: [{ value: 'cash', label: 'Cash' }],
            groupsCut: true,
            next: 'c1',
        }
        send.mockResolvedValueOnce(page)
        const signal = new AbortController().signal
        const body = { reportId: 'r1', param: 'accounts', search: 'ba', today: '2026-11-01' }
        await expect(fetchParamOptions(body, signal)).resolves.toEqual(page)
        expect(send).toHaveBeenCalledWith(PARAM_OPTIONS_PATH, {
            method: 'POST',
            body,
            signal,
            requestKey: null,
        })
        expect(PARAM_OPTIONS_PATH).toBe('/api/reports/param-options')
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

describe('rowsKeyOf', () => {
    const spec: ReportSpec = { version: 1, source: 'a.b', columns: [] }
    const options = { today: '2026-11-06' }

    it('keys a saved report on its id, whatever its params or options', () => {
        const key = rowsKeyOf({ reportId: 'r1', params: {}, options })
        expect(
            rowsKeyOf({
                reportId: 'r1',
                params: { day: 'x' },
                options: { ...options, expandAll: true },
            })
        ).toBe(key)
        expect(rowsKeyOf({ reportId: 'r2', params: {}, options })).not.toBe(key)
    })

    it('changes with a draft spec’s rows but not with its display or options', () => {
        const key = rowsKeyOf({ spec, params: {}, options })
        const display = { layouts: [{ width: 'mobile' as const, tracks: 1, cells: [] }] }
        expect(rowsKeyOf({ spec: { ...spec, display }, params: {}, options })).toBe(key)
        expect(
            rowsKeyOf({ spec, params: {}, options: { ...options, cursor: 'c', limit: 200 } })
        ).toBe(key)
        const sorted = { ...spec, sort: [{ col: 'name', dir: 'desc' as const }] }
        expect(rowsKeyOf({ spec: sorted, params: {}, options })).not.toBe(key)
    })
})
