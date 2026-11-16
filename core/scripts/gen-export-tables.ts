/**
 * Writes server/reports/export_tables.json: the two tables the server's report
 * export formats with, read from Intl in en-US as the client reads them.
 *
 *   currencies  every Intl currency code -> the text before the digits of a
 *               positive amount ("$", "€", "CA$", "CHF" and a no-break space)
 *   zones       every Intl time zone and every zone of Go's zoneinfo -> its
 *               words in the header's zone line (timeZoneWords,
 *               reports/lib/date-time.ts)
 *
 * The Go server has no Intl, so export_format.go reads this file. It is
 * generated once and committed; format-cases.json pins twelve currencies and
 * six zones, so a table that differs from Intl fails the vitest and Go runs.
 *
 *   cd tinycld/core && pnpm exec tsx scripts/gen-export-tables.ts
 *
 * Deterministic: keys are sorted, so a re-run on the same engine writes the
 * same bytes.
 */

import { execFileSync } from 'node:child_process'
import { writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { timeZoneWords } from '../reports/lib/date-time'

const here = dirname(fileURLToPath(import.meta.url))
const target = join(here, '..', 'server', 'reports', 'export_tables.json')

// The text before the integer part of a formatted amount, from formatToParts of
// a sample, as lib/money.ts formats one.
function prefixOf(format: Intl.NumberFormat, sample: number): string {
    const parts = format.formatToParts(sample)
    const integer = parts.findIndex(part => part.type === 'integer')
    return parts
        .slice(0, integer)
        .map(part => part.value)
        .join('')
}

function currencyPrefix(code: string): string {
    const format = new Intl.NumberFormat('en-US', {
        style: 'currency',
        currency: code,
        minimumFractionDigits: 2,
        maximumFractionDigits: 2,
    })
    const positive = prefixOf(format, 1)
    const parts = format.formatToParts(1)
    const after = parts.slice(parts.findIndex(part => part.type === 'fraction') + 1)
    // export_format.go writes a negative as "-" and the positive form, and the
    // amount ends at its cents; a currency Intl lays out otherwise needs a new rule.
    if (prefixOf(format, -1) !== `-${positive}` || after.length > 0) {
        throw new Error(`${code}: Intl lays out its amounts in a form the server does not write`)
    }
    return positive
}

function sortedObject(entries: [string, string][]): Record<string, string> {
    return Object.fromEntries(entries.sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0)))
}

const currencies = sortedObject(
    Intl.supportedValuesOf('currency').map(code => [code, currencyPrefix(code)])
)
// Intl lists each zone once, by its ICU name (Asia/Calcutta), while a reader's
// device may send any IANA name Go accepts (Asia/Kolkata). The zone names of
// Go's own zoneinfo (what readerZone loads) are added, so every zone the server
// reads has its words.
function goZoneNames(): string[] {
    const goroot = execFileSync('go', ['env', 'GOROOT'], { encoding: 'utf8' }).trim()
    const listing = execFileSync('unzip', ['-Z1', join(goroot, 'lib', 'time', 'zoneinfo.zip')], {
        encoding: 'utf8',
    })
    return listing.split('\n').filter(name => name !== '' && !name.endsWith('/'))
}

const zoneNames = new Set([...Intl.supportedValuesOf('timeZone'), ...goZoneNames()])
const zones = sortedObject([...zoneNames].map(zone => [zone, timeZoneWords(zone)]))

writeFileSync(target, `${JSON.stringify({ currencies, zones }, null, 4)}\n`)
