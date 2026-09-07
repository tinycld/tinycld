import {
    fitToolbarItems,
    keyToolbarItems,
    type ToolbarItem,
} from '@tinycld/core/components/toolbar/fit-toolbar-items'
import { describe, expect, it } from 'vitest'

const Icon = (() => null) as unknown as ToolbarItem extends { icon: infer I } ? I : never

const btn = (key: string): ToolbarItem => ({
    type: 'button',
    key,
    icon: Icon,
    label: key,
    onPress: () => {},
})
const sep = (): ToolbarItem => ({ type: 'separator' })
const spacer = (): ToolbarItem => ({ type: 'spacer' })
const pinned = (key: string): ToolbarItem => ({ type: 'custom', key, element: null })
const foldable = (key: string): ToolbarItem => ({
    type: 'custom',
    key,
    element: null,
    overflow: { label: key, onPress: () => {} },
})
const hidden = (key: string): ToolbarItem => ({
    type: 'custom',
    key,
    element: null,
    overflow: 'hide',
})

/** Every measurable item is `width` wide unless named in `custom`. */
function solve(
    items: ToolbarItem[],
    availableWidth: number,
    options: {
        width?: number
        custom?: Record<string, number>
        moreWidth?: number
        hasPermanentMore?: boolean
        gap?: number
    } = {}
) {
    const keyed = keyToolbarItems(items)
    const widths = new Map<string, number>()
    for (const entry of keyed) {
        if (entry.item.type === 'spacer') continue
        widths.set(entry.key, options.custom?.[entry.key] ?? options.width ?? 40)
    }
    const result = fitToolbarItems({
        items: keyed,
        widths,
        availableWidth,
        moreWidth: options.moreWidth ?? 34,
        hasPermanentMore: options.hasPermanentMore ?? false,
        gap: options.gap ?? 0,
    })
    return {
        visible: result.visible.map(entry => entry.key),
        overflow: result.overflow.map(entry => entry.key),
        showMore: result.showMore,
    }
}

describe('fitToolbarItems', () => {
    it('keeps everything and no More button when it all fits', () => {
        expect(solve([btn('a'), btn('b'), btn('c')], 120)).toEqual({
            visible: ['a', 'b', 'c'],
            overflow: [],
            showMore: false,
        })
    })

    it('charges the More button as soon as anything folds', () => {
        // 3 × 40 fits 120 exactly, but 118 forces a fold, and the More button
        // (34) then only leaves room for two.
        expect(solve([btn('a'), btn('b'), btn('c')], 118)).toEqual({
            visible: ['a', 'b'],
            overflow: ['c'],
            showMore: true,
        })
        // 100 with More reserved leaves 66: one button.
        expect(solve([btn('a'), btn('b'), btn('c')], 100).visible).toEqual(['a'])
    })

    it('never folds a pinned custom item, and folds its neighbours in order', () => {
        const result = solve([btn('a'), pinned('p'), btn('b')], 100, { custom: { p: 80 } })
        expect(result.visible).toEqual(['p'])
        expect(result.overflow).toEqual(['a', 'b'])
    })

    it('treats a spacer as zero width and never folds it', () => {
        const result = solve([btn('a'), spacer(), btn('b')], 80)
        expect(result).toEqual({ visible: ['a', 'spacer-1', 'b'], overflow: [], showMore: false })
        const folded = solve([btn('a'), spacer(), btn('b')], 78)
        expect(folded.visible).toEqual(['a', 'spacer-1'])
        expect(folded.overflow).toEqual(['b'])
    })

    it('trims separators at the visible edge and around folded groups', () => {
        const items = [btn('a'), sep(), btn('b'), sep(), btn('c')]
        // Folding c leaves a trailing separator, which is dropped and not charged:
        // a, sep, b = 90 + 34 More = 124.
        expect(solve(items, 124, { custom: { 'sep-1': 10, 'sep-3': 10 } })).toEqual({
            visible: ['a', 'sep-1', 'b'],
            overflow: ['c'],
            showMore: true,
        })
        // Folding b and c: the separator between them travels with them, the
        // one that led is trimmed.
        expect(solve(items, 74, { custom: { 'sep-1': 10, 'sep-3': 10 } })).toEqual({
            visible: ['a'],
            overflow: ['b', 'sep-3', 'c'],
            showMore: true,
        })
    })

    it('keeps a separator that still sits between two visible items', () => {
        const result = solve([btn('a'), sep(), pinned('p'), sep(), btn('b')], 124, {
            custom: { 'sep-1': 10, 'sep-3': 10 },
        })
        expect(result.visible).toEqual(['a', 'sep-1', 'p'])
        expect(result.overflow).toEqual(['b'])
    })

    it('collapses doubled separators left behind by a folded middle item', () => {
        const result = solve([btn('a'), sep(), btn('b'), sep(), pinned('p')], 120, {
            custom: { 'sep-1': 4, 'sep-3': 4 },
        })
        expect(result.visible).toEqual(['a', 'sep-1', 'p'])
        expect(result.overflow).toEqual(['b'])
    })

    it('always charges a permanent More button', () => {
        expect(solve([btn('a'), btn('b'), btn('c')], 120, { hasPermanentMore: true })).toEqual({
            visible: ['a', 'b'],
            overflow: ['c'],
            showMore: true,
        })
        expect(solve([btn('a')], 200, { hasPermanentMore: true })).toEqual({
            visible: ['a'],
            overflow: [],
            showMore: true,
        })
    })

    it('returns only the pinned items when nothing collapsible fits', () => {
        const result = solve([pinned('p'), btn('a'), btn('b')], 10)
        expect(result.visible).toEqual(['p'])
        expect(result.overflow).toEqual(['a', 'b'])
        expect(result.showMore).toBe(true)
    })

    it('charges the gap between every slot, the More button included', () => {
        expect(solve([btn('a'), btn('b')], 82, { gap: 2 }).visible).toEqual(['a', 'b'])
        const folded = solve([btn('a'), btn('b')], 80, { gap: 2 })
        expect(folded.visible).toEqual(['a'])
        // a (40) + gap (2) + More (34) = 76 fits in 80.
        expect(folded.showMore).toBe(true)
    })

    it('forgives a sub-pixel shortfall so a shrink-wrapped row does not fold what fits', () => {
        expect(solve([btn('a'), btn('b'), btn('c')], 119.5).visible).toEqual(['a', 'b', 'c'])
    })

    it('charges a shrinkable item at its floor so it truncates before others fold', () => {
        const title: ToolbarItem = { type: 'custom', key: 'title', element: null, minWidth: 100 }
        // Title measured 300 but may shrink to 100: 100 + 40 fits in 140.
        expect(solve([title, btn('a')], 140, { custom: { title: 300 } })).toEqual({
            visible: ['title', 'a'],
            overflow: [],
            showMore: false,
        })
        // Below the floor the button folds.
        expect(solve([title, btn('a')], 138, { custom: { title: 300 } }).overflow).toEqual(['a'])
    })

    it('drops informational items without listing them in the menu', () => {
        const result = solve([btn('a'), hidden('h'), foldable('f')], 80)
        expect(result.visible).toEqual(['a'])
        expect(result.overflow).toEqual(['f'])
    })
})
