// Who may do what with a saved report, as the sharing contracts' table says
// (docs/plans/2026-10-30-report-sharing.md). The server decides; this only
// chooses which actions to offer, so an action never appears that is refused.

export const SHARE_ROLES = [
    { value: 'viewer', label: 'Viewer' },
    { value: 'editor', label: 'Editor' },
] as const
export type ShareRole = (typeof SHARE_ROLES)[number]['value']

export interface ReportAccess {
    isOwner: boolean
    /** Owner or editor (direct or through a group): may change the report. */
    canEdit: boolean
    canShare: boolean
    /** An admin who may also edit (Ruling P4). */
    canPublish: boolean
    /** The owner, or an admin on a published report; never a shipped report. */
    canDelete: boolean
    isShipped: boolean
}

export const NO_ACCESS: ReportAccess = {
    isOwner: false,
    canEdit: false,
    canShare: false,
    canPublish: false,
    canDelete: false,
    isShipped: false,
}

export function reportAccess(input: {
    owner: string
    published: boolean
    shippedKey: string
    userId: string
    roles: readonly ShareRole[]
    isAdmin: boolean
}): ReportAccess {
    const isOwner = input.owner === input.userId
    const canEdit = isOwner || input.roles.includes('editor')
    const isShipped = input.shippedKey !== ''
    return {
        isOwner,
        canEdit,
        canShare: canEdit,
        canPublish: input.isAdmin && canEdit,
        canDelete: !isShipped && (isOwner || (input.isAdmin && input.published)),
        isShipped,
    }
}

/** A reader's share roles per report: one report may hold a direct and a derived row. */
export function rolesByReport(
    shares: readonly { report: string; role: ShareRole }[]
): Map<string, ShareRole[]> {
    const out = new Map<string, ShareRole[]>()
    for (const share of shares)
        out.set(share.report, [...(out.get(share.report) ?? []), share.role])
    return out
}
