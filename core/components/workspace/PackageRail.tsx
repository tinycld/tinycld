import { NotificationBell } from '@tinycld/core/components/NotificationBell'
import { OrgLogo } from '@tinycld/core/components/OrgLogo'
import { ImportIndicator } from '@tinycld/core/components/workspace/ImportIndicator'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useOrgInfo } from '@tinycld/core/lib/use-org-info'
import { useSortedPackages } from '@tinycld/core/lib/use-sorted-packages'
import { type Href, Link } from 'expo-router'
import { Building2, HelpCircle, type LucideIcon, Settings } from 'lucide-react-native'
import { Pressable, View } from 'react-native'
import { getIcon } from './package-icon-map'
import { UserMenu } from './UserMenu'

// The rail's visible icon column. Its 44pt touch targets are centred in it, so
// there is ~10pt of slack on each side before an icon reaches the rail's edge.
const RAIL_WIDTH = 64

// The rail's ACTUAL on-screen width: the icon column plus however much the
// left safe-area inset pushes it inward.
//
// insetLeft arrives side-corrected (WorkspaceLayout passes useDeviceInsets):
// ~59pt only in the landscape rotation that puts the sensor housing on the
// left, 0 in the other rotation and in portrait. So the rail is 123pt with the
// housing beside it and exactly RAIL_WIDTH everywhere else — the extra chrome
// exists only when hardware forces it.
//
// An earlier version capped the shift at 12pt to keep the width uniform, on the
// theory that the housing "does not intrude into the display's usable area
// beside it". It does: 12pt against a ~59pt housing left the top icons ~47pt
// underneath it, unreadable and untappable. The cap cannot apply to the shift
// alone either — the 44pt (w-11) icons need RAIL_WIDTH of space AFTER the
// padding, so width and shift move together.
//
// The background is unaffected either way — it always reaches the physical edge,
// so there is never a light gutter.
//
// Exported because anything positioned against the rail's right edge — the
// notification drawer — has to agree with it; a hardcoded 64 there leaves a gap
// once the rail widens in landscape.
export function railWidth(insetLeft: number): number {
    return RAIL_WIDTH + insetLeft
}

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
        <View
            // Width and paddingLeft both take the full inset, so the icon column
            // sits entirely clear of the sensor housing with its 64pt intact.
            // The background still reaches the physical edge — the parent applies
            // no horizontal padding, so this box starts at x=0 and its own
            // background covers the gutter it sits in.
            // pt-3 rather than py-3: the bottom pad adds the home-indicator
            // inset on top of the same base 12, keeping the last icon tappable.
            className="justify-between items-center pt-3"
            style={{
                backgroundColor: railBg,
                width: railWidth(insetLeft),
                paddingLeft: insetLeft,
                paddingBottom: 12 + insetBottom,
            }}
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
        </View>
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
    const href: Href = lastHref ? (lastHref as Href) : orgHref(slug as never)

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
