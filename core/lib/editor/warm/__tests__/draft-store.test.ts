import { beforeEach, describe, expect, it } from 'vitest'
import { createDraftStore, dropSurfaceDraft, subscribeToDrafts } from '../draft-store'

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
    // One store for the app, so state outlives a test. These cases pass without
    // this only because they happen to use distinct surface ids — reset so that
    // stays a guarantee rather than a coincidence.
    beforeEach(() => createDraftStore().clearAll())

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

    /**
     * The reason this store is reactive at all.
     *
     * A draft is stashed from an ASYNC read of the editor — its content cannot
     * be read synchronously, since on native that is a WebView round-trip — so
     * it lands after the render that would have displayed it. A plain Map stored
     * the text correctly and showed the user nothing: the composer read it
     * during render and never re-rendered to see it arrive.
     */
    it('notifies a subscriber when a draft lands', () => {
        const drafts = createDraftStore()
        const seen: (string | null)[] = []
        const unsubscribe = subscribeToDrafts(() => seen.push(drafts.take('composer:card1')))

        drafts.stash('composer:card1', 'arrived late')
        unsubscribe()

        expect(seen).toContain('arrived late')
    })

    it('drops one surface without touching the others', () => {
        const drafts = createDraftStore()
        drafts.stash('composer:card1', 'one')
        drafts.stash('composer:card2', 'two')
        dropSurfaceDraft('composer:card1')
        expect(drafts.take('composer:card1')).toBeNull()
        expect(drafts.take('composer:card2')).toBe('two')
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
