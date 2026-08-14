/**
 * Does a WebView-backed rich editor currently hold focus?
 *
 * This exists for the native shortcut provider. It decides whether a keypress
 * is a global shortcut or ordinary typing by asking
 * `TextInput.State.currentlyFocusedInput()`, which only knows about React
 * Native `TextInput`s. A rich editor is a WebView, so that check reports "no
 * input focused" while the user is mid-sentence, and every plain letter is
 * offered to the matcher as a shortcut — `e` opened the card title for editing
 * and swallowed the rest of the sentence.
 *
 * The count is a COUNT, not a boolean: a card detail mounts several editors
 * (description, comment composer, inline comment edit) and moving focus between
 * two of them interleaves the new one's focus with the old one's blur. A
 * boolean would be left false by that trailing blur even though an editor is
 * still focused.
 *
 * Deliberately module-global rather than context: the shortcut provider sits at
 * the app root, far above any editor, and reads this from a plain event handler
 * rather than during render — there is nothing for React to subscribe to.
 */

let focusedCount = 0

/** Called by the native editor host on every focus edge. */
export function setEditorFocused(isFocused: boolean): void {
    if (isFocused) {
        focusedCount += 1
        return
    }
    // Clamp: an unmatched blur (a blur delivered after the editor unmounted,
    // say) must not drive the count negative and latch the flag off forever.
    focusedCount = Math.max(0, focusedCount - 1)
}

/** True while any rich editor holds focus. */
export function isEditorFocused(): boolean {
    return focusedCount > 0
}

/**
 * Release a mount's focus claim.
 *
 * An editor that unmounts while focused never delivers its blur, so without
 * this the count stays above zero and shortcuts are suppressed app-wide. The
 * native host calls it from its unmount cleanup.
 */
export function releaseEditorFocus(wasFocused: boolean): void {
    if (wasFocused) setEditorFocused(false)
}

/** Test-only. The counter is module-global and would otherwise leak. */
export function resetEditorFocusState(): void {
    focusedCount = 0
}
