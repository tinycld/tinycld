import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { Link } from 'expo-router'
import { ShieldCheck } from 'lucide-react-native'
import { Pressable } from 'react-native'

// The admin entry into the in-shell Admin area. Self-gating: renders null for
// non-admins so PackageRail stays free of inline visibility logic.
export function AdminRailButton() {
    const { isAdmin } = useCurrentRole()
    const orgHref = useOrgHref()
    const railText = useThemeColor('rail-text')
    const railActive = useThemeColor('rail-active-text')
    const indicatorColor = useThemeColor('active-indicator')
    const isActive = useWorkspaceStore(s => s.activePkgSlug) === 'admin'

    if (!isAdmin) return null

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
