import { Platform } from 'react-native'

// On web, read `navigator.platform` — the field tinykeys resolves `$mod`
// from — so the key a hint names is the key that fires. The user agent can
// disagree with it (a spoofed or "request desktop site" agent), and then
// the app showed Ctrl for a shortcut that answered ⌘.
const IS_APPLE =
    Platform.OS === 'ios' ||
    Platform.OS === 'macos' ||
    (Platform.OS === 'web' &&
        typeof navigator !== 'undefined' &&
        /Mac|iPhone|iPad|iPod/.test(navigator.platform))

// Characters that are produced by Shift on a standard US layout. When a
// binding uses the Shift modifier purely to produce one of these characters
// (rather than to modify a letter like `Shift+F`), the Shift is redundant
// for display — the glyph already implies it.
const SHIFT_PRODUCED = new Set('?!@#$%^&*()_+{}|:"<>~')

/**
 * Format a key combo or sequence for display. Resolves $mod to ⌘ on Apple
 * platforms and Ctrl elsewhere, draws named keys as glyphs (KEY_GLYPHS), and
 * splits sequences on whitespace.
 *
 *   "$mod+Enter"         → [["⌘", "↵"]]
 *   "$mod+Alt+ArrowUp"   → [["⌘", "⌥", "↑"]]
 *   "Alt+t"              → [["⌥", "T"]]   (a letter after a modifier is a capital)
 *   "t i"                → [["t"], ["i"]]
 *   "Shift+?"            → [["?"]]   (Shift is implicit in the glyph)
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
    const hasModifier = parts.some(part => MODIFIERS.has(part))
    const lastIndex = parts.length - 1

    return parts.map((part, index) => {
        if (part === '$mod') return isApple ? '⌘' : 'Ctrl'
        if (part === 'Shift') return isApple ? '⇧' : 'Shift'
        if (part === 'Alt') return isApple ? '⌥' : 'Alt'
        if (part === 'Meta') return isApple ? '⌘' : 'Meta'
        if (part === 'Control') return isApple ? '⌃' : 'Ctrl'
        const glyph = KEY_GLYPHS[part]
        if (glyph) return glyph
        // A modifier-plus-letter reads as a capital on every key cap and
        // menu (⌘S, ⌥T). A plain letter keeps its case, because a capital
        // alone would read as Shift + the letter.
        if (hasModifier && index === lastIndex && /^[a-z]$/.test(part)) return part.toUpperCase()
        return part
    })
}

const MODIFIERS = new Set(['$mod', 'Meta', 'Control', 'Alt', 'Shift'])

const KEY_GLYPHS: Record<string, string> = {
    Enter: '↵',
    Escape: 'Esc',
    ArrowUp: '↑',
    ArrowDown: '↓',
    ArrowLeft: '←',
    ArrowRight: '→',
    Backspace: '⌫',
}

export const isApplePlatform = IS_APPLE
