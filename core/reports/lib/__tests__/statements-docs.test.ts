import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

// The help and the package docs name each statement extension (G1–G8), so a
// reader and a package author can find it.
const root = join(__dirname, '..', '..', '..', '..')
const read = (path: string) => readFileSync(join(root, path), 'utf8')

describe('statement docs', () => {
    it('has a help section for each statement extension', () => {
        const help = read('core/help/reports.md')
        for (const heading of [
            'Group by levels some rows do not have',
            'Column headings that name a day',
            'Statements: sections and computed rows',
            'Hide groups with nothing to show',
            'Group by a formula',
        ]) {
            expect(help).toContain(`## ${heading}`)
        }
    })

    it('lists every new field in the version 4 table', () => {
        const docs = read('docs/packages.md')
        const start = docs.indexOf('**Fields added in version 4.**')
        expect(start).toBeGreaterThan(-1)
        const table = docs.slice(start, docs.indexOf('\n\n', docs.indexOf('| Field |', start)))
        for (const field of [
            'groups[].blank',
            'groups[].hideEmpty',
            'columns[].heading',
            'columns[].hidden',
            'layout[].within',
            'layout[].cells',
        ]) {
            expect(table).toContain(`\`${field}\``)
        }
    })
})
