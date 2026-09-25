import { describe, expect, it } from 'vitest'
import { doneSummaryOf } from '../done-summary'

describe('doneSummaryOf', () => {
    it('joins each part that is not zero', () => {
        expect(doneSummaryOf({ appCount: 3, memberCount: 2, isMailOn: true })).toBe(
            '3 apps on · 2 people · email sending on'
        )
    })
    it('uses the singular for one', () => {
        expect(doneSummaryOf({ appCount: 1, memberCount: 1, isMailOn: false })).toBe(
            '1 app on · 1 person'
        )
    })
    it('is empty when nothing is set up', () => {
        expect(doneSummaryOf({ appCount: 0, memberCount: 0, isMailOn: false })).toBe('')
    })
})
