import { describe, expect, it } from 'vitest'
import * as ids from '../step-ids'

// A spec finds each control by one of these ids, so two sharing a value
// would make a locator match the wrong control.
describe('setup test ids', () => {
    it('are distinct', () => {
        const values = Object.entries(ids)
            .filter(([name]) => name.startsWith('SETUP_'))
            .map(([, value]) => value)
        expect(values.length).toBeGreaterThan(5)
        expect(new Set(values).size).toBe(values.length)
    })
})
