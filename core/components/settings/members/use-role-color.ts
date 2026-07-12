import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { type OrgRole, ROLE_SWATCH } from './types'

// Resolves each role's semantic foreground token to a runtime color value, for
// the handful of spots that need an actual color (a Lucide icon, an inline-styled
// border/fill) rather than a className. Hooks can't be called inside a `.map`, so
// this returns the whole role→color map up front for the caller to index.
export function useRoleColors(): Record<OrgRole, string> {
    return {
        owner: useThemeColor(ROLE_SWATCH.owner.fg),
        admin: useThemeColor(ROLE_SWATCH.admin.fg),
        member: useThemeColor(ROLE_SWATCH.member.fg),
        guest: useThemeColor(ROLE_SWATCH.guest.fg),
    }
}
