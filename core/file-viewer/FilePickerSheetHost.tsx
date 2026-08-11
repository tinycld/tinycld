import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { BottomDrawer } from '@tinycld/core/ui/bottom-drawer'
import { Camera, FileIcon, ImageIcon, type LucideIcon } from 'lucide-react-native'
import { useCallback } from 'react'
import { Platform, Pressable, Text, View } from 'react-native'
import { type PickerSource, usePickerSheetStore } from './picker-sheet-store'

/**
 * The one mount point for the native file-source chooser. Lives beside
 * MoreDrawer/NotificationDrawer in each layout's content region so the
 * BottomDrawer's bottom edge lands on the tab bar — see picker-sheet-store
 * for why it cannot render inline where pickFiles is called.
 */
export function FilePickerSheetHost() {
    const request = usePickerSheetStore(s => s.request)
    const close = usePickerSheetStore(s => s.close)

    const handleClose = useCallback(() => {
        const current = usePickerSheetStore.getState().request
        close()
        current?.onSelect(null)
    }, [close])

    const handleSelect = useCallback(async (source: PickerSource) => {
        // Snapshot before awaiting: if the user dismisses mid-launch the
        // store empties, but this pick still resolves its own awaiter.
        const current = usePickerSheetStore.getState().request
        if (!current) return
        await current.onSelect(source)
        usePickerSheetStore.setState(s => (s.request === current ? { request: null } : s))
    }, [])

    if (Platform.OS === 'web') return null
    const sources = request?.sources ?? []
    return (
        <BottomDrawer isOpen={request !== null} onClose={handleClose}>
            <View className="px-2 pb-4">
                <SourceRow
                    isVisible={sources.includes('photoLibrary')}
                    icon={ImageIcon}
                    label="Photo library"
                    onPress={() => handleSelect('photoLibrary')}
                />
                <SourceRow
                    isVisible={sources.includes('camera')}
                    icon={Camera}
                    label="Take a photo"
                    onPress={() => handleSelect('camera')}
                />
                <SourceRow
                    isVisible={sources.includes('documents')}
                    icon={FileIcon}
                    label="Documents"
                    onPress={() => handleSelect('documents')}
                />
            </View>
        </BottomDrawer>
    )
}

function SourceRow({
    isVisible,
    icon: Icon,
    label,
    onPress,
}: {
    isVisible: boolean
    icon: LucideIcon
    label: string
    onPress: () => void
}) {
    const foreground = useThemeColor('foreground')
    if (!isVisible) return null
    return (
        <Pressable
            onPress={onPress}
            className="flex-row items-center gap-3 px-3 py-3.5 rounded-md data-[hover=true]:bg-accent"
        >
            <Icon size={20} color={foreground} />
            <Text className="text-foreground text-base">{label}</Text>
        </Pressable>
    )
}
