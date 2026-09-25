import { describe, expect, it } from 'vitest'
import { doneHeadingOf, doneSummaryOf } from '../done-summary'

describe('doneSummaryOf', () => {
    it('joins each part that is not zero', () => {
        expect(doneSummaryOf({ appCount: 3, memberCount: 2 })).toBe('3 apps on · 2 people')
    })
    it('uses the singular for one', () => {
        expect(doneSummaryOf({ appCount: 1, memberCount: 1 })).toBe('1 app on · 1 person')
    })
    it('is empty when nothing is set up', () => {
        expect(doneSummaryOf({ appCount: 0, memberCount: 0 })).toBe('')
    })
})

describe('doneHeadingOf', () => {
    it('names the workspace', () => {
        expect(doneHeadingOf('Harbor Dental')).toEqual({
            heading: 'Harbor Dental is ready',
            buttonLabel: 'Open Harbor Dental',
        })
    })
    it('is neutral when no name was chosen', () => {
        expect(doneHeadingOf('')).toEqual({
            heading: 'Your workspace is ready',
            buttonLabel: 'Open your workspace',
        })
    })
})
