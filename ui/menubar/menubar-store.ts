import { useOpenMenuStore } from './open-menu-store'

// Selectors against the shared open-menu registry. Top-level menubar
// menus participate in the same single-open pool as the toolbar
// pickers — opening one closes whichever was open elsewhere — but the
// hover-swap behavior still needs to know whether the *currently open*
// menu is itself a menubar menu, so it can hand off to a sibling.
//
// Registry ids are scoped per menubar instance: `menubar:<scope>:<menuId>`.
// The scope is a stable per-MenuBar id (see MenuBarScopeContext) so that two
// menubars that happen to be mounted at the same time — e.g. a frozen calc
// screen still in the tree behind an active text screen — don't collide on a
// shared menuId like "format" and both light up at once.

const MENUBAR_PREFIX = 'menubar:'

export function menuBarRegistryId(menuId: string, scope = ''): string {
    return `${MENUBAR_PREFIX}${scope}:${menuId}`
}

// Returns the bare menuId of the currently-open menu *within the given scope*,
// or null if nothing is open, the open menu is not a menubar menu, or it
// belongs to a different menubar. Drives hover-swap, which only hands off
// between siblings of the same row.
export function useOpenMenuBarId(scope = ''): string | null {
    const scopePrefix = `${MENUBAR_PREFIX}${scope}:`
    return useOpenMenuStore(s => {
        if (s.openId == null) return null
        if (!s.openId.startsWith(scopePrefix)) return null
        return s.openId.slice(scopePrefix.length)
    })
}

export function useIsMenuBarOpen(menuId: string, scope = ''): boolean {
    const registryId = menuBarRegistryId(menuId, scope)
    return useOpenMenuStore(s => s.openId === registryId)
}
