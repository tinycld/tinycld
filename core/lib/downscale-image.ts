import { log } from '@tinycld/core/lib/logger'
import { manipulateAsync, SaveFormat } from 'expo-image-manipulator'
import { JPEG_QUALITY, MAX_AVATAR_EDGE } from './downscale-image-shared'

export { fitWithinMaxEdge, JPEG_QUALITY, MAX_AVATAR_EDGE } from './downscale-image-shared'

/**
 * Native entry (Metro resolves `.web.ts` over this file on web).
 *
 * Avatars store the picked image rather than a rasterized crop, so the upload
 * has to be small enough that a 4MB phone photo isn't downloaded to paint a
 * 24px circle. Capping the longest edge is what makes that trade affordable.
 *
 * Only `width` is passed to `resize`: expo-image-manipulator derives the other
 * dimension to preserve the aspect ratio, which is what we want for a
 * landscape source. A portrait source is handled by capping `height` instead —
 * passing both would stretch it.
 *
 * A failure here is not fatal: the original is uploaded and the server's
 * max-size check is the backstop. Losing a size optimization is a far better
 * outcome than losing the user's photo.
 */
export async function downscaleImage(
    uri: string,
    mimeType: string
): Promise<{ uri: string; mimeType: string }> {
    // PNG keeps its alpha: an org wordmark on a transparent ground matted onto
    // white would break dark mode. Photos behind a circular mask have no use
    // for transparency, so everything else re-encodes to JPEG.
    const isPng = mimeType === 'image/png'

    try {
        const { width, height } = await measureImage(uri)
        if (Math.max(width, height) <= MAX_AVATAR_EDGE) {
            return { uri, mimeType }
        }

        const resize = width >= height ? { width: MAX_AVATAR_EDGE } : { height: MAX_AVATAR_EDGE }
        const result = await manipulateAsync(uri, [{ resize }], {
            compress: isPng ? 1 : JPEG_QUALITY,
            format: isPng ? SaveFormat.PNG : SaveFormat.JPEG,
        })

        return { uri: result.uri, mimeType: isPng ? 'image/png' : 'image/jpeg' }
    } catch (err) {
        log.warn('core.avatar', 'native downscale failed; uploading original', {
            err: String(err),
        })
        return { uri, mimeType }
    }
}

/**
 * Source dimensions, needed to decide which edge to cap. A no-op
 * `manipulateAsync` call is the cheapest way to get them — React Native's
 * `Image.getSize` is callback-based and does not resolve `file://` URIs
 * reliably across platforms.
 */
async function measureImage(uri: string): Promise<{ width: number; height: number }> {
    const probe = await manipulateAsync(uri, [])
    return { width: probe.width, height: probe.height }
}
