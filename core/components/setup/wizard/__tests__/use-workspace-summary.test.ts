import { describe, expect, it } from 'vitest'
import { initialsOf } from '../use-workspace-summary'

describe('initialsOf', () => {
    it('takes the first letter of the first two words', () => {
        expect(initialsOf('Dana Reyes')).toBe('DR')
        expect(initialsOf('dana')).toBe('D')
        expect(initialsOf('Ana Maria Lopez')).toBe('AM')
    })
    it('ignores extra spaces and an empty name', () => {
        expect(initialsOf('  Dana   Reyes ')).toBe('DR')
        expect(initialsOf('')).toBe('')
    })
})
