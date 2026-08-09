import type { ReactNode } from 'react'
import { useState } from 'react'
import { Platform, Text, View } from 'react-native'
import { useThemeColor } from '../lib/use-app-theme'

interface DropZoneProps {
    children: ReactNode
    /** Called with the dropped files. Directories are ignored — see below. */
    onDrop: (files: File[]) => void
    isEnabled: boolean
    /** Overlay copy while a drag is over the zone. */
    label?: string
}

/**
 * Wraps a surface so files can be dragged onto it.
 *
 * Web only in effect: on native it renders its children in a plain flex View
 * and nothing else, so callers need no platform branch of their own.
 *
 * This handles FILES only. Dropping a folder yields no files here — drive
 * keeps the `webkitGetAsEntry` recursion for that, because it is the only
 * package with a folder model to put a tree into.
 */
export function DropZone({
    children,
    onDrop,
    isEnabled,
    label = 'Drop files to upload',
}: DropZoneProps) {
    const [isDragging, setIsDragging] = useState(false)
    const accentColor = useThemeColor('primary')

    if (Platform.OS !== 'web') {
        return <View className="flex-1">{children}</View>
    }

    const handleDragEnter = (e: React.DragEvent) => {
        if (!isEnabled) return
        e.preventDefault()
        e.stopPropagation()
        if (e.dataTransfer.types.includes('Files')) {
            setIsDragging(true)
        }
    }

    const handleDragOver = (e: React.DragEvent) => {
        if (!isEnabled) return
        e.preventDefault()
        e.stopPropagation()
    }

    const handleDragLeave = (e: React.DragEvent) => {
        if (!isEnabled) return
        e.preventDefault()
        e.stopPropagation()
        // dragleave also fires when the pointer crosses onto a CHILD element,
        // so the overlay may only be dismissed once the pointer is genuinely
        // outside the zone's own box.
        const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
        const { clientX, clientY } = e
        if (
            clientX <= rect.left ||
            clientX >= rect.right ||
            clientY <= rect.top ||
            clientY >= rect.bottom
        ) {
            setIsDragging(false)
        }
    }

    const handleDrop = (e: React.DragEvent) => {
        if (!isEnabled) return
        e.preventDefault()
        e.stopPropagation()
        setIsDragging(false)

        const files = Array.from(e.dataTransfer.files)
        if (files.length > 0) {
            onDrop(files)
        }
    }

    return (
        // biome-ignore lint/a11y/noStaticElementInteractions: drag-and-drop zone requires native DOM events
        <div
            role="presentation"
            style={{
                flex: 1,
                position: 'relative',
                display: 'flex',
                flexDirection: 'column',
                minHeight: 0,
            }}
            onDragEnter={handleDragEnter}
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            onDrop={handleDrop}
        >
            {children}
            {isDragging && isEnabled && (
                <View
                    className="absolute top-0 left-0 right-0 bottom-0 items-center justify-center rounded-xl bg-accent/10"
                    style={{
                        borderWidth: 2,
                        borderStyle: 'dashed',
                        borderColor: accentColor,
                        zIndex: 100,
                    }}
                    pointerEvents="none"
                >
                    <Text className="text-accent" style={{ fontSize: 20, fontWeight: '600' }}>
                        {label}
                    </Text>
                </View>
            )}
        </div>
    )
}
