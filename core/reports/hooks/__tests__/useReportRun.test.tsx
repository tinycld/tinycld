// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it } from 'vitest'
import { fakePbServer } from '../../../tests/fake-pb-server'
import type { RunBody } from '../../lib/run-client'
import type { ResultRow, RunResult } from '../../lib/types'
import { useReportRun } from '../useReportRun'

// In react-query a refetch and a next page cancel each other. A live rerun that
// lands while the list loads its next page must not drop that page: the list asks
// again only when its rows change, so a first page shorter than the view stays one
// page, with the report's Total under it (CI run 42367362188). And a next page asked
// for during a rerun must not drop the rerun's figures.

const TOPIC = 'salesstub/changes'

type Answer = (body: RunBody) => Promise<RunResult>
let answer: Answer = () => Promise.reject(new Error('no answer set'))
const server = fakePbServer({ '/api/reports/run': sent => answer(sent as RunBody) })

afterEach(cleanup)

const row = (id: string, kind: ResultRow['kind'] = 'detail'): ResultRow => ({
    id,
    kind,
    depth: 0,
    label: id,
    cells: {},
})

const result = (rows: ResultRow[], next = ''): RunResult => ({
    columns: [],
    rows,
    notices: [],
    topics: [TOPIC],
    page: next ? { next, size: rows.length } : null,
    params: [],
})

/** A server answer held until the test lets it go; an aborted request drops it. */
function held<T>(value: T) {
    let release = () => {}
    const promise = new Promise<T>(resolve => {
        release = () => resolve(value)
    })
    return { promise, release }
}

const body: RunBody = { reportId: 'r1', params: {}, options: { today: '2026-11-07' } }

function renderRun(queryClient = new QueryClient()) {
    const wrapper = ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
    return renderHook(() => useReportRun({ body, userId: 'u1' }), { wrapper })
}

/** The live rerun subscribes once the run lands; a signal before that is lost. */
const subscribed = () => waitFor(() => expect(server.topics()).toContain(TOPIC))

const signal = () => act(() => server.emit(TOPIC))

const rowIds = (run: { result: RunResult | undefined }) => run.result?.rows.map(r => r.id)

describe('useReportRun', () => {
    it('keeps a next page that a live rerun lands on', async () => {
        const first = result([row('g:A', 'group'), row('d:1')], 'c1')
        const second = result([row('d:2'), row('total', 'total')])
        let firstNextPage: ReturnType<typeof held<RunResult>> | undefined
        answer = sent => {
            if (!sent.options.cursor) return Promise.resolve(first)
            if (firstNextPage) return Promise.resolve(second)
            firstNextPage = held(second)
            return firstNextPage.promise
        }
        const { result: run } = renderRun()
        await waitFor(() => expect(run.current.hasMore).toBe(true))
        await subscribed()

        act(() => run.current.loadMore())
        await waitFor(() => expect(firstNextPage).toBeDefined())
        signal()
        // The rerun waits RERUN_DELAY_MS; the next page lands after it would start.
        await new Promise(resolve => setTimeout(resolve, 400))
        act(() => firstNextPage?.release())

        await waitFor(() => expect(rowIds(run.current)).toEqual(['g:A', 'd:1', 'd:2', 'total']))
        expect(run.current.hasMore).toBe(false)
    })

    it('keeps the figures of a rerun that a next page lands on', async () => {
        const first = result([row('g:A', 'group'), row('d:1')], 'c1')
        const changed = result([row('g:A', 'group'), row('d:1b')], 'c1')
        const second = result([row('d:2'), row('total', 'total')])
        let rerun: ReturnType<typeof held<RunResult>> | undefined
        let firstPages = 0
        answer = sent => {
            if (sent.options.cursor) return Promise.resolve(second)
            firstPages += 1
            if (firstPages === 1) return Promise.resolve(first)
            rerun = held(changed)
            return rerun.promise
        }
        const { result: run } = renderRun()
        await waitFor(() => expect(run.current.hasMore).toBe(true))
        await subscribed()

        signal()
        await waitFor(() => expect(rerun).toBeDefined())
        act(() => run.current.loadMore())
        act(() => rerun?.release())

        await waitFor(() => expect(rowIds(run.current)).toEqual(['g:A', 'd:1b', 'd:2', 'total']))
    })

    // A cached run refetches when it mounts again. A next page and a live rerun that
    // both wait on that one refetch must not cancel each other when it lands.
    it('keeps both a next page and a rerun that wait on the same refetch', async () => {
        const first = result([row('g:A', 'group'), row('d:1')], 'c1')
        const changed = result([row('g:A', 'group'), row('d:1b')], 'c1')
        const second = result([row('d:2'), row('total', 'total')])
        let firstPages = 0
        let mountRefetch: ReturnType<typeof held<RunResult>> | undefined
        answer = sent => {
            if (sent.options.cursor) return Promise.resolve(second)
            firstPages += 1
            if (firstPages === 1) return Promise.resolve(first)
            if (firstPages > 2) return Promise.resolve(changed)
            mountRefetch = held(first)
            return mountRefetch.promise
        }
        const queryClient = new QueryClient()
        const opened = renderRun(queryClient)
        await waitFor(() => expect(opened.result.current.hasMore).toBe(true))
        opened.unmount()

        const { result: run } = renderRun(queryClient)
        await waitFor(() => expect(mountRefetch).toBeDefined())
        await subscribed()
        act(() => run.current.loadMore())
        signal()
        // The rerun waits RERUN_DELAY_MS; the refetch lands after it joined.
        await new Promise(resolve => setTimeout(resolve, 400))
        act(() => mountRefetch?.release())

        await waitFor(() => expect(rowIds(run.current)).toEqual(['g:A', 'd:1b', 'd:2', 'total']))
        expect(run.current.hasMore).toBe(false)
    })

    it('shows more loading while a next page waits, and asks for it once', async () => {
        const first = result([row('g:A', 'group'), row('d:1')], 'c1')
        const second = result([row('d:2'), row('total', 'total')])
        let firstPages = 0
        let nextPages = 0
        let mountRefetch: ReturnType<typeof held<RunResult>> | undefined
        answer = sent => {
            if (sent.options.cursor) {
                nextPages += 1
                return Promise.resolve(second)
            }
            firstPages += 1
            if (firstPages !== 2) return Promise.resolve(first)
            mountRefetch = held(first)
            return mountRefetch.promise
        }
        const queryClient = new QueryClient()
        const opened = renderRun(queryClient)
        await waitFor(() => expect(opened.result.current.hasMore).toBe(true))
        opened.unmount()

        const { result: run } = renderRun(queryClient)
        await waitFor(() => expect(mountRefetch).toBeDefined())
        act(() => run.current.loadMore())
        expect(run.current.isLoadingMore).toBe(true)
        act(() => run.current.loadMore())
        act(() => mountRefetch?.release())

        await waitFor(() => expect(rowIds(run.current)).toEqual(['g:A', 'd:1', 'd:2', 'total']))
        expect(run.current.isLoadingMore).toBe(false)
        expect(nextPages).toBe(1)
    })
})
