import { FormErrorSummary, TextInput } from '@tinycld/core/ui/form'
import { Pressable, Text, View } from 'react-native'
import { useBackupNow } from './useBackups'

type Props = { isBusy: boolean }

export function BackupNowForm({ isBusy }: Props) {
    const { form, start } = useBackupNow()
    const { control, handleSubmit, formState } = form
    const onSubmit = handleSubmit(data => start.mutate(data))
    const isDisabled = isBusy || start.isPending || !formState.isValid

    return (
        <View className="rounded-xl border border-border bg-surface-secondary p-4 gap-3">
            <Text className="text-foreground font-semibold">Back up now</Text>
            <Text className="text-xs text-muted-foreground">
                The backup is streamed to a URL you provide (a presigned PUT to S3, R2, B2 or any
                compatible store). It is encrypted with your passphrase. If you lose the passphrase,
                the backup cannot be read.
            </Text>
            <FormErrorSummary errors={formState.errors} isEnabled={formState.isSubmitted} />
            <TextInput
                control={control}
                name="target"
                label="Upload URL"
                autoCapitalize="none"
                autoCorrect={false}
            />
            <TextInput control={control} name="passphrase" label="Passphrase" secureTextEntry />
            <TextInput
                control={control}
                name="confirm"
                label="Confirm passphrase"
                secureTextEntry
            />
            <Pressable
                onPress={onSubmit}
                disabled={isDisabled}
                testID="backup-start"
                className={isDisabled ? 'opacity-50' : ''}
            >
                <Text className="text-primary font-medium">
                    {isBusy ? 'A job is running…' : 'Start backup'}
                </Text>
            </Pressable>
        </View>
    )
}
