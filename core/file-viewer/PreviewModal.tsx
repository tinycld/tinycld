import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useSafeAreaPadding } from '@tinycld/core/lib/use-safe-area'
import { Dialog } from '@tinycld/core/ui/dialog'
import { ChevronLeft, ChevronRight, Download, X } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { GenericPreview } from './previews/GenericPreview'
import { getPreviewEntry } from './registry'
import type { FilePreviewSource, PreviewAction } from './types'

interface PreviewModalProps {
    isVisible: boolean
    source: FilePreviewSource | null
    onClose: () => void
    onNext?: () => void
    onPrevious?: () => void
    /** Called when the user clicks the toolbar download button. If omitted, the button is hidden. */
    onDownload?: () => void
    /** Consumer-supplied toolbar actions (e.g. mail's "Save to Drive"). Each is shown as an icon button before Download. */
    actions?: PreviewAction[]
}

export function PreviewModal({
    isVisible,
    source,
    onClose,
    onNext,
    onPrevious,
    onDownload,
    actions,
}: PreviewModalProps) {
    if (!source) return null

    // A lightbox: the content carries its own toolbar, so no title row, and
    // it is pinned to the dialog presentation — on a phone `full` is the
    // whole screen, which is what a preview wants rather than a sheet.
    return (
        <Dialog
            isOpen={isVisible}
            onClose={onClose}
            title={source.displayName}
            header={null}
            size="full"
            presentation="dialog"
            testID="file-preview-modal"
        >
            <PreviewModalContent
                source={source}
                onClose={onClose}
                onNext={onNext}
                onPrevious={onPrevious}
                onDownload={onDownload}
                actions={actions}
            />
        </Dialog>
    )
}

interface PreviewModalContentProps {
    source: FilePreviewSource
    onClose: () => void
    onNext?: () => void
    onPrevious?: () => void
    onDownload?: () => void
    actions?: PreviewAction[]
}

function PreviewModalContent({
    source,
    onClose,
    onNext,
    onPrevious,
    onDownload,
    actions,
}: PreviewModalContentProps) {
    const mutedColor = useThemeColor('muted-foreground')
    const toolbarPadding = useSafeAreaPadding({ horizontal: 16, top: 12 })

    const entry = getPreviewEntry(source.mimeType)
    const PreviewComponent = entry?.preview ?? GenericPreview

    return (
        <>
            <View
                className="flex-row items-center px-4 py-3 gap-3 border-b border-border"
                style={{
                    // A full-screen native modal: it bypasses every ancestor's
                    // padding, so it must clear the sensor housing itself. px-4
                    // (16pt) is nowhere near a landscape notch inset, which
                    // clipped the filename on one side and the close X on the
                    // other — leaving no way out of the viewer. Bottom padding
                    // is left to py-3: this is a top toolbar, not a full screen.
                    paddingLeft: toolbarPadding.paddingLeft,
                    paddingRight: toolbarPadding.paddingRight,
                    paddingTop: toolbarPadding.paddingTop,
                }}
            >
                <Text
                    numberOfLines={1}
                    className="flex-1 text-foreground"
                    style={{
                        fontSize: 16,
                        fontWeight: '600',
                    }}
                >
                    {source.displayName}
                </Text>
                <View className="flex-row items-center gap-1">
                    {onPrevious && (
                        <Pressable onPress={onPrevious} className="p-1.5 rounded-md" hitSlop={8}>
                            <ChevronLeft size={20} color={mutedColor} />
                        </Pressable>
                    )}
                    {onNext && (
                        <Pressable onPress={onNext} className="p-1.5 rounded-md" hitSlop={8}>
                            <ChevronRight size={20} color={mutedColor} />
                        </Pressable>
                    )}
                    {actions
                        ?.filter(action => action.isApplicable?.(source) ?? true)
                        .map(action => {
                            const ActionIcon = action.icon
                            return (
                                <Pressable
                                    key={action.id}
                                    onPress={() => action.onPress(source, { close: onClose })}
                                    disabled={action.isPending}
                                    className="flex-row items-center gap-1.5 px-2.5 py-1.5 rounded-md bg-surface-secondary border border-border"
                                    hitSlop={8}
                                    accessibilityLabel={action.label}
                                >
                                    <ActionIcon size={16} color={mutedColor} />
                                    <Text
                                        className="text-foreground"
                                        style={{ fontSize: 13, fontWeight: '500' }}
                                    >
                                        {action.label}
                                    </Text>
                                </Pressable>
                            )
                        })}
                    {onDownload && (
                        <Pressable onPress={onDownload} className="p-1.5 rounded-md" hitSlop={8}>
                            <Download size={18} color={mutedColor} />
                        </Pressable>
                    )}
                    <Pressable onPress={onClose} className="p-1.5 rounded-md ml-1" hitSlop={8}>
                        <X size={20} color={mutedColor} />
                    </Pressable>
                </View>
            </View>
            <View className="flex-1 overflow-hidden">
                <PreviewComponent
                    source={source}
                    onClose={onClose}
                    onNext={onNext}
                    onPrevious={onPrevious}
                />
            </View>
        </>
    )
}
