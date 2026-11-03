import type { Breakpoint } from '@tinycld/core/components/workspace/useBreakpoint'

/**
 * Where a report's side panel (the builder, Customize) draws: a sheet over the
 * whole report on a phone, a drawer over it on a tablet, inline beside it on a
 * desktop. The view decides from the same value what the panel covers (its body,
 * its "Updating…" slot), so this is the one place the rule lives.
 */
export type PanelSurface = 'sheet' | 'drawer' | 'inline'

export function panelSurface(breakpoint: Breakpoint): PanelSurface {
    if (breakpoint === 'mobile') return 'sheet'
    if (breakpoint === 'tablet') return 'drawer'
    return 'inline'
}
