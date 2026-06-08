// A long-press that opens a context menu must NOT also fire the underlying
// row's tap handler (which would, e.g., open a file the moment the user lifts
// their finger off the long-press). ContextMenuNative observes touches on an
// ancestor View without claiming the gesture responder, so the inner Pressable
// still delivers its onPress on release. There is no in-gesture way to retract
// that press from the ancestor, so we coordinate out of band: when a long-press
// opens the menu we stamp a timestamp here, and the row's press handler checks
// it and bows out if the release belongs to that just-opened gesture.
//
// A module-level singleton is sufficient because touch gestures are serialized
// — only one long-press can be resolving at a time — so there is no ambiguity
// about which press the stamp refers to.

let suppressUntil = 0

// Window covering the gap between the long-press firing and the finger lifting.
// Generous enough for a slow release, short enough that a deliberate tap a
// moment later is never swallowed.
const SUPPRESS_WINDOW_MS = 700

/** Called by ContextMenuNative the instant a long-press opens the menu. */
export function markContextMenuOpenedByLongPress(now: number): void {
    suppressUntil = now + SUPPRESS_WINDOW_MS
}

/**
 * Returns true if the current press should be ignored because it is the tail
 * of the long-press gesture that just opened a context menu. Consuming it
 * clears the flag so only the one press is suppressed.
 */
export function consumeContextMenuPressSuppression(now: number): boolean {
    if (now < suppressUntil) {
        suppressUntil = 0
        return true
    }
    return false
}
