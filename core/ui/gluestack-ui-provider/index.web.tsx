'use client'
import { OverlayProvider } from '@gluestack-ui/core/overlay/creator'
import { ToastProvider } from '@gluestack-ui/core/toast/creator'
import { COLOR_THEMES, DEFAULT_COLOR_THEME } from '@tinycld/core/lib/color-themes'
import React, { useEffect, useLayoutEffect } from 'react'
import { Uniwind } from 'uniwind'
import { script } from './script'

export type ModeType = 'light' | 'dark' | 'system'

const useSafeLayoutEffect = typeof window !== 'undefined' ? useLayoutEffect : useEffect

const MODES: ReadonlySet<ModeType> = new Set<ModeType>(['light', 'dark', 'system'])
const COLOR_THEME_SLUGS: ReadonlySet<string> = new Set(COLOR_THEMES.map(t => t.slug))

// The theme-init script is injected raw into an inline <script> before first
// paint, and mode/colorTheme originate from user_preferences via unvalidated
// `as` casts. Validate both against their allowlists so a crafted value can't
// break out of the injected string literal (stored self-XSS).
const safeMode = (mode: string): ModeType =>
    MODES.has(mode as ModeType) ? (mode as ModeType) : 'dark'
const safeColorTheme = (colorTheme?: string): string =>
    colorTheme && COLOR_THEME_SLUGS.has(colorTheme) ? colorTheme : DEFAULT_COLOR_THEME

export function GluestackUIProvider({
    mode = 'dark',
    colorTheme,
    ...props
}: {
    mode?: ModeType
    colorTheme?: string
    children?: React.ReactNode
}) {
    const validMode = safeMode(mode)
    const validColorTheme = safeColorTheme(colorTheme)

    const handleMediaQuery = React.useCallback(
        (e: MediaQueryListEvent) => {
            const resolvedMode = e.matches ? 'dark' : 'light'
            script(resolvedMode, validColorTheme)
            Uniwind.setTheme(resolvedMode)
        },
        [validColorTheme]
    )

    useSafeLayoutEffect(() => {
        if (validMode === 'system') return
        script(validMode, validColorTheme)
        Uniwind.setTheme(validMode)
    }, [validMode, validColorTheme])

    useSafeLayoutEffect(() => {
        if (validMode !== 'system') return
        const media = window.matchMedia('(prefers-color-scheme: dark)')
        media.addListener(handleMediaQuery)
        return () => media.removeListener(handleMediaQuery)
    }, [handleMediaQuery, validMode])

    return (
        <>
            <script
                suppressHydrationWarning
                // biome-ignore lint/security/noDangerouslySetInnerHtml: static, self-authored theme-init script (serialized local function) that must run before first paint to prevent a theme flash; mode/colorTheme are allowlist-validated and JSON.stringify-escaped, so no user input reaches the markup
                dangerouslySetInnerHTML={{
                    __html: `(${script.toString()})(${JSON.stringify(validMode)},${JSON.stringify(validColorTheme)})`,
                }}
            />
            <OverlayProvider>
                <ToastProvider>{props.children}</ToastProvider>
            </OverlayProvider>
        </>
    )
}
