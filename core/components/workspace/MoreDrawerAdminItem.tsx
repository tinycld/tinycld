import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useIsSuperAdmin } from '@tinycld/core/lib/use-is-super-admin'
import { useRouter } from 'expo-router'
import { ShieldCheck } from 'lucide-react-native'
import { Pressable, Text } from 'react-native'

// The super-admin entry into the in-shell Admin area within the mobile "More"
// drawer — the mobile counterpart of AdminRailButton. Self-gating: renders null
// for non-super-admins so MoreDrawer stays free of inline visibility logic.
// `onNavigate` lets the parent close the drawer after routing.
export function MoreDrawerAdminItem({ onNavigate }: { onNavigate: (action: () => void) => void }) {
    const isSuperAdmin = useIsSuperAdmin()
    const isActive = useWorkspaceStore(s => s.activePkgSlug) === 'admin'
    const router = useRouter()
    const orgHref = useOrgHref()
    const railText = useThemeColor('rail-text')
    const railActive = useThemeColor('rail-active-text')

    if (!isSuperAdmin) return null

    const color = isActive ? railActive : railText

    return (
        <Pressable
            testID="nav-admin"
            className="flex-row items-center gap-3.5 px-4 py-3.5 rounded-lg"
            onPress={() => onNavigate(() => router.push(orgHref('admin')))}
        >
            <ShieldCheck size={20} color={color} />
            <Text className="text-base font-medium" style={{ color }}>
                Admin
            </Text>
        </Pressable>
    )
}
