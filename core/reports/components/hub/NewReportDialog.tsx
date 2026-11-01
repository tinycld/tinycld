import { Dialog } from '@tinycld/core/ui/dialog'
import { Pressable, Text, View } from 'react-native'
import type { CatalogSource } from '../../lib/types'
import { REPORT_TAP_TARGET } from '../ReportHeader'

function NoSources({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text testID="reports-new-none" className="text-sm text-muted-foreground">
            No report sources are installed.
        </Text>
    )
}

interface NewReportDialogProps {
    isOpen: boolean
    sources: CatalogSource[]
    onPick(sourceKey: string): void
    onClose(): void
}

export function NewReportDialog({ isOpen, sources, onPick, onClose }: NewReportDialogProps) {
    const rows = sources.map(source => (
        <Pressable
            key={source.key}
            testID={`reports-new-source-${source.key}`}
            accessibilityRole="button"
            onPress={() => onPick(source.key)}
            className="justify-center rounded-md px-2"
            style={{ minHeight: REPORT_TAP_TARGET }}
        >
            <Text className="text-sm font-medium text-foreground">{source.label}</Text>
        </Pressable>
    ))
    return (
        <Dialog
            isOpen={isOpen}
            onClose={onClose}
            title="Choose what to report on"
            testID="reports-new-dialog"
        >
            <Dialog.Body>
                <View className="gap-1">
                    {rows}
                    <NoSources isVisible={sources.length === 0} />
                </View>
            </Dialog.Body>
        </Dialog>
    )
}
