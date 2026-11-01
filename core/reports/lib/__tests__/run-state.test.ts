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
            topics: [],
        })
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
})
