import type { ReactNode } from 'react'
import { View } from 'react-native'

export interface ShortcutsProviderProps {
    children: ReactNode
}

/**
 * Android: no hardware-keyboard shortcut capture.
 *
 * The default native implementation (provider.native.tsx) wraps the app in
 * react-native-external-keyboard's <KeyboardExtendedView> to receive physical
 * key events. On Android that view only receives keys by being a *focusable*
 * native view, and at the app root its autoFocus/canBeFocused grab Android view
 * focus app-wide — so a tapped TextInput never becomes the IME's served view
 * and the soft keyboard never opens. The library has no focus-free global
 * capture mode, so on Android we forgo hardware shortcuts (a marginal feature on
 * touch devices) and render children directly to keep text input working.
 *
 * Metro resolves this file for Android (.android beats .native); other native
 * platforms use provider.native.tsx, web uses provider.web.tsx.
 */
export function ShortcutsProvider({ children }: ShortcutsProviderProps) {
    return <View className="flex-1">{children}</View>
}
