import {
    filterTriggerItems,
    renderInsertTemplate,
    type TriggerItem,
    triggerPluginKey,
} from '@tinycld/core/lib/editor/rich/triggers'
import { describe, expect, it } from 'vitest'

const ROSTER: TriggerItem[] = [
    { id: 'u1', label: 'Ada Lovelace', secondary: 'ada@example.com' },
    { id: 'u2', label: 'Grace Hopper', secondary: 'grace@navy.mil' },
    { id: 'u3', label: 'Alan Turing', secondary: 'alan@example.com' },
    { id: 'u4', label: 'Katherine Johnson' },
]

describe('filterTriggerItems', () => {
    it('returns the whole pool for an empty query, up to the limit', () => {
        expect(filterTriggerItems(ROSTER, '', 10).map(i => i.id)).toEqual(['u1', 'u2', 'u3', 'u4'])
    })

    it('matches the label case-insensitively', () => {
        expect(filterTriggerItems(ROSTER, 'ADA', 10).map(i => i.id)).toEqual(['u1'])
    })

    it('matches the secondary line too, so an email finds someone', () => {
        expect(filterTriggerItems(ROSTER, 'navy.mil', 10).map(i => i.id)).toEqual(['u2'])
    })

    it('matches a substring anywhere, not just a prefix', () => {
        expect(filterTriggerItems(ROSTER, 'turing', 10).map(i => i.id)).toEqual(['u3'])
    })

    it('tolerates an item with no secondary line', () => {
        expect(filterTriggerItems(ROSTER, 'katherine', 10).map(i => i.id)).toEqual(['u4'])
    })

    it('trims the query, so a trailing space does not empty the list', () => {
        expect(filterTriggerItems(ROSTER, ' ada ', 10).map(i => i.id)).toEqual(['u1'])
    })

    it('caps at the limit', () => {
        expect(filterTriggerItems(ROSTER, '', 2)).toHaveLength(2)
    })

    it('returns nothing when nothing matches', () => {
        expect(filterTriggerItems(ROSTER, 'zzzz', 10)).toEqual([])
    })

    it('defaults the limit rather than returning an unbounded list', () => {
        const many = Array.from({ length: 50 }, (_, i) => ({ id: `u${i}`, label: `User ${i}` }))
        expect(filterTriggerItems(many, '').length).toBeLessThan(many.length)
    })
})

describe('renderInsertTemplate', () => {
    const item: TriggerItem = { id: 'abc123XYZ_-', label: 'Ada', secondary: 'ada@example.com' }

    it('substitutes the id and keeps the trailing space', () => {
        // The space is what lets someone keep typing after picking rather than
        // landing inside the token — cards depends on it.
        expect(renderInsertTemplate('[[@{id}]] ', item)).toBe('[[@abc123XYZ_-]] ')
    })

    it('substitutes label and secondary', () => {
        expect(renderInsertTemplate('{label} <{secondary}>', item)).toBe('Ada <ada@example.com>')
    })

    it('leaves an unknown placeholder verbatim rather than blanking it', () => {
        expect(renderInsertTemplate('{nope}-{id}', item)).toBe('{nope}-abc123XYZ_-')
    })

    it('leaves {secondary} verbatim when the item has none', () => {
        // Blanking would silently produce a different token than the author
        // wrote; leaving it visible makes the mistake findable.
        expect(renderInsertTemplate('{secondary}', { id: 'u1', label: 'Ada' })).toBe('{secondary}')
    })

    it('returns an empty string for an empty template, which inserts nothing', () => {
        expect(renderInsertTemplate('', item)).toBe('')
    })
})

describe('triggerPluginKey', () => {
    it('returns the SAME instance for an id', () => {
        // Load-bearing: the native bridge calls exitSuggestion with this key,
        // and a fresh PluginKey would resolve to no plugin state at all.
        expect(triggerPluginKey('cards-mention')).toBe(triggerPluginKey('cards-mention'))
    })

    it('separates distinct triggers', () => {
        expect(triggerPluginKey('cards-mention')).not.toBe(triggerPluginKey('emoji'))
    })
})
