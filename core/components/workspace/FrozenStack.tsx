import { Stack } from 'expo-router'
import type { ReactNode } from 'react'

/**
 * Like `FrozenStack` but uses the platform-default push animation (iOS
 * slide-from-right, Android fade-from-bottom). Use it for drill-down
 * navigators inside a package — list → detail → back — where the slide
 * cues "you're going deeper" / "you're coming back."
 *
 * Accepts optional children so individual screens can override their own
 * animation via `<Stack.Screen name="..." options={{ animation: ... }}/>`.
 */
export function FrozenSlideStack({ children }: { children?: ReactNode }) {
    return (
        <Stack
            screenOptions={{
                headerShown: false,
                freezeOnBlur: true,
                animation: 'default',
            }}
        >
            {children}
        </Stack>
    )
}
