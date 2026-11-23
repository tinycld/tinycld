// @vitest-environment happy-dom

// useUserPreference reads one user_preferences row and writes it back. These
// tests pin what a screen needs to build its own states on the hook: the read
// state (ready, failed, retry), the two write guards (no write before the read
// is ready; the row is found inside the mutation), and who reports a failed
// save (core's default toast, or the caller's onSaveError).

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { useToastStore } from '@tinycld/core/lib/stores/toast-store'
import { createCollection, disconnectRealtime } from 'pbtsdb/core'
import PocketBase, { BaseAuthStore } from 'pocketbase'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

interface PrefRow {
    id: string
    app: string
    key: string
    user: string
    value: unknown
}

type Write = { kind: 'create' | 'update'; id: string; data: Record<string, unknown> }

const h = vi.hoisted(() => ({
    user: null as { id: string } | null,
    collection: undefined as unknown,
}))

vi.mock('@tinycld/core/lib/auth', () => ({
    useAuth: () => ({ user: h.user, isLoggedIn: h.user !== null }),
}))
vi.mock('@tinycld/core/lib/pocketbase', () => ({
    useStore: () => [h.collection],
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

import { useUserPreference } from '../use-user-preference'

// pbtsdb reports a load error only for a 4xx other than 401/408/429; any other
// failure is retried forever. 400 is what a refused read looks like.
const refused = (message: string) => Object.assign(new Error(message), { status: 400 })

/**
 * A real pbtsdb collection over a PocketBase client whose record service
 * answers from `rows`. Each read and each write goes through a hook the test
 * can hold or fail, so the hook under test sees the server's real outcomes.
 */
function makePreferences(rows: PrefRow[]) {
    const writes: Write[] = []
    const server = {
        // Each read waits on this; a test replaces it to hold or fail a read.
        read: async (): Promise<void> => undefined,
        write: async (_write: Write): Promise<void> => undefined,
    }
    const matching = (filter: string | undefined) =>
        rows.filter(
            row =>
                !filter ||
                (filter.includes(`app = "${row.app}"`) &&
                    filter.includes(`key = "${row.key}"`) &&
                    filter.includes(`user = "${row.user}"`))
        )
    const save = async (write: Write) => {
        writes.push(write)
        await server.write(write)
        const existing = rows.find(row => row.id === write.id)
        const next = { ...existing, ...write.data, id: write.id } as PrefRow
        if (existing) Object.assign(existing, next)
        else rows.push(next)
        return next
    }
    const service = {
        getFullList: async (opts?: { filter?: string }) => {
            await server.read()
            return matching(opts?.filter)
        },
        getList: async (_page: number, _perPage: number, opts?: { filter?: string }) => {
            await server.read()
            const items = matching(opts?.filter)
            return { items, totalItems: items.length, totalPages: 1, page: 1, perPage: 500 }
        },
        create: (data: Record<string, unknown>) =>
            save({ kind: 'create', id: String(data.id), data }),
        update: (id: string, data: Record<string, unknown>) => save({ kind: 'update', id, data }),
    }
    const pb = new PocketBase('http://127.0.0.1:9', new BaseAuthStore())
    Object.defineProperty(pb, 'collection', { value: () => service })
    disconnectRealtime(pb)
    const collection = createCollection<{ user_preferences: { type: PrefRow } }>(pb)(
        'user_preferences',
        { syncMode: 'on-demand', realtime: 'query' }
    )
    h.collection = collection
    return { server, writes }
}

function wrapper() {
    const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
    return ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
    )
}

function renderPreference(options?: Parameters<typeof useUserPreference<string>>[3]) {
    return renderHook(() => useUserPreference('books', 'dashboard', 'default', options), {
        wrapper: wrapper(),
    })
}

const savedRow = (value: string): PrefRow => ({
    id: 'pref000000000a1',
    app: 'books',
    key: 'dashboard',
    user: 'u1',
    value,
})

// TanStack DB reports a failed live query with console.error; the read-error
// test causes one on purpose.
function silenceConsoleErrors() {
    return vi.spyOn(console, 'error').mockImplementation(() => undefined)
}

beforeEach(() => {
    h.user = { id: 'u1' }
    useToastStore.setState({ toasts: [] })
})

afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
})

