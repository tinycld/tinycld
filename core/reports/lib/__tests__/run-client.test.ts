import { QueryClient } from '@tanstack/react-query'
import { ClientResponseError } from 'pocketbase'
import { describe, expect, it, vi } from 'vitest'
import {
    fetchParamOptions,
    foldKeyOf,
    forgetSavedRuns,
    groupRowAt,
    isTransientRunError,
    lastGroupRowOf,
    PARAM_OPTIONS_PATH,
    RUN_PAGE_SIZE,
    RUN_QUERY_KEY,
    type RunBody,
    refusalOf,
    refusalSortProblem,
    rowsKeyOf,
    runFirstPage,
} from '../run-client'
import type { ReportSpec } from '../spec'
import type { ResultRow, RunResult } from '../types'

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

    // R11: no sort keeps today's key, so no list resets; each sort is its own list.
    it('adds the reader’s sort to the key, and only the sort', () => {
        const desc = [{ col: 'amount', dir: 'desc' as const }]
        expect(rowsKeyOf({ reportId: 'r1', params: {}, options })).toBe('report:r1')
        expect(rowsKeyOf({ reportId: 'r1', params: {}, options: { ...options, sort: [] } })).toBe(
            'report:r1'
        )
        const sorted = rowsKeyOf({
            reportId: 'r1',
            params: {},
            options: { ...options, sort: desc },
        })
        expect(sorted).toBe('report:r1|sort:[{"col":"amount","dir":"desc"}]')
        expect(
            rowsKeyOf({
                reportId: 'r1',
                params: {},
                options: { ...options, sort: [{ col: 'amount', dir: 'asc' }] },
            })
        ).not.toBe(sorted)
        expect(
            rowsKeyOf({
                reportId: 'r1',
                params: {},
                options: { ...options, sort: desc, expand: [['open']], cursor: 'c' },
            })
        ).toBe(sorted)

        const draft = rowsKeyOf({ spec, params: {}, options })
        expect(draft).toBe(`spec:${JSON.stringify(spec)}`)
        expect(rowsKeyOf({ spec, params: {}, options: { ...options, sort: desc } })).toBe(
            `${draft}|sort:[{"col":"amount","dir":"desc"}]`
        )
    })
})

// F11: a fold keeps the reader's place, so a run whose only change is a fold
// knows it is the same run.
describe('foldKeyOf', () => {
    const options = { today: '2026-11-06' }

    it('is the same for bodies that differ only in their fold or page', () => {
        const key = foldKeyOf({ reportId: 'r1', params: {}, options })
        for (const changed of [
            { expand: [['a']] },
            { expandAll: true },
            { cursor: 'c', limit: 5 },
        ]) {
            expect(
                foldKeyOf({ reportId: 'r1', params: {}, options: { ...options, ...changed } })
            ).toBe(key)
        }
    })

    it('changes with the report, its params, its sort and the day', () => {
        const key = foldKeyOf({ reportId: 'r1', params: {}, options })
        expect(foldKeyOf({ reportId: 'r2', params: {}, options })).not.toBe(key)
        expect(foldKeyOf({ reportId: 'r1', params: { day: 'x' }, options })).not.toBe(key)
        const sort = [{ col: 'amount', dir: 'desc' as const }]
        expect(foldKeyOf({ reportId: 'r1', params: {}, options: { ...options, sort } })).not.toBe(
            key
        )
        expect(
            foldKeyOf({ reportId: 'r1', params: {}, options: { today: '2026-11-07' } })
        ).not.toBe(key)
    })
})

