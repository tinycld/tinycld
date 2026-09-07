import { Menu } from '@tinycld/core/ui/menu'
import type { ReactNode } from 'react'
import { useMenuBarAllMenusDisabled, useMenuBarScope } from './MenuBarScopeContext'
import { MenuBarTrigger } from './MenuBarTrigger'
import { menuBarRegistryId, useIsMenuBarOpen } from './menubar-store'
import { useOpenMenuStore } from './open-menu-store'

interface MenuBarMenuProps {
    menuId: string
    label: string
    children: ReactNode
    /** When true, the trigger renders greyed-out and clicking/hovering
     *  does not open the menu. */
    isDisabled?: boolean
}

// MenuBarMenu binds one top-level menu to the shared open-menu registry. All
// menubar menus on screen consume the same controlled state so that clicking
// another trigger (or any toolbar dropdown) while one is open swaps cleanly,
// and hovering another menubar trigger swaps without a click (see
// MenuBarTrigger). Dismissal on an outside press is the Menu's own: the
// overlay engine closes the topmost layer, which drives `close()` here.
//
// Pinned to the popover presentation: a menubar exists only on the desktop
// layout, and its menus belong beside their triggers.
export function MenuBarMenu({ menuId, label, children, isDisabled }: MenuBarMenuProps) {
    const scope = useMenuBarScope()
    const allDisabled = useMenuBarAllMenusDisabled()
    // Per-menu prop overrides context when explicit, otherwise inherit
    // the "all menus disabled" flag set on the parent <MenuBar>.
    const effectiveDisabled = isDisabled ?? allDisabled
    const isOpen = useIsMenuBarOpen(menuId, scope) && !effectiveDisabled
    const open = useOpenMenuStore(s => s.open)
    const close = useOpenMenuStore(s => s.close)
    const registryId = menuBarRegistryId(menuId, scope)

    return (
        <Menu
            isOpen={isOpen}
            onOpenChange={next => {
                if (effectiveDisabled) return
                next ? open(registryId) : close()
            }}
            trigger={
                <MenuBarTrigger label={label} menuId={menuId} isDisabled={effectiveDisabled} />
            }
            placement="bottom-start"
            presentation="popover"
        >
            {children}
        </Menu>
    )
}
