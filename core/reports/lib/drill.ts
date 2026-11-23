import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useDetailSurface } from '@tinycld/core/lib/use-detail-surface'
import { useLocalSearchParams, useRouter } from 'expo-router'
import { useReportUIStore } from '../stores/report-ui-store'
import { textFromCents } from './money'
import { REPORTS_BASE, useReportBase } from './report-base'
import type { ParamDef } from './spec'
import type { DrillParams, ResultColumn, ResultRow } from './types'
import {
    clearedDrillKeys,
    DRILL_KEY,
    DRILL_PREFIX,
    FROM_KEY,
    paramKey,
    type SearchParams,
    serializeViewOptions,
    type ViewOptions,
} from './view-url'

// A drilled figure opens its column's target report filtered by the figure's
// `@drill` cell: the target's params in the run request's JSON form. The figure
// has no target defs, so each value is written as encodeParamValue writes the
// same value (R9); the tests read every form back through the view-url decoders.

function isRecord(value: unknown): value is Record<string, unknown> {
    return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/** A param's JSON value as the URL writes it; undefined leaves the key out (the target's default). */
export function encodeDrillValue(value: unknown): string | undefined {
    if (typeof value === 'string') return value
    if (typeof value === 'number') return String(value)
    if (Array.isArray(value)) return JSON.stringify(value)
    if (!isRecord(value)) return undefined
    if (typeof value.preset === 'string') return value.preset
    if (typeof value.from === 'string' && typeof value.to === 'string')
        return `${value.from}..${value.to}`
    if (typeof value.amount === 'number') {
        const text = textFromCents(value.amount)
        return typeof value.currency === 'string' && value.currency !== ''
            ? `${text} ${value.currency}`
            : text
    }
    return undefined
}

/** The figure's drill params, or null when its column has no drill or the row no `@drill` cell. */
export function drillCellParams(column: ResultColumn, row: ResultRow): DrillParams | null {
    if (!column.drill) return null
    const cell: unknown = row.cells[`${column.id}@drill`]
    return isRecord(cell) ? cell : null
}

function drillKeys(params: DrillParams, prefix: string): Record<string, string> {
    const out: Record<string, string> = {}
    for (const [id, value] of Object.entries(params)) {
        const text = encodeDrillValue(value)
        if (text !== undefined) out[`${prefix}${paramKey(id)}`] = text
    }
    return out
}

/**
 * The full report's screen path for a figure, with the source report (D3); wrap it
 * with orgHref. `base` is the route the report view is drawn under (useReportBase).
 */
export function drillHref(
    column: ResultColumn,
    row: ResultRow,
    sourceId: string,
    base: string = REPORTS_BASE
): string | null {
    const params = drillCellParams(column, row)
    if (!column.drill || !params) return null
    const query = new URLSearchParams({ ...drillKeys(params, ''), [FROM_KEY]: sourceId })
    return `${base}/${column.drill.reportId}?${query.toString()}`
}

/**
 * The full report's screen path for the panel's report as the reader left it
 * (Open as full report): its options without the panel's prefix, and the source.
 */
export function fullReportHref(
    reportId: string,
    defs: readonly ParamDef[],
    options: ViewOptions,
    sourceId: string,
    base: string = REPORTS_BASE
): string {
    const query = new URLSearchParams()
    for (const [key, value] of Object.entries(serializeViewOptions(defs, options))) {
        if (value !== undefined) query.set(key, value)
    }
    query.set(FROM_KEY, sourceId)
    return `${base}/${reportId}?${query.toString()}`
}

/** The route params that show a figure's drill in the panel; every older drill key is cleared. */
export function drillPanelParams(
    column: ResultColumn,
    row: ResultRow,
    current: SearchParams = {}
): Record<string, string | undefined> | null {
    const params = drillCellParams(column, row)
    if (!column.drill || !params) return null
    return {
        ...clearedDrillKeys(current),
        [DRILL_KEY]: column.drill.reportId,
        ...drillKeys(params, DRILL_PREFIX),
    }
}

export function firstOf(value: string | string[] | undefined): string | undefined {
    return Array.isArray(value) ? value[0] : value
}

const DRILL_PARAM_PREFIX = `${DRILL_PREFIX}${paramKey('')}`

/**
 * The panel's report and params the route holds. The panel's sort and display keys
 * are left out: only a change of the panel's chips unmarks the figure. router.setParams
 * keeps a cleared key as undefined, so those are left out too.
 */
export function shownDrillParams(search: SearchParams): SearchParams {
    return Object.fromEntries(
        Object.entries(search).filter(
            ([key, value]) =>
                (key === DRILL_KEY || key.startsWith(DRILL_PARAM_PREFIX)) && value !== undefined
        )
    )
}

/** Whether the panel shows exactly this figure's drill (the figure is then marked). */
export function isDrillShown(column: ResultColumn, row: ResultRow, search: SearchParams): boolean {
    const wanted = drillPanelParams(column, row)
    if (!wanted) return false
    const shown = Object.keys(shownDrillParams(search))
    return (
        shown.length === Object.keys(wanted).length &&
        shown.every(key => firstOf(search[key]) === wanted[key])
    )
}

/**
 * A figure press: beside the report when the screen hosts a panel and the window
 * is wide enough (Customize closes, one side surface at a time), else the full
 * report as its own screen.
 */
export function useOpenDrill({ sourceId, hasPanel }: { sourceId: string; hasPanel: boolean }) {
    const router = useRouter()
    const orgHref = useOrgHref()
    const base = useReportBase()
    const search = useLocalSearchParams()
    const surface = useDetailSurface()
    const closeCustomize = useReportUIStore(s => s.closeCustomize)
    const noteDrillOpen = useReportUIStore(s => s.noteDrillOpen)
    return (column: ResultColumn, row: ResultRow) => {
        if (hasPanel && surface === 'panel') {
            const next = drillPanelParams(column, row, search)
            if (!next) return
            router.setParams(next)
            noteDrillOpen()
            closeCustomize('main')
            return
        }
        const href = drillHref(column, row, sourceId, base)
        if (href) router.push(orgHref(href))
    }
}
