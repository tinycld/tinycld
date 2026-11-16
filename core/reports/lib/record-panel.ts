import type { DetailSurface } from '@tinycld/core/lib/detail-surface'
import type { ComponentType } from 'react'
import type { ReportPanelProps } from '../hooks/usePanelTitle'
import { firstOf } from './drill'
import type { ResultRow } from './types'
import { clearedDrillKeys, RECORD_KEY, type SearchParams } from './view-url'

/** One package's record panel, as the generator lists it (lib/generated/report-panels.ts). */
export type ReportPanelEntry = {
    slug: string
    path: string
    load: () => Promise<{ default: ComponentType<ReportPanelProps> }>
}

/** The registered panel an app path names, and the path's segments after `<slug>/<path>/`. */
export type RecordPanelMatch = { slug: string; path: string; segments: string[] }

type PanelKey = Pick<ReportPanelEntry, 'slug' | 'path'>

/**
 * The panel a row's `open` names (R10): one leading `/` is ignored, then
 * `<slug>/<path>/` and at least one segment, none empty. A path with a query or a
 * hash names none. When two paths of one package nest, the longer one wins.
 */
export function recordPanelOf(open: string, panels: readonly PanelKey[]): RecordPanelMatch | null {
    const path = open.startsWith('/') ? open.slice(1) : open
    if (/[?#]/.test(path)) return null
    let best: RecordPanelMatch | null = null
    for (const panel of panels) {
        const prefix = `${panel.slug}/${panel.path}/`
        if (!path.startsWith(prefix)) continue
        const segments = path.slice(prefix.length).split('/')
        if (segments.some(segment => segment === '')) continue
        if (best && best.path.length >= panel.path.length) continue
        best = { slug: panel.slug, path: panel.path, segments }
    }
    return best
}

/**
 * A row press opens its record beside the report only on a wide window
 * (`panel` surface) of a view that hosts the side panel, for a registered path;
 * otherwise it pushes the path as before (R9).
 */
export function rowOpensInPanel(input: {
    open: string | undefined
    surface: DetailSurface
    hasPanel: boolean
    panels: readonly PanelKey[]
}): boolean {
    if (!input.open || !input.hasPanel || input.surface !== 'panel') return false
    return recordPanelOf(input.open, input.panels) !== null
}

/** The route params that show a record beside the report; a drill shown there closes. */
export function recordPanelParams(
    open: string,
    current: SearchParams
): Record<string, string | undefined> {
    return { ...clearedDrillKeys(current), [RECORD_KEY]: open }
}

/**
 * The pressed row is marked while the panel shows its record (R11). Many rows can
 * open one record, so the row id decides; a reload has no pressed row (Q1).
 */
export function isRowShown(
    row: ResultRow,
    openedRow: string | null,
    search: SearchParams
): boolean {
    if (openedRow === null || row.id !== openedRow || !row.open) return false
    return firstOf(search[RECORD_KEY]) === row.open
}
