import AsyncStorage from '@react-native-async-storage/async-storage'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// pocketbase.ts pulls in the generated app config and the whole collection
// graph, none of which this module's behaviour depends on — it only needs
// realtime teardown to be callable.
vi.mock('../pocketbase', () => ({
    pb: {
        realtime: { unsubscribe: vi.fn() },
        cancelAllRequests: vi.fn(),
    },
}))

beforeEach(async () => {
    await AsyncStorage.clear()
    vi.resetModules()
})

describe('switchToServer', () => {
    // The invariant the whole design rests on. In this environment there is no
    // native reload, so the switch MUST refuse — the alternative (resolving
    // while doing nothing) leaves `pb` on the new server with every collection
    // still bound to the old one.
    it('refuses when the JS context cannot be restarted', async () => {
        const { ReloadUnavailableError } = await import('../reload-js-context')
        const { switchToServer } = await import('../switch-server')

        await expect(switchToServer('https://b.example.com')).rejects.toBeInstanceOf(
            ReloadUnavailableError
        )
    })

    // A refused switch must not leave the running context pointing somewhere its
    // module graph is not bound to.
    it('restores the previous address when the reload refuses', async () => {
        const { setResolvedAddress, getResolvedAddress } = await import('../server-address')
        const { switchToServer } = await import('../switch-server')

        setResolvedAddress('https://a.example.com')
        await expect(switchToServer('https://b.example.com')).rejects.toThrow()

        expect(getResolvedAddress()).toBe('https://a.example.com')
    })

    // ...but the PERSISTED pointer is deliberately left on the target, so a
    // manual restart completes the switch through the normal launch path.
    it('persists the target as active even when the reload refuses', async () => {
        const { setResolvedAddress } = await import('../server-address')
        const { switchToServer } = await import('../switch-server')

        setResolvedAddress('https://a.example.com')
        await expect(switchToServer('https://b.example.com')).rejects.toThrow()

        expect(await AsyncStorage.getItem('tinycld:server:app')).toBe('https://b.example.com')
        const saved = JSON.parse((await AsyncStorage.getItem('tinycld:servers')) ?? '[]')
        expect(saved.map((s: { origin: string }) => s.origin)).toContain('https://b.example.com')
    })

    it('tears down the old servers realtime stream before repointing', async () => {
        const { pb } = await import('../pocketbase')
        const { setResolvedAddress } = await import('../server-address')
        const { switchToServer } = await import('../switch-server')

        setResolvedAddress('https://a.example.com')
        await expect(switchToServer('https://b.example.com')).rejects.toThrow()

        expect(pb.realtime.unsubscribe).toHaveBeenCalled()
        expect(pb.cancelAllRequests).toHaveBeenCalled()
    })

    describe('per-server state', () => {
        // lastPackageHref holds deep-links to record ids on the OTHER server;
        // logout() wipes it for exactly this reason, and a switch never logs out.
        it('clears state that would be read back against the wrong server', async () => {
            await AsyncStorage.setItem('tinycld_sidebar_open', '{"lastPackageHref":{"mail":"/x"}}')
            await AsyncStorage.setItem('tinycld:anon-id', 'anon-1')
            await AsyncStorage.setItem('firstRun:mail', 'done')

            const { switchToServer } = await import('../switch-server')
            await expect(switchToServer('https://b.example.com')).rejects.toThrow()

            expect(await AsyncStorage.getItem('tinycld_sidebar_open')).toBeNull()
            expect(await AsyncStorage.getItem('tinycld:anon-id')).toBeNull()
            expect(await AsyncStorage.getItem('firstRun:mail')).toBeNull()
        })

        it('leaves other servers auth blobs alone', async () => {
            const { authKeyFor } = await import('../auth-storage')
            await AsyncStorage.setItem(authKeyFor('https://a.example.com'), 'blob-a')

            const { switchToServer } = await import('../switch-server')
            await expect(switchToServer('https://b.example.com')).rejects.toThrow()

            // The whole point: switching away does not sign you out of A.
            expect(await AsyncStorage.getItem(authKeyFor('https://a.example.com'))).toBe('blob-a')
        })
    })
})
