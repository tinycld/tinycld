import { create } from '@tinycld/core/lib/store'
import type { Expansion } from '../lib/rows'
import type { DisplayWidth } from '../lib/spec'

// Shared UI state of the reports screens. One chip popover or sheet is open at a
// time; open groups last for the session per report (not persisted, not in the
// URL); the share dialog can be opened from the hub and from a report; the email
// dialog from a report's Export menu.

export type ReportSurface =
    | { kind: 'param'; paramId: string }
    | { kind: 'add-filter' }
    | { kind: 'sort' }

/** The expansion key of the builder's draft before it is saved. */
export const NEW_REPORT_KEY = 'new'

/** The view a surface belongs to: the report, or the report its drill panel shows (D4). */
export type ReportUIScope = 'main' | 'drill'

interface ScopeState {
    surface: ReportSurface | null
    isCustomizeOpen: boolean
}

const CLOSED: ScopeState = { surface: null, isCustomizeOpen: false }

const scopeOf = (scope: ReportUIScope | undefined): ReportUIScope => scope ?? 'main'

const withScope = (
    state: ReportUIState,
    scope: ReportUIScope | undefined,
    patch: Partial<ScopeState>
) => {
    const key = scopeOf(scope)
    return { scopes: { ...state.scopes, [key]: { ...state.scopes[key], ...patch } } }
}

/** A record panel's title and the record path it names (R13). */
export type RecordTitle = { open: string; title: string }

/** The title the record panel shows for `open`: empty until its component names it. */
export function recordTitleOf(named: RecordTitle | null, open: string): string {
    return named?.open === open ? named.title : ''
}

interface ReportUIState {
    expansion: Record<string, Expansion>
    // Each view keeps its own popover and Customize, so a chip of one id in the
    // panel and in the report never opens the other's.
    scopes: Record<ReportUIScope, ScopeState>
    shareReportId: string | null
    setExpansion: (reportKey: string, expansion: Expansion) => void
    openSurface: (surface: ReportSurface, scope?: ReportUIScope) => void
    closeSurface: (scope?: ReportUIScope) => void
    // Closes the surface only while it is still of this kind: a menu closes itself
    // after an item opened the next surface, and that close must not take the new one.
    closeSurfaceOf: (kind: ReportSurface['kind'], scope?: ReportUIScope) => void
    openCustomize: (scope?: ReportUIScope) => void
    closeCustomize: (scope?: ReportUIScope) => void
    openShare: (reportId: string) => void
    closeShare: () => void
    /** The report whose "Email report" dialog is open; only the main view opens it. */
    emailReportId: string | null
    openEmail: (reportId: string) => void
    closeEmail: () => void
    // Counts figure presses that opened a drill into the panel; the panel title is
    // keyed on it, so only such a press moves the focus there.
    drillOpens: number
    noteDrillOpen: () => void
    // The drillOpens count whose press has moved the focus to the panel title: a mount
    // with no new press (a reload, a pasted link, a return to the report) leaves it.
    drillFocused: number
    noteDrillFocused: () => void
    /** The figure (figureKey) whose press opened the drill panel; its close sends the focus back there. */
    drillOrigin: string | null
    setDrillOrigin: (key: string | null) => void
    // The detail row whose press opened the record panel (D5, R11). Many rows can
    // open one record, so the row id marks the pressed one. Not in the URL: a reload
    // shows the panel with no row marked (Q1).
    openedRow: string | null
    // A row press that opens a record beside the report: marks the row and counts
    // the press (the panel title takes the focus). The report's Customize closes:
    // one side surface at a time. The title is not touched: it belongs to its
    // record path (recordTitle), and a press that opens the same record keeps it.
    openRecord: (rowId: string) => void
    recordOpens: number
    // The recordOpens count whose press has moved the focus to the panel title.
    recordFocused: number
    noteRecordFocused: () => void
    /**
     * The record panel's title, as the package's component names it
     * (usePanelTitle), with the record path it names. The header shows it only
     * while that path is the route's dr (recordTitleOf), so a replaced record never
     * shows the previous title (R13).
     */
    recordTitle: RecordTitle | null
    setRecordTitle: (open: string, title: string) => void
    // A visit opened from the hub starts with no surface open; its open groups stay.
    startVisit: () => void
    /** The builder's open record picker, by its value's test id; one at a time. */
    builderPicker: string | null
    setBuilderPicker: (picker: string | null) => void
    /** The width the builder's Arrange section edits (E14). */
    arrangeWidth: DisplayWidth
    setArrangeWidth: (width: DisplayWidth) => void
}

export const useReportUIStore = create<ReportUIState>()(set => ({
    expansion: {},
    scopes: { main: CLOSED, drill: CLOSED },
    shareReportId: null,
    builderPicker: null,
    drillOpens: 0,
    noteDrillOpen: () => set(state => ({ drillOpens: state.drillOpens + 1 })),
    drillFocused: 0,
    noteDrillFocused: () => set(state => ({ drillFocused: state.drillOpens })),
    drillOrigin: null,
    setDrillOrigin: key => set({ drillOrigin: key }),
    openedRow: null,
    openRecord: rowId =>
        set(state => ({
            ...withScope(state, 'main', { isCustomizeOpen: false }),
            openedRow: rowId,
            recordOpens: state.recordOpens + 1,
        })),
    recordOpens: 0,
    recordFocused: 0,
    noteRecordFocused: () => set(state => ({ recordFocused: state.recordOpens })),
    recordTitle: null,
    setRecordTitle: (open, title) => set({ recordTitle: { open, title } }),
    setBuilderPicker: picker => set({ builderPicker: picker }),
    arrangeWidth: 'mobile',
    setArrangeWidth: width => set({ arrangeWidth: width }),
    setExpansion: (reportKey, expansion) =>
        set(state => ({ expansion: { ...state.expansion, [reportKey]: expansion } })),
    openSurface: (surface, scope) => set(state => withScope(state, scope, { surface })),
    closeSurface: scope => set(state => withScope(state, scope, { surface: null })),
    closeSurfaceOf: (kind, scope) =>
        set(state =>
            state.scopes[scopeOf(scope)].surface?.kind === kind
                ? withScope(state, scope, { surface: null })
                : {}
        ),
    openCustomize: scope => set(state => withScope(state, scope, { isCustomizeOpen: true })),
    closeCustomize: scope => set(state => withScope(state, scope, { isCustomizeOpen: false })),
    openShare: reportId => set({ shareReportId: reportId }),
    closeShare: () => set({ shareReportId: null }),
    emailReportId: null,
    openEmail: reportId => set({ emailReportId: reportId }),
    closeEmail: () => set({ emailReportId: null }),
    startVisit: () =>
        set({
            scopes: { main: CLOSED, drill: CLOSED },
            builderPicker: null,
            drillOrigin: null,
            openedRow: null,
            recordTitle: null,
            recordOpens: 0,
            recordFocused: 0,
        }),
}))
