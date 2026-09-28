import { log } from '@tinycld/core/lib/logger'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createSearchAdapterLoader } from '../registry'

const adapter = { useSearchActions: () => ({ onSelect: () => undefined }) }

afterEach(() => {
    vi.restoreAllMocks()
})

describe('createSearchAdapterLoader', () => {
    it('loads a valid adapter once and caches it', async () => {
        const load = vi.fn().mockResolvedValue(adapter)
        const loader = createSearchAdapterLoader([
            { manifest: { slug: 'widgets' }, search: { load } },
        ])
        expect(await loader('widgets')).toBe(adapter)
        expect(await loader('widgets')).toBe(adapter)
        expect(load).toHaveBeenCalledTimes(1)
    })

    it('returns null for a package without search', async () => {
        const loader = createSearchAdapterLoader([{ manifest: { slug: 'widgets' } }])
        expect(await loader('widgets')).toBeNull()
    })

    it('returns null and logs when the module lacks useSearchActions', async () => {
        const warn = vi.spyOn(log, 'warn').mockImplementation(() => undefined)
        const loader = createSearchAdapterLoader([
            { manifest: { slug: 'widgets' }, search: { load: async () => ({ other: 1 }) } },
        ])
        expect(await loader('widgets')).toBeNull()
        expect(warn).toHaveBeenCalledWith('search.adapter', expect.any(String), {
            slug: 'widgets',
        })
    })
})
