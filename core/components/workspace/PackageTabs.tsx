import { useTermsBlocked } from '@tinycld/core/lib/packages/use-package-terms'
import { CoveredProvider, isRouteCovered } from '@tinycld/core/ui/overlay/covered'
import { Tabs } from 'expo-router'
import type { ReactElement, ReactNode } from 'react'
import { View } from 'react-native'
import { packageRegistry } from '../../lib/packages/static-registry'
import { PackageTermsGate, useCoveredContentProps } from '../package-terms/PackageTermsGate'
import { PackageAccessDenied } from './PackageAccessDenied'

// The routes app/a/(app)/ holds: the app-owned ones, plus one directory per bundled
// package with `routes`, which the generator writes. expo-router 58 renders only the
// screens a layout declares, so each one gets a <Tabs.Screen>. A package's route is
// named by its slug, which is what CoveredRoute reads.
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
 * mounted-but-frozen package subtree per rail click (read as "the app loads
 * twice", and on iOS as a hollow/skeleton workspace). A tab switch is a JUMP_TO,
 * which gives exactly one screen per route by construction, so nothing
 * accumulates. `lazy` defers a package's first mount until it's visited, and
 * `freezeOnBlur` keeps a visited package mounted-but-frozen so returning to it
 * is instant — the keep-warm behavior FrozenStack was only emulating.
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

/**
 * Holds the screens out of play while the terms cover is over them. Its own
 * component, so a change of the cover re-renders this View only: the Tabs
 * element it gets as children stays the same.
 */
function CoveredContent({ children }: { children: ReactNode }) {
    const viewProps = useCoveredContentProps()
    return (
        <View {...viewProps} className="flex-1" style={{ minHeight: 0 }}>
            {children}
        </View>
    )
}

/**
 * Each route's own covered state: a closed package's screen stays covered while
 * it is mounted and frozen behind an open package, so its dialogs, shortcuts
 * and inputs stay out. Only a route whose terms state changes sees a new value.
 */
function CoveredRoute({ routeName, children }: { routeName: string; children: ReactElement }) {
    const { blocked } = useTermsBlocked()
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
