import AsyncStorage from '@react-native-async-storage/async-storage'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const unregisterExpoPushToken = vi.fn(async () => {})
const disconnectServer = vi.fn(async () => {})
const resetSessionState = vi.fn(async () => {})

vi.mock('../expo-push', () => ({ unregisterExpoPushToken }))
vi.mock('../pocketbase', () => ({
    pb: { realtime: { unsubscribe: vi.fn() }, cancelAllRequests: vi.fn() },
    disconnectServer,
    resetSessionState,
}))

const SERVER_A = 'https://a.example.com'
const SERVER_B = 'https://b.example.com'

beforeEach(async () => {
    await AsyncStorage.clear()
    vi.clearAllMocks()
    vi.resetModules()
})

async function seedServer(origin: string, userId: string) {
    const { setActiveServer } = await import('../servers')
    const { authKeyFor } = await import('../auth-storage')
    await setActiveServer(origin)
    await AsyncStorage.setItem(
        authKeyFor(origin),
        JSON.stringify({ token: `token-${userId}`, record: { id: userId } })
    )
}

describe('forgetServer', () => {
    it('drops a non-active servers entry and token, leaving the session alone', async () => {
        await seedServer(SERVER_A, 'user-a')
        await seedServer(SERVER_B, 'user-b')

        const { setResolvedAddress } = await import('../server-address')
        const { authKeyFor } = await import('../auth-storage')
        const { readServers } = await import('../servers')
        const { forgetServer } = await import('../remove-server')

        setResolvedAddress(SERVER_A)
        const outcome = await forgetServer(SERVER_B)

        expect(outcome).toEqual({ status: 'removed' })
        expect((await readServers()).map(s => s.origin)).toEqual([SERVER_A])
        expect(await AsyncStorage.getItem(authKeyFor(SERVER_B))).toBeNull()
        // A's session is untouched.
        expect(await AsyncStorage.getItem(authKeyFor(SERVER_A))).toContain('token-user-a')
        expect(disconnectServer).not.toHaveBeenCalled()
    })

    // The regression this guards: unregistering through the shared `pb` would
    // delete the row on whichever server is ACTIVE — signing the user out of
    // one they are still using while the removed one keeps pushing.
    it('cancels push against the removed servers own origin and token', async () => {
        await seedServer(SERVER_A, 'user-a')
        await seedServer(SERVER_B, 'user-b')

        const { setResolvedAddress } = await import('../server-address')
        const { forgetServer } = await import('../remove-server')

        setResolvedAddress(SERVER_A)
        await forgetServer(SERVER_B)

        expect(unregisterExpoPushToken).toHaveBeenCalledWith('user-b', 'token-user-b', SERVER_B)
    })

    it('removing the active server switches to another one', async () => {
        await seedServer(SERVER_A, 'user-a')
        await seedServer(SERVER_B, 'user-b')

        const { setResolvedAddress } = await import('../server-address')
        const { forgetServer } = await import('../remove-server')

        setResolvedAddress(SERVER_B)
        // The switch restarts the JS context, which this environment cannot do,
        // so the refusal propagates — but only AFTER the removal is committed.
        await expect(forgetServer(SERVER_B)).rejects.toThrow()

        const { readServers } = await import('../servers')
        expect((await readServers()).map(s => s.origin)).toEqual([SERVER_A])
        expect(await AsyncStorage.getItem('tinycld:server:app')).toBe(SERVER_A)
    })

    it('removing the last server disconnects and awaits session teardown', async () => {
        await seedServer(SERVER_A, 'user-a')

        const { setResolvedAddress } = await import('../server-address')
        const { authKeyFor } = await import('../auth-storage')
        const { readServers } = await import('../servers')
        const { forgetServer } = await import('../remove-server')

        setResolvedAddress(SERVER_A)
        const outcome = await forgetServer(SERVER_A)

        expect(outcome).toEqual({ status: 'disconnected' })
        expect(await readServers()).toEqual([])
        expect(await AsyncStorage.getItem(authKeyFor(SERVER_A))).toBeNull()
        expect(disconnectServer).toHaveBeenCalled()
        // Awaited here rather than left to race, unlike logout()'s fire-and-forget.
        expect(resetSessionState).toHaveBeenCalled()
    })

    it('tolerates a server with no stored session', async () => {
        const { setActiveServer } = await import('../servers')
        await setActiveServer(SERVER_A)
        await setActiveServer(SERVER_B)

        const { setResolvedAddress } = await import('../server-address')
        const { forgetServer } = await import('../remove-server')

        setResolvedAddress(SERVER_A)
        await expect(forgetServer(SERVER_B)).resolves.toEqual({ status: 'removed' })
        expect(unregisterExpoPushToken).not.toHaveBeenCalled()
    })
})
