import { useMutation } from '@tanstack/react-query'
import {
    CancelButton,
    ConfirmInput,
    ContentPlanPicker,
    DangerButton,
    ErrorMessage,
    ModalShell,
    usePeers,
} from '@tinycld/core/components/settings/account/OffboardDialog'
import { type OffboardPlan, type OffboardResult, offboardMember } from '@tinycld/core/lib/account'
import { errorToString } from '@tinycld/core/lib/errors'
import { useState } from 'react'
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
    const [typed, setTyped] = useState('')
    const [error, setError] = useState<string | null>(null)
    const [plan, setPlan] = useState<OffboardPlan>(
        peers.length > 0
            ? { mode: 'reassign', successor_user_id: peers[0]?.id }
            : { mode: 'delete_my_data' }
    )

    const mutation = useMutation({
        mutationFn: () => offboardMember(userId, plan),
        onSuccess,
        onError: e => setError(errorToString(e)),
    })

    const matches = typed.trim() === displayName.trim()

    return (
        <ModalShell>
            <View className="gap-4">
                <Text className="text-foreground text-lg font-bold">Remove {displayName}?</Text>
                <Text className="text-muted-foreground text-sm">
                    Their account is anonymized and they're signed out everywhere. This cannot be
                    undone — <Text className="text-foreground font-semibold">disable</Text> the
                    account instead if the change might be temporary.
                </Text>

                <ContentPlanPicker
                    peers={peers}
                    subjectLabel={`${displayName}'s`}
                    value={plan}
                    onChange={setPlan}
                    disabled={mutation.isPending}
                />

                <ErrorMessage message={error} />

                <ConfirmInput
                    testID="remove-member-confirm"
                    label="Type their name to confirm"
                    placeholder={displayName}
                    value={typed}
                    onChangeText={setTyped}
                    editable={!mutation.isPending}
                />

                <DangerButton
                    testID="remove-member-submit"
                    label={`Remove ${displayName}`}
                    onPress={() => mutation.mutate()}
                    disabled={!matches || mutation.isPending}
                    isPending={mutation.isPending}
                />
                <CancelButton onPress={onClose} disabled={mutation.isPending} />
            </View>
        </ModalShell>
    )
}
