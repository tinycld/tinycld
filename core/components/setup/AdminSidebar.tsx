import {
    SidebarHeading,
    SidebarItem,
    SidebarNav,
} from '@tinycld/core/components/sidebar-primitives'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { usePathname, useRouter } from 'expo-router'
import { Building2, History, type LucideIcon, Package } from 'lucide-react-native'

interface AdminSection {
    slug: string
    label: string
    Icon: LucideIcon
    // Owner-only: installing or removing a package rebuilds the artifact the
    // whole deployment runs, so admins don't get it. requireOwner is the actual
    // enforcement — hiding the item just avoids offering a guaranteed 403.
    ownerOnly?: boolean
}

// Mirrors SetupDashboard's NAV — the same sections, now driven from the
// workspace PackageSidebar instead of the standalone console's internal rail.
const SECTIONS: AdminSection[] = [
    { slug: 'organizations', label: 'Organizations', Icon: Building2 },
    { slug: 'packages', label: 'Packages', Icon: Package, ownerOnly: true },
    { slug: 'builds', label: 'Build History', Icon: History },
]

interface AdminSidebarProps {
    isCollapsed: boolean
}

export default function AdminSidebar(_props: AdminSidebarProps) {
    const router = useRouter()
    const pathname = usePathname()
    const orgHref = useOrgHref()
    const { isOwner } = useCurrentRole()

    const sections = SECTIONS.filter(section => isOwner || !section.ownerOnly)

    return (
        <SidebarNav>
            <SidebarHeading>Admin</SidebarHeading>
            {sections.map(section => (
                <SidebarItem
                    key={section.slug}
                    label={section.label}
                    icon={section.Icon}
                    isActive={pathname.endsWith(`/admin/${section.slug}`)}
                    closesDrawer
                    onPress={() => router.push(orgHref(`admin/${section.slug}`))}
                />
            ))}
        </SidebarNav>
    )
}
