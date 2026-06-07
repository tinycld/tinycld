import type { Transaction } from '@tanstack/react-db'
import { newRecordId } from 'pbtsdb/core'
import { useMutation } from '../mutations'
import { parseMentions } from './mentions'

export interface CommentAuthorIdentity {
    userOrgId: string
    displayName: string
    email: string
}

// Resolve the author columns for a new comment row. author is the
// user_org id; author_name is snapshotted (name → email → 'Anonymous')
// so a later-removed user still renders a recognizable label.
export function resolveAuthorFields(id: CommentAuthorIdentity): {
    author: string
    author_name: string
} {
    return { author: id.userOrgId, author_name: id.displayName || id.email || 'Anonymous' }
}

type AnyTransaction = Transaction<Record<string, unknown>>

// Per-package shape supplied at call time: cell coords for calc,
// commentId + quoted_text for text. Each package picks its own.
export interface BaseAddArgs {
    driveItemId: string
    body: string
}

export interface BaseReplyArgs extends BaseAddArgs {
    parentId: string
}

// Shape of a single comment_mentions insert payload. Callers supply
// `insertMention` and a `commentCollection` name; the factory builds
// these from `parseMentions(body)`.
export interface CommentMentionInsert {
    id: string
    comment_collection: string
    comment_record: string
    drive_item: string
    mentioned_user_org: string
}

// Optional mentions support. When supplied, the add/reply mutations
// yield one extra insert per deduped mention after the comment row.
// `commentCollection` matches the comment table name ("calc_comments",
// "text_comments", ...) — the Go notify hook validates against the
// allowlist before notifying.
export interface CommentMentionsConfig {
    commentCollection: string
    insertMention: (row: CommentMentionInsert) => AnyTransaction
}

// The factory takes operation functions rather than a pbtsdb collection
// handle, sidestepping pbtsdb's heavily-overloaded `update` signature
// (which TS can't structurally unify with a single (id, mutator) call
// site). Callers pass anonymous functions that close over their
// `useStore('...')` result and call `.insert / .update / .delete`
// directly — the resulting types are precise per package.
export interface MakeCommentMutationsArgs<
    Insertable extends object,
    AddArgs extends BaseAddArgs,
    ReplyArgsT extends BaseReplyArgs,
> {
    insertRow: (row: Insertable) => AnyTransaction
    // The mutator only writes body / resolved_at — keep the draft type
    // narrow so pbtsdb's WritableObjectDeep (which marks omitOnInsert
    // fields optional) still satisfies it.
    updateRow: (
        id: string,
        mutator: (draft: { body: string; resolved_at: string }) => void
    ) => AnyTransaction
    deleteRow: (id: string) => AnyTransaction
    // Build the full insert row from the base columns and call-site
    // args. Generic over the package's args shape so callers preserve
    // their own typed surface (sheetId/row/col for calc, commentId for
    // text).
    buildInsert: (
        base: {
            id: string
            drive_item: string
            parent_comment: string
            body: string
            resolved_at: string
            author: string
            author_name: string
        },
        args: AddArgs | ReplyArgsT
    ) => Insertable
    // Author identity for new comment rows; supplied by the caller (member or guest).
    identity: CommentAuthorIdentity
    // Optional: when present, add/reply also insert one
    // comment_mentions row per deduped `[[@user_org_id]]` in the body.
    mentions?: CommentMentionsConfig
}

// Generator-based comment mutations, shared across packages. Author
// identity is read once at hook level and snapshotted into author_name
// so a removed user still renders something on future reads. Each
// package wraps this with a thin `useCommentMutations()` that supplies
// the pbtsdb operations and projects its args into the insert row.
export function useBaseCommentMutations<
    Insertable extends object,
    AddArgs extends BaseAddArgs = BaseAddArgs,
    ReplyArgsT extends BaseReplyArgs = BaseReplyArgs,
>(args: MakeCommentMutationsArgs<Insertable, AddArgs, ReplyArgsT>) {
    const { insertRow, updateRow, deleteRow, buildInsert, mentions, identity } = args
    const { author, author_name } = resolveAuthorFields(identity)

    // Build the comment_mentions inserts for a freshly-inserted comment
    // row. One row per deduped `[[@user_org_id]]` token in the body,
    // skipping self-mentions (notifying yourself is noise). When the
    // caller hasn't supplied a mentions config, returns an empty list.
    function mentionInserts(
        commentId: string,
        driveItemId: string,
        body: string
    ): AnyTransaction[] {
        if (!mentions) return []
        const parsed = parseMentions(body)
        const out: AnyTransaction[] = []
        for (const m of parsed) {
            if (m.userOrgId === identity.userOrgId) continue
            out.push(
                mentions.insertMention({
                    id: newRecordId(),
                    comment_collection: mentions.commentCollection,
                    comment_record: commentId,
                    drive_item: driveItemId,
                    mentioned_user_org: m.userOrgId,
                })
            )
        }
        return out
    }

    const add = useMutation({
        mutationFn: function* (a: AddArgs) {
            const commentId = newRecordId()
            yield insertRow(
                buildInsert(
                    {
                        id: commentId,
                        drive_item: a.driveItemId,
                        parent_comment: '',
                        body: a.body,
                        resolved_at: '',
                        author,
                        author_name,
                    },
                    a
                )
            )
            for (const tx of mentionInserts(commentId, a.driveItemId, a.body)) {
                yield tx
            }
        },
    })

    const reply = useMutation({
        mutationFn: function* (a: ReplyArgsT) {
            const commentId = newRecordId()
            yield insertRow(
                buildInsert(
                    {
                        id: commentId,
                        drive_item: a.driveItemId,
                        parent_comment: a.parentId,
                        body: a.body,
                        resolved_at: '',
                        author,
                        author_name,
                    },
                    a
                )
            )
            for (const tx of mentionInserts(commentId, a.driveItemId, a.body)) {
                yield tx
            }
        },
    })

    const editBody = useMutation({
        mutationFn: function* (a: { id: string; body: string }) {
            yield updateRow(a.id, draft => {
                draft.body = a.body
            })
        },
    })

    const resolve = useMutation({
        mutationFn: function* (a: { id: string }) {
            yield updateRow(a.id, draft => {
                draft.resolved_at = new Date().toISOString()
            })
        },
    })

    const reopen = useMutation({
        mutationFn: function* (a: { id: string }) {
            yield updateRow(a.id, draft => {
                draft.resolved_at = ''
            })
        },
    })

    const remove = useMutation({
        mutationFn: function* (a: { id: string }) {
            yield deleteRow(a.id)
        },
    })

    return { add, reply, editBody, resolve, reopen, remove }
}
