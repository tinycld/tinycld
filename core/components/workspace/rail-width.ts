// The rail's visible icon column. Its 44pt touch targets are centred in it, so
// there is ~10pt of slack on each side before an icon reaches the rail's edge.
const RAIL_WIDTH = 64

// The rail's ACTUAL on-screen width: the icon column plus however much the
// left safe-area inset pushes it inward.
//
// insetLeft arrives side-corrected (WorkspaceLayout passes useDeviceInsets):
// ~59pt only in the landscape rotation that puts the sensor housing on the
// left, 0 in the other rotation and in portrait. So the rail is 123pt with the
// housing beside it and exactly RAIL_WIDTH everywhere else — the extra chrome
// exists only when hardware forces it.
//
// An earlier version capped the shift at 12pt to keep the width uniform, on the
// theory that the housing "does not intrude into the display's usable area
// beside it". It does: 12pt against a ~59pt housing left the top icons ~47pt
// underneath it, unreadable and untappable. The cap cannot apply to the shift
// alone either — the 44pt (w-11) icons need RAIL_WIDTH of space AFTER the
// padding, so width and shift move together.
//
// The background is unaffected either way — it always reaches the physical edge,
// so there is never a light gutter.
//
// Anything positioned against the rail's right edge — the notification drawer,
// the toast stack — has to agree with it; a hardcoded 64 there leaves a gap
// once the rail widens in landscape. Kept in its own module so those consumers
// (and their unit tests) do not have to load the rail's icon map to read a
// number.
export function railWidth(insetLeft: number): number {
    return RAIL_WIDTH + insetLeft
}
