import type { ReactNode } from 'react'
import { useState } from 'react'
import { Platform, Text, View } from 'react-native'
import { isFileDrag } from '../lib/file-drop/file-drop-controller'
import { useThemeColor } from '../lib/use-app-theme'

interface DropZoneProps {
    children: ReactNode
    /** Called with the dropped files. Directories are ignored — see below. */
    onDrop: (files: File[]) => void
    isEnabled: boolean
    /** Overlay copy while a drag is over the zone. */
    label?: string
    testID?: string
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
    testID,
}: DropZoneProps) {
    const [isDragging, setIsDragging] = useState(false)
    const accentColor = useThemeColor('primary')

    if (Platform.OS !== 'web') {
        return <View className="flex-1">{children}</View>
    }

    // A disabled zone still claims a FILE drag: left alone, the browser
    // performs its default drop and replaces the app with the file. Only the
    // overlay and the upload are gated on isEnabled, as in use-file-drop.
    //
    // A drag an inner target already claimed (the editor takes an image drop)
    // is that target's: setting dropEffect to 'none' on it would cancel the
    // drop before the editor ever sees it.
    const claimDrag = (e: React.DragEvent) => {
        if (!isEnabled && (e.defaultPrevented || !isFileDrag(e.dataTransfer))) return false
        e.preventDefault()
        e.stopPropagation()
        if (isEnabled) return true
        e.dataTransfer.dropEffect = 'none'
        return false
    }

    const handleDragEnter = (e: React.DragEvent) => {
        if (claimDrag(e) && isFileDrag(e.dataTransfer)) {
            setIsDragging(true)
        }
    }

    const handleDragOver = (e: React.DragEvent) => {
        if (e.defaultPrevented) return
        claimDrag(e)
    }

    const handleDragLeave = (e: React.DragEvent) => {
        if (!claimDrag(e)) return
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
        if (e.defaultPrevented) {
            setIsDragging(false)
            return
        }
        if (!claimDrag(e)) return
        setIsDragging(false)

        const files = Array.from(e.dataTransfer.files)
        if (files.length > 0) {
            onDrop(files)
        }
    }

    return (
        <div
            // A static drop target on purpose: drag-and-drop is a pointer-only
            // convenience with no keyboard equivalent by nature, so the div
            // carries no interactive role. Each caller must offer a keyboard
            // path to the same action (a button that opens the file picker).
            // role="none" adds no semantics; it is here only because biome's
            // static-element rule accepts it on an element with handlers, so
            // do not read it as a deliberate accessibility choice.
            role="none"
            data-testid={testID}
            // A data attribute, not aria-disabled: aria-disabled is inherited,
            // so a zone that only refuses drops would mark every control inside
            // it disabled — a reader's whole card, comments and reactions too.
            data-enabled={isEnabled ? 'true' : 'false'}
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
                // Opaque on purpose: a translucent tint let the zone's own
                // text and buttons show through and collide with the label.
                <View
                    className="absolute top-0 left-0 right-0 bottom-0 items-center justify-center rounded-xl bg-accent"
                    style={{
                        borderWidth: 2,
                        borderStyle: 'dashed',
                        borderColor: accentColor,
                        zIndex: 100,
                    }}
                    pointerEvents="none"
                >
                    <Text
                        className="text-accent-foreground"
                        style={{ fontSize: 20, fontWeight: '600' }}
                    >
                        {label}
                    </Text>
                </View>
            )}
        </div>
    )
}
