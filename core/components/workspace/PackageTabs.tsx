import { Tabs } from 'expo-router'
import { View } from 'react-native'
import { PackageAccessDenied } from './PackageAccessDenied'

/**
 * Org-level package switcher — the navigator that swaps between packages
 * (mail, drive, calendar, …) and the app-owned admin/help/settings routes.
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
            <Tabs
                tabBar={() => null}
                backBehavior="history"
                screenOptions={{
                    headerShown: false,
                    freezeOnBlur: true,
                    lazy: true,
                    animation: 'none',
                }}
            />
            <PackageAccessDenied />
        </View>
    )
}
