export interface CommitState {
    /** Does this surface write on focus loss? An edit does; a composer does not. */
    commitOnBlur: boolean
    /** Has the session ever actually held focus? */
    hasFocused: boolean
    /** Has it already committed or cancelled? */
    isSettled: boolean
    /** Is a dialog (image picker, link prompt) holding the focus? */
    isDialogOpen: boolean
}

/**
 * Whether a blur should write.
 *
 * Every clause here exists because of a bug found on a device, and the stakes
 * are asymmetric: a missed commit loses an edit the user can redo, while a
 * spurious one writes text nobody typed.
 */
export function shouldCommitOnBlur(state: CommitState): boolean {
    if (!state.commitOnBlur) return false
    // The mount racing itself, not a person finishing an edit.
    if (!state.hasFocused) return false
    if (state.isSettled) return false
    // A detour inside the session, not the end of it.
    if (state.isDialogOpen) return false
    return true
}

/**
 * Whether a submitted value differs from what the session opened with.
 *
 * An unchanged edit is a cancel rather than a write — EditableText's rule. The
 * baseline is snapshotted at acquire, so a realtime update arriving mid-edit
 * cannot become the comparison target.
 */
export function isNoOpEdit(next: string, baseline: string): boolean {
    return next.trim() === baseline.trim()
}
