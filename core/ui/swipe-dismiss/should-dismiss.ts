// The drag-to-dismiss threshold for edge-anchored panels, as a pure,
// unit-testable function. Every caller's pan-gesture onEnd runs on the UI
// thread (a worklet) and therefore can't call a non-worklet JS function, so
// each one INLINES this same comparison rather than importing this. Keep them
// in sync — this version exists so the unit test can pin the contract without
// an on-device gesture.
//
// `dir` is the sign of the off-screen direction, which is what generalises one
// threshold over all four anchors: +1 for a panel that leaves rightward or
// downward (anchor 'right' / 'bottom'), -1 for one that leaves leftward or
// upward ('left' / 'top'). Multiplying both inputs by it reduces every anchor
// to the same "away from the edge is positive" comparison, so a flick TOWARD
// the edge can never dismiss.
export function shouldDismissSwipe(translation: number, velocity: number, dir: number): boolean {
    return dir * translation > 100 || dir * velocity > 500
}