describe('runFirstPage', () => {
    const body = { reportId: 'r1', params: {}, options: { today: '2026-11-06' } }
    const row = (id: string, kind: ResultRow['kind'] = 'detail'): ResultRow => ({
        id,
        kind,
        depth: 0,
        label: id,
        cells: {},
    })
    const result = (rows: ResultRow[], next: string): RunResult => ({
        columns: [],
        rows,
        notices: [],
        topics: [],
        page: next ? { next, size: rows.length } : null,
        params: [],
    })
    const sentOptions = () =>
        send.mock.calls.map(([, init]) => {
            const { limit, cursor } = (init as { body: RunBody }).body.options
            return { limit, cursor }
        })

    it('asks for one page of the usual size when there is no target', async () => {
        send.mockReset()
        send.mockResolvedValueOnce(result([row('d:1')], 'c1'))
        await expect(runFirstPage(body, 0)).resolves.toEqual({
            kind: 'ok',
            result: result([row('d:1')], 'c1'),
        })
        expect(sentOptions()).toEqual([{ limit: RUN_PAGE_SIZE, cursor: undefined }])
    })

    // A server that answers fewer rows than asked (here 2 a page) is followed page
    // by page until the target's detail rows are in, and the pages become one.
    it('joins pages until it holds the target detail rows, then follows the last cursor', async () => {
        send.mockReset()
        send.mockResolvedValueOnce(result([row('g:a', 'group'), row('d:1'), row('d:2')], 'c1'))
            .mockResolvedValueOnce(result([row('d:3'), row('d:4'), row('total', 'total')], 'c2'))
            .mockResolvedValueOnce(result([row('d:5')], ''))
        const outcome = await runFirstPage(body, 4)
        expect(outcome.kind === 'ok' && outcome.result.rows.map(r => r.id)).toEqual([
            'g:a',
            'd:1',
            'd:2',
            'd:3',
            'd:4',
            'total',
        ])
        expect(outcome.kind === 'ok' && outcome.result.page).toEqual({ next: 'c2', size: 3 })
        expect(sentOptions()).toEqual([
            { limit: RUN_PAGE_SIZE, cursor: undefined },
            { limit: RUN_PAGE_SIZE, cursor: 'c1' },
        ])
    })

    it('asks for as many rows as it still needs, up to the server’s 1,000', async () => {
        send.mockReset()
        send.mockResolvedValueOnce(result([row('d:1')], 'c1'))
        send.mockResolvedValueOnce(result([row('d:2')], ''))
        await runFirstPage(body, 1500)
        expect(sentOptions()).toEqual([
            { limit: 1000, cursor: undefined },
            { limit: 1000, cursor: 'c1' },
        ])
    })

    it('stops at the end of the run, and returns a refusal as it comes', async () => {
        send.mockReset()
        send.mockResolvedValueOnce(result([row('d:1')], ''))
        const outcome = await runFirstPage(body, 400)
        expect(outcome.kind === 'ok' && outcome.result.rows.map(r => r.id)).toEqual(['d:1'])
        expect(send).toHaveBeenCalledTimes(1)

        send.mockReset()
        send.mockResolvedValueOnce(result([row('d:1')], 'c1'))
        send.mockRejectedValueOnce(refused(408))
        await expect(runFirstPage(body, 400)).resolves.toEqual({ kind: 'timeout' })
    })

    // Expand all from closed groups: no detail rows were in view, so the reader's
    // place is the last group row; the pages join until they hold it.
    it('joins pages until it holds the row it must keep, even with no detail target', async () => {
        send.mockReset()
        send.mockResolvedValueOnce(result([row('g:a', 'group'), row('d:1'), row('d:2')], 'c1'))
            .mockResolvedValueOnce(result([row('d:3'), row('g:b', 'group'), row('d:4')], 'c2'))
            .mockResolvedValueOnce(result([row('d:5')], ''))
        const outcome = await runFirstPage(body, 0, undefined, 'g:b')
        expect(outcome.kind === 'ok' && outcome.result.rows.map(r => r.id)).toEqual([
            'g:a',
            'd:1',
            'd:2',
            'd:3',
            'g:b',
            'd:4',
        ])
        expect(outcome.kind === 'ok' && outcome.result.page).toEqual({ next: 'c2', size: 3 })
        expect(send).toHaveBeenCalledTimes(2)
    })

    it('keeps joining past the row it must keep until the detail target is in', async () => {
        send.mockReset()
        send.mockResolvedValueOnce(result([row('g:a', 'group'), row('d:1')], 'c1'))
            .mockResolvedValueOnce(result([row('d:2')], 'c2'))
            .mockResolvedValueOnce(result([row('d:3')], ''))
        const outcome = await runFirstPage(body, 2, undefined, 'g:a')
        expect(outcome.kind === 'ok' && outcome.result.rows.map(r => r.id)).toEqual([
            'g:a',
            'd:1',
            'd:2',
        ])
        expect(send).toHaveBeenCalledTimes(2)
    })

    // The first page is the usual one, as a fold at the top of the report needs no
    // more. Once a page has come and only the row is missing, each request asks for
    // the server's most, so a fold far down the report takes few requests.
    it('asks for the server’s 1,000 rows a request while it looks for the row it must keep', async () => {
        send.mockReset()
        send.mockResolvedValueOnce(result([row('g:a', 'group'), row('d:1'), row('d:2')], 'c1'))
            .mockResolvedValueOnce(result([row('d:3'), row('g:b', 'group')], 'c2'))
            .mockResolvedValueOnce(result([row('d:4')], ''))
        await runFirstPage(body, 0, undefined, 'g:b')
        expect(sentOptions()).toEqual([
            { limit: RUN_PAGE_SIZE, cursor: undefined },
            { limit: 1000, cursor: 'c1' },
        ])

        send.mockReset()
        send.mockResolvedValueOnce(result([row('g:a', 'group'), row('d:1'), row('d:2')], 'c1'))
            .mockResolvedValueOnce(result([row('d:3'), row('g:b', 'group')], 'c2'))
            .mockResolvedValueOnce(result([row('d:4')], ''))
        await runFirstPage(body, 2, undefined, 'g:b')
        expect(sentOptions()).toEqual([
            { limit: RUN_PAGE_SIZE, cursor: undefined },
            { limit: 1000, cursor: 'c1' },
        ])
    })

    it('stops at the end of the run when the row it must keep never comes', async () => {
        send.mockReset()
        send.mockResolvedValueOnce(
            result([row('g:a', 'group'), row('d:1')], 'c1')
        ).mockResolvedValueOnce(result([row('d:2')], ''))
        const outcome = await runFirstPage(body, 0, undefined, 'g:gone')
        expect(outcome.kind === 'ok' && outcome.result.rows.map(r => r.id)).toEqual([
            'g:a',
            'd:1',
            'd:2',
        ])
        expect(send).toHaveBeenCalledTimes(2)
    })
})

