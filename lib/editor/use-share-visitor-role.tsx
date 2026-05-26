// core/lib/editor/use-share-visitor-role.tsx
import { useQuery } from '@tanstack/react-query'
import { useAuth } from '@tinycld/core/lib/auth'
import { pb } from '@tinycld/core/lib/pocketbase'
import type { DriveShares, UserOrg } from '@tinycld/core/types/pbSchema'
import { type ShareSession, useShareSession } from '../anon-identity'

// Visitor-role classification used by both the share route (to decide
// whether to redirect a signed-in user to the workspace) and the share-
// editor mount hook (to decide between an anon and a guest mount).
//
// - 'loading': either the share session OR the membership lookup is in flight.
// - 'anon':    no authed PB session (the visitor is browsing anonymously).
// - 'guest':   authed, AND has a drive_shares row for this item whose
//              user_org.role === 'guest' in the owner's org. Guests must
//              stay on the share route — they don't have org access.
// - 'member':  authed, but NOT a guest of this item — i.e. a real org
//              member who arrived via the share link. The route should
//              redirect them to the workspace.
// - 'unknown': authed, but the drive_shares lookup returned nothing AND we
//              have no other signal. Treated as 'member' by the share
//              route (a signed-in user with no share row is presumed to
//              be reaching us with their own org access).
export type ShareVisitorRole = 'loading' | 'anon' | 'guest' | 'member' | 'unknown'

interface ShareVisitorRoleResult {
    role: ShareVisitorRole
    isLoading: boolean
    // When role === 'guest', these are populated for buildGuestMount.
    userOrgId?: string
    shareRole?: 'viewer' | 'commentor' | 'editor'
}

interface DriveShareWithUserOrg extends DriveShares {
    expand?: { user_org?: UserOrg }
}

// useShareLinkVisitorRole resolves the visitor's relationship to THIS
// share link's item. Centralizes the auth+drive_shares lookup so the
// share route and the editor-mount hook agree on the answer.
export function useShareLinkVisitorRole(token: string): ShareVisitorRoleResult {
    const auth = useAuth({ throwIfAnon: false })
    const { data: session, isLoading: sessionLoading } = useShareSession(token)

    const userId = auth.isLoggedIn ? auth.user.id : null
    const itemId = session?.itemId ?? null

    // Drive_shares row for THIS user and THIS item, expanding user_org so
    // we can read its role and capture the user_org.id (which the guest
    // mount needs as identity.userOrgId).
    const lookupQuery = useQuery<DriveShareWithUserOrg | null>({
        queryKey: ['share-visitor-role', token, userId, itemId],
        queryFn: async () => {
            if (!userId || !itemId) return null
            try {
                return await pb
                    .collection('drive_shares')
                    .getFirstListItem<DriveShareWithUserOrg>(
                        `item = "${itemId}" && user_org.user = "${userId}"`,
                        { expand: 'user_org' }
                    )
            } catch {
                // 404 / no row found → treat as no membership for this item.
                return null
            }
        },
        enabled: !!userId && !!itemId,
        staleTime: 5 * 60 * 1000,
        retry: false,
    })

    if (auth.isInitializing || sessionLoading) {
        return { role: 'loading', isLoading: true }
    }
    if (!auth.isLoggedIn) {
        return { role: 'anon', isLoading: false }
    }
    if (lookupQuery.isLoading) {
        return { role: 'loading', isLoading: true }
    }

    const row = lookupQuery.data
    const userOrg = row?.expand?.user_org
    if (row && userOrg?.role === 'guest') {
        // Guest provisioning (drive endpoints_share_otp.go) only ever writes
        // drive_shares.role = 'commentor' or 'editor'. An 'owner' (or any other
        // unexpected role) on a guest's drive_shares row indicates a
        // data-integrity violation upstream; downgrade to commentor (the
        // least-privilege share role) and log so the anomaly surfaces rather
        // than being silently coerced into editor capabilities.
        let shareRole: ShareSession['role']
        switch (row.role) {
            case 'editor':
            case 'commentor':
            case 'viewer':
                shareRole = row.role
                break
            default:
                console.warn(
                    `useShareLinkVisitorRole: unexpected drive_shares.role=${JSON.stringify(row.role)} on a guest user_org (item ${session?.itemId}); coercing to 'commentor'`
                )
                shareRole = 'commentor'
        }
        return {
            role: 'guest',
            isLoading: false,
            userOrgId: userOrg.id,
            shareRole,
        }
    }
    if (row) {
        // Authed user with a drive_shares row but not a guest — they're a
        // real member (owner/editor/commentor/viewer via their normal org
        // membership).
        return { role: 'member', isLoading: false }
    }
    // Authed, but no drive_shares row matched. Presume they're a member
    // reaching us through some other path; the share route will redirect.
    return { role: 'unknown', isLoading: false }
}
