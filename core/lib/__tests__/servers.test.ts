import AsyncStorage from '@react-native-async-storage/async-storage'
import { beforeEach, describe, expect, it } from 'vitest'
import { serverKeyFor } from '../app-updater/server-key'
import {
    addServer,
    MAX_SAVED_SERVERS,
    readServers,
    removeServer,
    setActiveServer,
} from '../servers'

// The stub's backing store is anchored to globalThis so it survives module
// resets; clear it explicitly between tests.
beforeEach(async () => {
    await AsyncStorage.clear()
})

async function origins(): Promise<string[]> {
    return (await readServers()).map(s => s.origin)
}

describe('saved server list', () => {
    it('adds a server and labels it with the hostname', async () => {
        await addServer('https://acme.tinycld.org')
        const [saved] = await readServers()
        expect(saved.origin).toBe('https://acme.tinycld.org')
        expect(saved.label).toBe('acme.tinycld.org')
    })

    // The plan's explicit requirement: typing a bare hostname must work, via the
    // same normalizeAddress the connect screens use.
    it('adds https:// to a bare hostname', async () => {
        const origin = await addServer('acme.tinycld.org')
        expect(origin).toBe('https://acme.tinycld.org')
    })

    it('strips a trailing slash', async () => {
        expect(await addServer('https://acme.tinycld.org/')).toBe('https://acme.tinycld.org')
    })

    it('de-duplicates spelling variants of one server via serverKeyFor', async () => {
        await addServer('acme.tinycld.org')
        await addServer('https://acme.tinycld.org')
        await addServer('https://acme.tinycld.org/')
        await addServer('https://ACME.tinycld.org')

        expect(await origins()).toEqual(['https://acme.tinycld.org'])
    })

    it('keeps distinct servers apart', async () => {
        await addServer('https://a.example.com')
        await addServer('https://b.example.com')
        expect(await origins()).toEqual(['https://a.example.com', 'https://b.example.com'])
    })

    // Two orgs on the hosted router are two subdomains — the same object to this
    // feature as two self-hosted boxes.
    it('treats two org subdomains as two servers', async () => {
        await addServer('acme.tinycld.org')
        await addServer('globex.tinycld.org')
        expect(await origins()).toHaveLength(2)
        expect(serverKeyFor('acme.tinycld.org')).not.toBe(serverKeyFor('globex.tinycld.org'))
    })

    it('caps the list, trimming oldest-first', async () => {
        for (let i = 0; i < MAX_SAVED_SERVERS + 3; i++) {
            await addServer(`https://s${i}.example.com`)
        }
        const saved = await origins()
        expect(saved).toHaveLength(MAX_SAVED_SERVERS)
        expect(saved[0]).toBe('https://s3.example.com')
        expect(saved.at(-1)).toBe(`https://s${MAX_SAVED_SERVERS + 2}.example.com`)
    })

    // A user can sit on one server for a long time while adding others, which
    // makes the ACTIVE entry the oldest. Evicting it would leave the active
    // pointer aimed at a server missing from its own list — the switcher would
    // omit the one you're on and offer only ones you're not.
    it('never evicts the active server, even when it is the oldest', async () => {
        await setActiveServer('https://s0.example.com')
        for (let i = 1; i <= MAX_SAVED_SERVERS; i++) {
            await addServer(`https://s${i}.example.com`)
        }

        const saved = await origins()
        expect(saved).toHaveLength(MAX_SAVED_SERVERS)
        expect(saved).toContain('https://s0.example.com')
        // The next-oldest evictable entry went instead.
        expect(saved).not.toContain('https://s1.example.com')
        expect(await AsyncStorage.getItem('tinycld:server:app')).toBe('https://s0.example.com')
    })

    it('removes a server by any spelling', async () => {
        await addServer('https://a.example.com')
        await addServer('https://b.example.com')

        const remaining = await removeServer('a.example.com')
        expect(remaining.map(s => s.origin)).toEqual(['https://b.example.com'])
    })

    it('survives a corrupt list rather than throwing', async () => {
        await AsyncStorage.setItem('tinycld:servers', '{not an array')
        expect(await readServers()).toEqual([])
    })

    it('drops malformed entries but keeps the rest of the list', async () => {
        await AsyncStorage.setItem(
            'tinycld:servers',
            JSON.stringify([{ origin: 'https://a.example.com' }, { label: 'no origin' }, null])
        )
        expect(await origins()).toEqual(['https://a.example.com'])
    })
})

describe('setActiveServer', () => {
    // The reason it is the only sanctioned writer: a raw writeCached would set an
    // active server with no matching list entry, so it would never appear in the
    // switcher.
    it('writes both the list entry and the active pointer', async () => {
        await setActiveServer('acme.tinycld.org')

        expect(await origins()).toEqual(['https://acme.tinycld.org'])
        expect(await AsyncStorage.getItem('tinycld:server:app')).toBe('https://acme.tinycld.org')
    })

    it('normalizes the active pointer to the same spelling as the list', async () => {
        await setActiveServer('https://acme.tinycld.org/')
        expect(await AsyncStorage.getItem('tinycld:server:app')).toBe('https://acme.tinycld.org')
    })

    it('re-activating an existing server does not duplicate it', async () => {
        await setActiveServer('https://a.example.com')
        await setActiveServer('https://b.example.com')
        await setActiveServer('https://a.example.com')

        expect(await origins()).toEqual(['https://a.example.com', 'https://b.example.com'])
        expect(await AsyncStorage.getItem('tinycld:server:app')).toBe('https://a.example.com')
    })
})
