import { describe, expect, it } from 'vitest'
import { taxonomySchema } from '../taxonomy-schema'

const row = (name: string) => ({ name, assignWhen: 'x', autoAction: false })

describe('taxonomySchema', () => {
    it('accepts up to 12 unique rows', () => {
        expect(
            taxonomySchema.safeParse({ rows: Array.from({ length: 12 }, (_, i) => row(`l${i}`)) })
                .success
        ).toBe(true)
    })
    it('rejects 13 rows, duplicates (case-insensitive), empty rules and empty list', () => {
        expect(
            taxonomySchema.safeParse({ rows: Array.from({ length: 13 }, (_, i) => row(`l${i}`)) })
                .success
        ).toBe(false)
        expect(taxonomySchema.safeParse({ rows: [row('Junk'), row('junk')] }).success).toBe(false)
        expect(
            taxonomySchema.safeParse({ rows: [{ name: 'a', assignWhen: '  ', autoAction: false }] })
                .success
        ).toBe(false)
        expect(taxonomySchema.safeParse({ rows: [] }).success).toBe(false)
    })
})
