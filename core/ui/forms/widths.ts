import type { FieldWidth } from '@tinycld/core/lib/forms/definition'

/**
 * Below this measured container width a field with a `narrowWidth` uses it
 * exactly. A field without one collapses: quarter becomes half; half, third
 * and full become full; auto (its content's width) stays auto. At or above
 * it, or while the container is unmeasured (0), every field keeps `width`.
 */
export const NARROW_BELOW = 640

export function effectiveWidth(
    width: FieldWidth,
    containerWidth: number,
    narrowWidth?: FieldWidth
): FieldWidth {
    if (containerWidth === 0 || containerWidth >= NARROW_BELOW) return width
    if (narrowWidth) return narrowWidth
    if (width === 'auto') return 'auto'
    if (width === 'quarter') return 'half'
    return 'full'
}

const CLASSES: Record<FieldWidth, string> = {
    full: 'basis-full',
    half: 'basis-1/2',
    third: 'basis-1/3',
    quarter: 'basis-1/4',
    // Sized by its content, never past the row: content wider than the form wraps inside the cell.
    auto: 'basis-auto max-w-full',
}

export function widthClass(width: FieldWidth): string {
    return CLASSES[width]
}
