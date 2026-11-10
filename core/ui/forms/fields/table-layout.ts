import type { ViewStyle } from 'react-native'

/** The gap between the wide grid's slots, px: one size so a host's own row can line up with them. */
export const TABLE_GRID_GAP = 8

/** A form field cell's border, px: always drawn (transparent when unselected) so selecting never shifts layout. */
export const FIELD_CELL_BORDER_WIDTH = 2

/** The row menu's box at the end of each grid row, px; the header's spacer is as wide so the slots line up. */
export const TABLE_ROW_MENU_WIDTH = 40

/** What sizes one wide-grid slot: a host column's weight and floor. */
interface SlotSize {
    flex?: number
    minWidth?: number
}

/**
 * A slot's wide-grid style, the one source of its size: its weight (default 1)
 * from basis 0, and its floor (default 0). Flex resolves the slot to the larger
 * of its floor and its weighted share. A floor of 0 also stops a cell's content
 * from widening a slot past its share.
 */
export function slotStyle({ flex, minWidth }: SlotSize): ViewStyle {
    return { flex: flex ?? 1, minWidth: minWidth ?? 0 }
}