describe('useUserPreference read state', () => {
    it('is not ready while the read is in flight, and writes nothing until it lands', async () => {
        const { server, writes } = makePreferences([savedRow('saved')])
        let release!: () => void
        server.read = () =>
            new Promise<void>(resolve => {
                release = resolve
            })

        const { result } = renderPreference()

        expect(result.current[2]).toMatchObject({ isReady: false, isError: false })
        expect(result.current[0]).toBe('default')
        act(() => result.current[1]('too early'))
        expect(writes).toEqual([])

        await waitFor(() => expect(release).toBeTypeOf('function'))
        await act(async () => release())
        await waitFor(() => expect(result.current[2].isReady).toBe(true))
        expect(result.current[0]).toBe('saved')
        expect(writes).toEqual([])
    })

    it('reports a failed read, refuses writes, and recovers on retry', async () => {
        const consoleError = silenceConsoleErrors()
        const { server, writes } = makePreferences([savedRow('saved')])
        server.read = async () => {
            throw refused('read refused')
        }

        const { result } = renderPreference()

        await waitFor(() => expect(result.current[2].isError).toBe(true))
        expect(result.current[2].isReady).toBe(false)
        expect(result.current[0]).toBe('default')
        act(() => result.current[1]('while failed'))
        expect(writes).toEqual([])
        expect(consoleError).toHaveBeenCalled()

        server.read = async () => undefined
        await act(() => result.current[2].retry())

        await waitFor(() => expect(result.current[2].isReady).toBe(true))
        expect(result.current[2].isError).toBe(false)
        expect(result.current[0]).toBe('saved')
    })

    it('is not ready and writes nothing while signed out', () => {
        h.user = null
        const { writes } = makePreferences([])

        const { result } = renderPreference()

        expect(result.current[2]).toMatchObject({ isReady: false, isError: false })
        act(() => result.current[1]('anon'))
        expect(writes).toEqual([])
    })
})

describe('useUserPreference writes', () => {
    it('updates the row it finds inside the mutation', async () => {
        const { writes } = makePreferences([savedRow('saved')])
        const { result } = renderPreference()
        await waitFor(() => expect(result.current[0]).toBe('saved'))

        act(() => result.current[1]('changed'))

        await waitFor(() => expect(writes).toHaveLength(1))
        expect(writes[0]).toMatchObject({ kind: 'update', id: 'pref000000000a1' })
        expect(writes[0].data).toMatchObject({ value: 'changed' })
        expect(result.current[0]).toBe('changed')
    })

    it('inserts a row when the loaded read found none', async () => {
        const { writes } = makePreferences([])
        const { result } = renderPreference()
        await waitFor(() => expect(result.current[2].isReady).toBe(true))

        act(() => result.current[1]('first'))

        await waitFor(() => expect(writes).toHaveLength(1))
        expect(writes[0].kind).toBe('create')
        expect(writes[0].data).toMatchObject({
            app: 'books',
            key: 'dashboard',
            user: 'u1',
            value: 'first',
        })
    })

    it("shows core's toast for a failed save when no onSaveError is given", async () => {
        const { server } = makePreferences([savedRow('saved')])
        server.write = async () => {
            throw refused('write refused')
        }
        const { result } = renderPreference()
        await waitFor(() => expect(result.current[0]).toBe('saved'))

        act(() => result.current[1]('lost'))

        await waitFor(() => expect(useToastStore.getState().toasts).toHaveLength(1))
        expect(useToastStore.getState().toasts[0]).toMatchObject({
            title: 'Something went wrong',
            variant: 'error',
        })
        await waitFor(() => expect(result.current[0]).toBe('saved'))
    })

    it('hands a failed save to onSaveError instead of the default toast', async () => {
        const { server } = makePreferences([savedRow('saved')])
        server.write = async () => {
            throw refused('write refused')
        }
        const onSaveError = vi.fn((_error: Error) => undefined)
        const { result } = renderPreference({ onSaveError })
        await waitFor(() => expect(result.current[0]).toBe('saved'))

        act(() => result.current[1]('lost'))

        await waitFor(() => expect(onSaveError).toHaveBeenCalledTimes(1))
        expect(onSaveError.mock.calls[0][0].message).toContain('write refused')
        await waitFor(() => expect(result.current[0]).toBe('saved'))
        expect(useToastStore.getState().toasts).toEqual([])
    })
})
