import { FormErrorSummary, TextInput, Toggle } from '@tinycld/core/ui/form'
import { Pressable, Text, View } from 'react-native'
import { useBackupRepository } from './useBackupRepository'

type Props = { isVisible: boolean; isBusy: boolean }

type TestMessage = { text: string; isError: boolean } | undefined

function describeTest(snapshots: number | undefined, error: unknown): TestMessage {
    if (error instanceof Error) return { text: error.message, isError: true }
    if (snapshots === undefined) return undefined
    return { text: `Connected. ${snapshots} snapshot(s) found.`, isError: false }
}

function TestResult({ message }: { message: TestMessage }) {
    if (!message) return null
    const toneClass = message.isError ? 'text-danger' : 'text-muted-foreground'
    return (
        <Text className={`text-xs ${toneClass}`} testID="repository-test-result">
            {message.text}
        </Text>
    )
}

export function RepositoryCard({ isVisible, isBusy }: Props) {
    const { form, save, test, generateKey, backupNow, isConfigured } = useBackupRepository()
    const { control, handleSubmit, formState } = form
    const onSave = handleSubmit(data => save.mutate(data))
    const onTest = handleSubmit(data => test.mutate(data))
    const testMessage = describeTest(test.data?.snapshots, test.error)
    const isBackupDisabled = isBusy || backupNow.isPending || !isConfigured

    if (!isVisible) return null

    return (
        <View
            className="rounded-xl border border-border bg-surface-secondary p-4 gap-3"
            testID="repository-card"
        >
            <Text className="text-foreground font-semibold">Proxmox Backup Server</Text>
            <Text className="text-xs text-muted-foreground">
                Scheduled backups go to a PBS datastore. Each run is a full snapshot, but PBS stores
                only what changed. Set retention with a prune job on PBS. If you use an encryption
                key, keep a copy somewhere else: without it the backups cannot be read.
            </Text>
            <FormErrorSummary errors={formState.errors} isEnabled={formState.isSubmitted} />
            <TextInput
                control={control}
                name="server"
                label="Server (host:port)"
                autoCapitalize="none"
                autoCorrect={false}
            />
            <TextInput
                control={control}
                name="fingerprint"
                label="Certificate fingerprint"
                autoCapitalize="none"
                autoCorrect={false}
            />
            <TextInput
                control={control}
                name="datastore"
                label="Datastore"
                autoCapitalize="none"
                autoCorrect={false}
            />
            <TextInput
                control={control}
                name="namespace"
                label="Namespace (optional)"
                autoCapitalize="none"
                autoCorrect={false}
            />
            <TextInput
                control={control}
                name="authId"
                label="API token ID"
                autoCapitalize="none"
                autoCorrect={false}
            />
            <TextInput control={control} name="secret" label="API token secret" secureTextEntry />
            <TextInput
                control={control}
                name="key"
                label="Encryption key file (optional)"
                multiline
                autoCapitalize="none"
                autoCorrect={false}
            />
            <Pressable onPress={() => generateKey.mutate()} testID="repository-generate-key">
                <Text className="text-primary font-medium">Generate a key</Text>
            </Pressable>
            <TextInput
                control={control}
                name="schedule"
                label="Schedule (cron)"
                autoCapitalize="none"
                autoCorrect={false}
            />
            <Toggle control={control} name="enabled" label="Back up on this schedule" />
            <View className="flex-row gap-4">
                <Pressable onPress={onTest} testID="repository-test">
                    <Text className="text-primary font-medium">Test connection</Text>
                </Pressable>
                <Pressable onPress={onSave} testID="repository-save">
                    <Text className="text-primary font-medium">Save</Text>
                </Pressable>
                <Pressable
                    onPress={() => backupNow.mutate()}
                    disabled={isBackupDisabled}
                    className={isBackupDisabled ? 'opacity-50' : ''}
                    testID="repository-backup-now"
                >
                    <Text className="text-primary font-medium">Back up now</Text>
                </Pressable>
            </View>
            <TestResult message={testMessage} />
        </View>
    )
}
