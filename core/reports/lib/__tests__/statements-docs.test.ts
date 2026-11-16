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
            'Show a row as a percent of a section',
            'Pivot by month across a whole period',
        ]) {
            expect(help).toContain(`## ${heading}`)
        }
    })

    it('names the pivot controls and the bucket words in the help', () => {
        const help = read('core/help/reports.md')
        expect(help).toContain('**Total after the pivot columns**')
        expect(help).toContain("**Percent of a section's total**")
        for (const words of ['"Jan 2026"', '"Q1 2026"', '"FY 2026"']) {
            expect(help).toContain(words)
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
            'columns[].formula',
            'pivot.range',
            'pivot.order',
            'columns[].pivot.labelType',
        ]) {
            expect(table).toContain(`\`${field}\``)
        }
    })

    it('names the percent of a section operator in the version 4 table', () => {
        const docs = read('docs/packages.md')
        const start = docs.indexOf('**Fields added in version 4.**')
        const table = docs.slice(start, docs.indexOf('\n\n', docs.indexOf('| Field |', start)))
        expect(table).toContain('`percentOfSection`')
    })

    it('names the Totals choice in the help', () => {
        const help = read('core/help/reports.md')
        expect(help).toContain('**Totals**')
        expect(help).toContain('**On every row**')
        expect(help).toContain('**Up to')
    })

    it('shows a union source in Report sources and blankAbove in the version 4 table', () => {
        const docs = read('docs/packages.md')
        const sources = docs.slice(docs.indexOf('### Report sources'))
        expect(sources).toContain('Parts: []reports.SourcePart{')
        expect(sources).toContain('Values:')
        const start = docs.indexOf('**Fields added in version 4.**')
        const table = docs.slice(start, docs.indexOf('\n\n', docs.indexOf('| Field |', start)))
        expect(table).toContain('`columns[].blankAbove`')
        expect(table).toContain('`unknown_group`')
    })

    it('names drills, open links and the drill panel in the help and the docs', () => {
        const help = read('core/help/reports.md')
        expect(help).toContain('## Open the rows behind a figure')
        expect(help).toContain("## Open a row's record")
        expect(help).toContain('1360 pixels')
        const docs = read('docs/packages.md')
        const start = docs.indexOf('**Fields added in version 4.**')
        const table = docs.slice(start, docs.indexOf('\n\n', docs.indexOf('| Field |', start)))
        expect(table).toContain('`columns[].drill`')
        expect(table).toContain('`detail.open`')
        const shippedAt = docs.indexOf('### Shipped and saved reports')
        const shipped = docs.slice(shippedAt, docs.indexOf('\n### ', shippedAt + 1))
        expect(shipped).toContain('DETAIL_PANEL_MIN_WINDOW')
        expect(shipped).toContain('1360 px')
    })

    it('names date-time columns, the time zone and the zone line in the help', () => {
        const help = read('core/help/reports.md')
        const at = help.indexOf('## Dates with a time')
        expect(at).toBeGreaterThan(-1)
        const section = help.slice(at, help.indexOf('\n## ', at + 1))
        expect(section).toContain('date and a time')
        expect(section).toContain('your time zone')
        expect(section).toContain('**Times in Central Time**')
        expect(section).toContain('The period chip and a day you type in a filter')
        const opens = help.slice(help.indexOf("## Open a row's record"))
        expect(opens).toContain('in a panel at the right')
    })

    it('names record panels, Select over text, view sources and the zone rule in the docs', () => {
        const docs = read('docs/packages.md')
        const fields = docs.slice(
            docs.indexOf('### Field reference'),
            docs.indexOf('#### Help screenshots')
        )
        expect(fields).toContain('| `reportPanels[]` |')
        const sources = docs.slice(
            docs.indexOf('### Report sources'),
            docs.indexOf('### Shipped and saved reports')
        )
        expect(sources).toContain('A `Select` field may read a text column')
        expect(sources).toContain('view collection')
        expect(sources).toContain('CAST(')
        expect(sources).toContain("in the reader's time zone")
        expect(sources).toContain('keep the UTC day')
        expect(sources).toContain('a day typed in a condition')
        expect(sources).toContain('with a `Date` column still compares in UTC')
        expect(sources).toContain('`timeZone` with every param-options request')
        expect(sources).toContain('dateTimeText')
        const shippedAt = docs.indexOf('### Shipped and saved reports')
        const shipped = docs.slice(shippedAt, docs.indexOf('\n### ', shippedAt + 1))
        for (const name of [
            'reportPanels',
            'ReportPanelProps',
            'usePanelTitle',
            '`dr=<app path>`',
        ]) {
            expect(shipped).toContain(name)
        }
        const generated = docs.slice(
            docs.indexOf('### D. `lib/generated/*`'),
            docs.indexOf('### E. ')
        )
        expect(generated).toContain('`report-panels.ts`')
    })
})
