import { markNavMilestone, markNavPress } from '@tinycld/core/lib/nav-perf'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useSortedPackages } from '@tinycld/core/lib/use-sorted-packages'
import { useRouter } from 'expo-router'
import { Ellipsis } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import { getIcon } from './package-icon-map'

export const MAX_VISIBLE_TABS = 4

export function MobileTabBar() {
    const railBg = useThemeColor('rail-background')
    const railText = useThemeColor('rail-text')
    const railActive = useThemeColor('rail-active-text')
    const indicatorColor = useThemeColor('active-indicator')
    const sorted = useSortedPackages()
    const activePkgSlug = useWorkspaceStore(s => s.activePkgSlug)
    const setActivePkgSlug = useWorkspaceStore(s => s.setActivePkgSlug)
    const lastPackageHref = useWorkspaceStore(s => s.lastPackageHref)
    const isMoreOpen = useWorkspaceStore(s => s.isMoreOpen)
    const setMoreOpen = useWorkspaceStore(s => s.setMoreOpen)
    const router = useRouter()
    const insets = useSafeAreaInsets()

    const visiblePkgs =
        sorted.length > MAX_VISIBLE_TABS ? sorted.slice(0, MAX_VISIBLE_TABS) : sorted

    return (
        <View
            className="flex-row pt-2 pb-1 border-t border-border items-center z-10"
            style={{
                backgroundColor: railBg,
                paddingBottom: insets.bottom,
            }}
        >
            {visiblePkgs.map(pkg => {
                const Icon = getIcon(pkg.nav?.icon ?? '')
                const isActive = activePkgSlug === pkg.slug
                if (isActive) markNavMilestone(pkg.slug, 'tabbar-indicator-rendered')
                const color = isActive ? railActive : railText
                return (
                    <Pressable
                        key={pkg.slug}
                        testID={`nav-${pkg.slug}`}
                        className="flex-1 items-center justify-center gap-1 py-1 relative"
                        onPress={() => {
                            // Light the tab up immediately, then defer the actual
                            // navigation to the next frame. setActivePkgSlug +
                            // router.navigate in the same tick batches the cheap
                            // indicator repaint together with the target screen's
                            // (heavy) first render, so the highlight only moves
                            // once that work yields. Splitting them lets the
                            // indicator commit on its own frame first — the tap
                            // feels instant regardless of how slow the target
                            // screen is to mount. ActivePkgSync re-affirms the
                            // slug once the route settles.
                            markNavPress(pkg.slug)
                            setMoreOpen(false)
                            setActivePkgSlug(pkg.slug)
                            // One rAF lets the indicator repaint commit on its
                            // own frame before navigation kicks off the target
                            // screen's render — enough to decouple the highlight
                            // without the longer wait runAfterInteractions imposes.
                            requestAnimationFrame(() => {
                                markNavMilestone(pkg.slug, 'navigate-called')
                                // Reopen the exact inner view the user left (a
                                // thread, a drive folder, …) via lastPackageHref,
                                // mirroring the desktop rail; fall back to the
                                // package root on first visit. The org-level
                                // switcher is a Tabs navigator, so this is a
                                // JUMP_TO to the package's existing frozen screen
                                // — one instance per route, no remount.
                                const target = lastPackageHref[pkg.slug] ?? `/${pkg.slug}`
                                router.navigate(target)
                            })
                        }}
                        accessibilityLabel={pkg.nav?.label}
                    >
                        {isActive ? (
                            <View
                                className="absolute -top-1 w-6 h-[3px] rounded-sm"
                                style={{ backgroundColor: indicatorColor }}
                            />
                        ) : null}
                        <Icon size={22} color={color} />
                        <Text className="text-[10px] font-medium" style={{ color }}>
                            {pkg.nav?.label}
                        </Text>
                    </Pressable>
                )
            })}
            <Pressable
                testID="nav-more"
                className="flex-1 items-center justify-center gap-1 py-1 relative"
                onPress={() => setMoreOpen(!isMoreOpen)}
                accessibilityLabel="More"
            >
                <Ellipsis size={22} color={railText} />
                <Text className="text-[10px] font-medium" style={{ color: railText }}>
                    More
                </Text>
            </Pressable>
        </View>
    )
}
