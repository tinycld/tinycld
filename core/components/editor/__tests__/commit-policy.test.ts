import { describe, expect, it } from 'vitest'
import { isNoOpEdit } from '../commit-policy'

/**
 * The blur-commit rules that used to live here are gone with the behaviour: a
 * session no longer ends on focus loss, so there is nothing left for them to
 * decide. What a submitted value MEANS is still policy, and still shared.
 */
describe('no-op edits', () => {
    it('treats an unchanged value as nothing to write', () => {
        expect(isNoOpEdit('same text', 'same text')).toBe(true)
    })

    it('ignores surrounding whitespace, which the editor adds on its own', () => {
        expect(isNoOpEdit('  same text\n', 'same text')).toBe(true)
    })

    it('treats a real change as a write', () => {
        expect(isNoOpEdit('new text', 'old text')).toBe(false)
    })

    /** An emptied editor is a deletion the caller must decide about, not a no-op. */
    it('does not call an emptied editor unchanged', () => {
        expect(isNoOpEdit('', 'had content')).toBe(false)
    })
})
