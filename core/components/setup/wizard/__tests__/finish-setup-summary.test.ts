import { describe, expect, it } from 'vitest'
import { finishSetupSubtitleOf } from '../finish-setup-summary'

describe('finishSetupSubtitleOf', () => {
    it('formats the done count against the total', () => {
        expect(finishSetupSubtitleOf({ doneCount: 2, total: 4 })).toBe('2 of 4 done')
    })
    it('handles zero done', () => {
        expect(finishSetupSubtitleOf({ doneCount: 0, total: 4 })).toBe('0 of 4 done')
    })
})
