// The drag-to-dismiss threshold for BottomDrawer, as a pure, unit-testable
// function. The component's pan-gesture onEnd runs on the UI thread (a worklet)
// and therefore can't call a non-worklet JS function, so it INLINES this same
// comparison rather than importing this. Keep the two in sync — this version
// exists so the unit test can pin the contract without an on-device gesture:
// dragged past 100px, or flicked downward faster than 500px/s, dismisses; a
// negative (upward) velocity never does.
export function shouldDismissDrawer(translationY: number, velocityY: number): boolean {
    return translationY > 100 || velocityY > 500
}
