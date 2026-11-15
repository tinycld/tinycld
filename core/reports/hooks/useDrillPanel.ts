import { DETAIL_PANEL_WIDTH } from '@tinycld/core/lib/detail-surface'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useDetailSurface } from '@tinycld/core/lib/use-detail-surface'
import { useLocalSearchParams, useRouter } from 'expo-router'
import { useCallback } from 'react'
import { firstOf, fullReportHref } from '../lib/drill'
import { type FocusTarget, focusFigure, focusNode } from '../lib/figure-focus'
import { UNREADABLE_SPEC } from '../lib/spec'
import { clearedDrillKeys, DRILL_KEY, DRILL_PREFIX } from '../lib/view-url'
import { useReportUIStore } from '../stores/report-ui-store'
import { useReportRecord } from './useReportRecord'
import { useReportView } from './useReportView'

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

/** Whether the drill panel shows as a page that covers the report beside it. */
export function useIsDrillPage(): boolean {
    const targetId = firstOf(useLocalSearchParams()[DRILL_KEY])
    const surface = useDetailSurface()
    return Boolean(targetId) && surface === 'page'
}
