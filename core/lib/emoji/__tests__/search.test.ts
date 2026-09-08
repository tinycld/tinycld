import { filterByKeyword, indexEmoji } from '@tinycld/core/lib/emoji/search'
import type { EmojiRecord } from '@tinycld/core/ui/emoji-picker/emoji-data'
import { beforeAll, describe, expect, it } from 'vitest'

// The real table, loaded the same way the picker loads it. Searching against
// a handful of fakes would not catch the cases that matter (incremental
// narrowing, punctuation in keywords, the first-character index).
let all: readonly EmojiRecord[]
let index: ReturnType<typeof indexEmoji>

beforeAll(async () => {
    const { EMOJI_DATA } = await import('@tinycld/core/ui/emoji-picker/emoji-data')
    all = EMOJI_DATA.flatMap(category => category.e)
    index = indexEmoji(all)
})

const find = (query: string, cache?: Map<string, readonly EmojiRecord[]>) =>
    filterByKeyword(query, all, index, cache)

const unifieds = (results: readonly EmojiRecord[] | null) => (results ?? []).map(e => e.u)

describe('filterByKeyword', () => {
    it('finds emoji by an obvious name', () => {
        expect(unifieds(find('thumbs up'))).toContain('1f44d')
        expect(unifieds(find('rocket'))).toContain('1f680')
        expect(unifieds(find('party popper'))).toContain('1f389')
    })

    it('matches on a partial term', () => {
        expect(unifieds(find('rock'))).toContain('1f680')
        expect(unifieds(find('smil')).length).toBeGreaterThan(5)
    })

    it('is case insensitive and ignores punctuation on both sides', () => {
        // The keyword is literally "thumbs up", with a space. Normalizing only
        // the query would strip the space from one side and never match.
        expect(unifieds(find('ROCKET'))).toContain('1f680')
        expect(unifieds(find('thumbs-up'))).toContain('1f44d')
        expect(unifieds(find('thumbsup'))).toContain('1f44d')
    })

    it('distinguishes "not searching" from "found nothing"', () => {
        // null lets the picker show its categories; [] shows an empty state.
        expect(find('')).toBeNull()
        expect(find('   ')).toBeNull()
        expect(find('zzzzzznotanemoji')).toEqual([])
    })
})

describe('incremental narrowing', () => {
    it('returns the same results with a warm cache as with none', () => {
        const cache = new Map<string, readonly EmojiRecord[]>()
        // Prime the cache the way typing does, one character at a time.
        for (const query of ['s', 'sm', 'smi', 'smil', 'smile']) find(query, cache)

        expect(unifieds(find('smile', cache))).toEqual(unifieds(find('smile')))
    })

    it('narrows from the longest cached prefix, not just the last query', () => {
        const cache = new Map<string, readonly EmojiRecord[]>()
        find('ro', cache)
        find('rock', cache)
        find('r', cache) // a shorter query lands in the cache too

        // 'rocke' extends 'rock' (4) and 'ro' (2) and 'r' (1); the result must
        // be correct regardless of which it narrows from.
        expect(unifieds(find('rocke', cache))).toEqual(unifieds(find('rocke')))
    })

    it('does not let a stale prefix hide a match', () => {
        const cache = new Map<string, readonly EmojiRecord[]>()
        const cold = unifieds(find('heart'))
        find('h', cache)
        find('he', cache)
        find('hea', cache)
        expect(unifieds(find('heart', cache))).toEqual(cold)
    })
})

describe('indexEmoji', () => {
    it('buckets by every alphanumeric character in a keyword', () => {
        const rocket = all.find(e => e.u === '1f680')!
        for (const character of new Set('rocket')) {
            expect(index.byCharacter.get(character)).toContain(rocket)
        }
    })

    it('covers the whole table', () => {
        const indexed = new Set([...index.byCharacter.values()].flat())
        // Every emoji has at least one alphanumeric keyword character.
        expect(indexed.size).toBe(all.length)
    })
})
