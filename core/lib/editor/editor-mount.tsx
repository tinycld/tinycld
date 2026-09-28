// core/lib/editor/editor-mount.tsx
import { createContext, type ReactNode, useContext } from 'react'

export type EditorRole = 'viewer' | 'commentor' | 'editor'

export interface EditorIdentity {
    // 'member' = authed member; 'guest' = lightweight share user; 'anon' = signed share session, no account.
    kind: 'member' | 'guest' | 'anon'
    // Single-org: there is one identifier. The old `userOrgId` companion held
    // the same users id under a junction-era name and was removed.
    userId?: string
    displayName: string
    color: string
}

export interface EditorCapabilities {
    canEdit: boolean
    canComment: boolean
    canUseFileActions: boolean
    canMention: boolean
}

export type RealtimeCredential = { kind: 'auth' } | { kind: 'shareSession'; token: string }

export interface EditorMount {
    itemId: string
    itemName: string
    itemFile: string
    mimeType: string
    identity: EditorIdentity
    role: EditorRole
    capabilities: EditorCapabilities
    realtimeCredential: RealtimeCredential
}

const EditorMountContext = createContext<EditorMount | null>(null)

export function EditorMountProvider({
    value,
    children,
}: {
    value: EditorMount
    children: ReactNode
}) {
    return <EditorMountContext.Provider value={value}>{children}</EditorMountContext.Provider>
}

export function useEditorMount(): EditorMount {
    const ctx = useContext(EditorMountContext)
    if (ctx == null) {
        throw new Error('useEditorMount must be used within an EditorMountProvider')
    }
    return ctx
}

// The same context, read without the requirement that it exist.
//
// `useEditorMount` throws by design: a component that genuinely needs the
// item, the room credential or the editing role is broken without them, and
// a silent `undefined` there would surface as a blank editor rather than a
// stack trace. But some components render both inside an editor and outside
// one — the shared comments composer is the case that forced this: it is
// mounted by the document screen (inside a provider) and by surfaces that
// portal or re-parent it, and a hook prop it calls unconditionally must not
// be able to take the whole screen down.
//
// Such a caller reads the mount optionally and degrades: absent a mount it
// has no editor-scoped capability to consult and falls back to whatever the
// app-level context (auth, role) already tells it.
export function useEditorMountOptional(): EditorMount | null {
    return useContext(EditorMountContext)
}
