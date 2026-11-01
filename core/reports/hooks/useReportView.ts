import { useBreakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import { useAuth } from '@tinycld/core/lib/auth'
import { log } from '@tinycld/core/lib/logger'
import { getResolvedAddress } from '@tinycld/core/lib/server-address'
import { useToastStore } from '@tinycld/core/lib/stores/toast-store'
import { useOrgInfo } from '@tinycld/core/lib/use-org-info'
import * as Clipboard from 'expo-clipboard'
import { router, useFocusEffect, useIsFocused, useLocalSearchParams } from 'expo-router'
import { useCallback, useEffectEvent } from 'react'
import type { ReportAccess } from '../lib/access'
import { todayIso } from '../lib/dates'
import { DEFAULT_DISPLAY, type DisplayOptions, shownColumns } from '../lib/format'
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
import {
    type Expansion,
    expansionOptions,
    foldAllLabel,
    foldAllTarget,
    hasRows,
    NO_EXPANSION,
    toggleGroup,
    visibleRows,
} from '../lib/rows'
import type { RunBody } from '../lib/run-client'
import { type BrokenMark, brokenProblems, type ParamDef, type ReportSpec } from '../lib/spec'
import type { Notice, ResolvedParam, ResultColumn, ResultRow } from '../lib/types'
import {
    parseViewOptions,
    reportLink,
    serializeViewOptions,
    type ViewOptions,
} from '../lib/view-url'
import { useReportUIStore } from '../stores/report-ui-store'
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

function runBody(
    source: ViewSource | null,
    defs: readonly ParamDef[],
    options: ViewOptions,
    expansion: Expansion
): RunBody | null {
    if (!source) return null
    const common = {
        params: postedParams(defs, options.params),
        options: { today: todayIso(), ...expansionOptions(expansion) },
    }
    return source.kind === 'saved'
        ? { reportId: source.reportId, ...common }
        : { spec: source.spec, ...common }
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
    onEdit?(): void
}

export function useReportView(input: ReportViewInput) {
    const { reportKey, defs, source, broken, isEditing } = input
    const { user } = useAuth()
    const search = useLocalSearchParams()
    const options = parseViewOptions(defs, search)
    const expansion = useReportUIStore(s => s.expansion[reportKey]) ?? NO_EXPANSION
    const setExpansion = useReportUIStore(s => s.setExpansion)
    const body = broken && !isEditing ? null : runBody(source, defs, options, expansion)
    const run = useReportRun({ body, userId: user.id })
    const breakpoint = useBreakpoint()
    const { org } = useOrgInfo()
    // A tab left for another package stays mounted (frozen); only the focused
    // screen draws surfaces.
    const isFocused = useIsFocused()
    const storedSurface = useReportUIStore(s => s.surface)
    const openSurface = useReportUIStore(s => s.openSurface)
    const closeSurface = useReportUIStore(s => s.closeSurface)
    const closeSurfaceOf = useReportUIStore(s => s.closeSurfaceOf)
    const isCustomizeOpen = useReportUIStore(s => s.isCustomizeOpen) && isFocused
    const openCustomize = useReportUIStore(s => s.openCustomize)
    const closeCustomize = useReportUIStore(s => s.closeCustomize)
    const rows = run.result?.rows ?? NO_ROWS
    const resolved: Record<string, ResolvedParam> = Object.fromEntries(
        (run.result?.params ?? []).map(p => [p.id, p])
    )
    const period = firstDateParam(defs)
    const periodResolved = period ? resolved[period.id] : undefined
    const action = emptyAction(defs, options.params)

    // setParams replaces the history entry, so a change adds none; defaults are left out.
    const write = (next: ViewOptions) => router.setParams(serializeViewOptions(defs, next))
    // A preset or a picked value closes its popover; a typed custom date keeps it open,
    // so a date error stays in view.
    const onApplyParam = (id: string, value: ParamValue, keepOpen = false) => {
        write({ ...options, params: { ...options.params, [id]: value } })
        if (!keepOpen) closeSurface()
    }
    const onRemoveParam = (id: string) => {
        write({
            ...options,
            params: Object.fromEntries(
                Object.entries(options.params).filter(([key]) => key !== id)
            ),
        })
        closeSurface()
    }
    const onEmptyAction = () => {
        if (action?.kind === 'clear-filters')
            write({ ...options, params: withoutOptional(defs, options.params) })
        if (action?.kind === 'preset')
            onApplyParam(action.paramId, { kind: 'preset', preset: action.preset })
    }
    const onCopyLink =
        source?.kind === 'saved'
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
        closeSurface()
        closeCustomize()
    })
    useFocusEffect(useCallback(() => () => closeOnBlur(), []))

    return {
        title: input.title,
        access: input.access,
        isEditing,
        onBack: input.onBack,
        onEdit: input.onEdit ?? null,
        options,
        display: options.display,
        state: viewState({ source, broken, isEditing, run }),
        // On a phone the builder's full-height sheet shows the report in its Preview,
        // so the view under it does not draw a second copy.
        isBodyShown: !(isEditing && breakpoint === 'mobile'),
        problems: broken && !isEditing ? brokenProblems(broken) : run.problems,
        run,
        rows: visibleRows(rows),
        columns: shownColumns(run.result?.columns ?? NO_COLUMNS, options.display.hidden),
        allColumns: run.result?.columns ?? NO_COLUMNS,
        notices: run.result?.notices ?? NO_NOTICES,
        isUpdating: run.isUpdating,
        breakpoint,
        orgName: org?.name ?? '',
        periodWords: periodWords(period, periodResolved),
        emptyMessage: emptyMessage(period, periodResolved),
        emptyAction: action,
        onEmptyAction,
        defs,
        controls: controlItems(defs, options.params),
        resolved,
        foldAllLabel: foldAllLabel(rows),
        onFoldAll: () => setExpansion(reportKey, foldAllTarget(rows)),
        onToggleGroup: (row: ResultRow) =>
            setExpansion(reportKey, toggleGroup(expansion, row, rows)),
        surface: isFocused ? storedSurface : null,
        onOpenSurface: openSurface,
        onCloseSurface: closeSurface,
        onCloseSurfaceOf: closeSurfaceOf,
        onApplyParam,
        onRemoveParam,
        isCustomizeOpen,
        onOpenCustomize: openCustomize,
        onCloseCustomize: closeCustomize,
        onDisplayChange: (display: DisplayOptions) => write({ ...options, display }),
        // The reader's params stay for the run; only the number display goes back.
        onResetDisplay: () => write({ ...options, display: DEFAULT_DISPLAY }),
        onCopyLink,
    }
}

export type ReportViewModel = ReturnType<typeof useReportView>
