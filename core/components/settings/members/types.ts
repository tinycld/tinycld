export type OrgRole = 'owner' | 'admin' | 'member' | 'guest'

export interface MemberRow {
    userId: string
    username: string
    name: string
    email: string
    role: OrgRole
    isPending: boolean
    isDemo: boolean
}

export type DrawerMode = { kind: 'closed' } | { kind: 'invite' } | { kind: 'view'; userId: string }

export const ROLE_DESCRIPTIONS: Record<OrgRole, string> = {
    owner: 'Full control — manages members, every setting, and every role including owner.',
    admin: 'Can manage members, settings, and all organization data.',
    member: 'Can use the organization day-to-day across installed packages.',
    guest: 'Limited access. Scoped to whatever you grant per package.',
}

// Role → semantic status tokens. Each role tracks the active theme via
// Tailwind semantic classes (owner=primary, admin=info, member=success,
// guest=warning) instead of a fixed light-mode hex that clashed in dark mode.
// `fg` is the theme-color name for the few spots that need a runtime color value
// (a Lucide icon color, or an inline-styled border); `text`/`bg`/`border` are the
// className parts (bg/border use an opacity modifier for the soft tint).
export interface RoleSwatch {
    fg: 'primary' | 'info' | 'success' | 'warning'
    text: string
    bg: string
    border: string
}

export const ROLE_SWATCH: Record<OrgRole, RoleSwatch> = {
    owner: {
        fg: 'primary',
        text: 'text-primary',
        bg: 'bg-primary/10',
        border: 'border-primary/40',
    },
    admin: { fg: 'info', text: 'text-info', bg: 'bg-info/10', border: 'border-info/40' },
    member: {
        fg: 'success',
        text: 'text-success',
        bg: 'bg-success/10',
        border: 'border-success/40',
    },
    guest: {
        fg: 'warning',
        text: 'text-warning',
        bg: 'bg-warning/10',
        border: 'border-warning/40',
    },
}

export const ROLE_ORDER: OrgRole[] = ['owner', 'admin', 'member', 'guest']

export const ROLE_LABELS: Record<OrgRole, string> = {
    owner: 'Owner',
    admin: 'Admin',
    member: 'Member',
    guest: 'Guest',
}
