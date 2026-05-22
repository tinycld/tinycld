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
