import { z } from '@tinycld/core/ui/form'

export const MAX_TAXONOMY_LABELS = 12

export const taxonomyRowSchema = z.object({
    name: z.string().trim().min(1, 'Enter a label name').max(60),
    assignWhen: z.string().trim().min(1, 'Describe when to use this label').max(2000),
    autoAction: z.boolean(),
})

export const taxonomySchema = z
    .object({
        rows: z
            .array(taxonomyRowSchema)
            .min(1, 'Add at least one label')
            .max(MAX_TAXONOMY_LABELS, `Use ${MAX_TAXONOMY_LABELS} labels or fewer`),
    })
    .superRefine((v, ctx) => {
        const seen = new Set<string>()
        v.rows.forEach((r, i) => {
            const key = r.name.toLowerCase()
            if (seen.has(key)) {
                ctx.addIssue({
                    code: 'custom',
                    path: ['rows', i, 'name'],
                    message: 'Each label can appear once',
                })
            }
            seen.add(key)
        })
    })

export type TaxonomyFormValues = z.infer<typeof taxonomySchema>

export type TaxonomyPreset = { id: string; label: string; rows: TaxonomyFormValues['rows'] }
