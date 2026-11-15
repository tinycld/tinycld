import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { type Href, useLocalSearchParams, useNavigation, useRouter } from 'expo-router'
import { CommonActions, type NavigationState } from 'expo-router/react-navigation'
import { useMemo, useRef } from 'react'
import { drillHref, isDrillShown, shownDrillParams, useOpenDrill } from '../lib/drill'
import { figureKey } from '../lib/figure-focus'
import {
    type OpenedRecord,
    openedRecordOf,
    returnStepOf,
    routeKeysOf,
    tabRouteOf,
} from '../lib/record-return'
import type { ResultColumn, ResultRow } from '../lib/types'
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
}

// Only the drill keys mark a figure, so the links change only when they do.
/**
 * The table's links, or undefined where figures and rows are plain (the builder's
 * preview passes no drill). They keep their identity while the drill keys stay, so
 * FlashList redraws the rows (extraData) only when a figure's mark may change.
 */
export function useRowLinks(drill: RowDrill | undefined): RowLinks | undefined {
    const router = useRouter()
    const navigation = useRouteNavigation()
    const orgHref = useOrgHref()
    const shown = useStable(shownDrillParams(useLocalSearchParams()))
    const openDrill = useOpenDrill({
        sourceId: drill?.sourceId ?? '',
        hasPanel: drill?.hasPanel ?? false,
    })
    // The press reads the newest handler, so the links need not change with it.
    const latest = useRef(openDrill)
    latest.current = openDrill
    const sourceId = drill?.sourceId
    const scope = drill?.scope ?? 'main'
    const setDrillOrigin = useReportUIStore(s => s.setDrillOrigin)
    return useMemo<RowLinks | undefined>(() => {
        if (sourceId === undefined) return undefined
        return {
            figureHref: (column, row) => {
                const href = drillHref(column, row, sourceId)
                return href === null ? null : orgHref(href)
            },
            onFigure: (column, row) => {
                setDrillOrigin(figureKey(scope, column.id, row.id))
                latest.current(column, row)
            },
            figureKey: (column, row) => figureKey(scope, column.id, row.id),
            isFigureShown: (column, row) => isDrillShown(column, row, shown),
            rowHref: row => (row.open ? orgHref(row.open) : null),
            onRow: row => {
                if (!row.open) return
                watchRecordReturn(navigation, tabNameOf(row.open))
                router.push(orgHref(row.open))
            },
        }
    }, [sourceId, scope, shown, orgHref, router, navigation, setDrillOrigin])
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
