import { log } from '@tinycld/core/lib/logger'

export { fitWithinMaxEdge, JPEG_QUALITY, MAX_AVATAR_EDGE } from './downscale-image-shared'

import { fitWithinMaxEdge, JPEG_QUALITY } from './downscale-image-shared'

/**
 * Web entry (Metro resolves this over `downscale-image.ts` on web).
 *
 * Web is the majority of users and the file picker hands us the raw file
 * directly, so this canvas downscale is where the size cap described in
 * `downscale-image-shared.ts` actually earns its keep — native falls back
 * to the picker's own compression (see `downscale-image.ts`).
 *
 * PNG sources keep their format so a transparent org wordmark survives dark
 * mode; everything else (user photos behind a circular mask, where
 * transparency is meaningless) re-encodes to JPEG.
 *
 * A downscale failure is not fatal: catch, log, and upload the original —
 * the server's max file size is the backstop.
 */
export async function downscaleImage(
    uri: string,
    mimeType: string
): Promise<{ uri: string; mimeType: string }> {
    const isPng = mimeType === 'image/png'
    try {
        const image = await loadImage(uri)
        const { width, height } = fitWithinMaxEdge(image.naturalWidth, image.naturalHeight)

        const canvas = document.createElement('canvas')
        canvas.width = width
        canvas.height = height
        const ctx = canvas.getContext('2d')
        if (!ctx) return { uri, mimeType }
        ctx.drawImage(image, 0, 0, width, height)

        const outputType = isPng ? 'image/png' : 'image/jpeg'
        const blob = await new Promise<Blob | null>(resolve => {
            canvas.toBlob(resolve, outputType, isPng ? undefined : JPEG_QUALITY)
        })
        if (!blob) return { uri, mimeType }

        return { uri: URL.createObjectURL(blob), mimeType: outputType }
    } catch (err) {
        log.warn('core.avatar', 'web downscale failed; uploading original', {
            err: err instanceof Error ? err.message : String(err),
        })
        return { uri, mimeType }
    }
}

function loadImage(uri: string): Promise<HTMLImageElement> {
    return new Promise((resolve, reject) => {
        const image = new window.Image()
        image.onload = () => resolve(image)
        image.onerror = () => reject(new Error('image decode failed'))
        image.src = uri
    })
}
