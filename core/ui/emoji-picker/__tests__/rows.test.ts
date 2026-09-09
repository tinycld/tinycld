import type { EmojiRecord } from '@tinycld/core/ui/emoji-picker/emoji-data'
import { type EmojiRow, toRows, toSearchRows } from '@tinycld/core/ui/emoji-picker/rows'
import { describe, expect, it } from 'vitest'

const cells = (row: EmojiRow) => (row.kind === 'emoji' ? row.emoji : [])

const emoji = (u: string): EmojiRecord => ({ u, n: [u] })
const many = (count: number) => Array.from({ length: count }, (_, i) => emoji(`e${i}`))

describe('toRows', () => {
    it('emits a header then chunks of 8', () => {
        const rows = toRows([{ category: 'objects', emoji: many(20) }])
        expect(rows[0]).toMatchObject({ kind: 'header', category: 'objects' })
        expect(rows.filter(r => r.kind === 'emoji')).toHaveLength(3) // 8 + 8 + 4
        expect(rows[1]).toMatchObject({ kind: 'emoji' })
        expect(cells(rows[1])).toHaveLength(8)
        expect(cells(rows[3])).toHaveLength(4)
    })

    it('skips an empty section rather than emitting a bare header', () => {
        // "Frequently used" is absent until someone has used something; a
        // header with nothing under it reads as a bug.
        const rows = toRows([
            { category: 'frequent', emoji: [] },
            { category: 'objects', emoji: many(3) },
        ])
        expect(rows.filter(r => r.kind === 'header')).toHaveLength(1)
        expect(rows[0]).toMatchObject({ category: 'objects' })
    })

    it('gives every row a unique key', () => {
        const rows = toRows([
            { category: 'a', emoji: many(9) },
            { category: 'b', emoji: many(9) },
        ])
        expect(new Set(rows.map(r => r.key)).size).toBe(rows.length)
    })

    it('returns nothing for no sections', () => {
        expect(toRows([])).toEqual([])
    })

    it('chunks at a caller-supplied width, for the wider sheet grid', () => {
        const rows = toRows([{ category: 'objects', emoji: many(20) }], 12)
        expect(rows.filter(r => r.kind === 'emoji')).toHaveLength(2) // 12 + 8
        expect(cells(rows[1])).toHaveLength(12)
        expect(cells(rows[2])).toHaveLength(8)
    })

    it('never chunks at zero or a fraction, which would not terminate', () => {
        expect(cells(toRows([{ category: 'o', emoji: many(3) }], 0)[1])).toHaveLength(1)
        expect(cells(toRows([{ category: 'o', emoji: many(9) }], 4.7)[1])).toHaveLength(4)
    })
})

describe('toSearchRows', () => {
    it('chunks without headers', () => {
        const rows = toSearchRows(many(10))
        expect(rows.every(r => r.kind === 'emoji')).toBe(true)
        expect(rows).toHaveLength(2)
    })

    it('returns nothing for no results', () => {
        expect(toSearchRows([])).toEqual([])
    })

    it('chunks at a caller-supplied width too', () => {
        const rows = toSearchRows(many(10), 5)
        expect(rows).toHaveLength(2)
        expect(cells(rows[0])).toHaveLength(5)
    })

    it('keys distinctly from category rows, so a re-render cannot collide', () => {
        const search = toSearchRows(many(3))
        const category = toRows([{ category: 'objects', emoji: many(3) }])
        const overlap = new Set(search.map(r => r.key))
        expect(category.filter(r => overlap.has(r.key))).toEqual([])
    })
})
