import { describe, expect, it } from 'vitest'
import { uniqueKey } from '../../../short-code'
import { keyFromLabel } from '../key'

describe('keyFromLabel', () => {
    it('slugifies to lowercase with underscores', () => {
        expect(keyFromLabel('Steps to Reproduce')).toBe('steps_to_reproduce')
        expect(keyFromLabel('  Sévérité!  ')).toBe('severite')
        expect(keyFromLabel('2nd Owner')).toBe('f_2nd_owner')
        expect(keyFromLabel('')).toBe('field')
    })
    it('caps at 40 characters', () => {
        expect(keyFromLabel('a'.repeat(60))).toHaveLength(40)
    })
})

describe('uniqueKey', () => {
    it('returns the base when free', () => {
        expect(uniqueKey('severity', new Set(['name']))).toBe('severity')
    })
    it('appends a counter when taken', () => {
        expect(uniqueKey('severity', new Set(['severity']))).toBe('severity_2')
        expect(uniqueKey('severity', new Set(['severity', 'severity_2']))).toBe('severity_3')
    })
    it('keeps the result within 40 characters', () => {
        const base = 'a'.repeat(40)
        expect(uniqueKey(base, new Set([base]))).toHaveLength(40)
    })
})
