// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// Mock PocketBase module so tests don't need a live server
const mockAuthStoreSave = vi.fn()
const mockGetOne = vi.fn()

vi.mock('@tinycld/core/lib/pocketbase', () => ({
    PB_SERVER_ADDR: 'http://localhost:8090',
    pb: {
        authStore: {
            save: mockAuthStoreSave,
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
    // zustand's create is curried: create<T>()(factory) — first call returns a
    // function that receives the factory. We implement a minimal in-memory store.
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
    // workspace-store wraps its factory in persist(...) — the mocked persist is
    // a pass-through that hands the factory back so the create() above can run
    // it unchanged. The asyncStorage export is referenced too; a no-op stub
    // satisfies the import without engaging any storage.
    persist: <T>(fn: T) => fn,
    asyncStorage: undefined,
}))

describe('auth-store OTP methods', () => {
    let requestShareOtp: (
        token: string,
        email: string
    ) => Promise<{ otpId: string | null; error: string | null }>
    let verifyShareOtp: (
        token: string,
        email: string,
        code: string,
        otpId: string
    ) => Promise<{ user: unknown; error: string | null }>

    beforeEach(async () => {
        vi.resetModules()
        // Re-import after resetting modules so mocks are fresh
        const { useAuthStore } = await import('@tinycld/core/lib/stores/auth-store')
        const state = useAuthStore.getState() as unknown as Record<string, unknown>
        requestShareOtp = state.requestShareOtp as typeof requestShareOtp
        verifyShareOtp = state.verifyShareOtp as typeof verifyShareOtp
    })

    afterEach(() => {
        vi.restoreAllMocks()
        mockAuthStoreSave.mockReset()
        mockGetOne.mockReset()
    })

    describe('requestShareOtp', () => {
        it('returns otpId on HTTP 200', async () => {
            vi.stubGlobal(
                'fetch',
                vi.fn().mockResolvedValue({
                    ok: true,
                    json: async () => ({ ok: true, otp_id: 'otp_abc123' }),
                    statusText: 'OK',
                })
            )

            const result = await requestShareOtp('share_tok', 'user@example.com')
            expect(result).toEqual({ otpId: 'otp_abc123', error: null })
        })

        it('surfaces server error message on HTTP 400', async () => {
            vi.stubGlobal(
                'fetch',
                vi.fn().mockResolvedValue({
                    ok: false,
                    status: 400,
                    statusText: 'Bad Request',
                    json: async () => ({ error: 'invalid email address' }),
                })
            )

            const result = await requestShareOtp('share_tok', 'not-an-email')
            expect(result).toEqual({ otpId: null, error: 'invalid email address' })
        })

        it('surfaces server message for viewer-only links', async () => {
            vi.stubGlobal(
                'fetch',
                vi.fn().mockResolvedValue({
                    ok: false,
                    status: 400,
                    statusText: 'Bad Request',
                    json: async () => ({ error: 'this link does not require sign-in' }),
                })
            )

            const result = await requestShareOtp('share_tok', 'user@example.com')
            expect(result).toEqual({ otpId: null, error: 'this link does not require sign-in' })
        })

        it('returns network error on fetch failure', async () => {
            vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network down')))

            const result = await requestShareOtp('share_tok', 'user@example.com')
            expect(result).toEqual({ otpId: null, error: 'network error' })
        })

        it('falls back to statusText when body has no error field', async () => {
            vi.stubGlobal(
                'fetch',
                vi.fn().mockResolvedValue({
                    ok: false,
                    status: 500,
                    statusText: 'Internal Server Error',
                    json: async () => ({}),
                })
            )

            const result = await requestShareOtp('share_tok', 'user@example.com')
            expect(result).toEqual({ otpId: null, error: 'Internal Server Error' })
        })
    })

    describe('verifyShareOtp', () => {
        const fakeRecord = {
            id: 'user_123',
            name: 'Guest User',
            email: 'guest@example.com',
        }
        const fakeOrgSlug = 'my-org'

        it('saves token to pb.authStore WITHOUT clearing first, returns user with primaryOrgSlug', async () => {
            vi.stubGlobal(
                'fetch',
                vi.fn().mockResolvedValue({
                    ok: true,
                    json: async () => ({ token: 'auth_tok_xyz', record: fakeRecord }),
                    statusText: 'OK',
                })
            )

            mockGetOne.mockResolvedValue({
                ...fakeRecord,
                expand: {
                    user_org_via_user: [
                        {
                            id: 'uo_1',
                            expand: { org: { id: 'org_1', slug: fakeOrgSlug } },
                        },
                    ],
                },
            })

            const result = await verifyShareOtp(
                'share_tok',
                'guest@example.com',
                '123456',
                'otp_abc'
            )

            // pb.authStore.save called with token + record (no clear)
            expect(mockAuthStoreSave).toHaveBeenCalledWith('auth_tok_xyz', fakeRecord)
            expect(mockAuthStoreSave).toHaveBeenCalledTimes(1)

            expect(result.error).toBeNull()
            expect(result.user).toMatchObject({
                id: 'user_123',
                email: 'guest@example.com',
                primaryOrgSlug: fakeOrgSlug,
                isDemo: false,
                isBetaTester: false,
            })
        })

        it('returns error on HTTP 400', async () => {
            vi.stubGlobal(
                'fetch',
                vi.fn().mockResolvedValue({
                    ok: false,
                    status: 400,
                    statusText: 'Bad Request',
                    json: async () => ({ error: 'invalid or expired code' }),
                })
            )

            const result = await verifyShareOtp(
                'share_tok',
                'guest@example.com',
                'badcode',
                'otp_abc'
            )
            expect(result).toEqual({ user: null, error: 'invalid or expired code' })
            expect(mockAuthStoreSave).not.toHaveBeenCalled()
        })
    })
})
