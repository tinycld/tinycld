import { describe, expect, it } from 'vitest'
import {
    newReportStep,
    packageReports,
    reportTermsGate,
    sharedReports,
    slugOfHubRow,
    slugOfSource,
} from '../hub-rows'

describe('the package of a source', () => {
    it('is the part of the source key before the dot', () => {
        expect(slugOfSource({ key: 'zoo.animals' })).toBe('zoo')
        expect(
            slugOfHubRow({ report: { source: 'acme_tools.parts', spec: { source: 'zoo.x' } } })
        ).toBe('acme_tools')
    })

    it('reads the spec of a report the server has not saved yet', () => {
        // The server writes `source`, so a just-inserted copy has only its spec's.
        expect(
            slugOfHubRow({ report: { source: undefined, spec: { source: 'zoo.animals' } } })
        ).toBe('zoo')
    })

    it('is the whole key when the key has no dot', () => {
        expect(slugOfSource({ key: 'zoo' })).toBe('zoo')
    })
})

describe('reportTermsGate', () => {
    const declares = (slug: string) => slug === 'zoo'
    const gate = (source: string, blocked: string[], isReady: boolean) =>
        reportTermsGate({ source, blocked: new Set(blocked), isReady, declaresTerms: declares })

    it('closes a report whose source belongs to a closed package', () => {
        expect(gate('zoo.animals', ['zoo'], true)).toBe('closed')
    })

    it('opens a report of a package whose terms are accepted', () => {
        expect(gate('zoo.animals', [], true)).toBe('open')
    })

    it('waits for the terms state only when the package declares terms', () => {
        expect(gate('zoo.animals', ['zoo'], false)).toBe('wait')
        expect(gate('acme.parts', ['zoo'], false)).toBe('open')
    })

    it('reads the package from the source key, not a look-alike', () => {
        expect(gate('zoology.animals', ['zoo'], true)).toBe('open')
    })
})

describe('newReportStep', () => {
    const step = (isCatalogReady: boolean, hasSource: boolean, terms: 'open' | 'wait' | 'closed') =>
        newReportStep({ isCatalogReady, hasSource, terms })

    it('shows the catalog frame until the catalog is ready', () => {
        expect(step(false, false, 'closed')).toBe('catalog')
    })

    it('waits for the terms state of the source package', () => {
        expect(step(true, true, 'wait')).toBe('wait')
    })

    it('treats a source of a closed package as unknown', () => {
        expect(step(true, true, 'closed')).toBe('unknown-source')
        expect(step(true, false, 'open')).toBe('unknown-source')
    })

    it('opens the builder for a known source of an open package', () => {
        expect(step(true, true, 'open')).toBe('build')
    })
})

const report = (id: string, title: string, source: string, updated: string) => ({
    id,
    title,
    source,
    updated,
})

const ledger = report('r1', 'Trial Balance', 'books.journal_lines', '2026-11-01 10:00:00.000Z')
const sales = report('r2', 'Sales by month', 'books.invoices', '2026-11-03 10:00:00.000Z')
const aged = report('r3', 'Aged receivables', 'books.invoices', '2026-11-02 10:00:00.000Z')
const other = report('r4', 'Animals', 'zoo.animals', '2026-11-04 10:00:00.000Z')

describe('sharedReports', () => {
    it('lists each report once, newest change first, with every role it reaches the reader by', () => {
        const { reports, roles } = sharedReports([
            { role: 'viewer', report: ledger },
            { role: 'editor', report: ledger },
            { role: 'viewer', report: sales },
            // A share whose report the reader cannot read (yet) has no report.
            { role: 'viewer', report: undefined },
        ])
        expect(reports.map(r => r.id)).toEqual(['r2', 'r1'])
        expect(roles.get('r1')).toEqual(['viewer', 'editor'])
        expect(roles.get('r2')).toEqual(['viewer'])
    })
})

describe('packageReports', () => {
    it('lists every readable report of the package once, by title', () => {
        const rows = packageReports({
            prefix: 'books.',
            mine: [sales],
            shared: [ledger, sales, other],
            published: [ledger, aged, other],
        })
        expect(rows.map(r => r.title)).toEqual([
            'Aged receivables',
            'Sales by month',
            'Trial Balance',
        ])
    })

    it('reads the package from the source key, not a look-alike', () => {
        const lookAlike = report('r5', 'Bookshelf', 'bookshelf.items', '2026-11-04 10:00:00.000Z')
        const rows = packageReports({
            prefix: 'books.',
            mine: [lookAlike],
            shared: [],
            published: [],
        })
        expect(rows).toEqual([])
    })

    it('orders equal titles by id, so the list does not reshuffle', () => {
        const a = report('b', 'Same', 'books.a', '2026-11-01 10:00:00.000Z')
        const b = report('a', 'Same', 'books.b', '2026-11-02 10:00:00.000Z')
        const rows = packageReports({ prefix: 'books.', mine: [a], shared: [], published: [b] })
        expect(rows.map(r => r.id)).toEqual(['a', 'b'])
    })
})
