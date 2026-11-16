import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useDetailSurface } from '@tinycld/core/lib/use-detail-surface'
import { type Href, useLocalSearchParams, useNavigation, useRouter } from 'expo-router'
import { CommonActions, type NavigationState } from 'expo-router/react-navigation'
import { useCallback, useMemo, useRef } from 'react'
import { drillHref, firstOf, isDrillShown, shownDrillParams, useOpenDrill } from '../lib/drill'
import { figureKey, rowFocusKey } from '../lib/figure-focus'
import { isRowShown, recordPanelParams, rowOpensInPanel } from '../lib/record-panel'
import {
    type OpenedRecord,
    openedRecordOf,
    returnStepOf,
    routeKeysOf,
    tabRouteOf,
} from '../lib/record-return'
import { registeredPanels } from '../lib/registered-panels'
import type { ResultColumn, ResultRow } from '../lib/types'
import { RECORD_KEY } from '../lib/view-url'
import { type ReportUIScope, useReportUIStore } from '../stores/report-ui-store'
import { useStable } from './useStable'

/** The report a view shows, for its figures' drills, and whether its screen hosts a drill panel. */
export interface RowDrill {
    sourceId: string
    hasPanel: boolean
    /** The view that draws the figures: it files them apart from the other view's. */
    scope: ReportUIScope
}

/** What a table's figures and rows open. */
export interface RowLinks {
    figureHref(column: ResultColumn, row: ResultRow): Href | null
    onFigure(column: ResultColumn, row: ResultRow): void
    figureKey(column: ResultColumn, row: ResultRow): string
    isFigureShown(column: ResultColumn, row: ResultRow): boolean
    rowHref(row: ResultRow): Href | null
    onRow(row: ResultRow): void
    /** The row label's key (rowFocusKey): the record panel's close sends the focus back there. */
    rowKey(row: ResultRow): string
    /** The record panel shows this row's record, opened by a press on this row (R11). */
    isRowShown(row: ResultRow): boolean
}

/**
 * A record's own screen, pushed in its package's tab; Back from it returns to the
 * report (watchRecordReturn). A row press and the record panel's Open as full
 * screen both open it.
 */
export function useOpenRecordScreen(): (path: string) => void {
    const router = useRouter()
    const navigation = useRouteNavigation()
    const orgHref = useOrgHref()
    return useCallback(
        (path: string) => {
            watchRecordReturn(navigation, tabNameOf(path))
            router.push(orgHref(path))
        },
        [navigation, router, orgHref]
    )
}

/**
 * A row press that opens its record beside the report (D5): a registered panel's
 * path, a wide window and a view that hosts the side panel. False: the row opens as
 * a screen.
 */
function useOpenRecordPanel(hasPanel: boolean) {
    const router = useRouter()
    const search = useLocalSearchParams()
    const surface = useDetailSurface()
    const openRecord = useReportUIStore(s => s.openRecord)
    return (row: ResultRow): boolean => {
        const open = row.open
        if (!open || !rowOpensInPanel({ open, surface, hasPanel, panels: registeredPanels })) {
            return false
        }
        router.setParams(recordPanelParams(open, search))
        openRecord(row.id)
        return true
    }
}

/**
 * The table's links, or undefined where figures and rows are plain (the builder's
 * preview passes no drill). They keep their identity while the drill keys, the
 * shown record and the pressed row stay, so FlashList redraws the rows (extraData)
 * only when a figure's or a row's mark may change.
 */
