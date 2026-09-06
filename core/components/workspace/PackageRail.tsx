import { NotificationBell } from '@tinycld/core/components/NotificationBell'
import { OrgLogo } from '@tinycld/core/components/OrgLogo'
import { ImportIndicator } from '@tinycld/core/components/workspace/ImportIndicator'
import { APP_PREFIX, useOrgHref } from '@tinycld/core/lib/org-routes'
import { usePackage } from '@tinycld/core/lib/packages/use-packages'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useOrgInfo } from '@tinycld/core/lib/use-org-info'
import { useSortedPackages } from '@tinycld/core/lib/use-sorted-packages'
import { type Href, Link } from 'expo-router'
import {
    Building2,
    HelpCircle,
    type LucideIcon,
    PanelLeftClose,
    PanelLeftOpen,
    Settings,
} from 'lucide-react-native'
import { Pressable, ScrollView, View } from 'react-native'
import { getIcon } from './package-icon-map'
import { railWidth } from './rail-width'
import { UserMenu } from './UserMenu'

// insetLeft is the side-corrected left safe-area inset (nonzero only when the
// sensor housing is physically on the left). The rail absorbs it rather than
// letting the parent pad the whole layout: that way the rail's own dark
// background fills the gutter edge to edge. Padding it upstream instead left a
// strip of app background beside the rail — the light band this fixes.
export function PackageRail({
    insetLeft = 0,
    insetBottom = 0,
}: {
    insetLeft?: number
    insetBottom?: number
}) {
    const railBg = useThemeColor('rail-background')
    const railText = useThemeColor('rail-text')
    const railActive = useThemeColor('rail-active-text')
    const indicatorColor = useThemeColor('active-indicator')
    const sorted = useSortedPackages()
    const activePkgSlug = useWorkspaceStore(s => s.activePkgSlug)
    const orgHref = useOrgHref()
    const { org } = useOrgInfo()

    return (
        <ScrollView
            // A ScrollView so short viewports (landscape phones) can still reach
            // every item — a plain View clipped whatever overflowed. `grow` on
            // the content container makes it fill the viewport when everything
            // fits, so justify-between keeps the utilities pinned to the bottom
            // exactly as before; only genuine overflow scrolls. Popovers are
            // unaffected: UserMenu portals its menu and the bell toggles the
            // NotificationDrawer rendered outside the rail.
            style={{ backgroundColor: railBg, width: railWidth(insetLeft), flexGrow: 0 }}
            // Width and paddingLeft both take the full inset, so the icon column
            // sits entirely clear of the sensor housing with its 64pt intact.
            // The background still reaches the physical edge — the parent applies
            // no horizontal padding, so this box starts at x=0 and its own
            // background covers the gutter it sits in.
            // pt-3 rather than py-3: the bottom pad takes the LARGER of the
            // base 12 and the home-indicator inset, keeping the last icon
            // tappable. A minimum, not an addend — summing them double-counts
            // and pushes the last icon needlessly far off the edge. See
            // useSafeAreaPadding, which expresses this same rule.
            contentContainerClassName="grow justify-between items-center pt-3"
            contentContainerStyle={{
                paddingLeft: insetLeft,
                paddingBottom: Math.max(12, insetBottom),
            }}
            showsVerticalScrollIndicator={false}
            alwaysBounceVertical={false}
        >
            <View className="items-center gap-1">
                <Link href={orgHref('')} asChild>
                    <Pressable
                        testID="nav-home"
                        className="w-11 h-11 rounded-xl justify-center items-center"
                        accessibilityLabel="Organization home"
                    >
                        <OrgLogo
                            org={org}
                            size={32}
                            fallback={<Building2 size={24} color={railText} />}
                        />
                    </Pressable>
                </Link>

                <View className="w-7 h-px opacity-20 my-2" style={{ backgroundColor: railText }} />

                {sorted.map(pkg => {
                    const Icon = getIcon(pkg.nav?.icon ?? '')
                    const isActive = activePkgSlug === pkg.slug
                    return (
                        <PackageRailItem
                            key={pkg.slug}
                            slug={pkg.slug}
                            label={pkg.nav?.label ?? ''}
                            Icon={Icon}
                            isActive={isActive}
                            activeColor={indicatorColor}
                            textColor={isActive ? railActive : railText}
                        />
                    )
                })}
            </View>

            <View className="items-center gap-2">
                <SidebarToggle color={railText} activePkgSlug={activePkgSlug} />
                <ImportIndicator />
                <NotificationBell color={railText} />

                <Link href={orgHref('help')} asChild>
                    <Pressable
                        testID="nav-help"
                        className="w-11 h-11 rounded-xl justify-center items-center"
                        accessibilityLabel="Help"
                    >
                        <HelpCircle size={22} color={railText} />
                    </Pressable>
                </Link>

                <Link href={orgHref('settings')} asChild>
                    <Pressable
                        testID="nav-settings"
                        className="w-11 h-11 rounded-xl justify-center items-center"
                        accessibilityLabel="Settings"
                    >
                        <Settings size={22} color={railText} />
                    </Pressable>
                </Link>

                <UserMenu />
            </View>
        </ScrollView>
    )
}

