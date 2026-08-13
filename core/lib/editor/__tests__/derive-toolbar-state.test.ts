import { describe, expect, it } from 'vitest'
import { deriveToolbarState } from '../derive-toolbar-state'

/**
 * The narrowing contract: every field requires its exact runtime type and
 * collapses to the documented default otherwise. `isEmpty` is the field
 * composers gate a Send button on, so its tri-state matters — `undefined`
 * (no stateUpdate yet) must stay distinguishable from `false` (has content),
 * because a consumer reads `isEmpty ?? true` to keep Send disabled until the
 * page has actually spoken.
 */
describe('deriveToolbarState', () => {
    it('maps isEmpty through when the wire value is a boolean', () => {
        expect(deriveToolbarState({ isEmpty: true }).isEmpty).toBe(true)
        expect(deriveToolbarState({ isEmpty: false }).isEmpty).toBe(false)
    })

    it('leaves isEmpty undefined before the first stateUpdate', () => {
        expect(deriveToolbarState({}).isEmpty).toBeUndefined()
    })

    it('collapses a malformed wire value to undefined rather than a truthy guess', () => {
        expect(deriveToolbarState({ isEmpty: 'yes' }).isEmpty).toBeUndefined()
        expect(deriveToolbarState({ isEmpty: 1 }).isEmpty).toBeUndefined()
        expect(deriveToolbarState({ isEmpty: null }).isEmpty).toBeUndefined()
    })

    it('applies the strict defaults for the other fields', () => {
        const state = deriveToolbarState({ isBoldActive: 'yes', activeHeadingLevel: '2' })
        expect(state.isBoldActive).toBe(false)
        expect(state.activeHeadingLevel).toBeNull()
        expect(state.selectionEmpty).toBe(true)
        expect(state.wordCount).toBeUndefined()
    })
})
