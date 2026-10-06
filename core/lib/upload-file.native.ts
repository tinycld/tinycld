import type { UploadFile } from '@tinycld/core/lib/upload-file-types'
import { File } from 'expo-file-system'

export type { UploadFile } from '@tinycld/core/lib/upload-file-types'

/**
 * Wraps a file on disk — a picker asset, a download in the cache — as an
 * upload part. An expo-file-system `File` is a Blob whose bytes stay on disk
 * until an upload reads them natively. Its own `name` and `type` come from the
 * path, which for a picked photo is a UUID, so both are pinned to the values
 * the caller knows.
 */
export function uploadFileFromUri(uri: string, name: string, type: string): UploadFile {
    const file = new File(uri)
    Object.defineProperty(file, 'name', { value: name })
    Object.defineProperty(file, 'type', { value: type })
    return file
}

/** The file's own URI, e.g. for an image preview. */
export function uploadFileUri(file: UploadFile): string {
    if (file instanceof File) return file.uri
    throw new Error('uploadFileUri: expected an expo-file-system File on native')
}

/**
 * An upload part for a local URI the app made itself — e.g. a downscaled
 * avatar. On native that URI is a file on disk, which is uploaded from there.
 */
export async function uploadFileFromLocalUri(
    uri: string,
    name: string,
    type: string
): Promise<UploadFile> {
    return uploadFileFromUri(uri, name, type)
}
