import { FormErrorSummary, TextInput, Toggle } from '@tinycld/core/ui/form'
import { Pressable, Text, View } from 'react-native'
import { useRestore } from './useBackups'

type Props = { isVisible: boolean; isBusy: boolean }

export function RestoreForm({ isVisible, isBusy }: Props) {
    const { form, start } = useRestore()
    const { control, handleSubmit, formState } = form
    const onSubmit = handleSubmit(data => start.mutate(data))
    const isDisabled = isBusy || start.isPending || !formState.isValid

    if (!isVisible) return null

    return (
        <View className="rounded-xl border border-danger p-4 gap-3" testID="restore-form">
            <Text className="text-foreground font-semibold">Restore from a backup</Text>
            <Text className="text-xs text-muted-foreground">
                A restore replaces everything in this organization with the contents of the archive.
                A safety copy of the current data is taken first, and it is listed in the history
                below. The app is unavailable while the restore runs. If the package set in the
                archive differs from the one installed here, use the CLI with --force.
            </Text>
            <FormErrorSummary errors={formState.errors} isEnabled={formState.isSubmitted} />
            <TextInput
                control={control}
                name="source"
                label="Archive URL"
                autoCapitalize="none"
                autoCorrect={false}
            />
            <TextInput control={control} name="passphrase" label="Passphrase" secureTextEntry />
            <Toggle
                control={control}
                name="acknowledged"
                label="I understand that all current data is replaced"
            />
            <Pressable
                onPress={onSubmit}
                disabled={isDisabled}
                testID="restore-start"
                className={isDisabled ? 'opacity-50' : ''}
            >
                <Text className="text-danger font-medium">
                    {isBusy ? 'A job is running…' : 'Restore'}
                </Text>
            </Pressable>
        </View>
    )
}
