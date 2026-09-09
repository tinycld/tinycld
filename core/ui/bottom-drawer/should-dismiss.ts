// Kept as a name only: the threshold now lives in ui/swipe-dismiss, which
// generalises it over an axis sign so one function serves all four anchors.
// This wrapper pins the bottom-sheet case (dir = +1) so the older spelling
// keeps working while consumers migrate off `BottomDrawer` — see
// docs/plans/overlay-engine.md.
import { shouldDismissSwipe } from '@tinycld/core/ui/swipe-dismiss/should-dismiss'

export function shouldDismissDrawer(translationY: number, velocityY: number): boolean {
    return shouldDismissSwipe(translationY, velocityY, 1)
}
