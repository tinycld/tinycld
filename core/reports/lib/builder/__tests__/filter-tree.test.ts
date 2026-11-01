import { describe, expect, it } from 'vitest'
import {
    addFilterItem,
    emptyCondition,
    emptyGroup,
    filterItemKeys,
    type GroupDraft,
    removeFilterItem,
    setMatchAt,
    updateFilterItem,
} from '../filter'

const condition = (field: string) => emptyCondition([{ field }])

const tree: GroupDraft = {
    kind: 'group',
    match: 'all',
    items: [
        condition('customer'),
        {
            kind: 'related',
            has: true,
            link: [{ link: 'sales' }],
            where: { kind: 'group', match: 'all', items: [condition('amount')] },
        },
        { kind: 'group', match: 'any', items: [condition('status')] },
    ],
}

describe('filter tree edits', () => {
    it('adds to the root, a related item’s conditions and a nested group', () => {
        expect(addFilterItem(emptyGroup(), [], condition('a')).items).toHaveLength(1)
        const related = addFilterItem(tree, [1], condition('cost')).items[1]
        expect(
            related?.kind === 'related' &&
                related.where.items.map(i => (i.kind === 'condition' ? i.path[0]?.field : ''))
        ).toEqual(['amount', 'cost'])
        const group = addFilterItem(tree, [2], condition('day')).items[2]
        expect(group?.kind === 'group' && group.items).toHaveLength(2)
    })

    it('updates an item by its path, inside a related item too', () => {
        const next = updateFilterItem(tree, [1, 0], item =>
            item.kind === 'condition' ? { ...item, op: 'gt', value: '5' } : item
        )
        const related = next.items[1]
        expect(related?.kind === 'related' && related.where.items[0]).toMatchObject({
            op: 'gt',
            value: '5',
        })
        expect(tree.items[1]).not.toBe(related)
    })

    it('removes an item and never the root', () => {
        expect(removeFilterItem(tree, [0]).items).toHaveLength(2)
        expect(removeFilterItem(tree, [2, 0]).items[2]).toMatchObject({ items: [] })
        expect(removeFilterItem(tree, [])).toBe(tree)
    })

    it('leaves the tree as it is for a path that names no item', () => {
        expect(updateFilterItem(tree, [9], () => condition('x'))).toEqual(tree)
        expect(removeFilterItem(tree, [0, 0])).toEqual(tree)
        expect(addFilterItem(tree, [0], condition('x'))).toEqual(tree)
    })

    it('keys items by what they filter, so typing a value keeps a row’s key', () => {
        const keys = filterItemKeys(tree.items)
        expect(new Set(keys).size).toBe(3)
        const typed = updateFilterItem(tree, [0], item =>
            item.kind === 'condition' ? { ...item, value: 'Acme' } : item
        )
        expect(filterItemKeys(typed.items)).toEqual(keys)
        // Removing the first item does not hand its key to the next one.
        expect(filterItemKeys(removeFilterItem(tree, [0]).items)).toEqual(keys.slice(1))
        const twice = filterItemKeys([condition('day'), condition('day')])
        expect(twice[0]).not.toBe(twice[1])
    })

    it('sets how a group or a related item’s conditions are joined', () => {
        expect(setMatchAt(tree, [], 'none').match).toBe('none')
        const related = setMatchAt(tree, [1], 'any').items[1]
        expect(related?.kind === 'related' && related.where.match).toBe('any')
    })
})
