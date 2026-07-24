// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// Mock PocketBase so login() runs without a live server. authWithPassword's
// return shape drives the authenticated-user projection under test. Multi-org
// is removed, so login() no longer resolves an org / primaryOrgSlug and never
// rejects a user for "no org" — any valid credentials sign in.
const mockAuthStoreClear = vi.fn()
const mockAuthWithPassword = vi.fn()

vi.mock('@tinycld/core/lib/pocketbase', () => ({
    PB_SERVER_ADDR: 'http://localhost:8090',
    pb: {
        authStore: {
            save: vi.fn(),
            clear: mockAuthStoreClear,
            onChange: vi.fn(() => () => {}),
            token: null as string | null,
            record: null,
        },
        collection: vi.fn(() => ({
            authWithPassword: mockAuthWithPassword,
        })),
    },
    authStoreReady: Promise.resolve(),
    getUserFromAuthStore: vi.fn(() => null),
    seedUser: vi.fn(() => Promise.resolve()),
    preloadStores: vi.fn(() => Promise.resolve()),
    resetSessionState: vi.fn(() => Promise.resolve()),
    refreshAuth: vi.fn(() => Promise.resolve(false)),
}))

vi.mock('@react-native-async-storage/async-storage', () => ({
    default: {
        getItem: vi.fn(() => Promise.resolve(null)),
        setItem: vi.fn(() => Promise.resolve()),
        removeItem: vi.fn(() => Promise.resolve()),
    },
}))

vi.mock('@tinycld/core/lib/errors', () => ({ captureException: vi.fn() }))

vi.mock('@tinycld/core/lib/store', () => ({
    create: () => (fn: unknown) => {
        let state = {} as Record<string, unknown>
        const set = (patch: Record<string, unknown>) => {
            state = { ...state, ...patch }
        }
        const get = () => state
        type Factory = (
            set: (patch: Record<string, unknown>) => void,
            get: () => Record<string, unknown>
        ) => Record<string, unknown>
        const methods = (fn as Factory)(set, get)
        state = { ...state, ...methods }
        const store = (selector: (s: typeof state) => unknown) => selector(state)
        store.getState = () => state
        store.setState = (patch: Record<string, unknown>) => {
            state = { ...state, ...patch }
        }
        return store
    },
    persist: <T>(fn: T) => fn,
    asyncStorage: undefined,
}))

const REGULAR_USER = {
    record: { id: 'u1', name: 'Owner', email: 'owner@example.com', role: 'owner' },
}
const DEMO_USER = {
    record: { id: 'u2', name: 'Demo', email: 'demo@example.com', is_demo: true },
}

describe('auth-store login()', () => {
    let login: ReturnType<typeof import('../auth-store').useAuthStore.getState>['login']

    beforeEach(async () => {
        vi.resetModules()
        mockAuthStoreClear.mockClear()
        mockAuthWithPassword.mockReset()
        const mod = await import('../auth-store')
        login = mod.useAuthStore.getState().login
    })

    afterEach(() => vi.clearAllMocks())

    it('signs in a user and returns the authenticated user projection', async () => {
        mockAuthWithPassword.mockResolvedValue(REGULAR_USER)
        const res = await login('owner@example.com', 'pw')
        expect(res.error).toBeNull()
        expect(res.user?.id).toBe('u1')
        expect(res.user?.email).toBe('owner@example.com')
        expect(res.user?.isDemo).toBe(false)
    })

    it('flags a demo user via the is_demo record field', async () => {
        mockAuthWithPassword.mockResolvedValue(DEMO_USER)
        const res = await login('demo@example.com', 'pw')
        expect(res.error).toBeNull()
        expect(res.user?.isDemo).toBe(true)
    })

    it('returns an error and no user when credentials are rejected', async () => {
        mockAuthWithPassword.mockRejectedValue(new Error('Failed to authenticate.'))
        const res = await login('nobody@example.com', 'wrong')
        expect(res.user).toBeNull()
        expect(res.error).toBe('Failed to authenticate.')
        // login() clears the session once at the start of every attempt.
        expect(mockAuthStoreClear).toHaveBeenCalledTimes(1)
    })
})
