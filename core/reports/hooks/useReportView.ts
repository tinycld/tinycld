import { breakpointFor, useBreakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import { useAuth } from '@tinycld/core/lib/auth'
import { log } from '@tinycld/core/lib/logger'
import { getResolvedAddress } from '@tinycld/core/lib/server-address'
import { useToastStore } from '@tinycld/core/lib/stores/toast-store'
import { useOrgInfo } from '@tinycld/core/lib/use-org-info'
import * as Clipboard from 'expo-clipboard'
import { router, useFocusEffect, useIsFocused, useLocalSearchParams } from 'expo-router'
import { useCallback, useEffectEvent, useMemo } from 'react'
import type { ReportAccess } from '../lib/access'
import { todayIso } from '../lib/dates'
import { DEFAULT_DISPLAY, type DisplayOptions, shownColumns, zoneLine } from '../lib/format'
import { panelSurface } from '../lib/panel-surface'
import { chipLabelAsks } from '../lib/param-options'
import {
    controlItems,
    emptyAction,
    emptyMessage,
    firstDateParam,
    type ParamValue,
    periodWords,
    postedParams,
    withoutOptional,
} from '../lib/params'
import { HUB_BACK_LABEL } from '../lib/report-back'
import {
    DEFAULT_EXPANSION,
    type Expansion,
    expansionOptions,
    foldAllLabel,
    foldAllTarget,
    hasRows,
    toggleGroup,
    visibleRows,
    withPendingOpen,
} from '../lib/rows'
import { type RunBody, refusalSortProblem, type ViewSort } from '../lib/run-client'
import {
    type BrokenMark,
    brokenProblems,
    type DisplayFit,
    type DisplayLayout,
    displayFitOf,
    type ParamDef,
    type ReportSpec,
} from '../lib/spec'
import { rowLevelsOf } from '../lib/table-lines'
import type { Notice, ResolvedParam, ResultColumn, ResultRow, RunResult } from '../lib/types'
import {
    clearedDrillKeys,
    decodeSort,
    encodeSort,
    nextSort,
    parseViewOptions,
    reportLink,
    SORT_KEY,
    serializeViewOptions,
    type ViewOptions,
} from '../lib/view-url'
import { type ReportSurface, type ReportUIScope, useReportUIStore } from '../stores/report-ui-store'
import { type OptionsSource, useParamOptionLabels } from './useParamOptions'
import { type ReportExportModel, useReportExport } from './useReportExport'
import { type ReportRun, useReportRun } from './useReportRun'

export type ViewSource = { kind: 'saved'; reportId: string } | { kind: 'draft'; spec: ReportSpec }
export type ViewState =
    | 'loading'
    | 'incomplete'
    | 'error'
    | 'timeout'
    | 'invalid'
    | 'missing'
    | 'no-access'
    | 'empty'
    | 'ready'

const NO_COLUMNS: ResultColumn[] = []
const NO_NOTICES: Notice[] = []
const NO_ROWS: ResultRow[] = []
const NO_LAYOUTS: readonly DisplayLayout[] = []
const NO_RESOLVED: ResolvedParam[] = []

function viewState(input: {
    source: ViewSource | null
    broken: BrokenMark | null
    isEditing: boolean
    run: ReportRun
}): ViewState {
    if (input.broken && !input.isEditing) return 'missing'
    if (!input.source) return input.isEditing ? 'incomplete' : 'loading'
    const { run } = input
    if (run.state === 'idle' || run.state === 'loading') return 'loading'
    // A failed live re-run keeps the figures it had: they stay in view, not an error.
    const hasFigures = run.result !== undefined
    if (run.state !== 'ready' && !(run.state === 'error' && hasFigures)) return run.state
    return run.result && hasRows(run.result.rows) ? 'ready' : 'empty'
}

// The reader's sort goes before the report's own (F8); the client sends one key.
function sortOptions(sort: ViewSort | null): { sort?: ViewSort[] } {
    return sort ? { sort: [sort] } : {}
}

function runBody(
    source: ViewSource | null,
    defs: readonly ParamDef[],
    options: ViewOptions,
    expansion: Expansion
): RunBody | null {
    if (!source) return null
    const common = {
        params: postedParams(defs, options.params),
        options: {
            today: todayIso(),
            ...expansionOptions(expansion),
            ...sortOptions(options.sort),
        },
    }
    return source.kind === 'saved'
        ? { reportId: source.reportId, ...common }
        : { spec: source.spec, ...common }
}

// The source's heading (E9), else the organization's name. Which one is known only
// once a run answers, so the line stays empty until then rather than showing the
// organization's name and then changing.
function headingOf(result: RunResult | undefined, orgName: string | undefined): string {
    if (!result) return ''
    return result.heading || orgName || ''
}

// A param's options are read against what the view runs: the saved report, or the draft.
function optionsSourceOf(source: ViewSource | null): OptionsSource | null {
    if (!source) return null
    return source.kind === 'saved' ? { reportId: source.reportId } : { spec: source.spec }
}

export interface ReportViewInput {
    /** Keys the open groups: the report id, or NEW_REPORT_KEY for an unsaved draft. */
    reportKey: string
    title: string
    defs: readonly ParamDef[]
    source: ViewSource | null
    access: ReportAccess
    broken: BrokenMark | null
    isEditing: boolean
    onBack(): void
    /** The back link's text; absent: "Reports" (the hub). */
    backLabel?: string
    onEdit?(): void
    /** Why a report that cannot run cannot be fixed here either (its source is gone). */
    missingNote?: string | null
    onShare?(): void
    onSaveAs?(): void
    /**
     * The spec whose rows the view draws (the saved report's, or the draft): its line
     * layouts per width (E14), how deep its rows go, and which group rows end in a
     * subtotal row.
     */
    spec?: ReportSpec | null
    /**
     * The key prefix of this view's options in the route (the drill panel's is
     * DRILL_PREFIX), so two views on one route never read or write each other's.
     */
    urlPrefix?: string
    /** The UI store scope of this view's popovers and Customize. */
    uiScope?: ReportUIScope
    /** The view's own width (the 480 px drill panel); absent: the window's. */
    viewWidth?: number
}

export function useReportView(input: ReportViewInput) {
    const { reportKey, defs, source, broken, isEditing } = input
    const prefix = input.urlPrefix ?? ''
    const scope = input.uiScope ?? 'main'
    const { user } = useAuth()
    const search = useLocalSearchParams()
    const options = parseViewOptions(defs, search, prefix)
    const expansion = useReportUIStore(s => s.expansion[reportKey]) ?? DEFAULT_EXPANSION
    const setExpansion = useReportUIStore(s => s.setExpansion)
    const body = broken && !isEditing ? null : runBody(source, defs, options, expansion)
    const run = useReportRun({ body, userId: user.id })
    const windowBreakpoint = useBreakpoint()
    // A view narrower than the window (the drill panel) lays itself out by its own width.
    const breakpoint =
        input.viewWidth === undefined ? windowBreakpoint : breakpointFor(input.viewWidth)
    // The builder and Customize draw on this surface; the view reads it too.
    const panel = panelSurface(breakpoint)
    const { org } = useOrgInfo()
    // A tab left for another package stays mounted (frozen); only the focused
    // screen draws surfaces.
    const isFocused = useIsFocused()
    const storedSurface = useReportUIStore(s => s.scopes[scope].surface)
    const openSurface = useReportUIStore(s => s.openSurface)
    const closeSurface = useReportUIStore(s => s.closeSurface)
    const closeSurfaceOf = useReportUIStore(s => s.closeSurfaceOf)
    const isCustomizeOpen = useReportUIStore(s => s.scopes[scope].isCustomizeOpen) && isFocused
    const openCustomize = useReportUIStore(s => s.openCustomize)
    const closeCustomize = useReportUIStore(s => s.closeCustomize)
    const closeShare = useReportUIStore(s => s.closeShare)
    const closeEmail = useReportUIStore(s => s.closeEmail)
    const rows = run.result?.rows ?? NO_ROWS
    const allColumns = run.result?.columns ?? NO_COLUMNS
    const columns = shownColumns(allColumns, options.display.hidden)
    const resolved: Record<string, ResolvedParam> = Object.fromEntries(
        (run.result?.params ?? []).map(p => [p.id, p])
    )
    const period = firstDateParam(defs)
    const periodResolved = period ? resolved[period.id] : undefined
    const action = emptyAction(defs, options.params)
    const optionsSource = optionsSourceOf(source)
    const posted = postedParams(defs, options.params)
    const optionLabels = useParamOptionLabels(
        optionsSource,
        chipLabelAsks(defs, options.params),
        posted
    )
    const state = viewState({ source, broken, isEditing, run })
    const exportModel = useReportExport({
        source,
        body,
        display: options.display,
        state,
        isUpdating: run.isUpdating,
        isEditing,
        isMainView: prefix === '',
        isFocused,
        title: input.title,
        defs,
        resolved: run.result?.params ?? NO_RESOLVED,
    })

    // setParams replaces the history entry, so a change adds none; defaults are left out.
    const write = (next: ViewOptions) => router.setParams(serializeViewOptions(defs, next, prefix))
    // A preset or a picked value closes its popover; a typed custom date keeps it open,
    // so a date error stays in view.
    const onApplyParam = (id: string, value: ParamValue, keepOpen = false) => {
        write({ ...options, params: { ...options.params, [id]: value } })
        if (!keepOpen) closeSurface(scope)
    }
    const onRemoveParam = (id: string) => {
        write({
            ...options,
            params: Object.fromEntries(
                Object.entries(options.params).filter(([key]) => key !== id)
            ),
        })
        closeSurface(scope)
    }
    const onEmptyAction = () => {
        if (action?.kind === 'clear-filters')
            write({ ...options, params: withoutOptional(defs, options.params) })
        if (action?.kind === 'preset')
            onApplyParam(action.paramId, { kind: 'preset', preset: action.preset })
    }
    // Only the main view copies a link (the panel's options are the full report's,
    // one press away).
    const onCopyLink =
        source?.kind === 'saved' && prefix === ''
            ? () => {
                  const link = reportLink(
                      getResolvedAddress() ?? '',
                      source.reportId,
                      defs,
                      options
                  )
                  Clipboard.setStringAsync(link)
                      .then(() =>
                          useToastStore.getState().addToast({
                              title: 'Link copied',
                              variant: 'success',
                              duration: 3000,
                          })
                      )
                      .catch((error: unknown) => {
                          log.warn('reports.copy-link', 'could not copy the link', { error })
                          useToastStore.getState().addToast({
                              title: 'Could not copy the link',
                              variant: 'error',
                              duration: 5000,
                          })
                      })
              }
            : null

    // A surface left open would come back with this screen, or show over the next one.
    const closeOnBlur = useEffectEvent(() => {
        closeSurface(scope)
        closeCustomize(scope)
        // Share and Email are global dialogs that only the main view opens; a panel view
        // must not close them.
        if (scope === 'main') {
            closeShare()
            closeEmail()
        }
    })
    useFocusEffect(useCallback(() => () => closeOnBlur(), []))

    // Kept while the run's rows and the open groups stay, so the table redraws
    // its rows only when they change. The spec says which group rows end in a
    // subtotal row, also before a later page brings it.
    const viewSpec = input.spec ?? null
    const shownRows = useMemo(() => visibleRows(rows, viewSpec), [rows, viewSpec])
    // The group the reader just opened shows open at once, with a spinner, until
    // the run that opens it lands (findings: a slow statement left the press silent).
    const pendingRows = useMemo(
        () => withPendingOpen(shownRows, expansion, viewSpec, run.isUpdating),
        [shownRows, expansion, viewSpec, run.isUpdating]
    )
    const onToggleGroup = useCallback(
        (row: ResultRow) => setExpansion(reportKey, toggleGroup(expansion, row, rows)),
        [setExpansion, reportKey, expansion, rows]
    )
    // Changes only with the sort, so the table's headings redraw only then. setParams
    // keeps the other params, so the press writes the sort key alone.
    const sortText = options.sort ? encodeSort(options.sort) : undefined
    const onSortColumn = useCallback(
        (col: string) => {
            const next = nextSort(decodeSort(sortText), col)
            router.setParams({ [`${prefix}${SORT_KEY}`]: next ? encodeSort(next) : undefined })
        },
        [sortText, prefix]
    )

    return {
        title: input.title,
        access: input.access,
        isEditing,
        onBack: input.onBack,
        backLabel: input.backLabel ?? HUB_BACK_LABEL,
        onEdit: input.onEdit ?? null,
        missingNote: input.missingNote ?? null,
        onShare: input.onShare ?? null,
        onSaveAs: input.onSaveAs ?? null,
        options,
        display: options.display,
        state,
        // On a phone the builder's full-height sheet shows the report in its Preview,
        // so the view under it does not draw a second copy.
        isBodyShown: !(isEditing && panel === 'sheet'),
        problems: broken && !isEditing ? brokenProblems(broken) : run.problems,
        run,
        rows: pendingRows,
        columns,
        // Required in the model, so a hand-built model states its line too (B4).
        zoneWords: zoneLine(columns),
        allColumns,
        notices: run.result?.notices ?? NO_NOTICES,
        isUpdating: run.isUpdating,
        // While the builder lies over the view (a sheet on a phone, a drawer on a
        // tablet) it shows "Updating…" in its own header; the view's slot then
        // stays, empty and silent, so one live region speaks.
        isUpdatingInBuilder: isEditing && panel !== 'inline',
        breakpoint,
        viewWidth: input.viewWidth,
        panelSurface: panel,
        layouts: input.spec?.display?.layouts ?? NO_LAYOUTS,
        fit: displayFitOf(input.spec),
        // Required in the model, so a model built by hand states its levels too
        // (none would draw the label at a flat report's floor). No spec yet: flat.
        rowLevels: rowLevelsOf(input.spec ?? null),
        heading: headingOf(run.result, org?.name),
        optionsSource,
        postedParams: posted,
        optionLabels,
        periodWords: periodWords(period, periodResolved),
        emptyMessage: emptyMessage(period, periodResolved),
        emptyAction: action,
        onEmptyAction,
        defs,
        controls: controlItems(defs, options.params),
        resolved,
        foldAllLabel: foldAllLabel(rows),
        onFoldAll: () => setExpansion(reportKey, foldAllTarget(rows)),
        onToggleGroup,
        surface: isFocused ? storedSurface : null,
        onOpenSurface: (surface: ReportSurface) => openSurface(surface, scope),
        onCloseSurface: () => closeSurface(scope),
        onCloseSurfaceOf: (kind: ReportSurface['kind']) => closeSurfaceOf(kind, scope),
        onApplyParam,
        onRemoveParam,
        isCustomizeOpen,
        // One side surface at a time: the main view's Customize closes the drill panel
        // (a figure press closes Customize, useOpenDrill).
        onOpenCustomize: () => {
            openCustomize(scope)
            if (scope === 'main') router.setParams(clearedDrillKeys(search))
        },
        onCloseCustomize: () => closeCustomize(scope),
        onDisplayChange: (display: DisplayOptions) => write({ ...options, display }),
        // The reader's params stay for the run; only the number display goes back.
        onResetDisplay: () => write({ ...options, display: DEFAULT_DISPLAY }),
        sort: options.sort,
        // A new sort is a new rows key (rowsKeyOf), so the list starts at the top (F11).
        onSortColumn,
        onClearSort: () => write({ ...options, sort: null }),
        onSetSort: (sort: ViewSort | null) => {
            write({ ...options, sort })
            closeSurface(scope)
        },
        // A run refused for the reader's sort offers Clear sort (R8).
        sortProblem: refusalSortProblem(run.problems),
        onCopyLink,
        // Null where Export is not offered (a draft, the builder, the drill panel).
        export: exportModel,
    }
}

// onSetSort, fit, backLabel, viewWidth and export are optional: a hand-built model
// has none of them; then the Sort chip is not shown, the table takes the default
// fit, the back link reads Reports, the view's width is the window's and the
// toolbar has no Export.
export type ReportViewModel = Omit<
    ReturnType<typeof useReportView>,
    'onSetSort' | 'fit' | 'backLabel' | 'viewWidth' | 'export'
> & {
    onSetSort?(sort: ViewSort | null): void
    fit?: DisplayFit
    backLabel?: string
    viewWidth?: number
    export?: ReportExportModel | null
}
