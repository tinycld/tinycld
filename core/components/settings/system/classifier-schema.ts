import { z } from '@tinycld/core/ui/form'

// Fields stay strings (the text inputs' native type) so a blank value is
// rejected here instead of coercing to 0, which would silently pause classification.
const numberText = z
    .string()
    .trim()
    .min(1, 'Enter a value')
    .refine(value => Number.isFinite(Number(value)), 'Enter a number')

export const classifierSchema = z.object({
    apiKey: z.string().trim(),
    budget: numberText.refine(value => Number(value) >= 0, 'Budget cannot be negative'),
    fullThreshold: numberText.refine(
        value => Number(value) >= 0 && Number(value) <= 1,
        'Use a value from 0 to 1'
    ),
})

export type ClassifierFormValues = z.infer<typeof classifierSchema>

export const CLASSIFIER_DEFAULTS = {
    budget: 5,
    fullThreshold: 0.7,
}
