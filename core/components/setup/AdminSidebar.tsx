import {
    SidebarHeading,
    SidebarItem,
    SidebarNav,
} from '@tinycld/core/components/sidebar-primitives'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { usePathname, useRouter } from 'expo-router'
import { Building2, History, type LucideIcon, Package, ShieldCheck } from 'lucide-react-native'

interface AdminSection {
    slug: string
    label: string
    Icon: LucideIcon
}

// Mirrors SetupDashboard's NAV — the same sections, now driven from the
// workspace PackageSidebar instead of the standalone console's internal rail.
const SECTIONS: AdminSection[] = [
    { slug: 'organizations', label: 'Organizations', Icon: Building2 },
    { slug: 'packages', label: 'Packages', Icon: Package },
    { slug: 'builds', label: 'Build History', Icon: History },
    { slug: 'super-admins', label: 'Super Admins', Icon: ShieldCheck },
]

interface AdminSidebarProps {
    isCollapsed: boolean
}

export default function AdminSidebar(_props: AdminSidebarProps) {
    const router = useRouter()
    const pathname = usePathname()
    const orgHref = useOrgHref()

    return (
        <SidebarNav>
            <SidebarHeading>Admin</SidebarHeading>
            {SECTIONS.map(section => (
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
