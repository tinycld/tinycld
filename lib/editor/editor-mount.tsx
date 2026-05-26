// core/lib/editor/editor-mount.tsx
import { createContext, type ReactNode, useContext } from 'react'

export type EditorRole = 'viewer' | 'commentor' | 'editor'

export interface EditorIdentity {
    // 'member' = authed org member; 'guest' = lightweight share user; 'anon' = signed share session, no account.
    kind: 'member' | 'guest' | 'anon'
    userId?: string
    userOrgId?: string
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
