import { createContext, useContext } from 'react'

// True inside the content of an overlay Drawer. An overlay Drawer is a
// gluestack portal, which draws after the app tree and so above the `sheet`
// overlay host inside the mobile content: a Sheet opened from the drawer reads
// this to render into the root host instead, or the drawer would cover it.
// An inline Drawer sits in the layout (no portal), so it does not provide it.
// Kept apart from the Drawer module so the Sheet can read it without loading
// gluestack's modal creator.
export const DrawerOverlayContext = createContext(false)

/** Whether this component renders inside an overlay Drawer's content. */
export function useIsInOverlayDrawer(): boolean {
    return useContext(DrawerOverlayContext)
}

// The Drawer's onClose, for its content: the swipe gesture on DrawerContent and
// the inline close button use it. Given by context rather than a prop through
// every call site. Ours rather than gluestack's ModalContext: that one is an
// implementation detail of the modal creator, and nothing guarantees it keeps
// carrying handleClose across a version bump.
export const DrawerCloseContext = createContext<(() => void) | undefined>(undefined)

/** The close callback of the Drawer this component renders inside, if any. */
export function useDrawerClose(): (() => void) | undefined {
    return useContext(DrawerCloseContext)
}
