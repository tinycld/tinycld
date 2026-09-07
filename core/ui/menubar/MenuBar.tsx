import { type ReactNode, useId } from 'react'
import { View } from 'react-native'
import { MenuBarAllMenusDisabledContext, MenuBarScopeContext } from './MenuBarScopeContext'

interface MenuBarProps {
    children: ReactNode
    /** When true, every <MenuBarMenu> inside renders its trigger greyed-out
     *  and non-opening. Used by read-only share viewers (anon links) so the
     *  menu structure is visible but no actions are available. */
    allMenusDisabled?: boolean
}

// MenuBar is the styled row that hosts <MenuBarMenu> children. It provides
// the consistent 28px-tall / sm-text / bottom-bordered presentation.
// Outside-press dismissal is each menu's own, through the overlay engine.
//
// Provides a stable per-instance scope so this menubar's menus key into the
// shared open-menu registry distinctly from any other menubar mounted at the
// same time (see MenuBarScopeContext).
export function MenuBar({ children, allMenusDisabled = false }: MenuBarProps) {
    const scope = useId()
    return (
        <MenuBarScopeContext.Provider value={scope}>
            <MenuBarAllMenusDisabledContext.Provider value={allMenusDisabled}>
                <View
                    className="flex-row items-center bg-background border-b border-border"
                    style={{ height: 28, paddingHorizontal: 4 }}
                >
                    {children}
                </View>
            </MenuBarAllMenusDisabledContext.Provider>
        </MenuBarScopeContext.Provider>
    )
}
