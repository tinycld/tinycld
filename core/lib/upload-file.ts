import type { UploadFile } from '@tinycld/core/lib/upload-file-types'

export type { UploadFile } from '@tinycld/core/lib/upload-file-types'

// Web: pickers, drops and generated files already hand over browser `File`s
// and Blobs, so there is no local file to wrap. upload-file.native.ts is the
// native variant.

export function uploadFileFromUri(_uri: string, _name: string, _type: string): UploadFile {
    throw new Error('uploadFileFromUri is native-only: on web, pickers already return File objects')
}

/** A URI that loads the file's bytes, e.g. for an image preview. Revoke it when done. */
export function uploadFileUri(file: UploadFile): string {
    return URL.createObjectURL(file)
}

/**
 * An upload part for a local URI the app made itself — e.g. a downscaled
 * avatar's `blob:` URI — read back into a `File`. The global fetch is right
 * here: the URI is local, never our server, so serverFetch's retry is moot.
 */
export async function uploadFileFromLocalUri(
    uri: string,
    name: string,
    type: string
): Promise<UploadFile> {
    const blob = await (await fetch(uri)).blob()
    return new File([blob], name, { type })
}
