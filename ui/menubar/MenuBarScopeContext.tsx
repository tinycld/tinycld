import { createContext, useContext } from 'react'

// Per-instance scope for a menubar. Each <MenuBar> generates a stable id
// (useId) and provides it here; MenuBarMenu / MenuBarTrigger fold it into the
// shared open-menu registry key (menubar:<scope>:<menuId>). Two menubars
// mounted at once — e.g. a frozen calc screen still in the DOM behind an
// active text screen — then key their identically-named menus ("format")
// distinctly, so opening one no longer lights up the other.
//
// Default "" keeps a MenuBarMenu used outside any <MenuBar> working, and
// matches the no-scope id form used by the unit tests.
export const MenuBarScopeContext = createContext<string>('')

export function useMenuBarScope(): string {
    return useContext(MenuBarScopeContext)
}

// When a <MenuBar> is rendered with `allMenusDisabled`, every MenuBarMenu
// inside it picks up the flag via this context and renders its trigger
// greyed-out + non-opening. Used by read-only share viewers (anon links)
// so per-menu files (FileMenu, EditMenu, …) don't need to thread a prop.
export const MenuBarAllMenusDisabledContext = createContext<boolean>(false)

export function useMenuBarAllMenusDisabled(): boolean {
    return useContext(MenuBarAllMenusDisabledContext)
}
