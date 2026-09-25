import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { pb } from '@tinycld/core/lib/pocketbase'
import { z } from '@tinycld/core/ui/form'
import type { UseFormGetValues, UseFormSetError } from 'react-hook-form'

export const inviteSchema = z.object({
    username: z
        .string()
        .regex(
            /^[a-z0-9][a-z0-9_-]{0,31}$/,
            'Use 1-32 chars: lowercase letters, digits, dash or underscore'
        ),
    email: z.string().email('Enter a valid email address').or(z.literal('')).optional(),
    role: z.enum(['admin', 'member', 'guest']),
})

export type InviteFormValues = z.infer<typeof inviteSchema>

export interface InviteResult {
    userId: string
    inviteUrl: string
}

/** Creates a pending member and returns the link that lets them set a password. */
export function useInviteMember(opts: {
    setError: UseFormSetError<InviteFormValues>
    getValues: UseFormGetValues<InviteFormValues>
    onInvited: (result: InviteResult) => void
}) {
    return useMutation({
        // Single-org: /api/invite-member returns `userId` (the user_org
        // junction is gone). Reading a junction-row id here yielded
        // undefined, so the link panel called
        // /api/invite-link/undefined/{rotate,send} and got a 404 — the
        // invite itself succeeded, only its follow-up actions were broken.
        mutationFn: async (data: InviteFormValues) =>
            pb.send<InviteResult>('/api/invite-member', {
                method: 'POST',
                body: JSON.stringify({
                    username: data.username.trim().toLowerCase(),
                    email: data.email?.trim() ?? '',
                    role: data.role,
                }),
                headers: { 'Content-Type': 'application/json' },
            }),
        onSuccess: data => opts.onInvited({ userId: data.userId, inviteUrl: data.inviteUrl }),
        onError: handleMutationErrorsWithForm({
            setError: opts.setError,
            getValues: opts.getValues,
        }),
    })
}
