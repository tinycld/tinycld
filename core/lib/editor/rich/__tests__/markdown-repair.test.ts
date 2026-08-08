import { describe, expect, it } from 'vitest'
import { findDamagedTableRows, repairMarkdown } from '../markdown-repair'

// These repair two defects in @tiptap/markdown's serializer that corrupt
// content rather than merely formatting it oddly. Both were found by
// round-tripping real card descriptions against the Go serializer's corpus.
//
// The overriding property is idempotence: the repaired output is compared
// against a stored baseline to decide whether a document changed, so a repair
// that kept adding fencing or escaping would rewrite every row on every save.

describe('code span fencing', () => {
    it('widens a single-backtick fence around content holding a backtick', () => {
        // The raw serializer emits `a ` b`, which re-parses as a broken span.
        expect(repairMarkdown('A span: `a ` b`.')).toBe('A span: `` a ` b ``.')
    })

    it('leaves a correctly fenced span alone', () => {
        const already = 'A span: `` a ` b ``.'
        expect(repairMarkdown(already)).toBe(already)
    })

    it('is idempotent', () => {
        const once = repairMarkdown('A span: `a ` b`.')
        expect(repairMarkdown(once)).toBe(once)
    })

    it('leaves ordinary code spans untouched', () => {
        expect(repairMarkdown('Call `useActiveBoard` now.')).toBe('Call `useActiveBoard` now.')
    })

    it('does not touch fenced code blocks', () => {
        const block = '```go\nfmt.Println("`")\n```\n'
        expect(repairMarkdown(block)).toBe(block)
    })

    it('handles a span made entirely of backticks', () => {
        const repaired = repairMarkdown('Literal: `` ` ``.')
        expect(repairMarkdown(repaired)).toBe(repaired)
    })
})

describe('table cell pipes', () => {
    // Damage here is detectable but not repairable: `| x | y | z |` in a
    // two-column table could be `x | y` + `z` or `x` + `y | z`, and picking
    // wrong corrupts the table in a way that looks intentional. Persisted
    // markdown comes from the Go serializer, which escapes correctly.
    it('reports a row with too many cells', () => {
        const broken = ['| a | b |', '| --- | --- |', '| x | y | z |'].join('\n')
        expect(findDamagedTableRows(broken)).toEqual(['| x | y | z |'])
    })

    it('reports nothing for a well-formed table', () => {
        const table = [
            '| Board size | First paint |',
            '| ---------- | ----------- |',
            '| 50 cards   | fine        |',
        ].join('\n')
        expect(findDamagedTableRows(table)).toEqual([])
        expect(repairMarkdown(table)).toBe(table)
    })

    it('treats an escaped pipe as part of its cell', () => {
        const table = ['| a | b |', '| --- | --- |', '| x \\| y | z |'].join('\n')
        expect(findDamagedTableRows(table)).toEqual([])
        expect(repairMarkdown(table)).toBe(table)
    })

    it('ignores prose that merely contains a pipe', () => {
        const prose = 'Run a | b in the shell.'
        expect(findDamagedTableRows(prose)).toEqual([])
        expect(repairMarkdown(prose)).toBe(prose)
    })
})

describe('whole documents', () => {
    it('leaves the seeded card description unchanged', () => {
        // The richest markdown the cards seed ships; it must survive untouched.
        const seed = [
            '## What we know',
            '',
            'Boards with **200+ cards** take several seconds to first paint.',
            '',
            '| Board size | First paint |',
            '| ---------- | ----------- |',
            '| 50 cards   | fine        |',
            '',
            '> Measure first.',
            '',
            'See [the notes](https://example.com/notes).',
            '',
        ].join('\n')
        expect(repairMarkdown(seed)).toBe(seed)
    })

    it('preserves modifier glyphs verbatim', () => {
        const typed = 'Press ⌘K then ⇧⌥P.'
        expect(repairMarkdown(typed)).toBe(typed)
    })
})
