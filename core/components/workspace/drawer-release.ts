const FLING_VELOCITY = 500

/**
 * Whether the mobile drawer should settle open when a pan is released.
 *
 * A fast fling decides on its own, but only when it points the same way as the
 * drag's net travel. Gesture-handler derives velocity from the last few input
 * events, so events stamped unevenly (a stall, then a burst 1 ms apart) can
 * report a huge velocity against the drag: a rightward open drag read as
 * -11,000 px/s and shut the drawer. Such a fling is noise, so the position
 * decides instead: open when more than half the panel shows.
 *
 * `position` is the panel's translateX: 0 fully open, -panelWidth fully shut.
 */
export function shouldSettleDrawerOpen(
    position: number,
    translation: number,
    velocity: number,
    panelWidth: number
): boolean {
    'worklet'
    const isFlingTrusted = velocity * translation >= 0
    if (isFlingTrusted && Math.abs(velocity) > FLING_VELOCITY) return velocity > 0
    return position > -panelWidth / 2
}
