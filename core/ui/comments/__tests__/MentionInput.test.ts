import { describe, expect, it } from 'vitest'
import { caretAfterEdit, detectTrigger, renderMentionsToText } from '../mention-input-helpers'

describe('detectTrigger', () => {
    it('returns null on an empty value', () => {
        expect(detectTrigger('', 0)).toBeNull()
    })

    it('detects @ at the start of the body', () => {
        const v = '@ali'
        expect(detectTrigger(v, 4)).toEqual({ atIndex: 0, caretIndex: 4, query: 'ali' })
    })

    it('detects @ after whitespace', () => {
        const v = 'hey @bob'
        expect(detectTrigger(v, 8)).toEqual({ atIndex: 4, caretIndex: 8, query: 'bob' })
    })

    it('does not trigger on @ inside a word (e.g. email)', () => {
        const v = 'mail user@example.com'
        // caret right after the @
        expect(detectTrigger(v, 10)).toBeNull()
        // caret in the middle of the domain
        expect(detectTrigger(v, 18)).toBeNull()
    })

    it('does not trigger when the @-query already contains whitespace', () => {
        const v = '@ali bob'
        // caret right after the space — the query has ended, no popover.
        expect(detectTrigger(v, 8)).toBeNull()
    })

    it('returns empty query just after @ is typed', () => {
        const v = 'hello @'
        expect(detectTrigger(v, 7)).toEqual({ atIndex: 6, caretIndex: 7, query: '' })
    })

    it('returns null when the caret is past the end of the buffer', () => {
        expect(detectTrigger('@ali', 999)).toBeNull()
    })
})

describe('renderMentionsToText', () => {
    it('replaces tokens with @<displayName> when known', () => {
        const names = new Map([['uo_alice', 'Alice']])
        expect(renderMentionsToText('hi [[@uo_alice]]', names)).toBe('hi @Alice')
    })

    it('falls back to @<id> when the user is unknown', () => {
        expect(renderMentionsToText('hi [[@uo_ghost]]', new Map())).toBe('hi @uo_ghost')
    })

    it('handles multiple tokens', () => {
        const names = new Map([
            ['uo_alice', 'Alice'],
            ['uo_bob', 'Bob'],
        ])
        expect(renderMentionsToText('cc [[@uo_alice]] [[@uo_bob]]', names)).toBe('cc @Alice @Bob')
    })

    it('leaves text without tokens untouched', () => {
        expect(renderMentionsToText('plain body', new Map())).toBe('plain body')
    })
})

// The caret arithmetic MentionInput runs on every keystroke. It matters
// because the caret is what `detectTrigger` reads and what a pick
// splices on: land it in the wrong place and the popover opens on an
// unrelated `@…` further along the body, then silently truncates
// everything between that `@` and the real caret.
describe('caretAfterEdit', () => {
    it('advances by the inserted length from a collapsed caret', () => {
        // "hey " + "@" typed at the end.
        const prev = { text: 'hey ', start: 4, end: 4 }
        expect(caretAfterEdit(prev, 'hey @')).toBe(5)
    })

    it('retreats by the deleted length from a collapsed caret', () => {
        // Backspace at the end of "hey @".
        const prev = { text: 'hey @', start: 5, end: 5 }
        expect(caretAfterEdit(prev, 'hey ')).toBe(4)
    })

    it('keeps the caret mid-body when the edit is not at the end', () => {
        // Typing "x" at offset 3 of "hey there".
        const prev = { text: 'hey there', start: 3, end: 3 }
        expect(caretAfterEdit(prev, 'heyx there')).toBe(4)
    })

    // The regression: a non-collapsed selection. Deleting it leaves the
    // caret where the selection began — NOT `start - (end - start)`,
    // which is what tracking only `selection.start` produced.
    it('leaves the caret at the selection start when a range is deleted', () => {
        // Select "there" (offsets 4..9) in "hey there" and press Delete.
        const prev = { text: 'hey there', start: 4, end: 9 }
        expect(caretAfterEdit(prev, 'hey ')).toBe(4)
    })

    it('puts the caret after the replacement when a range is typed over', () => {
        // Select "there" (offsets 4..9) in "hey there" and type "@".
        const prev = { text: 'hey there', start: 4, end: 9 }
        expect(caretAfterEdit(prev, 'hey @')).toBe(5)
    })

    it('puts the caret after a multi-character replacement of a range', () => {
        // Select "there" and paste "everyone".
        const prev = { text: 'hey there', start: 4, end: 9 }
        expect(caretAfterEdit(prev, 'hey everyone')).toBe(12)
    })

    it('handles a whole-body select-all then type', () => {
        const prev = { text: 'hey there', start: 0, end: 9 }
        expect(caretAfterEdit(prev, '@')).toBe(1)
    })

    it('clamps into the new text when the selection is stale', () => {
        // A programmatic reset can leave a selection past the new end.
        const prev = { text: 'hey there', start: 20, end: 20 }
        expect(caretAfterEdit(prev, '')).toBe(0)
    })

    // Select-range-then-type is exactly how a user replaces a word with
    // a mention, so assert the caret it produces actually opens the
    // trigger — the two helpers have to agree or the popover never shows.
    it('lands a caret that detectTrigger reads as an open mention', () => {
        const prev = { text: 'hey there', start: 4, end: 9 }
        const caret = caretAfterEdit(prev, 'hey @al')
        expect(caret).toBe(7)
        expect(detectTrigger('hey @al', caret)).toEqual({
            atIndex: 4,
            caretIndex: 7,
            query: 'al',
        })
    })
})
