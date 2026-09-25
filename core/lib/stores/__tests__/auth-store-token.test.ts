// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const fullRecord = { id: 'user_owner', name: 'Dana Reyes', email: 'dana@example.com' }
const authStore = {
    save: vi.fn(),
    clear: vi.fn(),
    onChange: vi.fn(() => () => {}),
    token: null as string | null,
    record: null as unknown,
}
const calls: string[] = []
const mockRefreshAuth = vi.fn()
const mockGetUser = vi.fn()
const mockSeedUser = vi.fn()
const mockPreloadStores = vi.fn()
const mockLogError = vi.fn()

vi.mock('@tinycld/core/lib/pocketbase', () => ({
    PB_SERVER_ADDR: 'http://localhost:8090',
    pb: { authStore, collection: vi.fn() },
    authStoreReady: Promise.resolve(),
    getUserFromAuthStore: () => mockGetUser(),
    seedUser: (record: unknown) => mockSeedUser(record),
    preloadStores: () => mockPreloadStores(),
    resetSessionState: vi.fn(() => Promise.resolve()),
    refreshAuth: () => mockRefreshAuth(),
}))

vi.mock('@tinycld/core/lib/errors', () => ({ captureException: vi.fn() }))

vi.mock('@tinycld/core/lib/logger', () => ({
    log: { error: mockLogError, warn: vi.fn(), info: vi.fn(), debug: vi.fn() },
}))

vi.mock('@tinycld/core/lib/store', () => ({
    create: () => (fn: unknown) => {
        let state = {} as Record<string, unknown>
        const set = (patch: Record<string, unknown>) => {
            state = { ...state, ...patch }
        }
        type Factory = (
            set: (patch: Record<string, unknown>) => void,
            get: () => Record<string, unknown>
        ) => Record<string, unknown>
        state = { ...state, ...(fn as Factory)(set, () => state) }
        const store = (selector: (s: typeof state) => unknown) => selector(state)
        store.getState = () => state
        store.setState = set
        return store
    },
    persist: <T>(fn: T) => fn,
    asyncStorage: undefined,
}))

type SignIn = (
    token: string,
    identity: { id: string; email: string }
) => Promise<{ user: unknown; error: string | null }>

describe('auth-store signInWithToken', () => {
    let signInWithToken: SignIn

    beforeEach(async () => {
        vi.resetModules()
        calls.length = 0
        mockRefreshAuth.mockImplementation(async () => {
            calls.push('refreshAuth')
            authStore.record = fullRecord
            return true
        })
        mockGetUser.mockImplementation(() => ({
            ...fullRecord,
            isDemo: false,
            isBetaTester: false,
        }))
        mockSeedUser.mockImplementation(async () => {
            calls.push('seedUser')
        })
        mockPreloadStores.mockImplementation(async () => {
            calls.push('preloadStores')
        })
        const { useAuthStore } = await import('@tinycld/core/lib/stores/auth-store')
        const state = useAuthStore.getState() as unknown as Record<string, unknown>
        signInWithToken = state.signInWithToken as SignIn
    })

    afterEach(() => {
        vi.clearAllMocks()
        authStore.record = null
    })

    it('saves the token, loads the full record, then refetches the stores', async () => {
        const result = await signInWithToken('tok', { id: 'user_owner', email: 'dana@example.com' })

        expect(authStore.save).toHaveBeenCalledWith(
            'tok',
            expect.objectContaining({ id: 'user_owner', collectionName: 'users' })
        )
        expect(calls).toEqual(['refreshAuth', 'seedUser', 'preloadStores'])
        expect(mockSeedUser).toHaveBeenCalledWith(fullRecord)
        expect(result).toMatchObject({ user: { id: 'user_owner' }, error: null })
    })

    it('reports an error when the token does not give a session', async () => {
        mockRefreshAuth.mockResolvedValue(false)
        mockGetUser.mockReturnValue(null)

        const result = await signInWithToken('dead', { id: 'x', email: 'x@example.com' })

        expect(result.user).toBeNull()
        expect(result.error).toBeTruthy()
        expect(mockPreloadStores).not.toHaveBeenCalled()
    })

    // The owner exists by now, so a failure here strands the person on the
    // sign-in screen; it must reach the log to be diagnosable.
    it('logs a failure to adopt the token', async () => {
        const failure = new Error('network down')
        mockRefreshAuth.mockRejectedValue(failure)

        const result = await signInWithToken('tok', { id: 'x', email: 'x@example.com' })

        expect(result.error).toBe('network down')
        expect(mockLogError).toHaveBeenCalledWith('core.setup', failure)
    })
})
