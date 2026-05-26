import { type ReactNode, useId } from 'react'
import { View } from 'react-native'
import { MenuBarAllMenusDisabledContext, MenuBarScopeContext } from './MenuBarScopeContext'
import { useOpenMenuOutsideClick } from './use-open-menu-outside-click'

interface MenuBarProps {
    children: ReactNode
    /** When true, every <MenuBarMenu> inside renders its trigger greyed-out
     *  and non-opening. Used by read-only share viewers (anon links) so the
     *  menu structure is visible but no actions are available. */
    allMenusDisabled?: boolean
}

// MenuBar is the styled row that hosts <MenuBarMenu> children. It
// provides the consistent 28px-tall / sm-text / bottom-bordered
// presentation and marks the row with data-tinycld-menu="row" so the
// outside-click handler recognises clicks anywhere within it (e.g.
// between two triggers) as "still inside the menu surface".
//
// Mounts the document-level outside-click handler once, on the
// assumption that any screen with a menubar wants its dropdowns to
// dismiss on outside click. The hook is web-only and idempotent if
// the consumer also calls it elsewhere.
//
// Provides a stable per-instance scope so this menubar's menus key into the
// shared open-menu registry distinctly from any other menubar mounted at the
// same time (see MenuBarScopeContext).
export function MenuBar({ children, allMenusDisabled = false }: MenuBarProps) {
    useOpenMenuOutsideClick()
    const scope = useId()
    return (
        <MenuBarScopeContext.Provider value={scope}>
            <MenuBarAllMenusDisabledContext.Provider value={allMenusDisabled}>
                <View
                    className="flex-row items-center bg-background border-b border-border"
                    style={{ height: 28, paddingHorizontal: 4 }}
                    {...(typeof document !== 'undefined' ? { 'data-tinycld-menu': 'row' } : {})}
                >
                    {children}
                </View>
            </MenuBarAllMenusDisabledContext.Provider>
        </MenuBarScopeContext.Provider>
    )
}
