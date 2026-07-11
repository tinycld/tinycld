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
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { useMutation } from '../mutations'

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
