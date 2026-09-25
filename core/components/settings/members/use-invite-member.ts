import {
    errorToString,
    extractValidationErrors,
    handleMutationErrorsWithForm,
} from '@tinycld/core/lib/errors'
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

interface InviteForm {
    setError: UseFormSetError<InviteFormValues>
    getValues: UseFormGetValues<InviteFormValues>
}

/**
 * Error handling for a screen with no toast-watching context (the setup
 * wizard): a refusal such as a seat limit has no field, and a toast would be
 * easy to miss next to the form that caused it, so it goes on the form itself.
 */
export function inviteErrorsOnForm(form: {
    setError: UseFormSetError<InviteFormValues>
    getValues: () => InviteFormValues
}) {
    return (error: unknown) => {
        const fields = extractValidationErrors(error)
        const known = Object.keys(form.getValues())
        if (fields && Object.keys(fields).every(f => known.includes(f))) {
            for (const [field, message] of Object.entries(fields)) {
                form.setError(field as keyof InviteFormValues, { type: 'manual', message })
            }
            return
        }
        form.setError('root', { type: 'server', message: errorToString(error) })
    }
}

/** Creates a pending member and returns the link that lets them set a password. */
export function useInviteMember(
    opts: InviteForm & {
        onInvited: (result: InviteResult) => void
        /** Show every refusal on the form instead of as a toast. */
        errorsOnForm?: boolean
    }
) {
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
        onError: opts.errorsOnForm
            ? inviteErrorsOnForm(opts)
            : handleMutationErrorsWithForm({
                  setError: opts.setError,
                  getValues: opts.getValues,
              }),
    })
}
