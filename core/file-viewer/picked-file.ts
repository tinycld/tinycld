import type { UploadFile } from '@tinycld/core/lib/upload-file-types'

/**
 * Pure helpers for normalizing file-picker results into an upload part.
 *
 * The React/Expo bits live in `use-pick-files.ts`, which passes the native
 * `uploadFileFromUri` in as `toFile`; these helpers are split out so the unit
 * tests can run in vitest's node environment.
 */

/** Wraps a picked file on disk as an upload part (core/lib/upload-file). */
export type ToUploadFile = (uri: string, name: string, type: string) => UploadFile

export interface PickedFile {
    name: string
    type: string
    size: number
    /**
     * The thing to put into FormData under the file field: the browser's
     * `File` on web, an expo-file-system `File` on native.
     */
    file: UploadFile
}

export interface DocumentAssetLike {
    uri: string
    name?: string | null
    mimeType?: string | null
    size?: number | null
}

export interface ImageAssetLike {
    uri: string
    fileName?: string | null
    mimeType?: string | null
    fileSize?: number | null
    type?: 'image' | 'video' | 'livePhoto' | 'pairedVideo' | string | null
}

export function documentAssetToPickedFile(
    asset: DocumentAssetLike,
    toFile: ToUploadFile
): PickedFile {
    const name = asset.name ?? deriveNameFromUri(asset.uri) ?? 'document'
    const type = asset.mimeType ?? 'application/octet-stream'
    const size = asset.size ?? 0
    return { name, type, size, file: toFile(asset.uri, name, type) }
}

export function imageAssetToPickedFile(asset: ImageAssetLike, toFile: ToUploadFile): PickedFile {
    const fallbackExt = asset.mimeType?.split('/')[1] ?? (asset.type === 'video' ? 'mp4' : 'jpg')
    const name =
        asset.fileName ?? deriveNameFromUri(asset.uri) ?? `IMG_${Date.now()}.${fallbackExt}`
    const type = asset.mimeType ?? (asset.type === 'video' ? 'video/mp4' : 'image/jpeg')
    const size = asset.fileSize ?? 0
    return { name, type, size, file: toFile(asset.uri, name, type) }
}

/** A browser File from an input or a drop, or any other UploadFile, as a PickedFile. */
export function uploadFileToPickedFile(file: UploadFile): PickedFile {
    return { name: file.name, type: file.type, size: file.size, file }
}

function deriveNameFromUri(uri: string): string | undefined {
    const trailing = uri.split('/').pop()
    if (!trailing) return undefined
    // Strip query string if present (e.g. content URIs sometimes include one).
    const clean = trailing.split('?')[0]
    return clean || undefined
}
