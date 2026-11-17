import type { OffboardPlan } from '@tinycld/core/lib/account'
import { confirmTextSchema, z } from '@tinycld/core/ui/form'

export const offboardPlanSchema = z
    .object({
        mode: z.enum(['keep', 'reassign', 'delete_my_data']),
        successor_user_id: z.string().optional(),
    })
    .refine(plan => plan.mode !== 'reassign' || Boolean(plan.successor_user_id), {
        message: 'Choose who inherits the content',
        path: ['successor_user_id'],
    }) satisfies z.ZodType<OffboardPlan>

/**
 * Self-delete confirms with the user's own email (any case). When a peer could
 * inherit the content, a plan must be chosen explicitly: a destructive option is
 * never pre-selected, and the picker shows exactly what the submit sends. With
 * no peers the picker is hidden and no plan is the deliberate leave-content path.
 */
export function deleteAccountSchemaFor(email: string, hasPeers: boolean) {
    return confirmTextSchema(email, { ignoreCase: true })
        .extend({ plan: offboardPlanSchema.optional() })
        .refine(values => !hasPeers || values.plan !== undefined, {
            message: 'Choose what happens to your files, documents and comments',
            path: ['plan'],
        })
}

/** Admin removal confirms with the member's display name, matched exactly. */
export function removeMemberSchemaFor(displayName: string) {
    return confirmTextSchema(displayName).extend({ plan: offboardPlanSchema.optional() })
}

/**
 * The plan an admin removal submits when the admin has not picked one: keep
 * the member's work, credited to the deleted user.
 */
export function defaultRemovalPlan(): OffboardPlan {
    return { mode: 'keep' }
}
