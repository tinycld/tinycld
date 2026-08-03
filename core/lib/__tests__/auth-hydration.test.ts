import AsyncStorage from '@react-native-async-storage/async-storage'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// Boot-ordering tests for the auth hydration in pocketbase.ts (step 1a). These
// exercise the module's real initialization, so each case resets modules and
// re-imports to simulate a fresh launch.
beforeEach(async () => {
    await AsyncStorage.clear()
    vi.resetModules()
    vi.useRealTimers()
})

const SERVER_A = 'https://a.example.com'
const SERVER_B = 'https://b.example.com'

const blobFor = (id: string) => JSON.stringify({ token: `token-${id}`, record: { id } })

describe('auth hydration', () => {
    it('hydrates from the ACTIVE servers scoped blob', async () => {
        const { authKeyFor } = await import('../auth-storage')
        await AsyncStorage.setItem(authKeyFor(SERVER_A), blobFor('user-a'))
        await AsyncStorage.setItem(authKeyFor(SERVER_B), blobFor('user-b'))

        const { setResolvedAddress } = await import('../server-address')
        setResolvedAddress(SERVER_B)

        const { authStoreReady, pb } = await import('../pocketbase')
        await authStoreReady

        // B's session, not A's — the invariant of per-server auth.
        expect(pb.authStore.token).toBe('token-user-b')
    })

    // Hydration now depends on the resolved address, so it must WAIT for one
    // rather than racing it — the old setTimeout(0) could fire pre-gate.
    it('waits for an address resolved after module init', async () => {
        const { authKeyFor } = await import('../auth-storage')
        await AsyncStorage.setItem(authKeyFor(SERVER_A), blobFor('user-a'))

        // Import BEFORE any address exists, as a real cold boot does.
        const { authStoreReady, pb } = await import('../pocketbase')
        const { setResolvedAddress } = await import('../server-address')

        setResolvedAddress(SERVER_A)
        await authStoreReady

        expect(pb.authStore.token).toBe('token-user-a')
    })

    // A fresh install sits on /connect with no address. authStoreReady must
    // still SETTLE — auth-store awaits it before setting hasHydrated, so a
    // promise that never resolves strands the app on a spinner.
    it('settles rather than hanging when no address ever resolves', async () => {
        vi.useFakeTimers()
        const { authStoreReady, pb } = await import('../pocketbase')

        await vi.advanceTimersByTimeAsync(11_000)
        await expect(authStoreReady).resolves.toBeUndefined()
        expect(pb.authStore.token).toBeFalsy()
    })

    it('migrates a legacy flat blob for the active server on first run', async () => {
        await AsyncStorage.setItem('pb_auth', blobFor('legacy-user'))

        const { setResolvedAddress } = await import('../server-address')
        setResolvedAddress(SERVER_A)

        const { authStoreReady, pb } = await import('../pocketbase')
        await authStoreReady

        // The upgrade path: an existing user is NOT signed out.
        expect(pb.authStore.token).toBe('token-legacy-user')

        const { authKeyFor } = await import('../auth-storage')
        expect(await AsyncStorage.getItem(authKeyFor(SERVER_A))).toBe(blobFor('legacy-user'))
        expect(await AsyncStorage.getItem('pb_auth')).toBeNull()
    })

    it('recovers from a corrupt blob by clearing it instead of throwing', async () => {
        const { authKeyFor } = await import('../auth-storage')
        await AsyncStorage.setItem(authKeyFor(SERVER_A), '{not json')

        const { setResolvedAddress } = await import('../server-address')
        setResolvedAddress(SERVER_A)

        const { authStoreReady, pb } = await import('../pocketbase')
        await expect(authStoreReady).resolves.toBeUndefined()

        expect(pb.authStore.token).toBeFalsy()
        expect(await AsyncStorage.getItem(authKeyFor(SERVER_A))).toBeNull()
    })

    it('writes saves to the key it hydrated from', async () => {
        const { setResolvedAddress } = await import('../server-address')
        setResolvedAddress(SERVER_A)

        const { authStoreReady, pb } = await import('../pocketbase')
        await authStoreReady

        pb.authStore.save('fresh-token', { id: 'user-a' } as never)
        // AsyncAuthStore.save is fire-and-forget against storage.
        await vi.waitFor(async () => {
            const { authKeyFor } = await import('../auth-storage')
            expect(await AsyncStorage.getItem(authKeyFor(SERVER_A))).toContain('fresh-token')
        })
    })
})
