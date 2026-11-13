import { z } from '@tinycld/core/ui/form'

export const classifierSchema = z.object({
    apiKey: z.string().trim(),
    budget: z.coerce.number().min(0, 'Budget cannot be negative'),
    fullThreshold: z.coerce.number().min(0).max(1, 'Use a value from 0 to 1'),
})

export type ClassifierFormValues = z.output<typeof classifierSchema>
export type ClassifierFormInput = z.input<typeof classifierSchema>

export const CLASSIFIER_DEFAULTS = {
    budget: 5,
    fullThreshold: 0.7,
}
