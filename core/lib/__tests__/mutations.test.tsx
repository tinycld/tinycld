// @vitest-environment happy-dom

// Guards the generator-vs-async detection in useMutation. The detection must
// key off the RETURNED iterator, not mutationFn.constructor.name — otherwise a
// wrapped generator (a decorator, a wrapping closure, or Babel's regenerator
// transpile that turns a generator into a plain fn returning a runtime iterator)
// is misclassified as an async fn, its raw generator is returned unconsumed,
// and NO transaction is ever awaited (a mutation that "succeeds" without
// persisting).

import type { Transaction } from '@tanstack/react-db'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import { useToastStore } from '@tinycld/core/lib/stores/toast-store'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useMutation } from '../mutations'

vi.mock('@tinycld/core/lib/pocketbase', () => ({
    notificationsCollection: {
        insert: vi.fn(() => ({ isPersisted: { promise: Promise.resolve() } })),
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

type FakeTransaction = Transaction<Record<string, unknown>>

// A stand-in for a pbtsdb Transaction: the mutation machinery only touches
// `isPersisted.promise`, so we stub that and cast to Transaction to satisfy the
// GeneratorMutationFn yield type (the generators below must typecheck).
function fakeTransaction() {
    let resolvePersist!: () => void
    const promise = new Promise<void>(resolve => {
        resolvePersist = resolve
    })
    const tx = { isPersisted: { promise } } as unknown as FakeTransaction
    return { tx, resolvePersist }
}

describe('useMutation generator detection', () => {
    it('awaits yielded transactions for a plain generator mutationFn', async () => {
        const { tx, resolvePersist } = fakeTransaction()
        const afterYield = vi.fn()

        const { result } = renderHook(
            () =>
                useMutation({
                    mutationFn: function* () {
                        yield tx
                        afterYield()
                        return 'done'
                    },
                }),
            { wrapper: wrapper() }
        )

        result.current.mutate()

        // The generator must be SUSPENDED at the yield until the transaction persists.
        await Promise.resolve()
        expect(afterYield).not.toHaveBeenCalled()

        resolvePersist()
        await waitFor(() => expect(result.current.isSuccess).toBe(true))
        expect(afterYield).toHaveBeenCalledTimes(1)
        expect(result.current.data).toBe('done')
    })

    it('still awaits transactions when the mutationFn is a plain fn returning an iterator (transpiled/wrapped)', async () => {
        const { tx, resolvePersist } = fakeTransaction()
        const afterYield = vi.fn()

        function* gen() {
            yield tx
            afterYield()
            return 'wrapped-done'
        }

        // The real-world failure mode (Babel regenerator transpile, a decorator,
        // or a wrapping closure): mutationFn is a PLAIN function — its
        // constructor.name is 'Function', not 'GeneratorFunction', so the old
        // check misclassified it as async — but calling it returns an iterator.
        // The robust detection must key off that returned iterator.
        const wrappedGen = (...args: []) => gen(...args)
        expect(wrappedGen.constructor.name).toBe('Function')

        const { result } = renderHook(() => useMutation({ mutationFn: wrappedGen }), {
            wrapper: wrapper(),
        })

        result.current.mutate()

        await Promise.resolve()
        expect(afterYield).not.toHaveBeenCalled()

        resolvePersist()
        await waitFor(() => expect(result.current.isSuccess).toBe(true))
        // If the wrapped generator were misclassified as async, the raw
        // generator object would be returned unconsumed: afterYield would never
        // run and data would be the iterator, not its return value.
        expect(afterYield).toHaveBeenCalledTimes(1)
        expect(result.current.data).toBe('wrapped-done')
    })

    it('awaits a plain async mutationFn without treating it as a generator', async () => {
        const asyncFn = vi.fn(async () => 'async-done')

        const { result } = renderHook(() => useMutation({ mutationFn: asyncFn }), {
            wrapper: wrapper(),
        })

        result.current.mutate()

        await waitFor(() => expect(result.current.isSuccess).toBe(true))
        expect(asyncFn).toHaveBeenCalledTimes(1)
        expect(result.current.data).toBe('async-done')
    })
})

// A mutation without an explicit onError used to fail as an optimistic update
// that silently reverted — the pre-multi-org review found ~36 call sites where
// a toggle or rename just no-opped with no feedback and nothing in Sentry.
// The wrapper now installs a default onError (toast + captureException) so a
// failure is always surfaced somewhere; an explicit onError replaces it.
describe('useMutation default onError', () => {
    beforeEach(() => {
        useToastStore.setState({ toasts: [] })
        vi.clearAllMocks()
    })

    it('surfaces a failed mutation as an error toast and captures it', async () => {
        const { captureExceptionToSentry } = await import('@tinycld/core/lib/sentry')

        const { result } = renderHook(
            () =>
                useMutation({
                    mutationFn: async () => {
                        throw new Error('rule denied the write')
                    },
                }),
            { wrapper: wrapper() }
        )

        result.current.mutate()
        await waitFor(() => expect(result.current.isError).toBe(true))

        const toasts = useToastStore.getState().toasts
        expect(toasts).toHaveLength(1)
        expect(toasts[0]).toMatchObject({ title: 'Something went wrong', variant: 'error' })
        expect(toasts[0].body).toContain('rule denied the write')
        expect(captureExceptionToSentry).toHaveBeenCalledWith(
            'mutation.error',
            expect.any(Error),
            expect.anything()
        )
    })

    it('stays silent on success', async () => {
        const { result } = renderHook(() => useMutation({ mutationFn: async () => 'ok' }), {
            wrapper: wrapper(),
        })

        result.current.mutate()
        await waitFor(() => expect(result.current.isSuccess).toBe(true))
        expect(useToastStore.getState().toasts).toHaveLength(0)
    })

    it('an explicit onError replaces the default entirely', async () => {
        const { captureExceptionToSentry } = await import('@tinycld/core/lib/sentry')
        const onError = vi.fn((_error: Error) => {})

        const { result } = renderHook(
            () =>
                useMutation({
                    mutationFn: async () => {
                        throw new Error('handled elsewhere')
                    },
                    onError,
                }),
            { wrapper: wrapper() }
        )

        result.current.mutate()
        await waitFor(() => expect(result.current.isError).toBe(true))

        expect(onError).toHaveBeenCalledTimes(1)
        expect(useToastStore.getState().toasts).toHaveLength(0)
        expect(captureExceptionToSentry).not.toHaveBeenCalled()
    })

    it('applies the default when a yielded transaction fails to persist', async () => {
        // The exact silent-revert scenario from the review: the optimistic
        // write is rejected server-side (e.g. by a collection rule), the
        // transaction's isPersisted promise rejects, and the local update
        // rolls back. Without the default onError nothing tells the user.
        const rejected = {
            isPersisted: { promise: Promise.reject(new Error('generator failed')) },
        } as unknown as FakeTransaction

        const { result } = renderHook(
            () =>
                useMutation({
                    mutationFn: function* () {
                        yield rejected
                    },
                }),
            { wrapper: wrapper() }
        )

        result.current.mutate()
        await waitFor(() => expect(result.current.isError).toBe(true))

        const toasts = useToastStore.getState().toasts
        expect(toasts).toHaveLength(1)
        expect(toasts[0].body).toContain('generator failed')
    })
})
