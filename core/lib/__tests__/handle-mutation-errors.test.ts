import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { useToastStore } from '@tinycld/core/lib/stores/toast-store'
import { BatchMutationError } from 'pbtsdb'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@tinycld/core/lib/pocketbase', () => ({
    notificationsCollection: {
        insert: vi.fn(() => ({ when: () => Promise.resolve() })),
    },
}))
vi.mock('@tinycld/core/lib/notifications', () => ({
    showOsNotification: vi.fn(() => Promise.resolve()),
}))

describe('handleMutationErrorsWithForm', () => {
    beforeEach(() => {
        useToastStore.setState({ toasts: [] })
    })
    afterEach(() => {
        useToastStore.setState({ toasts: [] })
        vi.restoreAllMocks()
    })

    it('sets field errors when they match form fields', () => {
        const setError = vi.fn()
        const getValues = () => ({ email: '', name: '' })
        const handler = handleMutationErrorsWithForm({ setError, getValues })
        handler({
            response: { data: { data: { email: { code: 'x', message: 'required' } } } },
        })
        expect(setError).toHaveBeenCalledWith(
            'email',
            expect.objectContaining({ message: 'required' })
        )
        expect(useToastStore.getState().toasts).toHaveLength(0)
    })

    it('emits mutation.error for non-validation errors', () => {
        const setError = vi.fn()
        const getValues = () => ({ email: '' })
        const handler = handleMutationErrorsWithForm({
            setError,
            getValues,
            operation: 'save settings',
        })
        handler(new Error('Network unreachable'))
        const toasts = useToastStore.getState().toasts
        expect(toasts).toHaveLength(1)
        expect(toasts[0]).toMatchObject({
            title: 'Something went wrong',
            variant: 'error',
        })
        expect(toasts[0].body).toContain('Network unreachable')
        expect(setError).not.toHaveBeenCalled()
    })

    it('emits mutation.error when validation has fields the form does not know about', () => {
        const setError = vi.fn()
        const getValues = () => ({ email: '' })
        const handler = handleMutationErrorsWithForm({ setError, getValues })
        handler({
            response: {
                data: { data: { unknown_field: { code: 'x', message: 'bad' } } },
            },
        })
        expect(setError).not.toHaveBeenCalled()
        expect(useToastStore.getState().toasts).toHaveLength(1)
    })
})

// An atomic write saves rows of several collections, and two of them can have
// a field of the same name. A field error belongs on the form only when the
// failed row is of the collection the form edits.
describe('handleMutationErrorsWithForm with a failed atomic write', () => {
    beforeEach(() => {
        useToastStore.setState({ toasts: [] })
    })
    afterEach(() => {
        useToastStore.setState({ toasts: [] })
    })

    const memoTooLong = (collection: string) =>
        new BatchMutationError({
            index: 3,
            collection,
            recordId: 'rec123',
            response: {
                status: 400,
                message: 'Failed to create record.',
                data: { memo: { code: 'too_long', message: 'Must be at most 10 characters.' } },
            },
            cause: new Error('Batch transaction failed.'),
        })

    it('sets the field error when the failed row is of the form’s collection', () => {
        const setError = vi.fn()
        const handler = handleMutationErrorsWithForm({
            setError,
            getValues: () => ({ memo: '' }),
            collection: 'orders',
        })
        handler(memoTooLong('orders'))
        expect(setError).toHaveBeenCalledWith(
            'memo',
            expect.objectContaining({ message: 'Must be at most 10 characters.' })
        )
        expect(useToastStore.getState().toasts).toHaveLength(0)
    })

    it('shows a toast, not a field error, when the failed row is of another collection', () => {
        const setError = vi.fn()
        const handler = handleMutationErrorsWithForm({
            setError,
            getValues: () => ({ memo: '' }),
            collection: 'orders',
        })
        handler(memoTooLong('order_lines'))
        expect(setError).not.toHaveBeenCalled()
        const toasts = useToastStore.getState().toasts
        expect(toasts).toHaveLength(1)
        expect(toasts[0].body).toContain('memo: Must be at most 10 characters.')
    })

    it('shows a toast when the form does not say which collection it edits', () => {
        const setError = vi.fn()
        const handler = handleMutationErrorsWithForm({ setError, getValues: () => ({ memo: '' }) })
        handler(memoTooLong('orders'))
        expect(setError).not.toHaveBeenCalled()
        expect(useToastStore.getState().toasts).toHaveLength(1)
    })
})

describe('captureException', () => {
    it('captureException delegates to log.error', async () => {
        const spy = vi.fn()
        vi.doMock('../logger', () => ({
            log: { error: spy, debug: vi.fn(), info: vi.fn(), warn: vi.fn() },
        }))
        vi.resetModules()

        const { captureException } = await import('../errors')
        const err = new Error('boom')
        captureException('example.create', err, { id: '1' })

        expect(spy).toHaveBeenCalledWith('example.create', err, { id: '1' })
    })
})
