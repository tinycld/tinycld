import { Menu } from '@tinycld/core/ui/menu'
import type { ReactNode } from 'react'
import { View } from 'react-native'
import { useMenuBarAllMenusDisabled, useMenuBarScope } from './MenuBarScopeContext'
import { MenuBarTrigger } from './MenuBarTrigger'
import { menuBarRegistryId, useIsMenuBarOpen } from './menubar-store'
import { useOpenMenuStore } from './open-menu-store'

interface MenuBarMenuProps {
    menuId: string
    label: string
    children: ReactNode
    /** When true, the trigger renders greyed-out and clicking/hovering
     *  does not open the menu. Children are still wired up but never
     *  mount the popover. */
    isDisabled?: boolean
}

// MenuBarMenu binds one top-level menu to the shared open-menu
// registry. All menubar menus on screen consume the same controlled
// state so that:
//   1. Clicking another trigger (or any toolbar dropdown) while one
//      is open swaps cleanly — opening any menu in the registry
//      implicitly closes whichever was open before.
//   2. Hovering another menubar trigger while one is open swaps
//      without clicking (see MenuBarTrigger's onHoverIn).
//   3. A document-level outside-click handler (mounted via
//      useOpenMenuOutsideClick at the screen root) closes the
//      active menu without each menu rendering its own Menu.Overlay
//      (which would intercept clicks on sibling triggers and break
//      the swap behavior).
//
// The data-tinycld-menu="content" wrapper marks Menu.Content's
// portaled subtree as "part of a tinycld menu", so the document
// handler can recognise clicks that should NOT close (e.g. clicking
// a Menu.Item or a Menu.SubTrigger inside the popover).
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
        >
            <MenuBarTrigger label={label} menuId={menuId} isDisabled={effectiveDisabled} />
            <Menu.Portal>
                <Menu.Content placement="bottom" align="start">
                    <View
                        {...(typeof document !== 'undefined'
                            ? { 'data-tinycld-menu': 'content' }
                            : {})}
                    >
                        {children}
                    </View>
                </Menu.Content>
            </Menu.Portal>
        </Menu>
    )
}
