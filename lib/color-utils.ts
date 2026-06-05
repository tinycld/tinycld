export function hexToRgba(hex: string, alpha: number): string {
    const { r, g, b } = hexToRgb(hex)
    return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

interface Rgb {
    r: number
    g: number
    b: number
}

/**
 * Parse a `#rgb` or `#rrggbb` hex string into 0–255 channels. Falls back to
 * black for malformed input so callers never produce `NaN` colors.
 */
export function hexToRgb(hex: string): Rgb {
    let h = hex.replace('#', '').trim()
    if (h.length === 3) {
        h = h[0] + h[0] + h[1] + h[1] + h[2] + h[2]
    }
    if (h.length !== 6 || /[^0-9a-fA-F]/.test(h)) {
        return { r: 0, g: 0, b: 0 }
    }
    return {
        r: Number.parseInt(h.slice(0, 2), 16),
        g: Number.parseInt(h.slice(2, 4), 16),
        b: Number.parseInt(h.slice(4, 6), 16),
    }
}

/**
 * Relative luminance (WCAG 2.x) of a color, 0 (black) – 1 (white).
 */
export function relativeLuminance(hex: string): number {
    const { r, g, b } = hexToRgb(hex)
    const toLinear = (c: number) => {
        const s = c / 255
        return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
    }
    return 0.2126 * toLinear(r) + 0.7152 * toLinear(g) + 0.0722 * toLinear(b)
}

/**
 * Mix `hex` toward black by `amount` (0 = unchanged, 1 = black). Used to pull a
 * too-light label color down to a readable shade without changing its hue.
 */
export function darken(hex: string, amount: number): string {
    const { r, g, b } = hexToRgb(hex)
    const k = Math.min(1, Math.max(0, amount))
    const toHex = (c: number) =>
        Math.round(c * (1 - k))
            .toString(16)
            .padStart(2, '0')
    return `#${toHex(r)}${toHex(g)}${toHex(b)}`
}

/**
 * A readable text color for a label whose accent is `hex`.
 *
 * Label badges paint the text in the label's own color over a 20%-opacity
 * version of that same color. Light accents (yellow, lime, pale blue) then
 * render as light-on-light and become unreadable. When the accent's luminance
 * exceeds `threshold`, darken it enough to read; otherwise use it as-is.
 *
 * `threshold` defaults to 0.45 — measured to sit just above the green/cyan
 * accents in the label palette (which read fine) and below `#eab308` yellow
 * (the reported unreadable badge), so only the genuinely-too-light accents get
 * pulled down.
 */
export function readableTextColor(hex: string, threshold = 0.45): string {
    const lum = relativeLuminance(hex)
    // darken(_, 0) also normalizes shorthand (#fff) to full 6-char hex.
    if (lum <= threshold) return darken(hex, 0)
    // The lighter the accent, the harder we pull it toward black. Cap the pull
    // so vivid mid-tones keep their identity while near-white still lands dark.
    const amount = Math.min(0.72, 0.5 + (lum - threshold))
    return darken(hex, amount)
}
