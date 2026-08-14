import { beforeEach, describe, expect, it } from 'vitest'
import {
    isEditorFocused,
    releaseEditorFocus,
    resetEditorFocusState,
    setEditorFocused,
} from '../editor-focus-state'

// This module is what stops a plain letter typed into a rich editor from being
// read as a global shortcut on native. The card detail binds `e` to "edit
// title", so a false negative here does not merely miss a shortcut — it steals
// the keystroke and appends the rest of the sentence to the card's title.

describe('editor focus state', () => {
    beforeEach(() => {
        resetEditorFocusState()
    })

    it('is unfocused before any editor reports in', () => {
        expect(isEditorFocused()).toBe(false)
    })

    it('tracks a single editor focusing and blurring', () => {
        setEditorFocused(true)
        expect(isEditorFocused()).toBe(true)
        setEditorFocused(false)
        expect(isEditorFocused()).toBe(false)
    })

    // The card detail mounts three editors. Moving from the description to the
    // comment composer delivers the new editor's focus BEFORE the old one's
    // blur, so a boolean would be left false by that trailing blur while an
    // editor is still focused — and the next letter would fire a shortcut.
    it('stays focused when focus moves between two editors', () => {
        setEditorFocused(true)
        setEditorFocused(true)
        setEditorFocused(false)
        expect(isEditorFocused()).toBe(true)
        setEditorFocused(false)
        expect(isEditorFocused()).toBe(false)
    })

    // A blur with no matching focus must not drive the count negative, or the
    // next real focus would leave it at zero and shortcuts would fire while the
    // user types.
    it('clamps unmatched blurs at zero', () => {
        setEditorFocused(false)
        setEditorFocused(false)
        expect(isEditorFocused()).toBe(false)

        setEditorFocused(true)
        expect(isEditorFocused()).toBe(true)
    })

    // An editor unmounted while focused never delivers a blur; without the
    // release its claim would latch and mute shortcuts for the whole session.
    it('releases a focused editor that unmounts', () => {
        setEditorFocused(true)
        releaseEditorFocus(true)
        expect(isEditorFocused()).toBe(false)
    })

    it('ignores the release of an editor that was not focused', () => {
        setEditorFocused(true)
        releaseEditorFocus(false)
        expect(isEditorFocused()).toBe(true)
    })
})
