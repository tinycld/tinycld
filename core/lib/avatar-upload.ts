import type { PickedFile } from '@tinycld/core/file-viewer/picked-file'
import { downscaleImage } from '@tinycld/core/lib/downscale-image'
import { uploadFileUri } from '@tinycld/core/lib/upload-file'

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
 * Turn a prepared image's URI back into bytes for FormData. `fetch` reads a
 * `blob:` URI on web and a local `file://` URI on native (expo/fetch), so the
 * same call works on both platforms.
 */
export async function avatarImageToBlob(image: PreparedAvatarImage): Promise<Blob> {
    const response = await fetch(image.uri)
    return response.blob()
}
