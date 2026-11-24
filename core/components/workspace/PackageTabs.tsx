import { termsRouteMayMount } from '@tinycld/core/lib/packages/terms-state'
import { declaresTerms, useTermsBlocked } from '@tinycld/core/lib/packages/use-package-terms'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { CoveredProvider, isRouteCovered, packageOfRoute } from '@tinycld/core/ui/overlay/covered'
import { Tabs } from 'expo-router'
import {
    createContext,
    type ReactElement,
    type ReactNode,
    useContext,
    useMemo,
    useRef,
} from 'react'
import { View } from 'react-native'
import { packageRegistry } from '../../lib/packages/static-registry'
import { PackageTermsGate, useCoveredContentProps } from '../package-terms/PackageTermsGate'
import { PackageAccessDenied } from './PackageAccessDenied'

// The routes app/a/(app)/ holds: the app-owned ones, plus one directory per bundled
// package with `routes`, which the generator writes. expo-router 58 renders only the
// screens a layout declares, so each one gets a <Tabs.Screen>. A package's route is
// named by its slug, or `slug/<file>` when its directory has no _layout; CoveredRoute
// reads the slug from either (packageOfRoute).
const APP_ROUTES = ['index', 'help', 'reports', 'settings']
const PACKAGE_ROUTES = packageRegistry.filter(p => p.routes?.directory).map(p => p.slug)
const SCREENS = [...APP_ROUTES, ...PACKAGE_ROUTES].map(name => (
    <Tabs.Screen key={name} name={name} />
))

/**
 * Org-level package switcher — the navigator that swaps between packages
 * and the app-owned admin/help/reports/settings routes.
 *
 * Replaces the earlier FrozenStack (an expo-router <Stack>). Under React
 * Navigation 7 a Stack's NAVIGATE only reuses the *currently-focused* route, so
 * switching to any other package PUSHED a fresh screen — leaking a duplicate
 * mounted package subtree per rail click (read as "the app loads twice", and
 * on iOS as a hollow/skeleton workspace). A tab switch is a JUMP_TO, which
 * gives exactly one screen per route by construction, so nothing accumulates.
 * `lazy` defers a package's first mount until it's visited, and a visited
 * package stays mounted so returning to it is instant — the keep-warm behavior
 * FrozenStack was only emulating. The installed expo-router's bottom tabs never
 * freeze a screen (ScreenFallback passes `freezeOnBlur: false` to each one), so
 * `freezeOnBlur` below has no effect: a blurred screen keeps rendering.
 *
 * We render our own chrome (PackageRail on desktop/tablet, MobileTabBar on
 * mobile), so the built-in tab bar is hidden. `backBehavior="history"` keeps a
 * focus-history stack for the tabs, which (a) lets web browser Back retrace the
 * packages you visited — matching the old Stack, and the reason a bare Tabs
 * regressed it: a tab JUMP_TO without history is a lateral replace, so Back
 * skipped the previous package — and (b) makes Android hardware-back walk that
 * same visit history. No `initialRouteName`: the installed package set is
 * dynamic per workspace assembly, so the org index route's redirect picks the
 * landing package instead.
 */
export function PackageTabs() {
    return (
        <View className="flex-1" style={{ minHeight: 0 }}>
            <CoveredContent>
                <Tabs
                    tabBar={() => null}
                    backBehavior="history"
                    screenLayout={coveredScreenLayout}
                    screenOptions={{
                        headerShown: false,
                        freezeOnBlur: true,
                        lazy: true,
                        animation: 'none',
                    }}
                >
                    {SCREENS}
                </Tabs>
            </CoveredContent>
            {/* Under the access cover: a member with no access reads that first. */}
            <PackageTermsGate />
            <PackageAccessDenied />
        </View>
    )
}

interface RouteTerms {
    blocked: ReadonlySet<string>
    isReady: boolean
    wasReady: boolean
}

// The terms state CoveredContent read, for each route below it. One read for the
// cover and the routes: a route then mounts in the same commit as the cover
// goes, never inside a cover that a second read has not lifted yet.
const RouteTermsContext = createContext<RouteTerms>({
    blocked: new Set(),
    isReady: true,
    wasReady: true,
})

/**
 * Holds the screens out of play while the terms cover is over them. Its own
 * component, so a change of the cover re-renders this View and the route
 * wrappers only: the Tabs element it gets as children stays the same.
 */
function CoveredContent({ children }: { children: ReactNode }) {
    const { blocked, isReady } = useTermsBlocked()
    const activePkgSlug = useWorkspaceStore(s => s.activePkgSlug)
    // A one-way latch written during render: it only turns true, so a repeated
    // render reads the same decision (termsRouteMayMount), and no effect is needed.
    const readyLatch = useRef(false)
    const wasReady = readyLatch.current
    readyLatch.current = wasReady || isReady
    const routeTerms = useMemo(() => ({ blocked, isReady, wasReady }), [blocked, isReady, wasReady])
    const viewProps = useCoveredContentProps(activePkgSlug != null && blocked.has(activePkgSlug))
    return (
        <View {...viewProps} className="flex-1" style={{ minHeight: 0 }}>
            <RouteTermsContext.Provider value={routeTerms}>{children}</RouteTermsContext.Provider>
        </View>
    )
}

/**
 * Each route's own covered state: a closed package's screen stays covered while
 * it is mounted behind an open package (bottom tabs do not freeze it), so its
 * dialogs, shortcuts and inputs stay out. Only a route whose terms state
 * changes sees a new value. The screen of a package that declares terms mounts
 * only once its state is known (the cover shows its loading frame until then),
 * so an open package's form mounts uncovered and keeps its autofocus.
 */
function CoveredRoute({ routeName, children }: { routeName: string; children: ReactElement }) {
    const { blocked, isReady, wasReady } = useContext(RouteTermsContext)
    if (!termsRouteMayMount(declaresTerms(packageOfRoute(routeName)), wasReady, isReady)) {
        return null
    }
    return <CoveredProvider value={isRouteCovered(routeName, blocked)}>{children}</CoveredProvider>
}

function coveredScreenLayout({
    route,
    children,
}: {
    route: { name: string }
    children: ReactElement
}) {
    return <CoveredRoute routeName={route.name}>{children}</CoveredRoute>
}
