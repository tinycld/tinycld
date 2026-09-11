export { fitWithinMaxEdge, JPEG_QUALITY, MAX_AVATAR_EDGE } from './downscale-image-shared'

/**
 * Native entry (Metro resolves `.web.ts` over this file on web).
 *
 * `expo-image-manipulator` is not installed in this workspace, and the
 * Global Constraints forbid adding a new npm dependency to get it. Rather
 * than fabricate a native resize, this is an identity passthrough: native
 * uploads the picked image unchanged and relies on `expo-image-picker`'s own
 * `quality` compression at the pick site to keep the upload reasonably
 * sized. The size cap this module exists for is only actually enforced on
 * web (see `downscale-image.web.ts`) — web is the majority of users, so
 * that is the path that matters most. The server-side max file size is the
 * backstop for anything that slips through uncapped.
 */
export async function downscaleImage(
    uri: string,
    mimeType: string
): Promise<{ uri: string; mimeType: string }> {
    return { uri, mimeType }
}
