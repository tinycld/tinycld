import { OverlayProvider } from '@gluestack-ui/core/overlay/creator'
import { ToastProvider } from '@gluestack-ui/core/toast/creator'
import type { ColorThemeSlug } from '@tinycld/core/lib/color-themes'
import { findColorTheme } from '@tinycld/core/lib/color-themes'
import { OverlayProvider as SurfaceOverlayProvider } from '@tinycld/core/ui/overlay'
import type React from 'react'
import { useEffect } from 'react'
import { View, type ViewProps } from 'react-native'
import { Uniwind } from 'uniwind'

export type ModeType = 'light' | 'dark' | 'system'

export function GluestackUIProvider({
    mode = 'dark',
    colorTheme,
    ...props
}: {
    mode?: ModeType
    colorTheme?: string
    children?: React.ReactNode
    style?: ViewProps['style']
}) {
    useEffect(() => {
        Uniwind.setTheme(mode === 'system' ? 'system' : mode)
    }, [mode])

    useEffect(() => {
        if (!colorTheme) return
        const theme = findColorTheme(colorTheme as ColorThemeSlug)
        const resolvedMode = mode === 'system' ? 'light' : mode
        const vars = resolvedMode === 'dark' ? theme.dark : theme.light
        Uniwind.updateCSSVariables(resolvedMode, vars)
    }, [colorTheme, mode])

    return (
        <View style={[{ flex: 1, height: '100%', width: '100%' }, props.style]}>
            {/* Our own overlay host (dialogs, sheets, popovers, menus) wraps
                gluestack's, which now serves only its toasts and drawers.
                OUTSIDE it, not inside: gluestack renders a drawer's content as
                a sibling of its own children, so a menu opened from a drawer
                must find our provider above that level. Our root host also
                renders after gluestack's portal items, so it paints above
                them. See docs/overlays.md. */}
            <SurfaceOverlayProvider>
                <OverlayProvider>
                    <ToastProvider>{props.children}</ToastProvider>
                </OverlayProvider>
            </SurfaceOverlayProvider>
        </View>
    )
}
