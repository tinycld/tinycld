import { DarkTheme, DefaultTheme, ThemeProvider } from '@react-navigation/native'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useThemePreference } from '@tinycld/core/lib/use-theme-preference'
import type { ReactNode } from 'react'

// React Navigation paints every navigator scene with its theme's
// `colors.background`, and Expo Router mounts navigators under the built-in
// `DefaultTheme` — whose background is grey95 (rgb 242,242,242). That light
// grey never switches with our dark mode, so it bleeds through any scene that
// leaves its own background transparent (most visibly the "read" mail rows,
// which are intentionally transparent). Wiring the navigation theme to our
// resolved color scheme + design tokens keeps scene backgrounds in sync with
// the rest of the app.
export function NavigationThemeProvider({ children }: { children: ReactNode }) {
    const { resolved } = useThemePreference()
    const background = useThemeColor('background')
    const card = useThemeColor('surface-secondary')
    const text = useThemeColor('foreground')
    const border = useThemeColor('border')
    const primary = useThemeColor('primary')

    const base = resolved === 'dark' ? DarkTheme : DefaultTheme
    const theme = {
        ...base,
        colors: { ...base.colors, background, card, text, border, primary },
    }

    return <ThemeProvider value={theme}>{children}</ThemeProvider>
}
