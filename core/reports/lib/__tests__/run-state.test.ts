import { describe, expect, it } from 'vitest'
import type { RunOutcome } from '../run-client'
import { type RunQueryState, runView } from '../run-state'
import type { RunResult } from '../types'

const result = (topics: string[]): RunResult => ({
    columns: [{ id: 'name', label: 'Name', type: 'text' }],
    rows: [{ id: 'd:1', kind: 'detail', depth: 0, label: 'Alpha', cells: { name: 'Alpha' } }],
    notices: [],
    topics,
    page: null,
    params: [],
})
const ok = (topics = ['collection/sales']): RunOutcome => ({ kind: 'ok', result: result(topics) })
const problem = { path: 'columns.0', code: 'unknown_field', message: 'no field "x"' }
const invalid: RunOutcome = { kind: 'invalid', message: 'bad', problems: [problem] }

const query = (extra: Partial<RunQueryState>): RunQueryState => ({
    hasBody: true,
    pages: [],
    isPending: false,
    isError: false,
    isLoadMoreError: false,
    isPlaceholderData: false,
    ...extra,
})

describe('runView', () => {
    it('is idle with no body', () => {
        expect(runView(query({ hasBody: false, isPending: true }))).toEqual({
            state: 'idle',
            result: undefined,
            problems: [],
            isUpdating: false,
            heldRows: [],
            topics: [],
        })
    })

    // R8: a failed next page keeps the pages already shown and the run ready; the
    // list ends in a row with Retry (useReportRun's loadMoreFailed).
    it('stays ready with its rows when only the next page failed', () => {
        const view = runView(query({ isError: true, isLoadMoreError: true, pages: [ok()] }))
        expect(view.state).toBe('ready')
        expect(view.result?.rows).toHaveLength(1)
    })

    it('is loading while the first run is pending', () => {
        const view = runView(query({ isPending: true }))
        expect(view.state).toBe('loading')
        expect(view.result).toBeUndefined()
        expect(view.isUpdating).toBe(false)
    })

    it('shows the figures and their topics once the run answers', () => {
        const view = runView(query({ pages: [ok()] }))
        expect(view.state).toBe('ready')
        expect(view.result?.rows.map(row => row.label)).toEqual(['Alpha'])
        expect(view.topics).toEqual(['collection/sales'])
    })

    it('keeps one result, rows and all, while the pages stay the same', () => {
        const pages = [ok()]
        const shown = runView(query({ pages }))
        // A changed run starts loading: the same pages stay in view.
        const updating = runView(query({ pages, isPlaceholderData: true }))
        expect(updating.result).toBe(shown.result)
        expect(runView(query({ pages: [ok()] })).result).not.toBe(shown.result)
    })

    it('keeps the previous figures in view, marked updating, while a changed run loads', () => {
        const view = runView(query({ pages: [ok()], isPlaceholderData: true }))
        expect(view.state).toBe('ready')
        expect(view.result?.rows).toHaveLength(1)
        expect(view.isUpdating).toBe(true)
    })

    it('does not show the previous run’s refusal while a changed run loads', () => {
        const view = runView(query({ pages: [invalid], isPlaceholderData: true }))
        expect(view.state).toBe('loading')
        expect(view.problems).toEqual([])
        expect(view.result).toBeUndefined()
        expect(view.isUpdating).toBe(false)
    })

    it('shows this run’s refusal with its problems, and no figures', () => {
        const view = runView(query({ pages: [invalid] }))
        expect(view.state).toBe('invalid')
        expect(view.problems).toEqual([problem])
        expect(view.result).toBeUndefined()
        expect(runView(query({ pages: [{ kind: 'no-access' }] })).state).toBe('no-access')
    })

    it('is an error when the run fails, keeping figures it already had', () => {
        expect(runView(query({ isError: true })).state).toBe('error')
        const kept = runView(query({ isError: true, pages: [ok()] }))
        expect(kept.state).toBe('error')
        expect(kept.result?.rows).toHaveLength(1)
    })

    const withTotal = (next: string): RunOutcome => {
        const base = result(['collection/sales'])
        return {
            kind: 'ok',
            result: {
                ...base,
                rows: [
                    ...base.rows,
                    { id: 'total', kind: 'total', depth: 0, label: 'Total', cells: {} },
                ],
                page: { size: 1, next },
            },
        }
    }

    it('holds the Total back while a later page is left, and shows it with the last page', () => {
        const first = runView(query({ pages: [withTotal('c1')] }))
        expect(first.result?.rows.map(row => row.id)).toEqual(['d:1'])
        expect(first.heldRows.map(row => row.id)).toEqual(['total'])
        const all = runView(query({ pages: [withTotal('c1'), withTotal('')] }))
        expect(all.result?.rows.map(row => row.id)).toEqual(['d:1', 'total'])
        expect(all.heldRows).toEqual([])
    })

    it('holds the Total back again when a re-run’s first page has a later page', () => {
        const pages = [withTotal('c1')]
        const shown = runView(query({ pages }))
        const updating = runView(query({ pages, isPlaceholderData: true }))
        expect(updating.result).toBe(shown.result)
        expect(updating.heldRows).toBe(shown.heldRows)
        expect(runView(query({ pages: [withTotal('c2')] })).heldRows.map(row => row.id)).toEqual([
            'total',
        ])
    })
})
