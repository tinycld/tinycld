import { useMutation } from '@tanstack/react-query'
import { disableAccount } from '@tinycld/core/lib/account'
import { errorToString } from '@tinycld/core/lib/errors'
import { useState } from 'react'
import { Text, View } from 'react-native'
import {
    CancelButton,
    ConfirmInput,
    DangerButton,
    ErrorMessage,
    ModalShell,
} from './OffboardDialog'

export interface DisableAccountFlowProps {
    isVisible: boolean
    onClose: () => void
    onSuccess: () => void
    email: string
}

// Reversible suspension. Nothing is deleted: the account, its files and its
// authored content all survive, and an owner/admin can restore access.
//
// No content picker here — that is the whole point of disable versus delete.
export function DisableAccountFlow(props: DisableAccountFlowProps) {
    if (!props.isVisible) return null
    return <DisableAccountFlowOpen {...props} />
}

function DisableAccountFlowOpen({ onClose, onSuccess, email }: DisableAccountFlowProps) {
    const [typed, setTyped] = useState('')
    const [error, setError] = useState<string | null>(null)

    const mutation = useMutation({
        mutationFn: () => disableAccount(email),
        onSuccess,
        onError: e => setError(errorToString(e)),
    })

    const matches = typed.trim().toLowerCase() === email.trim().toLowerCase()

    return (
        <ModalShell>
            <View className="gap-4">
                <Text className="text-foreground text-lg font-bold">Disable your account?</Text>
                <Text className="text-muted-foreground text-sm">
                    You'll be signed out everywhere and won't be able to sign in or open shared
                    files until an administrator restores your access. Nothing is deleted — your
                    files, documents and comments stay exactly as they are.
                </Text>

                <ErrorMessage message={error} />

                <ConfirmInput
                    testID="disable-account-confirm"
                    label="Type your email address to confirm"
                    placeholder={email}
                    value={typed}
                    onChangeText={setTyped}
                    editable={!mutation.isPending}
                />

                <DangerButton
                    testID="disable-account-submit"
                    label="Disable my account"
                    onPress={() => mutation.mutate()}
                    disabled={!matches || mutation.isPending}
                    isPending={mutation.isPending}
                />
                <CancelButton onPress={onClose} disabled={mutation.isPending} />
            </View>
        </ModalShell>
    )
}
