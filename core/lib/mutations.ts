import type { Transaction } from '@tanstack/react-db'
import {
    type UseMutationOptions,
    type UseMutationResult,
    useMutation as useTanStackMutation,
} from '@tanstack/react-query'
import { captureException, errorToString } from '@tinycld/core/lib/errors'
import { notify } from '@tinycld/core/lib/notify/dispatcher'

/**
 * Awaits each yielded Transaction sequentially, or an array of Transactions in parallel.
 *
 * @example
 * ```ts
 * await performMutations(function* () {
 *   yield addressesCollection.insert({ id: newRecordId(), ... })
 *   yield customersCollection.insert({ id: newRecordId(), ... })
 * })
 * ```
 */
type TransactionYield =
    | Transaction<Record<string, unknown>>
    | Transaction<Record<string, unknown>>[]

// Drive a Transaction-yielding generator to completion, awaiting each yielded
// Transaction (or array, in parallel) before resuming. Takes an already-created
// iterator so callers that must invoke the generator fn exactly once (to detect
// whether it IS a generator by inspecting its return value) can hand the result
// straight in without re-invoking.
async function drainTransactions<TResult>(
    gen: Generator<TransactionYield, TResult, void>
): Promise<TResult> {
    let result = gen.next()
    while (!result.done) {
        const value = result.value
        if (Array.isArray(value)) {
            await Promise.all(value.map(tx => tx.isPersisted.promise))
        } else {
            await value.isPersisted.promise
        }
        result = gen.next()
    }

    return result.value
}

export async function performMutations<TResult = void>(
    fn: () => Generator<
        Transaction<Record<string, unknown>> | Transaction<Record<string, unknown>>[],
        TResult,
        void
    >
): Promise<TResult> {
    return drainTransactions(fn())
}

// A generator's return value is an iterator: it has both `.next()` and is
// iterable via Symbol.iterator. Duck-typing on the RETURNED object is robust
// where `mutationFn.constructor.name === 'GeneratorFunction'` is not — a
// decorator, a wrapping closure, or Babel's regenerator transpile (which turns
// a generator into a plain function whose call returns a runtime iterator) all
// leave a fn whose constructor.name is 'Function', which the old check treated
// as async — returning the raw generator unconsumed so NO transaction was ever
// awaited (a mutation that "succeeds" without persisting).
function isTransactionGenerator(
    value: unknown
): value is Generator<TransactionYield, unknown, void> {
    return (
        value != null &&
        typeof (value as Iterator<unknown>).next === 'function' &&
        typeof (value as Iterable<unknown>)[Symbol.iterator] === 'function'
    )
}

type GeneratorMutationFn<TData, TVariables> = (
    variables: TVariables
) => Generator<
    Transaction<Record<string, unknown>> | Transaction<Record<string, unknown>>[],
    TData,
    void
>

type GeneratorMutationOptions<TData = unknown, TError = Error, TVariables = void> = Omit<
    UseMutationOptions<TData, TError, TVariables>,
    'mutationFn'
> & {
    mutationFn: GeneratorMutationFn<TData, TVariables>
}

type AsyncMutationOptions<TData = unknown, TError = Error, TVariables = void> = UseMutationOptions<
    TData,
    TError,
    TVariables
>

/**
 * Converts a generator function into an async function that awaits
 * each yielded Transaction via performMutations. Use this to wrap
 * generator-based mutationFn before passing to useMutation.
 *
 * @example
 * ```ts
 * const create = useMutation({
 *     mutationFn: mutation(function* (data: FormData) {
 *         yield contactsCollection.insert({ id: newRecordId(), ...data })
 *     }),
 * })
 * ```
 */
export function mutation<TData = void, TVariables = void>(
    genFn: GeneratorMutationFn<TData, TVariables>
): (variables: TVariables) => Promise<TData> {
    return (variables: TVariables) => performMutations(() => genFn(variables))
}

// The default onError for mutations that don't supply one. Without it a
// failed mutation is an optimistic update that silently reverts — a toggle or
// rename that just no-ops with no feedback and nothing in Sentry (the
// pre-multi-org review counted ~36 such call sites). Mirrors the two halves
// that already exist elsewhere: the toast is what handleMutationErrorsWithForm
// does for non-validation errors, the capture is what the global QueryCache
// onError does for reads.
function reportUnhandledMutationError(mutationKey: readonly unknown[] | undefined) {
    return (error: unknown) => {
        const operation = mutationKey ? mutationKey.join('.') : 'unhandled'
        const message = errorToString(error)
        captureException('mutation.error', error, { operation })
        notify.emit({
            event: 'mutation.error',
            title: 'Something went wrong',
            body: message,
            data: { operation, error: message },
        })
    }
}

/**
 * Wraps TanStack Query's useMutation with support for generator-based mutation functions.
 * Generator functions are auto-wrapped with performMutations so each yielded
 * Transaction is awaited before proceeding.
 *
 * A mutation that fails without an explicit `onError` surfaces a default
 * error toast and reports to Sentry, so optimistic updates never revert
 * silently. Passing `onError` (e.g. handleMutationErrorsWithForm) replaces
 * the default entirely; pass a no-op only when a failure is genuinely fine
 * to swallow, with a comment saying why.
 *
 * @example Generator-based (recommended for pbtsdb operations)
 * ```ts
 * const navigateBack = useNavigateBack(() => orgHref('contacts'))
 * const create = useMutation({
 *     mutationFn: function* (data: FormData) {
 *         yield contactsCollection.insert({ id: newRecordId(), ...data })
 *     },
 *     onSuccess: navigateBack,
 *     onError: handleMutationErrorsWithForm({ setError, getValues }),
 * })
 * ```
 *
 * @example Async (when you need non-pbtsdb async work)
 * ```ts
 * const create = useMutation({
 *     mutationFn: async (data: FormData) => {
 *         const geo = await geocode(data.address)
 *         await performMutations(function* () {
 *             yield contactsCollection.insert({ ...data, geo })
 *         })
 *     },
 * })
 * ```
 */
export function useMutation<TData = unknown, TError = Error, TVariables = void>(
    options:
        | GeneratorMutationOptions<TData, TError, TVariables>
        | AsyncMutationOptions<TData, TError, TVariables>
): UseMutationResult<TData, TError, TVariables> {
    const { mutationFn, ...restOptions } = options

    // A single wrapper that decides generator-vs-async by INVOKING the fn once
    // and inspecting what it returns — not by the fragile constructor.name of
    // the fn itself. The fn is called exactly once per mutation run (no
    // double-invocation of a non-idempotent async fn): a generator's call is
    // side-effect free until driven, so calling it here to test the result is
    // safe, and an async fn's returned promise is awaited directly.
    const wrappedMutationFn = mutationFn
        ? async (variables: TVariables): Promise<TData> => {
              const result = (
                  mutationFn as (
                      v: TVariables
                  ) => ReturnType<GeneratorMutationFn<TData, TVariables>>
              )(variables)
              if (isTransactionGenerator(result)) {
                  return drainTransactions(result) as Promise<TData>
              }
              return result as unknown as Promise<TData>
          }
        : undefined

    return useTanStackMutation({
        ...restOptions,
        onError: restOptions.onError ?? reportUnhandledMutationError(restOptions.mutationKey),
        mutationFn: wrappedMutationFn,
    } as UseMutationOptions<TData, TError, TVariables>)
}
