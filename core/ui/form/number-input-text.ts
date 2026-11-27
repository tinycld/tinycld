import type { KeyboardTypeOptions, PlatformOSType } from 'react-native'

export interface NumberTextRules {
    /** Accepts fractions; otherwise only an integer text. */
    allowDecimal?: boolean
    min?: number
    max?: number
}

/** The accepted text, and its number once the text is one (null for "", "-", "."). */
export interface ParsedNumberText {
    text: string
    value: number | null
}

const DECIMAL_TEXT = /^-?\d*\.?\d*$/

// A keyboard or a paste from a document can give U+2212 MINUS SIGN for a minus.
const MINUS_SIGN = /−/g

function inRange(value: number, { min, max }: NumberTextRules): boolean {
    if (min !== undefined && value < min) return false
    return max === undefined || value <= max
}

function parseDecimal(text: string, rules: NumberTextRules): ParsedNumberText | null {
    if (!DECIMAL_TEXT.test(text)) return null
    const value = Number(text)
    if (text === '' || !Number.isFinite(value)) return { text, value: null }
    return inRange(value, rules) ? { text, value } : null
}

function parseInteger(text: string, rules: NumberTextRules): ParsedNumberText | null {
    if (text === '' || text === '-') return { text, value: null }
    if (text.replace(/[^0-9-]/g, '') !== text) return null
    const value = Number.parseInt(text, 10)
    if (Number.isNaN(value)) return null
    return inRange(value, rules) ? { text, value } : null
}

/**
 * What a NumberInput keeps of a typed text: null refuses the keystroke (not a
 * number, or outside min and max). Decimals are kept as typed, so a caller
 * that limits them can say so with its own message.
 */
export function parseNumberText(raw: string, rules: NumberTextRules): ParsedNumberText | null {
    const text = raw.replace(MINUS_SIGN, '-')
    return rules.allowDecimal ? parseDecimal(text, rules) : parseInteger(text, rules)
}

/**
 * The soft keyboard for a NumberInput, from the same rules that parse its text:
 * a negative is allowed unless min is 0 or more.
 *
 * - Android: `numeric` has a minus and a point; `decimal-pad` drops the minus.
 * - iOS: `numeric` and `decimal-pad` have no minus, so a signed field gets
 *   `numbers-and-punctuation`.
 * - Web: react-native-web gives `numeric` inputmode="numeric" and `decimal-pad`
 *   inputmode="decimal"; no inputmode shows a minus on iPhone Safari, so a
 *   signed field gets `numbers-and-punctuation`, which react-native-web renders
 *   as a text input with no inputmode (the full keyboard has "-" and ".").
 */
export function numberKeyboardType(
    rules: Pick<NumberTextRules, 'allowDecimal' | 'min'>,
    os: PlatformOSType
): KeyboardTypeOptions {
    const allowsNegative = rules.min === undefined || rules.min < 0
    if (!allowsNegative) return rules.allowDecimal ? 'decimal-pad' : 'numeric'
    return os === 'android' ? 'numeric' : 'numbers-and-punctuation'
}
