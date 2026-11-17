import { describe, expect, it } from 'vitest'
import {
    defaultRemovalPlan,
    deleteAccountSchemaFor,
    offboardPlanSchema,
    removeMemberSchemaFor,
} from '../../components/settings/account/offboard-schemas'

describe('offboardPlanSchema', () => {
    it('accepts keep', () => {
        expect(offboardPlanSchema.safeParse({ mode: 'keep' }).success).toBe(true)
    })

    it('accepts delete and a reassign with a successor', () => {
        expect(offboardPlanSchema.safeParse({ mode: 'delete_my_data' }).success).toBe(true)
        expect(
            offboardPlanSchema.safeParse({ mode: 'reassign', successor_user_id: 'u2' }).success
        ).toBe(true)
    })

    it('rejects a reassign with no successor', () => {
        const result = offboardPlanSchema.safeParse({ mode: 'reassign', successor_user_id: '' })
        expect(result.error?.issues[0]?.path).toEqual(['successor_user_id'])
    })
})

describe('deleteAccountSchemaFor', () => {
    it('matches the email ignoring case and surrounding whitespace', () => {
        const schema = deleteAccountSchemaFor('Me@Example.com', false)
        expect(schema.safeParse({ confirm: ' me@example.com ' }).success).toBe(true)
        expect(schema.safeParse({ confirm: 'you@example.com' }).success).toBe(false)
    })

    it('needs no plan when nobody could inherit the content', () => {
        const schema = deleteAccountSchemaFor('me@example.com', false)
        expect(schema.safeParse({ confirm: 'me@example.com' }).success).toBe(true)
    })

    it('requires a plan when a peer could inherit the content', () => {
        const schema = deleteAccountSchemaFor('me@example.com', true)
        const result = schema.safeParse({ confirm: 'me@example.com' })
        expect(result.success).toBe(false)
        expect(result.error?.issues[0]?.path).toEqual(['plan'])
        expect(
            schema.safeParse({ confirm: 'me@example.com', plan: { mode: 'delete_my_data' } })
                .success
        ).toBe(true)
    })
})

describe('removeMemberSchemaFor', () => {
    it('matches the display name exactly, ignoring surrounding whitespace', () => {
        const schema = removeMemberSchemaFor('Ann Lee')
        expect(schema.safeParse({ confirm: ' Ann Lee ' }).success).toBe(true)
        expect(schema.safeParse({ confirm: 'ann lee' }).success).toBe(false)
    })
})

describe('defaultRemovalPlan', () => {
    it("keeps the member's work, credited to the deleted user", () => {
        expect(defaultRemovalPlan()).toEqual({ mode: 'keep' })
    })
})
