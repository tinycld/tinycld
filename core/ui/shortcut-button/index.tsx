import { Tooltip } from '@tinycld/core/components/Tooltip'
import { ariaKeyShortcuts, formatHint, type ShortcutHint } from '@tinycld/core/lib/shortcuts/hint'
import type { ScopeOwner } from '@tinycld/core/lib/shortcuts/scopes'
import { useWritesAvailable } from '@tinycld/core/lib/use-writes-available'
import { Button, ButtonIcon, ServerActionButton } from '@tinycld/core/ui/button'
import type { ComponentProps } from 'react'
import { Platform } from 'react-native'
import { ShortcutFace } from './ShortcutFace'
import { type ButtonShortcut, useButtonShortcut } from './useButtonShortcut'
import { usePressFlash } from './usePressFlash'

export type { ButtonShortcut } from './useButtonShortcut'

type ButtonProps = ComponentProps<typeof Button>

export interface ShortcutButtonProps {
    label: string
    shortcut: ButtonShortcut
    onPress: () => void
    /**
     * Render a ServerActionButton: disabled, shortcut included, while a write
     * cannot reach the server.
     */
    serverAction?: boolean
    icon?: ComponentProps<typeof ButtonIcon>['as']
    variant?: ButtonProps['variant']
    size?: ButtonProps['size']
    isDisabled?: boolean
    className?: string
    testID?: string
    accessibilityHint?: string
    /** Required for a non-global shortcut outside a dialog: the screen's `useShortcutScope()`. */
    scopeOwner?: ScopeOwner
}

/**
 * A button with a keyboard shortcut. Pressing the shortcut presses the
 * button; holding ⌘ (Ctrl off Apple) swaps the label for the shortcut; the
 * tooltip names it. The hint and tooltip are web only. See
 * docs/keyboard-shortcuts.md, "Shortcut buttons".
 */
export function ShortcutButton({
    label,
    shortcut,
    onPress,
    serverAction = false,
    icon,
    isDisabled = false,
    testID,
    accessibilityHint,
    variant,
    size,
    className,
    scopeOwner,
}: ShortcutButtonProps) {
    const writes = useWritesAvailable()
    const flash = usePressFlash()
    useButtonShortcut({
        label,
        shortcut,
        isDisabled: isDisabled || (serverAction && !writes.available),
        onPress: () => {
            flash.flash()
            onPress()
        },
        scopeOwner,
    })
    const hint = formatHint(shortcut.keys)
    const a11y = shortcutA11yProps(shortcut.keys, hint, accessibilityHint)
    const ActionButton = serverAction ? ServerActionButton : Button

    return (
        <Tooltip label={`${label} ${hint.text}`}>
            <ActionButton
                {...a11y}
                onPress={onPress}
                isDisabled={isDisabled}
                isPressed={flash.isOn}
                variant={variant}
                size={size}
                className={className}
                testID={testID}
            >
                <ShortcutIcon icon={icon} />
                <ShortcutFace label={label} hint={hint} />
            </ActionButton>
        </Tooltip>
    )
}

function ShortcutIcon({ icon }: { icon: ShortcutButtonProps['icon'] }) {
    if (!icon) return null
    return <ButtonIcon as={icon} />
}

// Web reads `aria-keyshortcuts`; a native screen reader gets the shortcut in
// the hint, since it has no such attribute.
function shortcutA11yProps(keys: string, hint: ShortcutHint, accessibilityHint?: string) {
    if (Platform.OS !== 'web') {
        return { accessibilityHint: accessibilityHint ?? `Shortcut: ${hint.text}` }
    }
    const aria = ariaKeyShortcuts(keys)
    if (!aria) return { accessibilityHint }
    return { accessibilityHint, 'aria-keyshortcuts': aria }
}
