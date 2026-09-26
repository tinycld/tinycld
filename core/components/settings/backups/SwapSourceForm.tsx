import { TextInput } from '@tinycld/core/ui/form'
import { Pressable, Text, View } from 'react-native'
import { useSwapSource } from './useBackups'

type Props = { isVisible: boolean; jobId: string }

export function SwapSourceForm({ isVisible, jobId }: Props) {
    const { form, swap } = useSwapSource(jobId)
    const { control, handleSubmit, formState } = form
    const onSubmit = handleSubmit(data => swap.mutate(data))
    const isDisabled = swap.isPending || !formState.isValid

    if (!isVisible) return null

    return (
        <View className="gap-2 mt-1">
            <Text className="text-xs text-muted-foreground">
                The download URL expired before the archive was read. Paste a fresh one to continue
                this restore.
            </Text>
            <TextInput
                control={control}
                name="source"
                label="Fresh URL"
                autoCapitalize="none"
                autoCorrect={false}
            />
            <Pressable
                onPress={onSubmit}
                disabled={isDisabled}
                testID={`backup-swap-${jobId}`}
                className={isDisabled ? 'opacity-50' : ''}
            >
                <Text className="text-primary font-medium">
                    {swap.isPending ? 'Sending…' : 'Continue restore'}
                </Text>
            </Pressable>
        </View>
    )
}
