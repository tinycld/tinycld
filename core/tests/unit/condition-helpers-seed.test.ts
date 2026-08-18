import { describe, expect, it } from 'vitest'
import type { ConditionsAst } from '../../lib/automation/condition-helpers'
import { addCondition, addGroup, seedFirstCondition } from '../../lib/automation/condition-helpers'
import { conditionsAstSchema } from '../../lib/automation/schemas'

const EMPTY: ConditionsAst = { match: 'all', groups: [] }

describe('seedFirstCondition', () => {
    it('promotes the rendered first row into one group holding one condition', () => {
        const ast = seedFirstCondition(EMPTY, {
            field: 'subject',
            op: 'contains',
            value: undefined,
        })
        expect(ast.groups).toHaveLength(1)
        expect(ast.groups[0].conditions).toHaveLength(1)
        expect(ast.groups[0].conditions[0]).toMatchObject({ field: 'subject', op: 'contains' })
    })

    it('assigns uids so the new rows have stable React keys', () => {
        // Without these, deleting a row while a sibling's Menu is open would
        // reconcile that Menu onto the wrong row (Menu owns isOpen internally).
        const ast = seedFirstCondition(EMPTY, { field: 'subject', op: 'contains' })
        expect(ast.groups[0].uid).toBeTruthy()
        expect(ast.groups[0].conditions[0].uid).toBeTruthy()
    })

    it('ignores a patch that names no field', () => {
        // Currently unreachable — ConditionRow's operator menu lists nothing
        // until a field is picked — but that invariant lives in a different
        // component. Seeding `field: ''` would produce a draft that throws in
        // draftToRecord, so the guard belongs here rather than in a sibling.
        expect(seedFirstCondition(EMPTY, { op: 'contains' })).toEqual(EMPTY)
    })

    it('replaces rather than appends, so the rendered row cannot double up', () => {
        // SyntheticFirstGroup only renders while groups is empty, so this is
        // defensive: a second call must not leave two groups behind.
        const once = seedFirstCondition(EMPTY, { field: 'subject', op: 'contains' })
        const twice = seedFirstCondition(once, { field: 'folder', op: 'is' })
        expect(twice.groups).toHaveLength(1)
    })

    it('preserves the top-level match mode', () => {
        const ast = seedFirstCondition(
            { match: 'any', groups: [] },
            { field: 'subject', op: 'contains' }
        )
        expect(ast.match).toBe('any')
    })

    // ConditionsCard's handleAddGroup composes these two for the FIRST group.
    // The offered row is render-only and unmounts as soon as a real group
    // exists, so appending an EMPTY first group would make the row the user was
    // looking at disappear and leave a group with nothing in it behind.
    it('composes with addGroup so a first group is never empty', () => {
        const ast = addCondition(addGroup(EMPTY), 0)
        expect(ast.groups).toHaveLength(1)
        expect(ast.groups[0].conditions).toHaveLength(1)
    })

    it('produces an AST the persistence schema accepts once a value is set', () => {
        const ast = seedFirstCondition(EMPTY, {
            field: 'subject',
            op: 'contains',
            value: 'invoice',
        })
        const parsed = conditionsAstSchema.parse(ast)
        // uid is builder-local and must never reach the server.
        expect(parsed.groups[0]).not.toHaveProperty('uid')
        expect(parsed.groups[0].conditions[0]).not.toHaveProperty('uid')
    })
})
