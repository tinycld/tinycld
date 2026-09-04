import { describe, expect, it, vi } from 'vitest'
import { createEventSourceLoader, deriveEventSources } from '../registry'
import type { EventSourceModule } from '../types'

const load = () => Promise.resolve({})

describe('deriveEventSources', () => {
    it('groups by target, sorted by order then contributor slug', () => {
        const derived = deriveEventSources([
            {
                manifest: { slug: 'tasks' },
                eventSources: [
                    { target: 'calendar', id: 'tasks-due', label: 'Tasks', order: 0, load },
                ],
            },
            {
                manifest: { slug: 'boards' },
                eventSources: [
                    { target: 'calendar', id: 'boards-due', label: 'Boards', order: 0, load },
                    { target: 'timeline', id: 'boards-activity', label: 'Boards', order: 5, load },
                ],
            },
            { manifest: { slug: 'contacts' } },
        ])
        expect(Object.keys(derived).sort()).toEqual(['calendar', 'timeline'])
        expect(derived.calendar.map(s => s.id)).toEqual(['boards-due', 'tasks-due'])
        expect(derived.timeline[0]).toMatchObject({
            contributorSlug: 'boards',
            id: 'boards-activity',
            order: 5,
        })
    })

    it('sorts by order before contributor slug', () => {
        const derived = deriveEventSources([
            {
                manifest: { slug: 'aaa' },
                eventSources: [{ target: 'calendar', id: 'late', label: 'L', order: 9, load }],
            },
            {
                manifest: { slug: 'zzz' },
                eventSources: [{ target: 'calendar', id: 'early', label: 'E', order: 1, load }],
            },
        ])
        expect(derived.calendar.map(s => s.id)).toEqual(['early', 'late'])
    })
})

describe('createEventSourceLoader', () => {
    const module: EventSourceModule = {
        useEventSource: () => ({ items: [], isLoading: false }),
    }

    it('loads a registered module once and caches it', async () => {
        const loadSpy = vi.fn().mockResolvedValue(module)
        const loader = createEventSourceLoader({
            calendar: [
                {
                    contributorSlug: 'boards',
                    id: 'boards-due',
                    label: 'C',
                    order: 0,
                    load: loadSpy,
                },
            ],
        })
        const first = await loader('calendar', 'boards-due')
        const second = await loader('calendar', 'boards-due')
        expect(first).toBe(module)
        expect(second).toBe(module)
        expect(loadSpy).toHaveBeenCalledTimes(1)
    })

    it('returns null for an unregistered source', async () => {
        const loader = createEventSourceLoader({})
        expect(await loader('calendar', 'boards-due')).toBeNull()
    })

    it('returns null when the module lacks a useEventSource function', async () => {
        const loader = createEventSourceLoader({
            calendar: [
                {
                    contributorSlug: 'boards',
                    id: 'boards-due',
                    label: 'C',
                    order: 0,
                    load: () => Promise.resolve({ somethingElse: true }),
                },
            ],
        })
        expect(await loader('calendar', 'boards-due')).toBeNull()
    })
})
