import type { PickedFile } from '@tinycld/core/file-viewer/picked-file'
import { downscaleImage } from '@tinycld/core/lib/downscale-image'
import { Platform } from 'react-native'

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
 * can load, then downscale it.
 *
 * On web `PickedFile.file` is a real `File`; `URL.createObjectURL` gives a
 * `blob:` URI the canvas-based web downscaler can decode. On native the same
 * field is a `{ uri, name, type, size }` object wearing a `File` cast (see
 * `PickedFile`'s own doc comment) — its `uri` is already loadable, and native
 * `downscaleImage` is a deliberate no-op passthrough.
 */
export async function prepareAvatarImage(picked: PickedFile): Promise<PreparedAvatarImage> {
    const sourceUri =
        Platform.OS === 'web'
            ? URL.createObjectURL(picked.file)
            : (picked.file as unknown as { uri: string }).uri
    return downscaleImage(sourceUri, picked.type)
}

/**
 * Turn a prepared image's URI back into bytes for FormData. `fetch` reads a
 * `blob:` URI on web and a local `file://`/content URI on native — the same
 * call works on both platforms, and native's polyfilled `fetch` supports it.
 */
export async function avatarImageToBlob(image: PreparedAvatarImage): Promise<Blob> {
    const response = await fetch(image.uri)
    return response.blob()
}
