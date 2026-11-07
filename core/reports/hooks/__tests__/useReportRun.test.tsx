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

// F11 and F7: a fold keeps the reader's place, and loads no further than that place.
// With every group closed the first page holds every group row of the report; the
// place is the group row in view (the list reports its top row), not the last one.
describe('useReportRun after a fold', () => {
    const collapsed: RunBody = {
        reportId: 'r1',
        params: {},
        options: { today: '2026-11-07', expand: [] },
    }
    const expanded: RunBody = {
        reportId: 'r1',
        params: {},
        options: { today: '2026-11-07', expandAll: true },
    }
    // Three groups of four detail rows; the server answers 2 detail rows a page.
    const closedPage = result([
        row('g:a', 'group'),
        row('g:b', 'group'),
        row('g:c', 'group'),
        row('total', 'total'),
    ])
    const openPages = [
        result([row('g:a', 'group'), row('d:a1'), row('d:a2')], 'c1'),
        result([row('d:a3'), row('d:a4')], 'c2'),
        result([row('s:a', 'subtotal'), row('g:b', 'group'), row('d:b1'), row('d:b2')], 'c3'),
        result([row('d:b3'), row('d:b4')], 'c4'),
        result([row('s:b', 'subtotal'), row('g:c', 'group'), row('d:c1'), row('d:c2')], 'c5'),
        result([row('d:c3'), row('d:c4'), row('total', 'total')]),
    ]
    let sent: RunBody[] = []

    // The closed pages the server answers in turn (the last one again after them).
    function serve(closedPages: RunResult[] = [closedPage]) {
        sent = []
        let closedRuns = 0
        answer = request => {
            sent.push(request)
            if (!request.options.expandAll) {
                const at = Math.min(closedRuns, closedPages.length - 1)
                closedRuns += 1
                return Promise.resolve(closedPages[at])
            }
            const at = request.options.cursor ? Number(request.options.cursor.slice(1)) : 0
            return Promise.resolve(openPages[at])
        }
    }

    function renderFold() {
        const queryClient = new QueryClient()
        const wrapper = ({ children }: { children: ReactNode }) => (
            <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
        )
        return renderHook(
            ({ runBody }: { runBody: RunBody }) => useReportRun({ body: runBody, userId: 'u1' }),
            { wrapper, initialProps: { runBody: collapsed } }
        )
    }

    async function expandAll(fold: ReturnType<typeof renderFold>) {
        sent = []
        fold.rerender({ runBody: expanded })
        await waitFor(() => expect(fold.result.current.result?.rows[1]?.id).toBe('d:a1'))
        return fold.result.current
    }

    // The list reports the index of its top row in view (the rows' order on screen).
    async function expandAllWithTopRow(topIndex: number | null) {
        serve()
        const fold = renderFold()
        await waitFor(() =>
            expect(rowIds(fold.result.current)).toEqual(rowIds({ result: closedPage }))
        )
        if (topIndex !== null) act(() => fold.result.current.onTopRowIndex?.(topIndex))
        return expandAll(fold)
    }

    it('Expand all at the top of closed groups asks for one page', async () => {
        const run = await expandAllWithTopRow(0)
        expect(sent).toHaveLength(1)
        expect(rowIds(run)).toEqual(['g:a', 'd:a1', 'd:a2'])
        expect(run.hasMore).toBe(true)
    })

    it('Expand all joins pages up to the group in view, and no further', async () => {
        const run = await expandAllWithTopRow(1)
        expect(sent.map(request => request.options.cursor)).toEqual([undefined, 'c1', 'c2'])
        expect(rowIds(run)).toEqual([
            'g:a',
            'd:a1',
            'd:a2',
            'd:a3',
            'd:a4',
            's:a',
            'g:b',
            'd:b1',
            'd:b2',
        ])
    })

    it('Expand all with no row reported in view asks for one page', async () => {
        await expandAllWithTopRow(null)
        expect(sent).toHaveLength(1)
    })

    it('a row in view that the shown pages do not hold adds nothing to the fold', async () => {
        await expandAllWithTopRow(9)
        expect(sent).toHaveLength(1)
    })

    // FlashList reports only when the indices in view change. A live rerun that
    // changes the rows under the same indices reports nothing, so the place is the
    // row now at the reported index, not the row that was there before.
    it('takes the row now at the reported index when the rows change under it', async () => {
        const before = result([row('g:a', 'group'), row('g:c', 'group'), row('total', 'total')])
        serve([before, closedPage])
        const fold = renderFold()
        await waitFor(() => expect(rowIds(fold.result.current)).toEqual(['g:a', 'g:c', 'total']))
        act(() => fold.result.current.onTopRowIndex?.(1))
        await subscribed()
        signal()
        await waitFor(() =>
            expect(rowIds(fold.result.current)).toEqual(rowIds({ result: closedPage }))
        )
        await expandAll(fold)
        expect(sent.map(request => request.options.cursor)).toEqual([undefined, 'c1', 'c2'])
    })
})
