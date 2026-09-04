import { chipsToText } from '@tinycld/core/lib/search/chip-text'
import { parseQuery } from '@tinycld/core/lib/search/parse-query'
import { describe, expect, it } from 'vitest'

const SLUGS = ['mail', 'drive', 'boards', 'contacts']

describe('parseQuery — chips', () => {
    it('turns a matching word followed by a colon into a chip', () => {
        expect(parseQuery('mail: budget', SLUGS)).toEqual({
            chips: ['mail'],
            include: ['budget'],
            exclude: [],
            remainder: 'budget',
        })
    })

    it('leaves a non-matching word with a colon as literal text', () => {
        expect(parseQuery('budget: q3', SLUGS)).toEqual({
            chips: [],
            include: ['budget', 'q3'],
            exclude: [],
            remainder: 'budget: q3',
        })
    })

    // The regression test for "mail server migration": a package name typed
    // WITHOUT a colon must stay searchable text, or that email is unfindable.
    it('leaves a package name without a colon as searchable text', () => {
        expect(parseQuery('mail server', SLUGS)).toEqual({
            chips: [],
            include: ['mail', 'server'],
            exclude: [],
            remainder: 'mail server',
        })
    })

    it('accepts multiple chips', () => {
        expect(parseQuery('mail: drive: budget', SLUGS)).toEqual({
            chips: ['mail', 'drive'],
            include: ['budget'],
            exclude: [],
            remainder: 'budget',
        })
    })

    it('ignores a duplicate chip but still consumes the word', () => {
        expect(parseQuery('mail: mail: budget', SLUGS)).toEqual({
            chips: ['mail'],
            include: ['budget'],
            exclude: [],
            remainder: 'budget',
        })
    })

    it('matches a slug case-insensitively', () => {
        expect(parseQuery('Mail: budget', SLUGS)).toEqual({
            chips: ['mail'],
            include: ['budget'],
            exclude: [],
            remainder: 'budget',
        })
    })
})

describe('parseQuery — chip-after-text ordering (C1 regression)', () => {
    // THE BUG: textAfterChips used to strip the chip prefix by COMPUTED
    // LENGTH (text.slice(chipsToText(chips).length)), which assumes chips are
    // always a leading prefix of the raw text. parseQuery recognizes `pkg:`
    // ANYWHERE in the string, so typing a chip-forming token AFTER free text
    // ("budget mail: q3") made the slice cut into "budget" instead of the
    // chip prefix — the word survived the first parse and was destroyed on
    // the next keystroke once the corrupted display got written back to the
    // store. parseQuery now recognizes chips ONLY from a leading run at the
    // very start of input, so a later `pkg:`-shaped token is parsed as plain
    // text — never promoted to a chip, and never dropped either.
    it('keeps free text that precedes a chip-forming token as literal words', () => {
        expect(parseQuery('budget mail: q3', SLUGS)).toEqual({
            chips: [],
            include: ['budget', 'mail', 'q3'],
            exclude: [],
            remainder: 'budget mail: q3',
        })
    })

    // Seeded-chip case: a real leading chip (as the palette seeds on open)
    // followed by free text, followed by a SECOND `pkg:`-shaped token. The
    // second token must not be promoted to a chip (only the leading run
    // counts) and "budget" must not be lost.
    it('does not promote a second pkg: token once free text has broken the leading run', () => {
        expect(parseQuery('boards: budget drive: q3', SLUGS)).toEqual({
            chips: ['boards'],
            include: ['budget', 'drive', 'q3'],
            exclude: [],
            remainder: 'budget drive: q3',
        })
    })

    // The palette displays chipsToText(chips) + remainder and writes that
    // same reconstruction back to the store on every keystroke
    // (onChangeRemainder). If that reconstruction ever drops a word, the
    // word is gone for good the next time the user types. This simulates the
    // exact multi-keystroke sequence from the bug report: open with no
    // chips, type "budget mail:", then continue typing " q3".
    it('loses no word across a simulated keystroke round-trip', () => {
        let storeText = ''
        const type = (nextRemainder: string) => {
            const parsed = parseQuery(storeText, SLUGS)
            storeText = chipsToText(parsed.chips) + nextRemainder
        }

        type('budget mail:')
        let parsed = parseQuery(storeText, SLUGS)
        expect(parsed.include).toContain('budget')

        type(`${parsed.remainder} q3`)
        parsed = parseQuery(storeText, SLUGS)

        expect(parsed.include).toEqual(['budget', 'mail', 'q3'])
        expect(parsed.chips).toEqual([])
    })
})

describe('parseQuery — negation', () => {
    it('splits a boundary hyphen into an exclusion', () => {
        expect(parseQuery('budget -draft', SLUGS)).toEqual({
            chips: [],
            include: ['budget'],
            exclude: ['draft'],
            remainder: 'budget -draft',
        })
    })

    // The hyphen is in the FTS strip set precisely because of filenames like
    // this one. A mid-token hyphen must stay literal.
    it('keeps a mid-token hyphen literal', () => {
        expect(parseQuery('budget-2026.xlsx', SLUGS)).toEqual({
            chips: [],
            include: ['budget-2026.xlsx'],
            exclude: [],
            remainder: 'budget-2026.xlsx',
        })
    })

    it('excludes when the hyphen starts the input', () => {
        expect(parseQuery('-draft', SLUGS)).toEqual({
            chips: [],
            include: [],
            exclude: ['draft'],
            remainder: '-draft',
        })
    })

    it('drops a bare hyphen with no attached term', () => {
        expect(parseQuery('budget - draft', SLUGS)).toEqual({
            chips: [],
            include: ['budget', 'draft'],
            exclude: [],
            remainder: 'budget - draft',
        })
    })

    it('strips all leading hyphens from exclusion terms', () => {
        expect(parseQuery('--draft', SLUGS)).toEqual({
            chips: [],
            include: [],
            exclude: ['draft'],
            remainder: '--draft',
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

    it('preserves operator words embedded in hyphenated literals', () => {
        expect(parseQuery('plan-NOT-final.docx', SLUGS)).toEqual({
            chips: [],
            include: ['plan-NOT-final.docx'],
            exclude: [],
            remainder: 'plan-NOT-final.docx',
        })
    })
})

describe('parseQuery — empty input', () => {
    it('returns empty arrays for blank input', () => {
        expect(parseQuery('   ', SLUGS)).toEqual({
            chips: [],
            include: [],
            exclude: [],
            remainder: '   ',
        })
    })
})
