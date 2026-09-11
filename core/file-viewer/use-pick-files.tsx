import { notify } from '@tinycld/core/lib/notify'
import * as DocumentPicker from 'expo-document-picker'
import * as ImagePicker from 'expo-image-picker'
import { useCallback } from 'react'
import { Platform } from 'react-native'
import {
    documentAssetToPickedFile,
    imageAssetToPickedFile,
    type PickedFile,
    webFileToPickedFile,
} from './picked-file'
import { type PickerSource, usePickerSheetStore } from './picker-sheet-store'

export type { PickerSource }

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

export function usePickFiles() {
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
        // Multiple sources: the chooser sheet renders in FilePickerSheetHost at
        // the layout level (see picker-sheet-store for why not inline here).
        return new Promise(resolve => {
            usePickerSheetStore.getState().open({
                sources: normalized.sources,
                onSelect: async source => {
                    if (!source) {
                        resolve([])
                        return
                    }
                    resolve(await launchSource(source, normalized))
                },
            })
        })
    }, [])

    return { pickFiles }
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
        // In the DOM while the dialog is open: a detached input can be
        // garbage-collected mid-pick, and its change event dies with it.
        input.style.display = 'none'
        document.body.appendChild(input)
        let settled = false
        const settle = (files: PickedFile[]) => {
            if (settled) return
            settled = true
            input.remove()
            resolve(files)
        }
        input.onchange = () => {
            const files = input.files ? Array.from(input.files).map(webFileToPickedFile) : []
            settle(files)
        }
        // Dismissal fires 'cancel' in every supported browser (Chromium 113+,
        // Safari 16.4+, Firefox 91+). There is deliberately NO window-focus
        // fallback: the change event lands after the window refocuses, by an
        // OS-dependent delay no grace period can bound, so a focus-based
        // cancel detector discards real selections.
        input.addEventListener('cancel', () => settle([]))
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
