import { embedOriginsSchema } from '@tinycld/core/lib/embed-origins'
import { z } from 'zod'

export const publicationSchema = z.object({
    enabled: z.boolean(),
    allowedDomains: embedOriginsSchema,
    requirePow: z.boolean(),
    confirmation: z.string().trim().min(1, 'Write a message').max(4000, 'At most 4000 characters'),
    retentionDays: z.number().int().min(1).max(3650),
    maxUploadMb: z.number().int().min(1).max(100),
})

export type PublicationValues = z.infer<typeof publicationSchema>

export const PUBLICATION_DEFAULTS: PublicationValues = {
    enabled: false,
    allowedDomains: '',
    requirePow: false,
    confirmation: 'Thank you. We received your submission.',
    retentionDays: 90,
    maxUploadMb: 25,
}
