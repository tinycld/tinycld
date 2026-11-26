import { describe, expect, it } from 'vitest'
import { deriveCode, uniqueKey } from '../short-code'

// The same vectors are in books/server/record_codes_test.go and are checked against the
// Books backfill migration; change all three together.
const DERIVE_15: [string, string][] = [
    ['Aspen Ridge Dental', 'ARD'],
    ['Acme Rental Depot', 'ARD'],
    ['Acme', 'ACME'],
    ['  Acme  ', 'ACME'],
    ['Café Noir', 'CN'],
    ["O'Brien & Sons", 'OBS'],
    ['Smith-Jones Ltd', 'SJL'],
    ['3M', '3M'],
    ['Verylongsinglewordname', 'VERYLONGSINGLEW'],
    ['A B C D E F G H I J K L M N O P Q', 'ABCDEFGHIJKLMNO'],
    ['X', ''],
    ['李明', ''],
    ['Ærø', ''],
    ['', ''],
]

describe('deriveCode', () => {
    it.each(DERIVE_15)('%s → %s (max 15)', (name, code) => {
        expect(deriveCode(name, { max: 15 })).toBe(code)
    })

    it('cuts to the max it is given', () => {
        expect(deriveCode('Verylongsinglewordname', { max: 10 })).toBe('VERYLONGSI')
    })
})

describe('uniqueKey with a separator and a max', () => {
    const code = { separator: '-', max: 15 }

    it('keeps a free base', () => {
        expect(uniqueKey('ARD', new Set(), code)).toBe('ARD')
    })

    it('adds -2, then -3', () => {
        expect(uniqueKey('ARD', new Set(['ARD']), code)).toBe('ARD-2')
        expect(uniqueKey('ARD', new Set(['ARD', 'ARD-2']), code)).toBe('ARD-3')
    })

    it('cuts the base so the suffix fits', () => {
        expect(uniqueKey('VERYLONGSINGLEW', new Set(['VERYLONGSINGLEW']), code)).toBe(
            'VERYLONGSINGL-2'
        )
        const taken = new Set(['VERYLONGSINGLEW'])
        for (let n = 2; n <= 9; n++) taken.add(`VERYLONGSINGL-${n}`)
        expect(uniqueKey('VERYLONGSINGLEW', taken, code)).toBe('VERYLONGSING-10')
    })

    it('defaults to the forms builder key shape', () => {
        expect(uniqueKey('severity', new Set(['severity']))).toBe('severity_2')
    })
})
