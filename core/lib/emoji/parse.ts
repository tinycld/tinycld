/**
 * Unified codepoint sequence -> the glyph itself.
 *
 * The whole of what we take from emoji-picker-react's rendering: the picker
 * shows native text, not CDN images, so this plus a <Text> is the renderer.
 * Works identically in RN and on web.
 */
export function parseNativeEmoji(unified: string): string {
    return unified
        .split('-')
        .map(hex => String.fromCodePoint(Number.parseInt(hex, 16)))
        .join('')
}
