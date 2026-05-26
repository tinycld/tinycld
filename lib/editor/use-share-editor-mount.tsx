// core/lib/editor/use-share-editor-mount.tsx

import { useAuth } from '@tinycld/core/lib/auth'
import { colorForUser } from '@tinycld/core/lib/util/color'
import { useMemo } from 'react'
import { type ShareSession, useShareSession } from '../anon-identity'
import type { EditorCapabilities, EditorMount, EditorRole } from './editor-mount'
import { useShareLinkVisitorRole } from './use-share-visitor-role'

// buildAnonMount constructs an EditorMount from a resolved ShareSession.
// All anon mounts are read-only in this phase; editable anon + anon
// comments are later phases.
//
// Capabilities notes:
//   - viewer:    all caps false — pure read-only view.
//   - commentor: all caps false — commentor comment capability lands in Phase 3.
//   - editor:    all caps false — the standalone anon EDITOR experience is future
//                work; for now even an editor-role anon link opens read-only via
//                this mount.
export function buildAnonMount(session: ShareSession): EditorMount {
    const capabilities = {
        canEdit: false,
        canComment: false,
        canUseFileActions: false,
        canMention: false,
    }

    return {
        itemId: session.itemId,
        itemName: session.name,
        // Anon has no file string; doc bytes arrive via the realtime WS bootstrap.
        itemFile: '',
        mimeType: session.mimeType,
        identity: {
            kind: 'anon',
            // No userId / userOrgId for anonymous visitors.
            displayName: session.displayName,
            color: colorForUser(session.anonId),
        },
        role: session.role,
        capabilities,
        realtimeCredential: { kind: 'shareSession', token: session.sessionToken },
    }
}

// Capabilities by role for a guest visitor (signed in via OTP, has a
// drive_shares row + user_org row with role='guest').
//   - viewer:    defensive — viewer links don't require sign-in, but if a
//                guest somehow lands here their caps are all false.
//   - commentor: canComment only. NO file actions and NO mention support
//                (security: prevents the org-roster leak through mention
//                autocomplete and prevents writes to drive_items).
//   - editor:    canEdit + canComment. SAME restriction — no file actions
//                or mentions for guests at any role.
function capabilitiesForGuest(role: EditorRole): EditorCapabilities {
    if (role === 'editor') {
        return {
            canEdit: true,
            canComment: true,
            canUseFileActions: false,
            canMention: false,
        }
    }
    if (role === 'commentor') {
        return {
            canEdit: false,
            canComment: true,
            canUseFileActions: false,
            canMention: false,
        }
    }
    // viewer (defensive)
    return {
        canEdit: false,
        canComment: false,
        canUseFileActions: false,
        canMention: false,
    }
}

export interface BuildGuestMountInput {
    session: ShareSession
    userId: string
    userOrgId: string
    userName: string
    role: EditorRole
}

// buildGuestMount produces an EditorMount for a signed-in GUEST of a
// share link. Same shape as the anon mount but the identity is 'guest'
// and the realtime credential is the visitor's real PB token — the
// broker's standard Authorize path admits them via their drive_shares
// row, so no shareSession token is needed.
export function buildGuestMount({
    session,
    userId,
    userOrgId,
    userName,
    role,
}: BuildGuestMountInput): EditorMount {
    return {
        itemId: session.itemId,
        itemName: session.name,
        // Guests, like anons, fetch doc bytes through the realtime
        // bootstrap; we don't ship them the raw file string.
        itemFile: '',
        mimeType: session.mimeType,
        identity: {
            kind: 'guest',
            userId,
            userOrgId,
            displayName: userName,
            // Stable per-userId color (same algorithm as anons → guests
            // who later return as anons keep the same color and vice
            // versa). Using the userId means the color persists across
            // sign-in/sign-out within a share link surface.
            color: colorForUser(userId),
        },
        role,
        capabilities: capabilitiesForGuest(role),
        // Real PB token — broker admits via drive_shares Authorize.
        realtimeCredential: { kind: 'auth' },
    }
}

// useShareEditorMount resolves the visitor's role for this share link
// and builds an EditorMount that matches: a guest mount (real PB token +
// role-derived capabilities) for signed-in guests, an anon mount
// (shareSession token + read-only) otherwise. The hook is the single
// source of truth for "what mount do I render here", so swapping between
// anon and guest after OTP verify is automatic via the auth-store →
// useAuth re-render chain.
export function useShareEditorMount(token: string): {
    mount: EditorMount | null
    isLoading: boolean
    error: unknown
} {
    const { data: session, isLoading: sessionLoading, error } = useShareSession(token)
    const auth = useAuth({ throwIfAnon: false })
    const visitor = useShareLinkVisitorRole(token)

    const userId = auth.isLoggedIn ? auth.user.id : null
    const userName = auth.isLoggedIn ? auth.user.name : null

    // Memoize on the stable scalar fields so the mount object reference is
    // stable across re-renders even if the parent re-renders often —
    // keeping `session` itself out of the deps is intentional (it's a
    // fresh object on every refetch).
    // biome-ignore lint/correctness/useExhaustiveDependencies: deps are scalar fields of session/visitor; passing `session` itself would defeat the memo
    const mount = useMemo(() => {
        if (!session) return null
        if (
            visitor.role === 'guest' &&
            userId &&
            userName &&
            visitor.userOrgId &&
            visitor.shareRole
        ) {
            return buildGuestMount({
                session,
                userId,
                userOrgId: visitor.userOrgId,
                userName,
                role: visitor.shareRole,
            })
        }
        return buildAnonMount(session)
    }, [
        session?.sessionToken,
        session?.anonId,
        session?.displayName,
        session?.role,
        session?.itemId,
        session?.name,
        session?.mimeType,
        visitor.role,
        visitor.userOrgId,
        visitor.shareRole,
        userId,
        userName,
    ])

    return {
        mount,
        isLoading: sessionLoading || visitor.isLoading,
        error,
    }
}
