import { Platform } from 'react-native'

const IS_APPLE =
    Platform.OS === 'ios' ||
    Platform.OS === 'macos' ||
    (Platform.OS === 'web' &&
        typeof navigator !== 'undefined' &&
        /Mac|iPhone|iPad|iPod/.test(navigator.userAgent))

// Characters that are produced by Shift on a standard US layout. When a
// binding uses the Shift modifier purely to produce one of these characters
// (rather than to modify a letter like `Shift+F`), the Shift is redundant
// for display — the glyph already implies it.
const SHIFT_PRODUCED = new Set('?!@#$%^&*()_+{}|:"<>~')

/**
 * Format a key combo or sequence for display. Resolves $mod to ⌘ on Apple
 * platforms and Ctrl elsewhere, and splits sequences on whitespace.
 *
 *   "$mod+Enter" → [["⌘", "Enter"]]
 *   "t i"        → [["t"], ["i"]]
 *   "Shift+?"    → [["?"]]   (Shift is implicit in the glyph)
 *
 * `isApple` is a parameter so a test can format for both platforms.
 */
export function formatKeys(keys: string, isApple = IS_APPLE): string[][] {
    return keys
        .split(/\s+/)
        .filter(Boolean)
        .map(combo => formatCombo(combo, isApple))
}

function formatCombo(combo: string, isApple: boolean): string[] {
    const rawParts = combo.split('+')
    const last = rawParts[rawParts.length - 1]
    // If the only modifier is Shift and the key is a Shift-produced glyph,
    // drop Shift from the display. `Shift+F` keeps Shift (F is a letter),
    // but `Shift+?` collapses to `?`.
    const isShiftGlyph =
        rawParts.length === 2 && rawParts[0] === 'Shift' && SHIFT_PRODUCED.has(last)
    const parts = isShiftGlyph ? [last] : rawParts

    return parts.map(part => {
        if (part === '$mod') return isApple ? '⌘' : 'Ctrl'
        if (part === 'Shift') return isApple ? '⇧' : 'Shift'
        if (part === 'Alt') return isApple ? '⌥' : 'Alt'
        if (part === 'Meta') return isApple ? '⌘' : 'Meta'
        if (part === 'Control') return isApple ? '⌃' : 'Ctrl'
        if (part === 'Enter') return '↵'
        if (part === 'Escape') return 'Esc'
        return part
    })
}

export const isApplePlatform = IS_APPLE
