import { pb } from '@tinycld/core/lib/pocketbase'

export type LeaveOrgMode = 'reassign' | 'delete_my_data' | 'delete_org'

export interface LeaveOrgPlan {
    mode: LeaveOrgMode
    /**
     * Required only when mode === 'reassign'.
     *
     * Omit to let the server auto-pick the oldest owner in the org, falling
     * back to the oldest non-guest peer (who gets promoted to owner inside
     * the leave-org transaction). Pass an explicit user_org id to override.
     * The id must belong to a peer of the same org; the server rejects
     * cross-org / unknown ids with 400.
     */
    successor_user_org_id?: string
}

export interface LeaveOrgResult {
    org_deleted: boolean
    user_anonymized: boolean
    records_reassigned: number
    records_deleted: number
}

export interface LeaveOrgPreviewPeer {
    user_org_id: string
    user_id: string
    name: string
    email: string
    role: string
}

export interface LeaveOrgPreview {
    org_id: string
    org_name: string
    sole_member: boolean
    sole_owner: boolean
    counts: Record<string, number>
    peers: LeaveOrgPreviewPeer[]
}

export async function fetchLeaveOrgPreview(userOrgId: string): Promise<LeaveOrgPreview> {
    return (await pb.send(
        `/api/account/leave-org/preview?user_org_id=${encodeURIComponent(userOrgId)}`,
        { method: 'GET' }
    )) as LeaveOrgPreview
}

export async function postLeaveOrg(userOrgId: string, plan: LeaveOrgPlan): Promise<LeaveOrgResult> {
    return (await pb.send('/api/account/leave-org', {
        method: 'POST',
        body: JSON.stringify({ user_org_id: userOrgId, plan }),
        headers: { 'Content-Type': 'application/json' },
    })) as LeaveOrgResult
}

// Labels keyed by "<collection>.<field>" for the preview counts panel. The
// backend returns generic identifiers; the UI maps them to human copy.
// Unknown keys fall back to a humanized version of the key itself.
const COUNT_LABELS: Record<string, string> = {
    'calendar_events.created_by': 'Calendar events',
    'drive_items.created_by': 'Drive items',
    'drive_shares.created_by': 'Drive shares',
    'drive_item_versions.created_by': 'Drive item versions',
    'drive_share_links.created_by': 'Drive share links',
    'drive_preview_comments.author_user_org': 'Drive preview comments',
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
