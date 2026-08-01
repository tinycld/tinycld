// core/lib/editor/use-share-visitor-role.tsx
import { useQuery } from '@tanstack/react-query'
import { useAuth } from '@tinycld/core/lib/auth'
import { captureException } from '@tinycld/core/lib/errors'
import { pb } from '@tinycld/core/lib/pocketbase'
import { type ShareSession, useShareSession } from '../anon-identity'

// drive_shares is owned by @tinycld/drive — its row shape isn't part of
// pbSchema in an app assembled without drive (e.g. core's standalone CI).
// Declare the minimal local shape this hook needs; the runtime collection
// is provided by drive's migrations, and the API contract is stable
// enough that this local copy doesn't drift in practice. Mirrors the
// pattern used by google-takeout-import for its mail-collection types.
interface DriveSharesRow {
    id: string
    item: string
    user: string
    role: 'owner' | 'editor' | 'commentor' | 'viewer'
}

// Visitor-role classification used by both the share route (to decide
// whether to redirect a signed-in user to the workspace) and the share-
// editor mount hook (to decide between an anon and a guest mount).
//
// - 'loading': either the share session OR the membership lookup is in flight.
// - 'anon':    no authed PB session (the visitor is browsing anonymously).
// - 'guest':   authed, AND has a drive_shares row for this item whose
//              user.role === 'guest'. Guests must stay on the share route —
//              they don't have workspace access.
// - 'member':  authed, but NOT a guest of this item — i.e. a real member who
//              arrived via the share link. The route should redirect them to
//              the workspace.
// - 'unknown': authed, but the drive_shares lookup returned nothing AND we
//              have no other signal. Treated as 'member' by the share
//              route (a signed-in user with no share row is presumed to
//              be reaching us with their own org access).
export type ShareVisitorRole = 'loading' | 'anon' | 'guest' | 'member' | 'unknown'

interface ShareVisitorRoleResult {
    role: ShareVisitorRole
    isLoading: boolean
    // When role === 'guest', shareRole is populated for buildGuestMount.
    shareRole?: 'viewer' | 'commentor' | 'editor'
}

// drive_shares.user is a direct relation to users (drive's migration renamed
// it from user_org when the junction was removed). Only its role is read here.
// Minimal local shape so core typechecks standalone (the drive collection isn't
// in core's pbSchema).
interface ShareUser {
    id: string
    role: 'owner' | 'admin' | 'member' | 'guest'
}

interface DriveShareWithUser extends DriveSharesRow {
    expand?: { user?: ShareUser }
}

// useShareLinkVisitorRole resolves the visitor's relationship to THIS
// share link's item. Centralizes the auth+drive_shares lookup so the
// share route and the editor-mount hook agree on the answer.
export function useShareLinkVisitorRole(token: string): ShareVisitorRoleResult {
    const auth = useAuth({ throwIfAnon: false })
    const { data: session, isLoading: sessionLoading } = useShareSession(token)

    const userId = auth.isLoggedIn ? auth.user.id : null
    const itemId = session?.itemId ?? null

    // Drive_shares row for THIS user and THIS item, expanding the user so
    // we can read their role.
    const lookupQuery = useQuery<DriveShareWithUser | null>({
        queryKey: ['share-visitor-role', token, userId, itemId],
        queryFn: async () => {
            if (!userId || !itemId) return null
            try {
                // Guest share-access lookup — a direct filter on the share's
                // user + expand for a user who has no pbtsdb store; cached via
                // React Query. This file is exempted from the
                // pbtsdb-no-raw-pb-access plugin in biome.json.
                //
                // Single-org: `user` is a direct users relation. The old
                // `user_org.user` filter walked a relation on a field drive had
                // renamed, so the query threw, the catch below swallowed it, and
                // every guest silently fell back to an anon mount.
                return await pb.collection('drive_shares').getFirstListItem<DriveShareWithUser>(
                    pb.filter('item = {:itemId} && user = {:userId}', {
                        itemId,
                        userId,
                    }),
                    { expand: 'user' }
                )
            } catch (err) {
                // 404 / no row found is a genuine "no membership". Anything
                // else — network, auth, schema drift — is exactly the class
                // this bare catch once hid (the user_org filter bug), so it
                // must reach Sentry even though the safe fallback is the same.
                const status = (err as { status?: number } | null)?.status
                if (status !== 404) {
                    captureException('editor.shareVisitorRole.membership', err, { itemId })
                }
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
    const shareUser = row?.expand?.user
    if (row && shareUser?.role === 'guest') {
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
                captureException(
                    'editor.shareVisitorRole.unexpectedRole',
                    new Error(`unexpected drive_shares.role=${JSON.stringify(row.role)}`),
                    { itemId: session?.itemId }
                )
                shareRole = 'commentor'
        }
        return {
            role: 'guest',
            isLoading: false,
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
