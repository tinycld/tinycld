/**
 * Pure avatar math and formatting, deliberately free of React.
 *
 * Avatar and AvatarCropper must agree exactly on the crop transform or the
 * cropper's preview lies about the stored result, so that math lives here and
 * is tested directly rather than through either component.
 */

export const AVATAR_COLORS = [
    '#3b82f6',
    '#22c55e',
    '#a855f7',
    '#f97316',
    '#ec4899',
    '#ef4444',
    '#eab308',
    '#06b6d4',
] as const

/** [background, foreground] pairs for the `soft` palette (members, sharing). */
export const SOFT_AVATAR_PALETTE = [
    ['#e0f2fe', '#0369a1'],
    ['#dcfce7', '#047857'],
    ['#fef3c7', '#b45309'],
    ['#fce7f3', '#be185d'],
    ['#ede9fe', '#6d28d9'],
    ['#ffedd5', '#c2410c'],
    ['#cffafe', '#0e7490'],
    ['#fee2e2', '#b91c1c'],
] as const

const MAX_ZOOM = 8

function hashString(value: string): number {
    let hash = 0
    for (let i = 0; i < value.length; i++) {
        hash = (hash << 5) - hash + value.charCodeAt(i)
        hash |= 0
    }
    return Math.abs(hash)
}

/**
 * Deterministic background color for a key. Pass a stable id (not a name) so
 * editing a display name doesn't reshuffle the color.
 */
export function avatarColor(key: string): string {
    return AVATAR_COLORS[hashString(key) % AVATAR_COLORS.length] as string
}

export function softAvatarColors(key: string): readonly [string, string] {
    const pair = SOFT_AVATAR_PALETTE[hashString(key) % SOFT_AVATAR_PALETTE.length]
    return (pair ?? SOFT_AVATAR_PALETTE[0]) as readonly [string, string]
}

/**
 * WCAG relative luminance of a `#rrggbb` color, in [0, 1].
 * https://www.w3.org/WAI/GL/wiki/Relative_luminance
 */
function relativeLuminance(hex: string): number {
    const channels = [1, 3, 5].map(i => Number.parseInt(hex.slice(i, i + 2), 16) / 255)
    const [r, g, b] = channels.map(c => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4))
    return 0.2126 * (r ?? 0) + 0.7152 * (g ?? 0) + 0.0722 * (b ?? 0)
}

/** WCAG contrast ratio between two `#rrggbb` colors, in [1, 21]. */
function contrastRatio(a: string, b: string): number {
    const [lighter, darker] = [relativeLuminance(a), relativeLuminance(b)].sort((x, y) => y - x)
    return ((lighter ?? 0) + 0.05) / ((darker ?? 0) + 0.05)
}

const READABLE_DARK = '#0f172a'
const READABLE_LIGHT = '#ffffff'

/**
 * A readable foreground for an arbitrary background color, for the soft
 * palette's new "user picked a color" state. A known SOFT_AVATAR_PALETTE
 * background reuses its designed pairing; anything else picks whichever of
 * a near-black or white text yields the higher WCAG contrast ratio against
 * that background — every AVATAR_COLORS swatch clears 4.5:1 against
 * READABLE_DARK, so this always beats the flat white this replaces.
 */
export function readableForegroundFor(backgroundColor: string): string {
    const paletteMatch = SOFT_AVATAR_PALETTE.find(([bg]) => bg === backgroundColor)
    if (paletteMatch) return paletteMatch[1]

    const darkContrast = contrastRatio(backgroundColor, READABLE_DARK)
    const lightContrast = contrastRatio(backgroundColor, READABLE_LIGHT)
    return darkContrast >= lightContrast ? READABLE_DARK : READABLE_LIGHT
}

/**
 * Two-letter initials. Prefers first+last of a name; falls back to the email's
 * local part, which is often the only identity we have in sharing dialogs.
 */
export function resolveInitials(name: string, email?: string): string {
    const source = name.trim() || (email ?? '').trim()
    if (!source) return '?'

    const parts = source.split(/\s+/).filter(Boolean)
    if (parts.length >= 2) {
        const first = parts[0]?.[0] ?? ''
        const last = parts[parts.length - 1]?.[0] ?? ''
        return `${first}${last}`.toUpperCase()
    }

    const atIndex = source.indexOf('@')
    if (atIndex > 0) {
        const local = source.slice(0, atIndex)
        const localParts = local.split(/[._-]/).filter(Boolean)
        if (localParts.length >= 2) {
            const a = localParts[0]?.[0] ?? ''
            const b = localParts[1]?.[0] ?? ''
            return `${a}${b}`.toUpperCase()
        }
        return local.slice(0, 2).toUpperCase()
    }

    return source.slice(0, 2).toUpperCase()
}

/**
 * Where the image sits inside the circle. `x`/`y` are the focal point as
 * fractions of the source in [0,1]; `zoom` is the scale relative to cover-fit.
 */
export interface CropRect {
    x: number
    y: number
    zoom: number
}

export const DEFAULT_CROP: CropRect = { x: 0.5, y: 0.5, zoom: 1 }

function clampNumber(value: number, min: number, max: number): number {
    if (!Number.isFinite(value)) return min
    return Math.min(max, Math.max(min, value))
}

export function clampCrop(rect: CropRect): CropRect {
    return {
        x: clampNumber(rect.x, 0, 1),
        y: clampNumber(rect.y, 0, 1),
        zoom: clampNumber(rect.zoom, 1, MAX_ZOOM),
    }
}

/**
 * Stored crops are user-supplied JSON that may predate a schema change, so
 * anything unparseable or out of range degrades to center-cover rather than
 * throwing in the middle of a list render.
 */
export function parseCrop(raw: string | null | undefined): CropRect {
    if (!raw) return DEFAULT_CROP
    let parsed: unknown
    try {
        parsed = JSON.parse(raw)
    } catch {
        return DEFAULT_CROP
    }
    if (parsed === null || typeof parsed !== 'object') return DEFAULT_CROP
    const candidate = parsed as Partial<Record<keyof CropRect, unknown>>
    if (
        typeof candidate.x !== 'number' ||
        typeof candidate.y !== 'number' ||
        typeof candidate.zoom !== 'number'
    ) {
        return DEFAULT_CROP
    }
    return clampCrop({ x: candidate.x, y: candidate.y, zoom: candidate.zoom })
}

export function serializeCrop(rect: CropRect): string {
    return JSON.stringify(clampCrop(rect))
}

/**
 * Turn a crop rect into the image box and offset to render inside a
 * `size`-square frame. The offsets are clamped so the scaled image always
 * covers the frame — a gap would show the background through the photo.
 */
export function cropToTransform(
    rect: CropRect,
    size: number
): { width: number; height: number; translateX: number; translateY: number } {
    const { x, y, zoom } = clampCrop(rect)
    const scaled = size * zoom
    const overflow = scaled - size
    const translateX = -overflow * x
    const translateY = -overflow * y
    return {
        width: scaled,
        height: scaled,
        translateX: translateX || 0,
        translateY: translateY || 0,
    }
}
