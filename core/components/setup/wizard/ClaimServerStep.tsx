import { useQueryClient } from '@tanstack/react-query'
import { useMutation } from '@tinycld/core/lib/mutations'
import { NEEDS_SETUP_QUERY_KEY } from '@tinycld/core/lib/setup/use-needs-setup'
import { Controller, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { useRef } from 'react'
import { Text, View } from 'react-native'
import { CodeInput } from './CodeInput'
import { ServerLogPreview } from './ServerLogPreview'
import { SetupContinueButton } from './SetupContinueButton'
import { StepHeading } from './StepHeading'
import { postSetup, type SetupErrorBody, SetupRequestError } from './setup-api'

export function claimErrorMessage(body: SetupErrorBody | null): string {
    if (!body) return 'The server did not answer. Check that it is running, then try again.'
    return body.error ?? 'That code does not match. Check the server log for the latest code.'
}

/** The server's refusal body, or null when it did not answer. */
export function refusalBodyOf(error: unknown): SetupErrorBody | null {
    if (!(error instanceof SetupRequestError) || error.status === null) return null
    return error.body ?? {}
}

const codeSchema = z.object({
    code: z.string().regex(/^[A-Z0-9]{4}-?[A-Z0-9]{4}$/i, 'Enter the 8-character code'),
})

type CodeForm = z.infer<typeof codeSchema>

function Notice({ message }: { message: string | undefined }) {
    if (!message) return null
    return (
        <View className="rounded-lg border border-danger/30 bg-danger-soft px-3.5 py-3">
            <Text className="text-sm text-danger">{message}</Text>
        </View>
    )
}

function useClaimServer(
    initialCode: string | undefined,
    notice: string | undefined,
    onVerified: (code: string) => void
) {
    const queryClient = useQueryClient()
    const form = useForm<CodeForm>({
        resolver: zodResolver(codeSchema),
        defaultValues: { code: initialCode ?? '' },
    })

    const verify = useMutation({
        mutationFn: (code: string) => postSetup<object>('verify', { code }),
        onSuccess: (_data, code) => onVerified(code),
        onError: error => {
            const body = refusalBodyOf(error)
            // Someone else finished setup: let the route send this person to sign in.
            if (body?.reason === 'done') {
                queryClient.invalidateQueries({ queryKey: NEEDS_SETUP_QUERY_KEY })
            }
            form.setError('code', { type: 'server', message: claimErrorMessage(body) })
        },
    })

    // The printed link carries the code, so verify it once without a click.
    // A ref, not an effect: this is a one-shot action on first render. Not
    // after a rejection: the link's code just failed, and a retry costs a try.
    const autoTried = useRef(false)
    if (initialCode && !notice && !autoTried.current) {
        autoTried.current = true
        verify.mutate(initialCode)
    }

    const onSubmit = form.handleSubmit(data => verify.mutate(data.code))
    return { form, onSubmit, isPending: verify.isPending }
}

/** Proves the person can read the server log before anyone may create the owner. */
export function ClaimServerStep({
    initialCode,
    notice,
    onVerified,
}: {
    initialCode: string | undefined
    /** Why the person was sent back here from the account step. */
    notice: string | undefined
    onVerified: (code: string) => void
}) {
    const { form, onSubmit, isPending } = useClaimServer(initialCode, notice, onVerified)
    const codeError = form.formState.errors.code?.message
    return (
        <View className="gap-5">
            <StepHeading
                title="Claim this server"
                lead="Enter the setup code from the server log. Only someone with access to the server can see it."
            />
            <Notice message={notice} />
            <View className="gap-3">
                <Text className="text-sm font-semibold text-foreground">Setup code</Text>
                <Controller
                    control={form.control}
                    name="code"
                    render={({ field }) => (
                        <CodeInput value={field.value} onChangeText={field.onChange} />
                    )}
                />
                <Notice message={codeError} />
            </View>
            <SetupContinueButton onPress={onSubmit} isDisabled={isPending} />
            <View className="mt-2 gap-2.5">
                <Text className="text-[13px] text-muted-foreground">
                    Look for this box in the server log.
                </Text>
                <ServerLogPreview code="" />
                <Text className="text-[13px] leading-5 text-muted-foreground">
                    Code not in the log? Restart the server. A new code prints each time it starts
                    until the server is claimed.
                </Text>
            </View>
        </View>
    )
}
