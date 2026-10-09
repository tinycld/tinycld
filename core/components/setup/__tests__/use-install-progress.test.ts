// @vitest-environment happy-dom

// useInstallProgress now watches the pkg_install_log row directly (a pbtsdb
// live query by job_id) instead of an EventSource + durable-poll fallback, so
// these tests drive it by varying what the mocked `pkg_install_log` collection
// holds for a job id — the same shape a real server write to the row takes —
// rather than faking SSE events or HTTP responses.

import { createCollection, localOnlyCollectionOptions } from '@tanstack/db'
import { cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

interface InstallLogRow {
    id: string
    job_id: string
    status: 'running' | 'success' | 'failed' | 'rolled_back'
    error: string
    steps: { step: string; progress: number; message: string }[]
}

function rowCollection(rows: InstallLogRow[]) {
    return createCollection(
        localOnlyCollectionOptions({
            id: `pkg-install-log-${Math.random()}`,
            getKey: (r: InstallLogRow) => r.id,
            initialData: rows,
        })
    )
}

const h = vi.hoisted(() => ({ collection: null as unknown }))

vi.mock('@tinycld/core/lib/pocketbase', () => ({
    useStore: () => [h.collection],
}))

import { useInstallProgress } from '../use-install-progress'

afterEach(cleanup)

describe('useInstallProgress', () => {
    it('reports running with the accumulated steps while the row is unfinished', async () => {
        h.collection = rowCollection([
            {
                id: 'log1',
                job_id: 'job_1',
                status: 'running',
                error: '',
                steps: [
                    { step: 'Checking the package', progress: 1, message: 'Checking' },
                    { step: 'Downloading the package', progress: 2, message: 'npm pack' },
                ],
            },
        ])
        const onSuccess = vi.fn()

        const { result } = renderHook(() => useInstallProgress(true, 'job_1', onSuccess))

        await waitFor(() => expect(result.current.steps.length).toBe(2))
        expect(result.current.status).toBe('running')
        expect(result.current.error).toBeNull()
        expect(onSuccess).not.toHaveBeenCalled()
    })

    it('resolves to success and fires onSuccess once the row finishes', async () => {
        h.collection = rowCollection([
            { id: 'log2', job_id: 'job_2', status: 'success', error: '', steps: [] },
        ])
        const onSuccess = vi.fn()

        const { result } = renderHook(() => useInstallProgress(true, 'job_2', onSuccess))

        await waitFor(() => expect(result.current.status).toBe('success'))
        expect(onSuccess).toHaveBeenCalledTimes(1)
    })

    it('surfaces the row error when the job fails', async () => {
        h.collection = rowCollection([
            {
                id: 'log3',
                job_id: 'job_3',
                status: 'failed',
                error: 'npm pack: exit 1',
                steps: [],
            },
        ])
        const onSuccess = vi.fn()

        const { result } = renderHook(() => useInstallProgress(true, 'job_3', onSuccess))

        await waitFor(() => expect(result.current.status).toBe('failed'))
        expect(result.current.error).toBe('npm pack: exit 1')
        expect(onSuccess).not.toHaveBeenCalled()
    })

    // 'rolled_back' is the server's terminal status after a post-activation
    // health-check rollback. It carries no error string of its own (the row's
    // `error` field is empty), so the hook supplies the explanatory message —
    // the UI would otherwise show a blank failure.
    it('maps a rolled-back row to failed with an explanatory message', async () => {
        h.collection = rowCollection([
            { id: 'log4', job_id: 'job_4', status: 'rolled_back', error: '', steps: [] },
        ])
        const onSuccess = vi.fn()

        const { result } = renderHook(() => useInstallProgress(true, 'job_4', onSuccess))

        await waitFor(() => expect(result.current.status).toBe('failed'))
        expect(result.current.error).toMatch(/roll(ed)? back/i)
        expect(onSuccess).not.toHaveBeenCalled()
    })

    it('stays idle with no steps while inactive or without a job id', () => {
        h.collection = rowCollection([])
        const onSuccess = vi.fn()

        const { result } = renderHook(() => useInstallProgress(false, null, onSuccess))

        expect(result.current.steps).toEqual([])
        expect(result.current.status).toBe('running')
        expect(onSuccess).not.toHaveBeenCalled()
    })
})
