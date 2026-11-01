import { create } from '@tinycld/core/lib/store'
import { type Expansion, NO_EXPANSION } from '../lib/rows'

// Shared UI state of the reports screens. One chip popover or sheet is open at a
// time; open groups last for the session per report (not persisted, not in the
// URL); the share dialog can be opened from the hub and from a report.

export type ReportSurface = { kind: 'param'; paramId: string } | { kind: 'add-filter' }

/** The expansion key of the builder's draft before it is saved. */
export const NEW_REPORT_KEY = 'new'

interface ReportUIState {
    expansion: Record<string, Expansion>
    surface: ReportSurface | null
    isCustomizeOpen: boolean
    shareReportId: string | null
    setExpansion: (reportKey: string, expansion: Expansion) => void
    openSurface: (surface: ReportSurface) => void
    closeSurface: () => void
    // Closes the surface only while it is still of this kind: a menu closes itself
    // after an item opened the next surface, and that close must not take the new one.
    closeSurfaceOf: (kind: ReportSurface['kind']) => void
    openCustomize: () => void
    closeCustomize: () => void
    openShare: (reportId: string) => void
    closeShare: () => void
    startVisit: (reportKey: string) => void
}

export const useReportUIStore = create<ReportUIState>()(set => ({
    expansion: {},
    surface: null,
    isCustomizeOpen: false,
    shareReportId: null,
    setExpansion: (reportKey, expansion) =>
        set(state => ({ expansion: { ...state.expansion, [reportKey]: expansion } })),
    openSurface: surface => set({ surface }),
    closeSurface: () => set({ surface: null }),
    closeSurfaceOf: kind => set(state => (state.surface?.kind === kind ? { surface: null } : {})),
    openCustomize: () => set({ isCustomizeOpen: true }),
    closeCustomize: () => set({ isCustomizeOpen: false }),
    openShare: reportId => set({ shareReportId: reportId }),
    closeShare: () => set({ shareReportId: null }),
    startVisit: reportKey =>
        set(state => ({
            expansion: { ...state.expansion, [reportKey]: NO_EXPANSION },
            surface: null,
            isCustomizeOpen: false,
        })),
}))
