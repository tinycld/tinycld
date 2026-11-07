// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { RunBody } from '../../lib/run-client'
import type { ResultRow, RunResult } from '../../lib/types'
import { useReportRun } from '../useReportRun'

// In react-query a refetch and a next page cancel each other. A live rerun that
// lands while the list loads its next page must not drop that page: the list asks
// again only when its rows change, so a first page shorter than the view stays one
// page, with the report's Total under it (CI run 42367362188). And a next page asked
// for during a rerun must not drop the rerun's figures.

const h = vi.hoisted(() => ({
    send: (_path: string, _init: { body: RunBody; signal?: AbortSignal }) =>
        Promise.resolve({} as unknown),
    signalTopic: () => {},
}))

vi.mock('@tinycld/core/lib/pocketbase', () => ({
    pb: {
        send: (path: string, init: { body: RunBody; signal?: AbortSignal }) => h.send(path, init),
    },
}))

vi.mock('pbtsdb', () => ({
    RealtimeSubscribeAbortedError: class extends Error {},
    subscribeTopic: (_pb: unknown, _topic: string, onMessage: () => void) => {
        h.signalTopic = onMessage
        return Promise.resolve(async () => {})
    },
}))

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
    topics: ['salesstub/changes'],
    page: next ? { next, size: rows.length } : null,
    params: [],
})

/** A server answer held until the test lets it go; the request's signal rejects it. */
function held<T>(value: T, signal?: AbortSignal) {
    let release = () => {}
    const promise = new Promise<T>((resolve, reject) => {
        release = () => resolve(value)
        signal?.addEventListener('abort', () => reject(new Error('aborted')))
    })
    return { promise, release }
}

const body: RunBody = { reportId: 'r1', params: {}, options: { today: '2026-11-07' } }

function renderRun() {
    const queryClient = new QueryClient()
    const wrapper = ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
    return renderHook(() => useReportRun({ body, userId: 'u1' }), { wrapper })
}

const rowIds = (run: { result: RunResult | undefined }) => run.result?.rows.map(r => r.id)

describe('useReportRun', () => {
    it('keeps a next page that a live rerun lands on', async () => {
        const first = result([row('g:A', 'group'), row('d:1')], 'c1')
        const second = result([row('d:2'), row('total', 'total')])
        let firstNextPage: ReturnType<typeof held<RunResult>> | undefined
        h.send = (_path, { body: sent, signal }) => {
            if (!sent.options.cursor) return Promise.resolve(first)
            if (firstNextPage) return Promise.resolve(second)
            firstNextPage = held(second, signal)
            return firstNextPage.promise
        }
        const { result: run } = renderRun()
        await waitFor(() => expect(run.current.hasMore).toBe(true))

        act(() => run.current.loadMore())
        await waitFor(() => expect(firstNextPage).toBeDefined())
        act(() => h.signalTopic())
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
        h.send = (_path, { body: sent, signal }) => {
            if (sent.options.cursor) return Promise.resolve(second)
            firstPages += 1
            if (firstPages === 1) return Promise.resolve(first)
            rerun = held(changed, signal)
            return rerun.promise
        }
        const { result: run } = renderRun()
        await waitFor(() => expect(run.current.hasMore).toBe(true))

        act(() => h.signalTopic())
        await waitFor(() => expect(rerun).toBeDefined())
        act(() => run.current.loadMore())
        act(() => rerun?.release())

        await waitFor(() => expect(rowIds(run.current)).toEqual(['g:A', 'd:1b', 'd:2', 'total']))
    })
})
