// Multi-org is removed. There is no in-app cross-org list anymore — org switching
// is deferred to a parent-domain cookie (not built yet). This hook is kept as a
// stub so the org-switcher UI (UserMenu / MoreDrawer) compiles and renders nothing.
export interface UserOrgEntry {
    id: string
    name: string
    slug: string
    logo?: string
}

export function useUserOrgs(): UserOrgEntry[] {
    return []
}
