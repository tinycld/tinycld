// Fixed geometry for the picker.
//
// Popover derives its own maxHeight by measuring scroll content via onLayout.
// A virtualized child reports a size that fights that measurement, so the
// picker declares its dimensions instead of negotiating: the surface is told
// the width, and the grid sits in a fixed-height box.

export const EMOJI_SIZE = 32
export const EMOJI_PER_ROW = 8
export const GRID_PADDING = 8

/** Surface width, matching EMOJI_PER_ROW cells plus padding. */
export const PICKER_WIDTH = EMOJI_PER_ROW * EMOJI_SIZE + GRID_PADDING * 2

/** Height of the scrolling grid alone — the header and nav sit above it. */
export const GRID_HEIGHT = 400 - 44 - 40

/** Row height in the virtualized list. */
export const ROW_HEIGHT = EMOJI_SIZE

/** Category section headers are their own rows in the flattened list. */
export const SECTION_HEADER_HEIGHT = 28
