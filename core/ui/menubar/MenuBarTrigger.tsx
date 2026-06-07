import { Menu } from '@tinycld/core/ui/menu'
import { Pressable, Text } from 'react-native'
import { useMenuBarScope } from './MenuBarScopeContext'
import { menuBarRegistryId, useOpenMenuBarId } from './menubar-store'
import { useOpenMenuStore } from './open-menu-store'

interface MenuBarTriggerProps {
    label: string
    menuId: string
    isDisabled?: boolean
}

// MenuBarTrigger is the styled label-only button that opens one of the
// menubar menus. Hovering it while another *menubar* menu is already
// open swaps to this one — that's the Sheets/Excel menubar feel where
// the user runs the pointer along the row and the popovers slide
// along. The hover is no-op when no menubar menu is open (a cold
// cursor passing the row doesn't start opening menus) and also no-op
// when a non-menubar menu — e.g. a toolbar color picker — is open,
// since the user is interacting with a different control.
//
// When `isDisabled`, the trigger renders greyed-out, swallows hover-swap,
// and the underlying Menu.Trigger receives `disableClick` so the popover
// never opens. Used by anon/read-only share viewers to surface that the
// menus exist but no actions are available.
export function MenuBarTrigger({ label, menuId, isDisabled = false }: MenuBarTriggerProps) {
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
        <Menu.Trigger disableClick={isDisabled}>
            <Pressable
                accessibilityRole="button"
                accessibilityLabel={label}
                accessibilityState={{ disabled: isDisabled }}
                disabled={isDisabled}
                onHoverIn={handleHoverIn}
                className={`px-3 h-7 justify-center rounded ${
                    isDisabled ? 'opacity-40' : 'hover:bg-surface-secondary'
                }`}
                {...(typeof document !== 'undefined' ? { 'data-tinycld-menu': 'trigger' } : {})}
            >
                <Text className="text-sm text-foreground">{label}</Text>
            </Pressable>
        </Menu.Trigger>
    )
}
