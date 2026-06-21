import { notify } from '@tinycld/core/lib/notify'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { BottomDrawer } from '@tinycld/core/ui/bottom-drawer'
import * as DocumentPicker from 'expo-document-picker'
import * as ImagePicker from 'expo-image-picker'
import { Camera, FileIcon, ImageIcon, type LucideIcon } from 'lucide-react-native'
import { useCallback, useMemo, useState } from 'react'
import { Platform, Pressable, Text, View } from 'react-native'
import {
    documentAssetToPickedFile,
    imageAssetToPickedFile,
    type PickedFile,
    webFileToPickedFile,
} from './picked-file'

export type PickerSource = 'photoLibrary' | 'camera' | 'documents'

export interface PickFilesOptions {
    /** Which sources to offer on mobile. Ignored on web (always opens a file input). */
    sources?: PickerSource[]
    /** Allow multi-select where supported. Defaults to true. */
    multiple?: boolean
    /** Restrict the picker by MIME type — passed to the underlying expo/document picker where supported. */
    mimeTypes?: string[]
}

interface NormalizedOptions {
    sources: PickerSource[]
    multiple: boolean
    mimeTypes?: string[]
}

interface PendingPick {
    options: NormalizedOptions
    resolve: (files: PickedFile[]) => void
}

export function usePickFiles() {
    const [pending, setPending] = useState<PendingPick | null>(null)

    const finishPending = useCallback((next: PendingPick | null, files: PickedFile[]) => {
        // Caller is responsible for passing the same `pending` snapshot they observed
        // so we resolve exactly the right awaiter.
        setPending(prev => (prev === next ? null : prev))
        next?.resolve(files)
    }, [])

    const pickFiles = useCallback((options: PickFilesOptions = {}): Promise<PickedFile[]> => {
        const normalized: NormalizedOptions = {
            sources: options.sources ?? ['photoLibrary', 'camera', 'documents'],
            multiple: options.multiple ?? true,
            mimeTypes: options.mimeTypes,
        }
        // Web: skip the ActionSheet entirely; open a hidden file input.
        if (Platform.OS === 'web') {
            return openWebFileInput(normalized)
        }
        // If only one source is requested, skip the chooser and launch directly.
        if (normalized.sources.length === 1) {
            return launchSource(normalized.sources[0], normalized)
        }
        return new Promise(resolve => {
            setPending({ options: normalized, resolve })
        })
    }, [])

    const handleClose = useCallback(() => {
        // Resolve whichever pending we were holding with [].
        setPending(prev => {
            prev?.resolve([])
            return null
        })
    }, [])

    const handleSourceSelected = useCallback(
        async (source: PickerSource) => {
            // Snapshot the pending pick BEFORE awaiting the picker. If the user
            // dismisses the sheet mid-flight, finishPending sees a different
            // `prev` and leaves it alone; we still resolve our own snapshot.
            const snapshot = pending
            if (!snapshot) return
            const result = await launchSource(source, snapshot.options)
            finishPending(snapshot, result)
        },
        [pending, finishPending]
    )

    const ActionSheetElement = useMemo(() => {
        if (Platform.OS === 'web') return null
        const isOpen = pending !== null
        const sources = pending?.options.sources ?? []
        return (
            <BottomDrawer isOpen={isOpen} onClose={handleClose}>
                <View className="px-2 pb-4">
                    {sources.includes('photoLibrary') && (
                        <PickerRow
                            icon={ImageIcon}
                            label="Photo library"
                            onPress={() => handleSourceSelected('photoLibrary')}
                        />
                    )}
                    {sources.includes('camera') && (
                        <PickerRow
                            icon={Camera}
                            label="Take a photo"
                            onPress={() => handleSourceSelected('camera')}
                        />
                    )}
                    {sources.includes('documents') && (
                        <PickerRow
                            icon={FileIcon}
                            label="Documents"
                            onPress={() => handleSourceSelected('documents')}
                        />
                    )}
                </View>
            </BottomDrawer>
        )
    }, [pending, handleClose, handleSourceSelected])

    return { pickFiles, ActionSheetElement }
}

function PickerRow({
    icon: Icon,
    label,
    onPress,
}: {
    icon: LucideIcon
    label: string
    onPress: () => void
}) {
    const foreground = useThemeColor('foreground')
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

function openWebFileInput(options: {
    multiple: boolean
    mimeTypes?: string[]
}): Promise<PickedFile[]> {
    if (typeof document === 'undefined') return Promise.resolve([])
    return new Promise(resolve => {
        const input = document.createElement('input')
        input.type = 'file'
        if (options.multiple) input.multiple = true
        if (options.mimeTypes && options.mimeTypes.length > 0) {
            input.accept = options.mimeTypes.join(',')
        }
        let settled = false
        const settle = (files: PickedFile[]) => {
            if (settled) return
            settled = true
            resolve(files)
        }
        input.onchange = () => {
            const files = input.files ? Array.from(input.files).map(webFileToPickedFile) : []
            settle(files)
        }
        // Modern browsers fire 'cancel' when the chooser is dismissed without a
        // selection (Chromium 113+, Safari 16.4+). Older browsers don't, so we
        // also resolve [] when the window regains focus and no change has fired
        // shortly after — covers the long-tail without leaking the resolver.
        input.addEventListener('cancel', () => settle([]))
        const onWindowFocus = () => {
            // Defer briefly so the 'change' event has a chance to fire first.
            setTimeout(() => settle([]), 200)
        }
        window.addEventListener('focus', onWindowFocus, { once: true })
        input.click()
    })
}

async function launchSource(
    source: PickerSource,
    options: NormalizedOptions
): Promise<PickedFile[]> {
    if (source === 'documents') {
        const result = await DocumentPicker.getDocumentAsync({
            multiple: options.multiple,
            type: options.mimeTypes,
        })
        if (result.canceled) return []
        return result.assets.map(documentAssetToPickedFile)
    }
    if (source === 'photoLibrary') {
        const result = await ImagePicker.launchImageLibraryAsync({
            mediaTypes: ['images', 'videos'],
            allowsMultipleSelection: options.multiple,
            quality: 1,
            exif: false,
        })
        if (result.canceled) return []
        return result.assets.map(imageAssetToPickedFile)
    }
    if (source === 'camera') {
        const permission = await ImagePicker.requestCameraPermissionsAsync()
        if (!permission.granted) {
            notify.emit({
                event: 'mail.attachments_rejected',
                title: 'Camera access required',
                body: 'Grant camera permission in Settings to take a photo.',
                durationMs: 5000,
                data: { reason: 'camera-permission-denied' },
            })
            return []
        }
        const result = await ImagePicker.launchCameraAsync({
            mediaTypes: ['images'],
            quality: 1,
            exif: false,
        })
        if (result.canceled) return []
        return result.assets.map(imageAssetToPickedFile)
    }
    return []
}
