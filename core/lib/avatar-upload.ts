import type { PickedFile } from '@tinycld/core/file-viewer/picked-file'
import { downscaleImage } from '@tinycld/core/lib/downscale-image'
import {
    type UploadFile,
    uploadFileFromLocalUri,
    uploadFileUri,
} from '@tinycld/core/lib/upload-file'

/**
 * A downscaled image ready to hand to `AvatarCropper` (as `imageUri`) and,
 * once the user commits a crop, to upload.
 */
export interface PreparedAvatarImage {
    uri: string
    mimeType: string
}

/**
 * Resolve a `PickedFile` into a URI `downscaleImage` (and then `AvatarCropper`)
 * can load, then downscale it: a `blob:` URI the canvas-based web downscaler
 * can decode on web, the file's own URI on native (where `downscaleImage` is a
 * deliberate no-op passthrough).
 */
export async function prepareAvatarImage(picked: PickedFile): Promise<PreparedAvatarImage> {
    return downscaleImage(uploadFileUri(picked.file), picked.type)
}

/**
 * The prepared image as an upload part named `<baseName>.<ext>`: read back from
 * its `blob:` URI on web, uploaded from its file on disk on native.
 */
export function avatarUploadFile(
    image: PreparedAvatarImage,
    baseName: string
): Promise<UploadFile> {
    const name = `${baseName}.${image.mimeType.split('/')[1] ?? 'jpg'}`
    return uploadFileFromLocalUri(image.uri, name, image.mimeType)
}
