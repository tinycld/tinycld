import { formatBytes, formatTimeAgo } from '@tinycld/core/lib/format-utils'
import * as Clipboard from 'expo-clipboard'
import { Pressable, Text, View } from 'react-native'
import { SwapSourceForm } from './SwapSourceForm'
import { type BackupRow, isAwaitingRestart, useDismissPreRestoreKey } from './useBackups'

type Props = { rows: BackupRow[] }

const KIND_LABEL: Record<string, string> = {
    manual: 'Manual backup',
    scheduled: 'Scheduled backup',
    pre_restore: 'Pre-restore safety copy',
    restore: 'Restore',
}

const STATUS_LABEL: Record<string, string> = {
    succeeded: 'Succeeded',
    failed: 'Failed',
    running: 'Running',
    waiting_for_source: 'Waiting for source',
    interrupted: 'Interrupted',
}

const FAILED_STATUSES = new Set(['failed', 'interrupted'])

export function BackupHistory({ rows }: Props) {
    if (rows.length === 0) {
        return <Text className="text-sm text-muted-foreground">No backups yet.</Text>
    }
    return (
        <View className="gap-2">
            <Text className="text-foreground font-semibold">History</Text>
            {rows.map(row => (
                <HistoryRow key={row.id} row={row} />
            ))}
        </View>
    )
}

function HistoryRow({ row }: { row: BackupRow }) {
    const size = row.bytes ? formatBytes(row.bytes) : '—'
    const who = row.initiatorName ?? 'System'
    return (
        <View className="rounded-lg border border-border p-3 gap-1" testID={`backup-row-${row.id}`}>
            <View className="flex-row justify-between">
                <Text className="text-foreground">{KIND_LABEL[row.kind] ?? row.kind}</Text>
                <StatusBadge status={row.status} />
            </View>
            <Text className="text-xs text-muted-foreground">
                {formatTimeAgo(row.started)} · {who} · {size}
            </Text>
            <ErrorLine message={row.error} />
            <AwaitingRestartLine isVisible={isAwaitingRestart(row)} />
            <SwapSourceForm isVisible={row.status === 'waiting_for_source'} jobId={row.id} />
            <PreRestoreKey row={row} />
        </View>
    )
}

// A staged restore nothing is going to restart for. The status badge already
// says "Running", which is true and useless on its own: nothing will move it.
function AwaitingRestartLine({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text className="text-xs text-warning">Restart the server to complete this restore.</Text>
    )
}

function StatusBadge({ status }: { status: BackupRow['status'] }) {
    const toneClass = FAILED_STATUSES.has(status) ? 'text-danger' : 'text-muted-foreground'
    return <Text className={`text-xs ${toneClass}`}>{STATUS_LABEL[status] ?? status}</Text>
}

function ErrorLine({ message }: { message: string }) {
    if (!message) return null
    return <Text className="text-xs text-danger">{message}</Text>
}

// The identity that unlocks the safety copy taken before this restore. It is
// shown only while the restore could still need rolling back — once the restore
// has succeeded, the safety copy has served its purpose.
function PreRestoreKey({ row }: { row: BackupRow }) {
    const { identity, isDismissed, dismiss } = useDismissPreRestoreKey(row)
    const isRelevant = row.kind === 'restore' && row.status !== 'succeeded'

    if (!identity || isDismissed || !isRelevant) return null

    return (
        <View className="gap-1 mt-1">
            <Text className="text-xs text-warning">
                Save this key — it is the only way to read the safety copy taken before this
                restore.
            </Text>
            <Text className="font-mono text-xs text-foreground" selectable>
                {identity}
            </Text>
            <View className="flex-row gap-4">
                <Pressable
                    testID={`backup-key-copy-${row.id}`}
                    onPress={() => Clipboard.setStringAsync(identity)}
                >
                    <Text className="text-xs text-primary">Copy key</Text>
                </Pressable>
                <Pressable testID={`backup-key-dismiss-${row.id}`} onPress={dismiss}>
                    <Text className="text-xs text-muted-foreground">I saved it</Text>
                </Pressable>
            </View>
        </View>
    )
}
