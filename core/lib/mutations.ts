import type { Transaction } from '@tanstack/react-db'
import {
    type UseMutationOptions,
    type UseMutationResult,
    useMutation as useTanStackMutation,
} from '@tanstack/react-query'
import { captureException, errorToString } from '@tinycld/core/lib/errors'
import { log } from '@tinycld/core/lib/logger'
import { notify } from '@tinycld/core/lib/notify/dispatcher'
import { useConnectivityStore } from '@tinycld/core/lib/stores/connectivity-store'
import { writesAvailability } from '@tinycld/core/lib/writes-available'
import { batch } from 'pbtsdb'

// A generator yields what it must wait for: a pbtsdb Transaction (one request
// for one row) or a Promise (an atomic() batch), alone or as an array to wait
// for in parallel.
type MutationYield = Transaction<Record<string, unknown>> | Promise<unknown>
type TransactionYield = MutationYield | MutationYield[]

const settled = (value: MutationYield): Promise<unknown> =>
    'when' in value ? value.when('settled') : value

// Drive a Transaction-yielding generator to completion, awaiting each yielded
// Transaction or Promise (or array, in parallel) before resuming. Takes an
// already-created iterator so callers that must invoke the generator fn exactly
// once (to detect whether it IS a generator by inspecting its return value) can
// hand the result straight in without re-invoking.
async function drainTransactions<TResult>(
    gen: Generator<TransactionYield, TResult, void>
): Promise<TResult> {
    let result = gen.next()
    while (!result.done) {
        const value = result.value
        if (Array.isArray(value)) {
            await Promise.all(value.map(settled))
        } else {
            await settled(value)
        }
        result = gen.next()
    }

    return result.value
}

/**
 * Awaits each yielded Transaction or Promise sequentially, or an array of them in parallel.
 *
 * @example
 * ```ts
 * await performMutations(function* () {
 *   yield addressesCollection.insert({ id: newRecordId(), ... })
 *   yield customersCollection.insert({ id: newRecordId(), ... })
 * })
 * ```
 */
export async function performMutations<TResult = void>(
    fn: () => Generator<TransactionYield, TResult, void>
): Promise<TResult> {
    return drainTransactions(fn())
}

/**
 * Runs the collection mutations made in `fn` as one request and one database
 * transaction: all of them are saved, or none. Use it when a partial result
 * is a defect (a document and its lines, a transfer between two rows).
 *
 * `fn` is synchronous: the calls only record the mutations, and the one
 * request is sent when `fn` returns. Never `await` inside it.
 *
 * @example
 * ```ts
 * const save = useMutation({
 *     mutationFn: (values: FormValues) =>
 *         atomic(() => {
 *             ordersCollection.update(id, draft => { draft.note = values.note })
 *             for (const line of values.lines) linesCollection.insert(line)
 *         }),
 *     onError: handleMutationErrorsWithForm({ setError, getValues }),
 * })
 * ```
 *
 * It does not detect a stale editor; it only makes the write atomic. File
 * uploads cannot be part of it.
 */
export const atomic = batch

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
) => Generator<TransactionYield, TData, void>

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
// rename that just no-ops with no feedback and nothing in Sentry (a
// call-site audit counted ~36 such call sites). Mirrors the two halves
// that already exist elsewhere: the toast is what handleMutationErrorsWithForm
// does for non-validation errors, the capture is what the global QueryCache
// onError does for reads.
function reportUnhandledMutationError(mutationKey: readonly unknown[] | undefined) {
    return (error: unknown) => {
        const operation = mutationKey ? mutationKey.join('.') : 'unhandled'
        const message = errorToString(error)
        // Offline, a failed save is expected rather than a bug: say why in
        // words instead of the raw network error, and keep it out of Sentry.
        const writes = writesAvailability(useConnectivityStore.getState())
        if (!writes.available) {
            log.info('mutation.offline', message, { operation })
            notify.emit({
                event: 'mutation.error',
                title: "Your change wasn't saved",
                body: writes.reason,
                data: { operation, error: message },
            })
            return
        }
        captureException('mutation.error', error, { operation })
        notify.emit({
            event: 'mutation.error',
            title: 'Something went wrong',
            body: message,
            data: { operation, error: message },
        })
    }
}

// A single wrapper that decides generator-vs-async by INVOKING the fn once
// and inspecting what it returns — not by the fragile constructor.name of
// the fn itself. The fn is called exactly once per mutation run (no
// double-invocation of a non-idempotent async fn): a generator's call is
// side-effect free until driven, so calling it here to test the result is
// safe, and an async fn's returned promise is awaited directly.
// A named function, not an arrow inside useMutation's conditional: @babel/parser
// 7.29.13 refuses an arrow with typed parameters and a return type as a
// conditional's consequent ("Did not expect a type annotation here"), which
// broke the web export.
function wrapMutationFn<TData, TVariables>(
    mutationFn: (v: TVariables) => ReturnType<GeneratorMutationFn<TData, TVariables>>
) {
    return async (variables: TVariables): Promise<TData> => {
        const result = mutationFn(variables)
        if (isTransactionGenerator(result)) {
            return drainTransactions(result) as Promise<TData>
        }
        return result as unknown as Promise<TData>
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

    const wrappedMutationFn = mutationFn
        ? wrapMutationFn(
              mutationFn as (v: TVariables) => ReturnType<GeneratorMutationFn<TData, TVariables>>
          )
        : undefined

    return useTanStackMutation({
        ...restOptions,
        onError: restOptions.onError ?? reportUnhandledMutationError(restOptions.mutationKey),
        mutationFn: wrappedMutationFn,
    } as UseMutationOptions<TData, TError, TVariables>)
}
