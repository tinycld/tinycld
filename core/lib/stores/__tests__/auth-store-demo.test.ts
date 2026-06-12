// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const mockAuthStoreSave = vi.fn()
const mockAuthStoreClear = vi.fn()
const mockGetOne = vi.fn()

vi.mock('@tinycld/core/lib/pocketbase', () => ({
    PB_SERVER_ADDR: 'http://localhost:8090',
    pb: {
        authStore: {
            save: mockAuthStoreSave,
            clear: mockAuthStoreClear,
            onChange: vi.fn(() => () => {}),
            token: null as string | null,
            record: null,
        },
        collection: vi.fn(() => ({
            getOne: mockGetOne,
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

vi.mock('@tinycld/core/lib/errors', () => ({
    captureException: vi.fn(),
}))

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

describe('auth-store startDemo', () => {
    const demoServer = 'https://tinycld.org'
    const fakeRecord = {
        id: 'user_demo',
        name: 'Demo Tour',
        email: 'demo@tinycld.org',
    }

    let startDemo: (serverAddr: string) => Promise<{ user: unknown; error: string | null }>

    beforeEach(async () => {
        vi.resetModules()
        const { useAuthStore } = await import('@tinycld/core/lib/stores/auth-store')
        const state = useAuthStore.getState() as unknown as Record<string, unknown>
        startDemo = state.startDemo as typeof startDemo
    })

    afterEach(() => {
        vi.restoreAllMocks()
        mockAuthStoreSave.mockReset()
        mockAuthStoreClear.mockReset()
        mockGetOne.mockReset()
    })

    it('adopts the demo envelope and pins the demo org', async () => {
        vi.stubGlobal(
            'fetch',
            vi.fn().mockResolvedValue({
                ok: true,
                json: async () => ({ token: 'demo_tok', record: fakeRecord }),
                statusText: 'OK',
            })
        )
        mockGetOne.mockResolvedValue({
            ...fakeRecord,
            expand: {
                user_org_via_user: [
                    { id: 'uo_demo', expand: { org: { id: 'org_demo', slug: 'demo' } } },
                ],
            },
        })

        const result = await startDemo(demoServer)

        expect(mockAuthStoreSave).toHaveBeenCalledWith('demo_tok', fakeRecord)
        expect(result.error).toBeNull()
        expect(result.user).toMatchObject({
            id: 'user_demo',
            email: 'demo@tinycld.org',
            primaryOrgSlug: 'demo',
            isDemo: true,
        })
    })

    it('hits /api/demo/start on the provided server', async () => {
        const fetchMock = vi.fn().mockResolvedValue({
            ok: true,
            json: async () => ({ token: 'demo_tok', record: fakeRecord }),
            statusText: 'OK',
        })
        vi.stubGlobal('fetch', fetchMock)
        mockGetOne.mockResolvedValue({
            ...fakeRecord,
            expand: {
                user_org_via_user: [
                    { id: 'uo_demo', expand: { org: { id: 'org_demo', slug: 'demo' } } },
                ],
            },
        })

        await startDemo(demoServer)

        expect(fetchMock).toHaveBeenCalledWith(
            'https://tinycld.org/api/demo/start',
            expect.objectContaining({ method: 'POST' })
        )
    })

    it('returns an error and does not save auth on HTTP failure', async () => {
        vi.stubGlobal(
            'fetch',
            vi.fn().mockResolvedValue({
                ok: false,
                status: 503,
                statusText: 'Service Unavailable',
                json: async () => ({}),
            })
        )

        const result = await startDemo(demoServer)

        expect(result.user).toBeNull()
        expect(result.error).toBe('Server returned HTTP 503')
        expect(mockAuthStoreSave).not.toHaveBeenCalled()
    })

    it('errors and clears auth when the demo org is missing from the membership', async () => {
        vi.stubGlobal(
            'fetch',
            vi.fn().mockResolvedValue({
                ok: true,
                json: async () => ({ token: 'demo_tok', record: fakeRecord }),
                statusText: 'OK',
            })
        )
        mockGetOne.mockResolvedValue({ ...fakeRecord, expand: { user_org_via_user: [] } })

        const result = await startDemo(demoServer)

        expect(result.user).toBeNull()
        expect(result.error).toBe('Demo workspace is unavailable')
        expect(mockAuthStoreClear).toHaveBeenCalled()
    })

    it('rejects a malformed envelope', async () => {
        vi.stubGlobal(
            'fetch',
            vi.fn().mockResolvedValue({
                ok: true,
                json: async () => ({ token: 'demo_tok' }),
                statusText: 'OK',
            })
        )

        const result = await startDemo(demoServer)

        expect(result.user).toBeNull()
        expect(result.error).toBe('Malformed demo response')
        expect(mockAuthStoreSave).not.toHaveBeenCalled()
    })
})
