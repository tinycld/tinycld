/**
 * An upper estimate of a text's drawn width in px, with no layout pass. The app
 * draws text in the platform's system font (react-native-web's system stack on
 * web, SF on iOS, Roboto on Android, Liberation Sans on Linux). Each character
 * class below is the widest of those fonts, rounded up, so a column sized by it
 * never cuts its text. Pure and import-free: a column measure, an export at a
 * page width and an e2e calibration all use the same numbers.
 */
export type TextStyle = { size: 'sm' | 'xs'; weight: 'normal' | 'semibold'; uppercase?: boolean }

/** text-sm and text-xs in px. */
export const FONT_PX = { sm: 14, xs: 12 } as const

type CharClass = 'digit' | 'narrow' | 'wide' | 'upper' | 'lower' | 'other' | 'far'

const EM: Record<TextStyle['weight'], Record<CharClass, number>> = {
    normal: {
        digit: 0.62,
        // Roboto Medium's r (0.351 em) bounds Android's regular r; SF and Arial are narrower.
        narrow: 0.36,
        wide: 0.95,
        upper: 0.78,
        lower: 0.61,
        other: 0.62,
        far: 1,
    },
    semibold: {
        digit: 0.65,
        narrow: 0.4,
        wide: 0.97,
        upper: 0.8,
        lower: 0.64,
        other: 0.66,
        far: 1,
    },
}

// No backtick: SF draws it at half an em.
const NARROW = new Set([..."iljfrtI.,:;'!| "])
const WIDE = new Set([...'mwMW%&'])
const FAR = new Set(['@'])
// Past Latin Extended-B: other scripts, CJK, emoji and wide symbols (—, …).
const LAST_LATIN = 0x24f

function classOf(ch: string): CharClass {
    if (NARROW.has(ch)) return 'narrow'
    if (WIDE.has(ch)) return 'wide'
    if (FAR.has(ch) || (ch.codePointAt(0) ?? 0) > LAST_LATIN) return 'far'
    if (ch >= '0' && ch <= '9') return 'digit'
    if (ch !== ch.toLowerCase()) return 'upper'
    if (ch !== ch.toUpperCase()) return 'lower'
    return 'other'
}

export function textWidth(text: string, style: TextStyle): number {
    const shown = style.uppercase ? text.toUpperCase() : text
    const em = EM[style.weight]
    let sum = 0
    for (const ch of shown) sum += em[classOf(ch)]
    return Math.ceil(sum * FONT_PX[style.size])
}

/** The widest word: the least width a text can wrap into without cutting a word. */
export function longestWordWidth(text: string, style: TextStyle): number {
    return Math.max(0, ...text.split(/\s+/).map(word => textWidth(word, style)))
}
