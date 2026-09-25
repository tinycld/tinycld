import { describe, expect, it } from 'vitest'
import { claimErrorMessage } from '../ClaimServerStep'

describe('claimErrorMessage', () => {
    it('uses the server message', () => {
        expect(
            claimErrorMessage({
                error: 'Too many tries. Wait 10 minutes, or restart the server for a new code.',
                reason: 'locked',
            })
        ).toMatch(/Too many tries/)
    })
    it('explains a network failure without apologizing', () => {
        expect(claimErrorMessage(null)).toBe(
            'The server did not answer. Check that it is running, then try again.'
        )
    })
})
