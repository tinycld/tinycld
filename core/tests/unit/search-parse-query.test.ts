import { parseQuery } from '@tinycld/core/lib/search/parse-query'
import { describe, expect, it } from 'vitest'

const SLUGS = ['mail', 'drive', 'cards', 'contacts']

describe('parseQuery — chips', () => {
    it('turns a matching word followed by a colon into a chip', () => {
        expect(parseQuery('mail: budget', SLUGS)).toEqual({
            chips: ['mail'],
            include: ['budget'],
            exclude: [],
        })
    })

    it('leaves a non-matching word with a colon as literal text', () => {
        expect(parseQuery('budget: q3', SLUGS)).toEqual({
            chips: [],
            include: ['budget', 'q3'],
            exclude: [],
        })
    })

    // The regression test for "mail server migration": a package name typed
    // WITHOUT a colon must stay searchable text, or that email is unfindable.
    it('leaves a package name without a colon as searchable text', () => {
        expect(parseQuery('mail server', SLUGS)).toEqual({
            chips: [],
            include: ['mail', 'server'],
            exclude: [],
        })
    })

    it('accepts multiple chips', () => {
        expect(parseQuery('mail: drive: budget', SLUGS)).toEqual({
            chips: ['mail', 'drive'],
            include: ['budget'],
            exclude: [],
        })
    })

    it('ignores a duplicate chip but still consumes the word', () => {
        expect(parseQuery('mail: mail: budget', SLUGS)).toEqual({
            chips: ['mail'],
            include: ['budget'],
            exclude: [],
        })
    })

    it('matches a slug case-insensitively', () => {
        expect(parseQuery('Mail: budget', SLUGS)).toEqual({
            chips: ['mail'],
            include: ['budget'],
            exclude: [],
        })
    })
})

describe('parseQuery — negation', () => {
    it('splits a boundary hyphen into an exclusion', () => {
        expect(parseQuery('budget -draft', SLUGS)).toEqual({
            chips: [],
            include: ['budget'],
            exclude: ['draft'],
        })
    })

    // The hyphen is in the FTS strip set precisely because of filenames like
    // this one. A mid-token hyphen must stay literal.
    it('keeps a mid-token hyphen literal', () => {
        expect(parseQuery('budget-2026.xlsx', SLUGS)).toEqual({
            chips: [],
            include: ['budget-2026.xlsx'],
            exclude: [],
        })
    })

    it('excludes when the hyphen starts the input', () => {
        expect(parseQuery('-draft', SLUGS)).toEqual({
            chips: [],
            include: [],
            exclude: ['draft'],
        })
    })

    it('drops a bare hyphen with no attached term', () => {
        expect(parseQuery('budget - draft', SLUGS)).toEqual({
            chips: [],
            include: ['budget', 'draft'],
            exclude: [],
        })
    })
})

describe('parseQuery — operator stripping', () => {
    it.each([
        ['a && b', ['a', 'b']],
        ['a || b', ['a', 'b']],
        ['a AND b', ['a', 'b']],
        ['a OR b', ['a', 'b']],
        ['a NOT b', ['a', 'b']],
        ['!urgent', ['urgent']],
        ['"quoted phrase"', ['quoted', 'phrase']],
        ['(grouped)', ['grouped']],
    ])('strips operators from %s', (input, expected) => {
        expect(parseQuery(input, SLUGS).include).toEqual(expected)
    })
})

describe('parseQuery — empty input', () => {
    it('returns empty arrays for blank input', () => {
        expect(parseQuery('   ', SLUGS)).toEqual({ chips: [], include: [], exclude: [] })
    })
})
