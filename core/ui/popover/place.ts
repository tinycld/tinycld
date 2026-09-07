/**
 * Where an anchored surface goes. Pure arithmetic over rectangles, so every
 * rule here is pinned by `core/tests/unit/place-popover.test.ts` rather than
 * by an e2e run that happens to open a menu near an edge.
 */

export interface Rect {
    x: number
    y: number
    width: number
    height: number
}

export interface Size {
    width: number
    height: number
}

export type Side = 'top' | 'bottom' | 'left' | 'right'
export type Align = 'start' | 'center' | 'end'
/** `bottom-end`, `top`, `right-start`… A bare side aligns `start`. */
export type Placement = Side | `${Side}-${Align}`

export interface PlacementResult {
    /** Window coordinates of the surface's top-left corner. */
    top: number
    left: number
    /** Room on the resolved side; the surface scrolls inside it. */
    maxHeight: number
    maxWidth: number
    /** The side the surface ended up on, after any flip. */
    side: Side
}

const GAP = 4
const MARGIN = 8

export function parsePlacement(placement: Placement): { side: Side; align: Align } {
    const [side, align] = placement.split('-') as [Side, Align | undefined]
    return { side, align: align ?? 'start' }
}

interface PlacePopoverArgs {
    anchor: Rect
    /** Null before the surface has measured itself; the result then assumes a zero-size surface. */
    size: Size | null
    viewport: Size
    placement?: Placement
    /** Space between anchor and surface. */
    gap?: number
    /** Space kept between the surface and the viewport edge. */
    margin?: number
}

/**
 * Preferred side first. If the surface does not fit there, the opposite side
 * if it fits, else whichever side has more room — and the height is capped to
 * that room either way, so a surface taller than the viewport scrolls instead
 * of bouncing between two sides that each clip it.
 */
export function placePopover({
    anchor,
    size,
    viewport,
    placement = 'bottom',
    gap = GAP,
    margin = MARGIN,
}: PlacePopoverArgs): PlacementResult {
    const { side: wanted, align } = parsePlacement(placement)
    const width = size?.width ?? 0
    const height = size?.height ?? 0

    const room = {
        top: anchor.y - gap - margin,
        bottom: viewport.height - (anchor.y + anchor.height) - gap - margin,
        left: anchor.x - gap - margin,
        right: viewport.width - (anchor.x + anchor.width) - gap - margin,
    }
    const side = resolveSide(wanted, room, side => (isVertical(side) ? height : width))

    let top: number
    let left: number
    if (isVertical(side)) {
        left = alignOn(align, anchor.x, anchor.width, width)
        top = side === 'bottom' ? anchor.y + anchor.height + gap : anchor.y - gap - height
    } else {
        top = alignOn(align, anchor.y, anchor.height, height)
        left = side === 'right' ? anchor.x + anchor.width + gap : anchor.x - gap - width
    }

    const maxHeight = isVertical(side) ? room[side] : viewport.height - 2 * margin
    const maxWidth = isVertical(side) ? viewport.width - 2 * margin : room[side]

    // Clamp into the viewport; a surface taller than its room is capped, so
    // clamping only ever moves it, never hides an edge.
    const shownHeight = Math.min(height, maxHeight)
    const shownWidth = Math.min(width, maxWidth)
    left = clamp(left, margin, Math.max(margin, viewport.width - shownWidth - margin))
    top = clamp(top, margin, Math.max(margin, viewport.height - shownHeight - margin))

    return { top, left, maxHeight, maxWidth, side }
}

function resolveSide(
    wanted: Side,
    room: Record<Side, number>,
    needed: (side: Side) => number
): Side {
    const opposite = OPPOSITE[wanted]
    if (room[wanted] >= needed(wanted)) return wanted
    if (room[opposite] >= needed(opposite)) return opposite
    return room[opposite] > room[wanted] ? opposite : wanted
}

const OPPOSITE: Record<Side, Side> = { top: 'bottom', bottom: 'top', left: 'right', right: 'left' }

function isVertical(side: Side): boolean {
    return side === 'top' || side === 'bottom'
}

function alignOn(align: Align, anchorStart: number, anchorSize: number, size: number): number {
    if (align === 'start') return anchorStart
    if (align === 'end') return anchorStart + anchorSize - size
    return anchorStart + anchorSize / 2 - size / 2
}

function clamp(value: number, min: number, max: number): number {
    return Math.min(Math.max(value, min), max)
}

interface PlaceSubmenuArgs {
    /** The parent surface, window coordinates. */
    parent: Rect
    /** The row that opened the submenu, window coordinates. */
    row: Rect
    size: Size | null
    viewport: Size
    /** How far the submenu overlaps the parent's edge, so the two read as one surface. */
    overlap?: number
    /** The parent's top padding: lifts the submenu so its first row sits level with the trigger row. */
    lift?: number
    margin?: number
}

/**
 * A submenu opens beside its parent, level with the row that opened it, and
 * flips to the parent's other side when the viewport edge is in the way.
 * Result is in window coordinates; the caller converts to the parent's box.
 */
export function placeSubmenu({
    parent,
    row,
    size,
    viewport,
    overlap = 4,
    lift = 4,
    margin = MARGIN,
}: PlaceSubmenuArgs): { top: number; left: number } {
    const width = size?.width ?? 0
    const height = size?.height ?? 0

    let left = parent.x + parent.width - overlap
    if (left + width > viewport.width - margin) left = parent.x - width + overlap
    if (left < margin) left = margin

    let top = row.y - lift
    if (top + height > viewport.height - margin) {
        top = Math.max(margin, viewport.height - height - margin)
    }
    if (top < margin) top = margin

    return { top, left }
}
