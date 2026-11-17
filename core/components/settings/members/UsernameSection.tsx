import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import { FormErrorSummary, TextInput, useForm, zodResolver } from '@tinycld/core/ui/form'
import { View } from 'react-native'
import { type RenameFormValues, renameSchema } from './username-schema'

// An admin renames a member's login. The member signs in with the new name at
// once.
export function UsernameSection({
    isVisible,
    userId,
    username,
}: {
    isVisible: boolean
    userId: string
    username: string
}) {
    if (!isVisible) return null
    return <UsernameForm userId={userId} username={username} />
}

function UsernameForm({ userId, username }: { userId: string; username: string }) {
    const [usersCollection] = useStore('users')
    const {
        control,
        handleSubmit,
        setError,
        getValues,
        formState: { errors, isSubmitted, isDirty, isValid },
    } = useForm<RenameFormValues>({
        mode: 'onChange',
        resolver: zodResolver(renameSchema),
        values: { username },
    })
    const rename = useMutation({
        mutationFn: mutation(function* ({ username: next }: RenameFormValues) {
            yield usersCollection.update(userId, draft => {
                draft.username = next
            })
        }),
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })
    const onSubmit = () => void handleSubmit(data => rename.mutate(data))()

    return (
        <View className="gap-2">
            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />
            <TextInput
                control={control}
                name="username"
                testID="rename-username"
                label="Username"
                hint="Used to sign in."
                autoCapitalize="none"
                autoCorrect={false}
            />
            <ServerActionButton
                size="sm"
                testID="rename-username-submit"
                onPress={onSubmit}
                isDisabled={!isDirty || !isValid || rename.isPending}
            >
                <ButtonText>{rename.isPending ? 'Saving…' : 'Change username'}</ButtonText>
            </ServerActionButton>
        </View>
    )
}
