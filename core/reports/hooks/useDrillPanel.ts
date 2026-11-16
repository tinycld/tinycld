import { DETAIL_PANEL_WIDTH } from '@tinycld/core/lib/detail-surface'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useDetailSurface } from '@tinycld/core/lib/use-detail-surface'
import { useLocalSearchParams, useRouter } from 'expo-router'
import { useCallback } from 'react'
import { firstOf, fullReportHref } from '../lib/drill'
import { type FocusTarget, focusFigure, focusNode, rowFocusKey } from '../lib/figure-focus'
import { recordPanelOf } from '../lib/record-panel'
import { registeredPanels } from '../lib/registered-panels'
import { UNREADABLE_SPEC } from '../lib/spec'
import { clearedDrillKeys, DRILL_KEY, DRILL_PREFIX, RECORD_KEY } from '../lib/view-url'
import { recordTitleOf, useReportUIStore } from '../stores/report-ui-store'
import { useReportRecord } from './useReportRecord'
import { useReportView } from './useReportView'
import { useOpenRecordScreen } from './useRowLinks'

export interface DrillPanelInput {
    /** The report the panel shows (the route's `d`). */
    targetId: string
    /** The report beside it, for Open as full report's back link. */
    sourceId: string
    sourceTitle: string
}

/**
 * The drill panel's report: a second report view on the source's route, whose
 * options carry DRILL_PREFIX and whose popovers live in the `drill` scope. On a
 * window under 1360 px it is a page over the report; only its width input and the
 * header change, so a crossing neither remounts nor reruns it.
 */
export function useDrillPanel({ targetId, sourceId, sourceTitle }: DrillPanelInput) {
    const router = useRouter()
    const orgHref = useOrgHref()
    const search = useLocalSearchParams()
    const surface = useDetailSurface()
    const record = useReportRecord(targetId)
    // Closing is a view change on the source's route, as opening was: no history entry.
    // ✕, Escape and the page's back link send the focus back to the figure that
    // opened the panel, when it is still drawn.
    const onClose = () => {
        router.setParams(clearedDrillKeys(search))
        focusFigure(useReportUIStore.getState().drillOrigin)
    }
    const view = useReportView({
        reportKey: `${DRILL_PREFIX}${targetId}`,
        title: record.record?.title ?? '',
        defs: record.spec?.params ?? [],
        source: record.record && record.spec ? { kind: 'saved', reportId: targetId } : null,
        access: record.access,
        broken: record.broken ?? (record.spec ? null : UNREADABLE_SPEC),
        isEditing: false,
        onBack: onClose,
        spec: record.spec,
        urlPrefix: DRILL_PREFIX,
        uiScope: 'drill',
        viewWidth: surface === 'panel' ? DETAIL_PANEL_WIDTH : undefined,
    })
    // On open the focus moves to the panel's title, so a keyboard reader continues there.
    // The title view is keyed by the figure presses that opened a drill, so only a press
    // remounts it: the panel's own controls, a window crossing and Back/Forward leave it
    // alone. A mount with no new press (a reload or a pasted link with `d`) keeps the
    // focus where the page put it (spec §4: "On open").
    const titleKey = useReportUIStore(s => s.drillOpens)
    const titleRef = useCallback((node: FocusTarget | null) => {
        const ui = useReportUIStore.getState()
        if (!node || ui.drillOpens <= ui.drillFocused) return
        ui.noteDrillFocused()
        focusNode(node)
    }, [])
    return {
        targetId,
        title: record.record?.title ?? '',
        ariaLabel: record.record?.title || 'Report',
        isPage: surface === 'page',
        recordState: record.state,
        retryRecord: record.retry,
        view,
        backLabel: sourceTitle,
        titleRef,
        titleKey,
        onClose,
        onOpenFull: () =>
            router.push(orgHref(fullReportHref(targetId, view.defs, view.options, sourceId))),
    }
}

export type DrillPanelModel = ReturnType<typeof useDrillPanel>

export interface RecordPanelInput {
    /** The app path the panel shows (the route's `dr`). */
    open: string
    /** The report beside it, for the page form's back link. */
    sourceTitle: string
}

/**
 * The record panel (D5): a package's component for the route's `dr`, beside the
 * report on a wide window and a page over it below 1360 px (style values only, so
 * a crossing never remounts it). Null when no registered panel names the path (a
 * stale link, a removed package): the report shows alone (R10).
 */
export function useRecordPanel({ open, sourceTitle }: RecordPanelInput) {
    const router = useRouter()
    const search = useLocalSearchParams()
    const surface = useDetailSurface()
    const openScreen = useOpenRecordScreen()
    // The title of this record only: a replaced record shows none until its
    // component names it, and the same record keeps its own (R13).
    const title = useReportUIStore(s => recordTitleOf(s.recordTitle, open))
    // Keyed by the row presses that opened a record, as the drill panel's title is:
    // only a press moves the focus to the title, never a reload or a pasted link.
    const titleKey = useReportUIStore(s => s.recordOpens)
    const titleRef = useCallback((node: FocusTarget | null) => {
        const ui = useReportUIStore.getState()
        if (!node || ui.recordOpens <= ui.recordFocused) return
        ui.noteRecordFocused()
        focusNode(node)
    }, [])
    const match = recordPanelOf(open, registeredPanels)
    const entry = match
        ? registeredPanels.find(panel => panel.slug === match.slug && panel.path === match.path)
        : undefined
    if (!match || !entry) return null
    return {
        open,
        title,
        ariaLabel: title || 'Record',
        segments: match.segments,
        Component: entry.Component,
        isPage: surface === 'page',
        backLabel: sourceTitle,
        titleRef,
        titleKey,
        // Closing is a view change on the source's route; the focus goes back to the
        // pressed row's label when it is still drawn (R12).
        onClose: () => {
            router.setParams(clearedDrillKeys(search))
            const row = useReportUIStore.getState().openedRow
            focusFigure(row === null ? null : rowFocusKey('main', row))
        },
        onOpenFull: () => openScreen(open),
        // The boundary calls it after a failure: the next record loads the chunk again (R14).
        reloadChunk: entry.reload,
    }
}

export type RecordPanelModel = NonNullable<ReturnType<typeof useRecordPanel>>

/** Whether the side panel (a drill or a registered record) shows as a page that covers the report. */
export function useIsDrillPage(): boolean {
    const search = useLocalSearchParams()
    const surface = useDetailSurface()
    const targetId = firstOf(search[DRILL_KEY])
    const open = firstOf(search[RECORD_KEY])
    const hasRecord = open !== undefined && recordPanelOf(open, registeredPanels) !== null
    return surface === 'page' && (Boolean(targetId) || hasRecord)
}
