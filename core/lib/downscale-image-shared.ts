/**
 * Pure sizing math shared by the native and web `downscaleImage` entries.
 *
 * Avatars store the picked image as-is and apply the user's crop as a
 * rectangle at render time rather than rasterizing a cropped copy — one
 * render path serves both web and native, and the crop stays re-editable
 * forever. The cost is that every avatar render downloads the full stored
 * image, so a 4MB phone photo would be fetched just to paint a 24px circle.
 * Capping the longest edge at 1024px is what makes that trade affordable.
 *
 * No platform APIs and no React here — this is what the unit test imports,
 * sidestepping Metro's `.web.ts` / native platform resolution entirely.
 */
export const MAX_AVATAR_EDGE = 1024
export const JPEG_QUALITY = 0.85

export function fitWithinMaxEdge(
    width: number,
    height: number,
    maxEdge = MAX_AVATAR_EDGE
): { width: number; height: number } {
    const longest = Math.max(width, height)
    if (longest <= maxEdge) return { width, height }

    const scale = maxEdge / longest
    return {
        width: Math.max(1, Math.round(width * scale)),
        height: Math.max(1, Math.round(height * scale)),
    }
}
