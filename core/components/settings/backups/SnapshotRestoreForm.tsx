import { formatBytes, formatTimeAgo } from '@tinycld/core/lib/format-utils'
import { ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import { FormErrorSummary, RadioInput, Toggle } from '@tinycld/core/ui/form'
import { Text, View } from 'react-native'
import { useSnapshotRestore, useSnapshots } from './useBackupRepository'

type Props = { isVisible: boolean; isBusy: boolean }

function ListError({ error }: { error: unknown }) {
    if (!(error instanceof Error)) return null
    return <Text className="text-xs text-danger">{error.message}</Text>
}

export function SnapshotRestoreForm({ isVisible, isBusy }: Props) {
    const { data: snapshots, error } = useSnapshots(isVisible)
    const { form, start } = useSnapshotRestore()
    const { control, handleSubmit, formState } = form
    const onSubmit = handleSubmit(data => start.mutate(data))
    const isDisabled = isBusy || start.isPending || !formState.isValid
    const options = (snapshots ?? []).map(snapshot => ({
        value: snapshot.ref,
        label: `${formatTimeAgo(snapshot.created)} · ${formatBytes(snapshot.bytes)}`,
    }))

    if (!isVisible) return null

    return (
        <View className="rounded-xl border border-danger p-4 gap-3" testID="snapshot-restore-form">
            <Text className="text-foreground font-semibold">Restore from the repository</Text>
            <ListError error={error} />
            <FormErrorSummary errors={formState.errors} isEnabled={formState.isSubmitted} />
            <RadioInput control={control} name="snapshot" label="Snapshot" options={options} />
            <Toggle
                control={control}
                name="acknowledged"
                label="I understand that all current data is replaced"
            />
            <ServerActionButton
                variant="link"
                onPress={onSubmit}
                isDisabled={isDisabled}
                testID="snapshot-restore-start"
            >
                <ButtonText className="text-danger">
                    {isBusy ? 'A job is running…' : 'Restore'}
                </ButtonText>
            </ServerActionButton>
        </View>
    )
}