/**
 * Collapses/reopens the docked package sidebar. Rendered only when the active
 * package contributes a sidebar — without it there is no panel to toggle, and
 * historically the store's isSidebarOpen had no UI writer at all, so a
 * persisted `false` left the sidebar stuck closed with no way back.
 */
function SidebarToggle({ color, activePkgSlug }: { color: string; activePkgSlug: string | null }) {
    const isSidebarOpen = useWorkspaceStore(s => s.isSidebarOpen)
    const toggleSidebar = useWorkspaceStore(s => s.toggleSidebar)
    const activePkg = usePackage(activePkgSlug ?? '')
    if (activePkg?.sidebar == null) return null

    const Icon = isSidebarOpen ? PanelLeftClose : PanelLeftOpen
    return (
        <Pressable
            testID="nav-sidebar-toggle"
            onPress={toggleSidebar}
            className="w-11 h-11 rounded-xl justify-center items-center"
            accessibilityRole="button"
            accessibilityLabel={isSidebarOpen ? 'Hide sidebar' : 'Show sidebar'}
        >
            <Icon size={20} color={color} />
        </Pressable>
    )
}

function PackageRailItem({
    slug,
    label,
    Icon,
    isActive,
    activeColor,
    textColor,
}: {
    slug: string
    label: string
    Icon: LucideIcon
    isActive: boolean
    activeColor: string
    textColor: string
}) {
    const orgHref = useOrgHref()
    const lastHref = useWorkspaceStore(s => s.lastPackageHref[slug])
    // lastHref is a fully-formed pathname captured at runtime when a
    // file screen mounted (e.g. /calc/abc123). Expo Router's
    // typed-routes can't statically verify a runtime-built string, so we
    // cast through Href. orgHref(slug as never) handles the same gap on
    // the fallback path — slug is a string at compile time, the typed
    // Href union expects a literal route, hence `as never`.
    // Only trust a stored href that carries the current prefix: it is persisted
    // state, so a pre-/a value (or one written by an older build) would
    // navigate to a dead route. Falling back to the package root degrades to
    // "lands on the list" instead of "lands on +not-found".
    const href: Href = lastHref?.startsWith(`${APP_PREFIX}/`)
        ? (lastHref as Href)
        : orgHref(slug as never)

    return (
        <View className="relative w-11 h-11 items-center justify-center">
            {isActive && (
                <View
                    className="absolute w-1 h-5 rounded-sm"
                    style={{ backgroundColor: activeColor, left: -10 }}
                />
            )}
            <Link href={href} asChild>
                <Pressable
                    testID={`nav-${slug}`}
                    className="w-11 h-11 rounded-xl justify-center items-center"
                    style={isActive ? { backgroundColor: `${activeColor}22` } : undefined}
                    accessibilityLabel={label}
                >
                    <Icon size={22} color={textColor} />
                </Pressable>
            </Link>
        </View>
    )
}
