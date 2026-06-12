import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useIsSuperAdmin } from '@tinycld/core/lib/use-is-super-admin'
import { Link } from 'expo-router'
import { ShieldCheck } from 'lucide-react-native'
import { Pressable } from 'react-native'

// The super-admin entry into the in-shell Admin area. Self-gating: renders null
// for non-super-admins so PackageRail stays free of inline visibility logic.
export function AdminRailButton() {
    const isSuperAdmin = useIsSuperAdmin()
    const orgHref = useOrgHref()
    const railText = useThemeColor('rail-text')
    const railActive = useThemeColor('rail-active-text')
    const indicatorColor = useThemeColor('active-indicator')
    const isActive = useWorkspaceStore(s => s.activePkgSlug) === 'admin'

    if (!isSuperAdmin) return null

    return (
        <Link href={orgHref('admin')} asChild>
            <Pressable
                testID="nav-admin"
                className="w-11 h-11 rounded-xl justify-center items-center"
                style={isActive ? { backgroundColor: `${indicatorColor}22` } : undefined}
                accessibilityLabel="Admin"
            >
                <ShieldCheck size={22} color={isActive ? railActive : railText} />
            </Pressable>
        </Link>
    )
}
