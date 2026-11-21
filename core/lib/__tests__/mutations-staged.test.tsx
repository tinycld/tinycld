// @vitest-environment happy-dom

// Guards the two halves of useStagedMutation: the stage step of every call runs
// when mutate is called, and the commits go out one at a time in call order.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useStagedMutation } from '../mutations'

type Commit = { resolve: () => void; reject: (error: Error) => void }

const events: string[] = []
const commits: Commit[] = []

vi.mock('pbtsdb', () => ({
    batch: vi.fn(),
    stagedBatch: (fn: () => void) => {
        fn()
        return {
            commit: () =>
                new Promise<void>((resolve, reject) => {
                    events.push('commit started')
                    commits.push({ resolve, reject })
                }),
            rollback: () => {},
        }
    },
}))
vi.mock('@tinycld/core/lib/pocketbase', () => ({
    notificationsCollection: {
        insert: vi.fn(() => ({ when: () => Promise.resolve() })),
    },
}))
vi.mock('@tinycld/core/lib/notifications', () => ({
    showOsNotification: vi.fn(() => Promise.resolve()),
}))
vi.mock('@tinycld/core/lib/sentry', () => ({
    captureExceptionToSentry: vi.fn(),
    addBreadcrumbToSentry: vi.fn(),
}))

function wrapper() {
    const client = new QueryClient({
        defaultOptions: { mutations: { retry: false } },
    })
    return ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
    )
}

function renderStaged(onError?: (error: Error) => void) {
    return renderHook(
        () =>
            useStagedMutation({
                scope: { id: 'test.staged' },
                stage: (title: string) => {
                    events.push(`staged ${title}`)
                    return `id of ${title}`
                },
                onError,
            }),
        { wrapper: wrapper() }
    )
}

describe('useStagedMutation', () => {
    beforeEach(() => {
        events.length = 0
        commits.length = 0
    })

    it('stages every call at once and commits one at a time in call order', async () => {
        const { result } = renderStaged()

        let first: Promise<string> = Promise.resolve('')
        let second: Promise<string> = Promise.resolve('')
        act(() => {
            first = result.current.mutateAsync('first')
            second = result.current.mutateAsync('second')
        })

        await waitFor(() => expect(commits).toHaveLength(1))
        expect(events).toEqual(['staged first', 'staged second', 'commit started'])

        commits[0].resolve()
        await expect(first).resolves.toBe('id of first')
        await waitFor(() => expect(commits).toHaveLength(2))

        commits[1].resolve()
        await expect(second).resolves.toBe('id of second')
        expect(events).toEqual([
            'staged first',
            'staged second',
            'commit started',
            'commit started',
        ])
    })

    it('commits the next call after a call that failed', async () => {
        const onError = vi.fn()
        const { result } = renderStaged(onError)

        let first: Promise<string> = Promise.resolve('')
        let second: Promise<string> = Promise.resolve('')
        act(() => {
            first = result.current.mutateAsync('first')
            second = result.current.mutateAsync('second')
        })
        first.catch(() => {})

        await waitFor(() => expect(commits).toHaveLength(1))
        commits[0].reject(new Error('refused'))
        await expect(first).rejects.toThrow('refused')

        await waitFor(() => expect(commits).toHaveLength(2))
        commits[1].resolve()
        await expect(second).resolves.toBe('id of second')
        expect(onError).toHaveBeenCalledTimes(1)
    })
})