describe('lastGroupRowOf', () => {
    const rowOf = (id: string, kind: ResultRow['kind']): ResultRow => ({
        id,
        kind,
        depth: 0,
        label: id,
        cells: {},
    })
    const resultOf = (rows: ResultRow[]): RunResult => ({
        columns: [],
        rows,
        notices: [],
        topics: [],
        page: null,
        params: [],
    })

    it('reads the last group row across the pages, in document order', () => {
        expect(
            lastGroupRowOf([
                resultOf([rowOf('g:a', 'group'), rowOf('d:1', 'detail')]),
                resultOf([rowOf('g:b', 'group'), rowOf('s:b', 'subtotal')]),
                resultOf([rowOf('d:2', 'detail'), rowOf('total', 'total')]),
            ])
        ).toBe('g:b')
    })

    it('is undefined when no page has a group row', () => {
        expect(lastGroupRowOf([resultOf([rowOf('d:1', 'detail')])])).toBeUndefined()
        expect(lastGroupRowOf([])).toBeUndefined()
    })
})

describe('groupRowAt', () => {
    const rowOf = (id: string, kind: ResultRow['kind']): ResultRow => ({
        id,
        kind,
        depth: 0,
        label: id,
        cells: {},
    })
    const resultOf = (rows: ResultRow[]): RunResult => ({
        columns: [],
        rows,
        notices: [],
        topics: [],
        page: null,
        params: [],
    })
    const pages = [
        resultOf([rowOf('g:a', 'group'), rowOf('d:1', 'detail')]),
        resultOf([rowOf('s:a', 'subtotal'), rowOf('g:b', 'group'), rowOf('d:2', 'detail')]),
        resultOf([rowOf('g:c', 'group'), rowOf('total', 'total')]),
    ]

    it('is the row itself when it is a group row', () => {
        expect(groupRowAt(pages, 'g:b')).toBe('g:b')
    })

    it('is the nearest group row above any other row, across the pages', () => {
        expect(groupRowAt(pages, 'd:1')).toBe('g:a')
        expect(groupRowAt(pages, 'd:2')).toBe('g:b')
        expect(groupRowAt(pages, 'total')).toBe('g:c')
    })

    it('is undefined for a row the pages do not hold, or one before any group', () => {
        expect(groupRowAt(pages, 'g:gone')).toBeUndefined()
        expect(groupRowAt(pages, undefined)).toBeUndefined()
        expect(groupRowAt([resultOf([rowOf('d:1', 'detail')])], 'd:1')).toBeUndefined()
    })
})

describe('refusalSortProblem', () => {
    // R8: a refusal of the reader's sort is offered "Clear sort".
    it('is the message of the first problem under options.sort', () => {
        expect(
            refusalSortProblem([
                { path: 'columns.0.id', code: 'invalid', message: 'not this one' },
                {
                    path: 'options.sort.0.col',
                    code: 'unknown_column',
                    message: 'the report has no column "nosuch"',
                },
            ])
        ).toBe('the report has no column "nosuch"')
        expect(
            refusalSortProblem([
                {
                    path: 'options.sort',
                    code: 'invalid',
                    message: 'a pivot report has no opened rows to sort',
                },
            ])
        ).toBe('a pivot report has no opened rows to sort')
    })

    it('is null for any other refusal', () => {
        expect(refusalSortProblem([])).toBeNull()
        expect(
            refusalSortProblem([
                { path: 'options.sorted', code: 'invalid', message: 'x' },
                { path: 'source', code: 'unknown_source', message: 'y' },
            ])
        ).toBeNull()
    })
})
