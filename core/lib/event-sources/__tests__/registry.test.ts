import { log } from '@tinycld/core/lib/logger'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createEventSourceLoader, deriveEventSources } from '../registry'
import type { EventSourceModule } from '../types'

const load = () => Promise.resolve({})

describe('deriveEventSources', () => {
    it('groups by target, sorted by order then contributor slug', () => {
        const derived = deriveEventSources([
            {
                manifest: { slug: 'widgets' },
                eventSources: [
                    { target: 'sprockets', id: 'widgets-due', label: 'Widgets', order: 0, load },
                ],
            },
            {
                manifest: { slug: 'gadgets' },
                eventSources: [
                    { target: 'sprockets', id: 'gadgets-due', label: 'Gadgets', order: 0, load },
                    {
                        target: 'timeline',
                        id: 'gadgets-activity',
                        label: 'Gadgets',
                        order: 5,
                        load,
                    },
                ],
            },
            { manifest: { slug: 'contacts' } },
        ])
        expect(Object.keys(derived).sort()).toEqual(['sprockets', 'timeline'])
        expect(derived.sprockets.map(s => s.id)).toEqual(['gadgets-due', 'widgets-due'])
        expect(derived.timeline[0]).toMatchObject({
            contributorSlug: 'gadgets',
            id: 'gadgets-activity',
            order: 5,
        })
    })

    it('sorts by order before contributor slug', () => {
        const derived = deriveEventSources([
            {
                manifest: { slug: 'aaa' },
                eventSources: [{ target: 'sprockets', id: 'late', label: 'L', order: 9, load }],
            },
            {
                manifest: { slug: 'zzz' },
                eventSources: [{ target: 'sprockets', id: 'early', label: 'E', order: 1, load }],
            },
        ])
        expect(derived.sprockets.map(s => s.id)).toEqual(['early', 'late'])
    })
})

describe('createEventSourceLoader', () => {
    const module: EventSourceModule = {
        useEventSource: () => ({ items: [], isLoading: false }),
    }

    it('loads a registered module once and caches it', async () => {
        const loadSpy = vi.fn().mockResolvedValue(module)
        const loader = createEventSourceLoader({
            sprockets: [
                {
                    contributorSlug: 'gadgets',
                    id: 'gadgets-due',
                    label: 'C',
                    order: 0,
                    load: loadSpy,
                },
            ],
        })
        const first = await loader('sprockets', 'gadgets-due')
        const second = await loader('sprockets', 'gadgets-due')
        expect(first).toBe(module)
        expect(second).toBe(module)
        expect(loadSpy).toHaveBeenCalledTimes(1)
    })

    it('returns null for an unregistered source', async () => {
        const loader = createEventSourceLoader({})
        expect(await loader('sprockets', 'gadgets-due')).toBeNull()
    })

    afterEach(() => {
        vi.restoreAllMocks()
    })

    it('returns null and logs when the module lacks a useEventSource function', async () => {
        const warn = vi.spyOn(log, 'warn').mockImplementation(() => undefined)
        const loader = createEventSourceLoader({
            sprockets: [
                {
                    contributorSlug: 'gadgets',
                    id: 'gadgets-due',
                    label: 'C',
                    order: 0,
                    load: () => Promise.resolve({ somethingElse: true }),
                },
            ],
        })
        expect(await loader('sprockets', 'gadgets-due')).toBeNull()
        expect(warn).toHaveBeenCalledWith('event-sources.load', expect.any(String), {
            target: 'sprockets',
            sourceId: 'gadgets-due',
            contributorSlug: 'gadgets',
        })
    })
})
