import { forwardRef } from 'react'
import { Pressable, Text, type View } from 'react-native'
import { useMenuBarScope } from './MenuBarScopeContext'
import { menuBarRegistryId, useOpenMenuBarId } from './menubar-store'
import { useOpenMenuStore } from './open-menu-store'

interface MenuBarTriggerProps {
    label: string
    menuId: string
    isDisabled?: boolean
    /** Injected by the Menu that this trigger opens. */
    onPress?: () => void
}

// The label-only button that opens one of the menubar menus. Hovering it
// while another *menubar* menu is already open swaps to this one — the
// Sheets/Excel feel where the pointer runs along the row and the popovers
// slide along. No-op when nothing is open (a cold cursor passing the row
// does not start opening menus) and when a non-menubar menu is open, since
// the user is on a different control.
//
// A forwardRef Pressable, because the Menu clones its trigger with the ref
// it measures and the `onPress` that toggles it.
export const MenuBarTrigger = forwardRef<View, MenuBarTriggerProps>(function MenuBarTrigger(
    { label, menuId, isDisabled = false, onPress },
    ref
) {
    const scope = useMenuBarScope()
    const openMenuBarId = useOpenMenuBarId(scope)
    const open = useOpenMenuStore(s => s.open)

    const handleHoverIn = () => {
        if (isDisabled) return
        if (openMenuBarId != null && openMenuBarId !== menuId) {
            open(menuBarRegistryId(menuId, scope))
        }
    }

    return (
        <Pressable
            ref={ref}
            accessibilityRole="button"
            accessibilityLabel={label}
            accessibilityState={{ disabled: isDisabled }}
            disabled={isDisabled}
            onPress={isDisabled ? undefined : onPress}
            onHoverIn={handleHoverIn}
            className={`px-3 h-7 justify-center rounded ${
                isDisabled ? 'opacity-40' : 'hover:bg-surface-secondary'
            }`}
        >
            <Text className="text-sm text-foreground">{label}</Text>
        </Pressable>
    )
})
