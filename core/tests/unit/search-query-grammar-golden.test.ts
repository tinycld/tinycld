import { readFileSync } from 'node:fs'
import path from 'node:path'
import { parseQuery } from '@tinycld/core/lib/search/parse-query'
import { describe, expect, it } from 'vitest'

// The SAME file the Go port's test reads (cli/parse_query_test.go). Keeping one
// fixture rather than two copies is the only thing that keeps the terminal's
// grammar identical to the palette's: a rule changed in one language fails the
// other language's test instead of silently diverging.
const FIXTURE = path.join(__dirname, '../../../cli/testdata/query-grammar.json')

interface GoldenCase {
    name: string
    input: string
    chips: string[]
    include: string[]
    exclude: string[]
}

const golden: { slugs: string[]; cases: GoldenCase[] } = JSON.parse(readFileSync(FIXTURE, 'utf8'))

describe('parseQuery — shared golden fixture', () => {
    it.each(golden.cases.map(c => [c.name, c] as const))('%s', (_name, c) => {
        const parsed = parseQuery(c.input, golden.slugs)
        expect({
            chips: parsed.chips,
            include: parsed.include,
            exclude: parsed.exclude,
        }).toEqual({ chips: c.chips, include: c.include, exclude: c.exclude })
    })
})
