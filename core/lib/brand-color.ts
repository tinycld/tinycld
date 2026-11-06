import { z } from '@tinycld/core/ui/form'

// The color documents print with when none is set: the server's
// branding.DefaultColor (mailer.BrandColor). A data value, not a theme color.
export const DEFAULT_BRAND_COLOR = '#0d9488'

// The org_branding.brand_color pattern (2090000012_org_branding_brand_color.js).
const HEX_COLOR = /^#[0-9a-fA-F]{6}$/

export const brandColorSchema = z.object({
    brand_color: z
        .string()
        .trim()
        .refine(value => value === '' || HEX_COLOR.test(value), {
            message: 'Enter # and six hex digits, like #0d9488',
        }),
})

export type BrandColorValues = z.infer<typeof brandColorSchema>

// The color the preview swatch shows: the typed color once it is valid, else the default.
export function swatchColor(value: string): string {
    const trimmed = value.trim()
    return HEX_COLOR.test(trimmed) ? trimmed : DEFAULT_BRAND_COLOR
}
