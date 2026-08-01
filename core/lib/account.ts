import { pb } from '@tinycld/core/lib/pocketbase'

// Account lifecycle: suspend (reversible) or delete (irreversible).
//
// Single-org: this replaces the former leave-org client. "Leaving the
// organization" is meaningless when the deployment IS the org — the former
// /api/account/leave-org endpoints were pure cross-org membership management
// and no longer exist server-side.
//
// These go through Go endpoints rather than pbtsdb writes for three reasons
// the schema cannot express:
//   1. PocketBase collection rules can't constrain WHICH fields a write
//      touches, so `disabled` is kept out of the users field guard's
//      self-editable allowlist — otherwise a suspended user could clear their
//      own flag with a plain record update.
//   2. Refusing a disabled user's sign-in is a hook, not a rule (PB has no
//      authRule).
//   3. Delete settles authored content and anonymizes inside one transaction;
//      a client can't do that atomically.

// OffboardMode decides what happens to the records the user authored.
// Omitting the mode entirely (see deleteAccount) leaves content in place.
export type OffboardMode = 'reassign' | 'delete_my_data'

export interface OffboardPlan {
    mode: OffboardMode
    // Required when mode === 'reassign': the user id inheriting the content.
    successor_user_id?: string
}

export interface OffboardResult {
    user_anonymized: boolean
    records_reassigned: number
    records_deleted: number
}

// disableAccount suspends the caller's own account. Reversible by an
// owner/admin via enableAccount. The server rotates the token key, so every
// session — including this one — dies immediately; callers should log out.
export async function disableAccount(email: string): Promise<void> {
    await pb.send('/api/account/disable', {
        method: 'POST',
        body: JSON.stringify({ email }),
        headers: { 'Content-Type': 'application/json' },
    })
}

// enableAccount restores a suspended account. Admin/owner only — the subject
// can't ask for it themselves, since they can no longer authenticate.
export async function enableAccount(userId: string): Promise<void> {
    await pb.send('/api/account/enable', {
        method: 'POST',
        body: JSON.stringify({ user_id: userId }),
        headers: { 'Content-Type': 'application/json' },
    })
}

// deleteAccount irreversibly anonymizes the caller's own account, confirmed by
// re-typing their email.
//
// `plan` is optional: omitting it leaves authored content in place (attributed
// to "Deleted user"), which is the safe default — a malformed request must
// never be read as "delete everything I ever wrote".
export async function deleteMyAccount(email: string, plan?: OffboardPlan): Promise<OffboardResult> {
    return (await pb.send('/api/account/delete', {
        method: 'POST',
        body: JSON.stringify(plan ? { email, plan } : { email }),
        headers: { 'Content-Type': 'application/json' },
    })) as OffboardResult
}

// offboardMember removes ANOTHER user. Admin/owner only. Separate from
// deleteAccount because that route is self-only by construction (it confirms
// by the caller's own email); an admin can neither type the target's email nor
// be the subject.
export async function offboardMember(userId: string, plan: OffboardPlan): Promise<OffboardResult> {
    return (await pb.send('/api/admin/users/offboard', {
        method: 'POST',
        body: JSON.stringify({ user_id: userId, plan }),
        headers: { 'Content-Type': 'application/json' },
    })) as OffboardResult
}

// Labels keyed by "<collection>.<field>" for the record-counts panel. The
// backend returns generic identifiers; the UI maps them to human copy.
// Unknown keys fall back to a humanized version of the key itself.
const COUNT_LABELS: Record<string, string> = {
    'calendar_events.created_by': 'Calendar events',
    'drive_items.created_by': 'Drive items',
    'drive_shares.created_by': 'Drive shares',
    'drive_item_versions.created_by': 'Drive item versions',
    'drive_share_links.created_by': 'Drive share links',
    'calc_comments.author': 'Spreadsheet comments',
    'text_comments.author': 'Document comments',
}

export function labelForCount(key: string): string {
    return COUNT_LABELS[key] ?? humanizeKey(key)
}

function humanizeKey(key: string): string {
    const [collection] = key.split('.')
    return collection.replace(/_/g, ' ').replace(/^\w/, c => c.toUpperCase())
}
