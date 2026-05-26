import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { PB_SERVER_ADDR } from '@tinycld/core/lib/config'
import { pb } from '@tinycld/core/lib/pocketbase'

// PreviewCommentAnchor mirrors the server JSON anchor shapes. Calc uses
// the column letter (as shown in the grid header, e.g. "B") plus the
// 1-based row number; text uses character offsets over the rendered text.
export type PreviewCommentAnchor =
    | { sheet_id?: string; row: number; col: string }
    | { start: number; end: number }

export interface PreviewCommentRow {
    id: string
    drive_item: string
    anchor_kind: 'calc_cell' | 'text_range'
    anchor: PreviewCommentAnchor | null
    quoted_text: string
    parent_comment: string
    body: string
    resolved_at: string
    author_user_org: string
    author_anon_id: string
    author_name: string
    created: string
    updated: string
}

// PreviewCommentIdentity is who the client is acting as. For an anon
// visitor this is the share session; for a logged-in org member it's
// resolved from the PB auth store + their user_org id.
export interface PreviewCommentIdentity {
    /** Share-session token for anon visitors; undefined for org members. */
    sessionToken?: string
    /** The current actor's id for ownership checks: anon_id or user_org id. */
    currentActorId: string
}

function authHeaders(identity: PreviewCommentIdentity): Record<string, string> {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    // Logged-in members send their PB token; the server prefers re.Auth.
    if (!identity.sessionToken && pb.authStore.token) {
        headers.Authorization = `Bearer ${pb.authStore.token}`
    }
    if (identity.sessionToken) {
        headers['X-Share-Session'] = identity.sessionToken
    }
    return headers
}

function withSession(url: string, identity: PreviewCommentIdentity): string {
    if (!identity.sessionToken) return url
    const sep = url.includes('?') ? '&' : '?'
    return `${url}${sep}session=${encodeURIComponent(identity.sessionToken)}`
}

const base = () => `${PB_SERVER_ADDR}/api/drive/preview-comments`

export interface CreatePreviewCommentArgs {
    anchorKind: 'calc_cell' | 'text_range'
    anchor: PreviewCommentAnchor
    quotedText?: string
    parentComment?: string
    body: string
}

// usePreviewComments loads the comment list for a shared item and exposes
// create / edit / resolve / reopen / delete mutations, all routed through
// the drive preview-comment endpoints (which authorize via the share
// session or PB auth).
export function usePreviewComments(itemId: string, identity: PreviewCommentIdentity) {
    const queryClient = useQueryClient()
    const queryKey = ['preview-comments', itemId]

    const list = useQuery<PreviewCommentRow[]>({
        queryKey,
        queryFn: async () => {
            const resp = await fetch(withSession(`${base()}?item=${itemId}`, identity), {
                headers: authHeaders(identity),
            })
            if (!resp.ok) throw new Error(`load comments failed: HTTP ${resp.status}`)
            const data = await resp.json()
            return data.comments ?? []
        },
        enabled: !!itemId,
    })

    const invalidate = () => queryClient.invalidateQueries({ queryKey })

    const add = useMutation({
        mutationFn: async (args: CreatePreviewCommentArgs) => {
            const resp = await fetch(withSession(base(), identity), {
                method: 'POST',
                headers: authHeaders(identity),
                body: JSON.stringify({
                    item: itemId,
                    anchor_kind: args.anchorKind,
                    anchor: args.anchor,
                    quoted_text: args.quotedText,
                    parent_comment: args.parentComment,
                    body: args.body,
                }),
            })
            if (!resp.ok) throw new Error(`create comment failed: HTTP ${resp.status}`)
            return resp.json()
        },
        onSuccess: invalidate,
    })

    const patch = useMutation({
        mutationFn: async (vars: { id: string; body?: string; resolved?: boolean }) => {
            const resp = await fetch(withSession(`${base()}/${vars.id}`, identity), {
                method: 'PATCH',
                headers: authHeaders(identity),
                body: JSON.stringify({ body: vars.body, resolved: vars.resolved }),
            })
            if (!resp.ok) throw new Error(`update comment failed: HTTP ${resp.status}`)
            return resp.json()
        },
        onSuccess: invalidate,
    })

    const remove = useMutation({
        mutationFn: async (id: string) => {
            const resp = await fetch(withSession(`${base()}/${id}`, identity), {
                method: 'DELETE',
                headers: authHeaders(identity),
            })
            if (!resp.ok) throw new Error(`delete comment failed: HTTP ${resp.status}`)
            return resp.json()
        },
        onSuccess: invalidate,
    })

    return {
        comments: list.data ?? [],
        isLoading: list.isLoading,
        error: list.error,
        add,
        editBody: (id: string, body: string) => patch.mutateAsync({ id, body }),
        resolve: (id: string) => patch.mutateAsync({ id, resolved: true }),
        reopen: (id: string) => patch.mutateAsync({ id, resolved: false }),
        remove: (id: string) => remove.mutateAsync(id),
    }
}
