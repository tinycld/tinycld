import { HelpIcon } from '@tinycld/core/components/help/HelpIcon'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { Text, View } from 'react-native'
import { BackupHistory } from './BackupHistory'
import { BackupNowForm } from './BackupNowForm'
import { CliCard } from './CliCard'
import { RepositoryCard } from './RepositoryCard'
import { RestoreForm } from './RestoreForm'
import { SnapshotRestoreForm } from './SnapshotRestoreForm'
import { useRepositoryState } from './useBackupRepository'
import { lastBackedUp, useBackupRows } from './useBackups'

export function BackupsSection() {
    const { isOwner } = useCurrentRole()
    const { data: rows } = useBackupRows()
    const status = lastBackedUp(rows)
    const isBusy = rows?.some(
        row => row.status === 'running' || row.status === 'waiting_for_source'
    )
    const toneClass = status.isStale ? 'text-warning' : 'text-muted-foreground'
    const { isConfigured: isRepositoryConfigured, isManaged: isRepositoryManaged } =
        useRepositoryState()

    return (
        <View className="gap-6" testID="settings-section-backups">
            <View className="flex-row items-center gap-1.5">
                <Text className={`text-sm ${toneClass}`} testID="backups-last">
                    {status.label}
                </Text>
                <HelpIcon topic="core:backups" />
            </View>
            <BackupNowForm isBusy={isBusy === true} />
            <RestoreForm isVisible={isOwner} isBusy={isBusy === true} />
            <RepositoryCard isVisible={!isRepositoryManaged} isBusy={isBusy === true} />
            <SnapshotRestoreForm
                isVisible={isOwner && isRepositoryConfigured}
                isBusy={isBusy === true}
            />
            <BackupHistory rows={rows ?? []} />
            <CliCard />
        </View>
    )
}
