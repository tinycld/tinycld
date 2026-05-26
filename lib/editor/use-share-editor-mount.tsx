// core/lib/editor/use-share-editor-mount.tsx
import { useMemo } from 'react'
import { useShareSession, type ShareSession } from '../anon-identity'
import type { EditorMount } from './editor-mount'

// colorForAnon produces a deterministic HSL color from an anon id.
// Mirrors calc's colorForUser (same hash → hsl(hue, 70%, 45%)) —
// duplicated here intentionally to avoid cross-package coupling.
function colorForAnon(anonId: string): string {
    let h = 0
    for (let i = 0; i < anonId.length; i++) {
        h = (h * 31 + anonId.charCodeAt(i)) >>> 0
    }
    const hue = h % 360
    return `hsl(${hue}, 70%, 45%)`
}

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
            color: colorForAnon(session.anonId),
        },
        role: session.role,
        capabilities,
        realtimeCredential: { kind: 'shareSession', token: session.sessionToken },
    }
}

// useShareEditorMount resolves an anonymous share session and builds a
// read-only EditorMount from it. Used by the public share viewer route
// (View 3) to give anonymous visitors an editor context without an
// authenticated PocketBase identity.
export function useShareEditorMount(token: string): {
    mount: EditorMount | null
    isLoading: boolean
    error: unknown
} {
    const { data: session, isLoading, error } = useShareSession(token)

    const mount = useMemo(
        () => (session ? buildAnonMount(session) : null),
        // Memoize on the stable scalar fields so the mount object reference
        // is stable across re-renders even if the parent re-renders often.
        // eslint-disable-next-line react-hooks/exhaustive-deps
        [
            session?.sessionToken,
            session?.anonId,
            session?.displayName,
            session?.role,
            session?.itemId,
            session?.name,
            session?.mimeType,
        ]
    )

    return { mount, isLoading, error }
}
