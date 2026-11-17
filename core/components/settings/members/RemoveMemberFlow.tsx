import {
    CancelButton,
    ConfirmInput,
    ContentPlanPicker,
    DangerButton,
    ErrorMessage,
    ModalShell,
    usePeers,
} from '@tinycld/core/components/settings/account/OffboardDialog'
import {
    defaultRemovalPlan,
    removeMemberSchemaFor,
} from '@tinycld/core/components/settings/account/offboard-schemas'
import { type OffboardPlan, type OffboardResult, offboardMember } from '@tinycld/core/lib/account'
import { errorToString } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { Controller, isFormSubmittable, useForm, zodResolver } from '@tinycld/core/ui/form'
import { useMemo } from 'react'
import { Text, View } from 'react-native'

export interface RemoveMemberFlowProps {
    isVisible: boolean
    onClose: () => void
    onSuccess: (result: OffboardResult) => void
    userId: string
    displayName: string
}

// Admin removal of ANOTHER user. Split out of the former dual-mode
// LeaveOrgFlow because the self and admin paths differ in the two things that
// matter most: who the subject is, and how the action is confirmed.
//
// An admin cannot type the target's email (they may not know it, and it isn't
// theirs to confirm with), so confirmation is by typed display name. The
// endpoint differs too — /api/account/* is self-only by construction.
export function RemoveMemberFlow(props: RemoveMemberFlowProps) {
    if (!props.isVisible) return null
    return <RemoveMemberFlowOpen {...props} />
}

function RemoveMemberFlowOpen({ onClose, onSuccess, userId, displayName }: RemoveMemberFlowProps) {
    const peers = usePeers(userId)
    const schema = useMemo(() => removeMemberSchemaFor(displayName), [displayName])
    const {
        control,
        handleSubmit,
        setError,
        watch,
        formState: { isValid, errors },
    } = useForm({
        resolver: zodResolver(schema),
        defaultValues: { confirm: '', plan: undefined },
        mode: 'onChange',
    })
    // Until the admin picks, the plan is the default (keep), so the highlighted
    // option and the submitted plan stay the same thing.
    const plan = watch('plan') ?? defaultRemovalPlan()

    const mutation = useMutation({
        mutationFn: (chosen: OffboardPlan) => offboardMember(userId, chosen),
        onSuccess,
        onError: e => setError('root', { type: 'server', message: errorToString(e) }),
    })

    const submit = handleSubmit(() => mutation.mutate(plan))

    return (
        <ModalShell>
            <View className="gap-4">
                <Text className="text-foreground text-lg font-bold">Remove {displayName}?</Text>
                <Text className="text-muted-foreground text-sm">
                    They're signed out everywhere and leave the members list. Their username becomes
                    free. This cannot be undone —{' '}
                    <Text className="text-foreground font-semibold">disable</Text> the account
                    instead if the change might be temporary.
                </Text>

                <Controller
                    control={control}
                    name="plan"
                    render={({ field }) => (
                        <ContentPlanPicker
                            peers={peers}
                            subjectLabel={`${displayName}'s`}
                            value={plan}
                            onChange={field.onChange}
                            disabled={mutation.isPending}
                            allowKeep
                        />
                    )}
                />

                <ErrorMessage message={errors.root?.message ?? null} />

                <Controller
                    control={control}
                    name="confirm"
                    render={({ field }) => (
                        <ConfirmInput
                            testID="remove-member-confirm"
                            label="Type their name to confirm"
                            placeholder={displayName}
                            value={field.value}
                            onChangeText={field.onChange}
                            editable={!mutation.isPending}
                        />
                    )}
                />

                <DangerButton
                    testID="remove-member-submit"
                    label={`Remove ${displayName}`}
                    onPress={submit}
                    disabled={!isFormSubmittable({ isValid, errors }) || mutation.isPending}
                    isPending={mutation.isPending}
                />
                <CancelButton onPress={onClose} disabled={mutation.isPending} />
            </View>
        </ModalShell>
    )
}
