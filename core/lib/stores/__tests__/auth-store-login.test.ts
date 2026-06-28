// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// Mock PocketBase so login() runs without a live server. authWithPassword's
// return shape drives the org-resolution branch under test.
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
    fetchAndSeedUserOrg: vi.fn(() => Promise.resolve()),
    preloadStores: vi.fn(() => Promise.resolve()),
    seedUserOrg: vi.fn(() => Promise.resolve()),
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

const ORG_USER = {
    record: {
        id: 'u1',
        name: 'Owner',
        email: 'owner@example.com',
        expand: {
            user_org_via_user: [{ id: 'uo1', expand: { org: { id: 'o1', slug: 'acme' } } }],
        },
    },
}
const SUPER_ADMIN_NO_ORG = {
    record: {
        id: 'admin1',
        name: 'admin',
        email: 'admin@tinycld.org',
        expand: { super_admins_via_user: { id: 'sa1' } }, // present → super admin; no user_org
    },
}
const REGULAR_NO_ORG = {
    record: { id: 'u2', name: 'Nobody', email: 'nobody@example.com', expand: {} },
}

describe('auth-store login() org resolution', () => {
    // biome-ignore lint/suspicious/noExplicitAny: store login signature is broad here
    let login: (id: string, pw: string) => Promise<{ user: any; error: string | null }>

    beforeEach(async () => {
        vi.resetModules()
        mockAuthStoreClear.mockClear()
        mockAuthWithPassword.mockReset()
        const mod = await import('../auth-store')
        // biome-ignore lint/suspicious/noExplicitAny: test harness reaches the store action
        login = (mod as any).useAuthStore.getState().login
    })

    afterEach(() => vi.clearAllMocks())

    it('signs in a user with an org and returns its slug', async () => {
        mockAuthWithPassword.mockResolvedValue(ORG_USER)
        const res = await login('owner@example.com', 'pw')
        expect(res.error).toBeNull()
        expect(res.user?.primaryOrgSlug).toBe('acme')
    })

    // login() clears auth once at the start of every attempt; the meaningful
    // difference is whether the no-org branch clears AGAIN (rejecting) or keeps
    // the freshly-authed session (super admin).
    it('signs in a super admin with NO org: no error, no slug, session kept', async () => {
        mockAuthWithPassword.mockResolvedValue(SUPER_ADMIN_NO_ORG)
        const res = await login('admin@tinycld.org', 'pw')
        expect(res.error).toBeNull()
        expect(res.user).not.toBeNull()
        expect(res.user?.primaryOrgSlug).toBeUndefined()
        // Only the start-of-login clear — the session is NOT torn down.
        expect(mockAuthStoreClear).toHaveBeenCalledTimes(1)
    })

    it('rejects a regular user with no org and clears the session', async () => {
        mockAuthWithPassword.mockResolvedValue(REGULAR_NO_ORG)
        const res = await login('nobody@example.com', 'pw')
        expect(res.user).toBeNull()
        expect(res.error).toBe('No organization associated with this account')
        // Start-of-login clear + the rejection clear.
        expect(mockAuthStoreClear).toHaveBeenCalledTimes(2)
    })
})