export function useRowLinks(drill: RowDrill | undefined): RowLinks | undefined {
    const orgHref = useOrgHref()
    const search = useLocalSearchParams()
    const shown = useStable(shownDrillParams(search))
    const shownRecord = firstOf(search[RECORD_KEY])
    const openedRow = useReportUIStore(s => s.openedRow)
    const openScreen = useOpenRecordScreen()
    const openDrill = useOpenDrill({
        sourceId: drill?.sourceId ?? '',
        hasPanel: drill?.hasPanel ?? false,
    })
    const openPanel = useOpenRecordPanel(drill?.hasPanel ?? false)
    // The presses read the newest handlers, so the links need not change with them.
    const latest = useRef({ openDrill, openPanel })
    latest.current = { openDrill, openPanel }
    const sourceId = drill?.sourceId
    const scope = drill?.scope ?? 'main'
    const setDrillOrigin = useReportUIStore(s => s.setDrillOrigin)
    return useMemo<RowLinks | undefined>(() => {
        if (sourceId === undefined) return undefined
        const record = { [RECORD_KEY]: shownRecord }
        return {
            figureHref: (column, row) => {
                const href = drillHref(column, row, sourceId)
                return href === null ? null : orgHref(href)
            },
            onFigure: (column, row) => {
                setDrillOrigin(figureKey(scope, column.id, row.id))
                latest.current.openDrill(column, row)
            },
            figureKey: (column, row) => figureKey(scope, column.id, row.id),
            isFigureShown: (column, row) => isDrillShown(column, row, shown),
            rowHref: row => (row.open ? orgHref(row.open) : null),
            onRow: row => {
                if (!row.open || latest.current.openPanel(row)) return
                openScreen(row.open)
            },
            rowKey: row => rowFocusKey(scope, row.id),
            isRowShown: row => isRowShown(row, openedRow, record),
        }
    }, [sourceId, scope, shown, shownRecord, openedRow, orgHref, openScreen, setDrillOrigin])
}

// useNavigation's own default type (its getState may be undefined), read off a call
// with no type argument so it follows expo-router.
function useRouteNavigation() {
    return useNavigation()
}

type Navigation = ReturnType<typeof useRouteNavigation>

// The org's package tabs: the nearest tab navigator above the report that holds the
// target's tab. A package may render the view inside its own nested tabs, which do not.
function tabsOf(navigation: Navigation, tabName: string): Navigation | undefined {
    let current: Navigation | undefined = navigation
    while (current && !holdsTab(current.getState(), tabName)) current = current.getParent()
    return current
}

function holdsTab(state: NavigationState | undefined, tabName: string): boolean {
    return state?.type === 'tab' && state.routeNames.includes(tabName)
}

// A record's path starts with its package's slug, which names the package's tab.
function tabNameOf(path: string): string {
    return path.replace(/^\//, '').split(/[/?#]/)[0] ?? ''
}

// One opened record is watched at a time: a later open replaces the watch.
let stopWatch: (() => void) | undefined

/**
 * A record opens in its package's tab, on top of the screens that tab already holds
 * (a form the reader left half filled stays mounted under it). The record screen's
 * Back pops to those screens, not to the report, and no package knows it was opened
 * from a report. So core watches the tabs: when the route the open added leaves its
 * tab by a Back while that tab is shown, core shows the report's tab again, whose
 * state and URL (params, sort, Customize, panel keys) were never touched (spec D2,
 * §3). The package's stack stays whole.
 *
 * The tabs' `state` event runs after React commits the Back, so the package's earlier
 * screen can show for a moment before the report. On web the Back moves the browser
 * to that screen's entry and the return adds the report's entry after it, so browser
 * Back from the report goes to that screen and Forward has nothing.
 */
function watchRecordReturn(navigation: Navigation, tabName: string) {
    stopWatch?.()
    const tabs = tabsOf(navigation, tabName)
    const state = tabs?.getState()
    const reportTab = state?.routes[state.index]
    if (!tabs || !state || !reportTab || reportTab.name === tabName) return
    const before = routeKeysOf(tabRouteOf(state, tabName)?.state)
    let opened: OpenedRecord | undefined
    const stop = () => {
        unsubscribe()
        if (stopWatch === stop) stopWatch = undefined
    }
    const unsubscribe = tabs.addListener('state', event => {
        const next = event.data.state
        if (!opened) {
            opened = openedRecordOf(next, tabName, before)
            return
        }
        const step = returnStepOf(next, opened)
        if (step === 'wait') return
        stop()
        if (step === 'return') {
            // merge keeps the report tab's route and its params as they are.
            tabs.dispatch({
                ...CommonActions.navigate({ name: reportTab.name, merge: true }),
                target: next.key,
            })
        }
    })
    stopWatch = stop
}
