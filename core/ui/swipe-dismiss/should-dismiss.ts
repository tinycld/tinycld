// The drag-to-dismiss threshold for edge-anchored panels. It is a worklet so
// every pan gesture's onEnd (which runs on the UI thread) calls this one copy,
// and the unit test pins the same code the gestures run.
//
// `dir` is the sign of the off-screen direction, which is what generalises one
// threshold over all four anchors: +1 for a panel that leaves rightward or
// downward (anchor 'right' / 'bottom'), -1 for one that leaves leftward or
// upward ('left' / 'top'). Multiplying both inputs by it reduces every anchor
// to the same "away from the edge is positive" comparison, so a flick TOWARD
// the edge can never dismiss.
//
// A flick whose sign is opposite to the drag's net travel is not trusted:
// gesture-handler derives velocity from the last few input events, and events
// stamped unevenly (a stall, then a burst 1 ms apart) can report a huge
// velocity against the drag. The distance decides then.
export function shouldDismissSwipe(translation: number, velocity: number, dir: number): boolean {
    'worklet'
    const isFlickTrusted = velocity * translation >= 0
    return dir * translation > 100 || (isFlickTrusted && dir * velocity > 500)
}
