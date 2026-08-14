import { describe, expect, it } from 'vitest'
import { isNoOpEdit, shouldCommitOnBlur } from '../commit-policy'

/**
 * Each of these rules was found on a device, and each protects a write. They
 * live in core so a second consumer inherits the fixes rather than
 * rediscovering them.
 */
describe('commit on blur', () => {
    const base = { commitOnBlur: true, hasFocused: true, isSettled: false, isDialogOpen: false }

    it('commits when a focused session loses focus', () => {
        expect(shouldCommitOnBlur(base)).toBe(true)
    })

    /**
     * The composer has no blur-commit: leaving it must not post a comment.
     */
    it('never commits a surface that does not commit on blur', () => {
        expect(shouldCommitOnBlur({ ...base, commitOnBlur: false })).toBe(false)
    })

    /**
     * The editor opens with autofocus at a placeholder height, and until the
     * page reports its real content height the caret can land outside the
     * visible box and blur immediately. Since a blur COMMITS, that saved a
     * comment nobody had touched.
     */
    it('never commits before the session has held focus', () => {
        expect(shouldCommitOnBlur({ ...base, hasFocused: false })).toBe(false)
    })

    /**
     * Save and the blur-commit race each other: pressing Save blurs the editor.
     * Both writing would submit twice.
     */
    it('does not commit again once the session has settled', () => {
        expect(shouldCommitOnBlur({ ...base, isSettled: true })).toBe(false)
    })

    /**
     * The image picker and link prompt both steal focus. Treating that as the
     * end of the session unmounts the surface the picked image is about to be
     * inserted into.
     */
    it('does not commit while a dialog holds the focus', () => {
        expect(shouldCommitOnBlur({ ...base, isDialogOpen: true })).toBe(false)
    })
})

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
