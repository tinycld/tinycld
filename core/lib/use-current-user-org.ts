import { useAuth } from '@tinycld/core/lib/auth'

// Single-org deployment: feature records that used to reference the caller's
// `user_org` membership row now reference the `users` record directly. This shim
// returns an object whose `id` is the current user's id, so existing call sites
// that read `userOrg?.id` and store it as an owner/creator FK keep working with
// the new value (a users id). The `orgSlug` argument is ignored.
export function useCurrentUserOrg(_orgSlug?: string) {
    const { user } = useAuth()
    return user?.id ? { id: user.id } : null
}
