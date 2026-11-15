import { useWindowSizeStore } from '@tinycld/core/lib/stores/window-size-store'

export type Breakpoint = 'desktop' | 'tablet' | 'mobile'

// The breakpoint of a width: the window's (useBreakpoint), or a view's own (a 480 px
// drill panel draws as a phone).
export function breakpointFor(width: number): Breakpoint {
    if (width >= 1024) return 'desktop'
    if (width >= 768) return 'tablet'
    return 'mobile'
}

// Reads the breakpoint via a primitive selector. Zustand's default
// equality is Object.is, so consumers re-render only when the bucket flips
// — strictly cheaper than the previous useWindowDimensions implementation,
// which fired on every pixel-level resize event for every consumer.
export function useBreakpoint(): Breakpoint {
    return useWindowSizeStore(s => breakpointFor(s.width))
}

export function useWindowWidth(): number {
    return useWindowSizeStore(s => s.width)
}

export function useWindowHeight(): number {
    return useWindowSizeStore(s => s.height)
}
