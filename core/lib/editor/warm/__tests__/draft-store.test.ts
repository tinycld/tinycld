import { describe, expect, it } from 'vitest'
import { createDraftStore } from '../draft-store'

/**
 * With one shared editor, a half-typed comment no longer survives in a mounted
 * composer — the instance moves to whatever the user tapped. The draft store is
 * where that text lives instead: stashed on release, re-seeded on acquire.
 *
 * Scoped to the composer and to the life of the open card. CardDetail is
 * already keyed on the card id at both mount sites, so a card switch drops the
 * store with the subtree and there is no eviction policy to get wrong.
 */
describe('draft store', () => {
    it('has nothing for an untouched surface', () => {
        expect(createDraftStore().take('composer:card1')).toBeNull()
    })

    it('returns what was stashed', () => {
        const drafts = createDraftStore()
        drafts.stash('composer:card1', 'half a thought')
        expect(drafts.take('composer:card1')).toBe('half a thought')
    })

    /** Re-acquiring twice without typing must not lose the draft. */
    it('does not consume the draft on read', () => {
        const drafts = createDraftStore()
        drafts.stash('composer:card1', 'still here')
        drafts.take('composer:card1')
        expect(drafts.take('composer:card1')).toBe('still here')
    })

    it('keeps drafts separate per surface', () => {
        const drafts = createDraftStore()
        drafts.stash('composer:card1', 'one')
        drafts.stash('composer:card2', 'two')
        expect(drafts.take('composer:card1')).toBe('one')
    })

    it('replaces an earlier draft for the same surface', () => {
        const drafts = createDraftStore()
        drafts.stash('composer:card1', 'first')
        drafts.stash('composer:card1', 'second')
        expect(drafts.take('composer:card1')).toBe('second')
    })

    /**
     * An empty editor is not a draft. Stashing one would re-seed an empty
     * string over the placeholder and make the composer look broken.
     */
    it('treats empty content as no draft', () => {
        const drafts = createDraftStore()
        drafts.stash('composer:card1', 'typed')
        drafts.stash('composer:card1', '   \n  ')
        expect(drafts.take('composer:card1')).toBeNull()
    })

    it('clears on commit', () => {
        const drafts = createDraftStore()
        drafts.stash('composer:card1', 'sent now')
        drafts.clear('composer:card1')
        expect(drafts.take('composer:card1')).toBeNull()
    })

    it('clears everything when the card closes', () => {
        const drafts = createDraftStore()
        drafts.stash('composer:card1', 'one')
        drafts.stash('composer:card2', 'two')
        drafts.clearAll()
        expect(drafts.take('composer:card1')).toBeNull()
        expect(drafts.take('composer:card2')).toBeNull()
    })
})
