import { describe, expect, it } from 'vitest'
import { isTwelveHour } from '../date-time'
import {
    canExport,
    type ExportFormat,
    ExportRefusal,
    exportFileName,
    exportOffered,
    exportRequestOf,
} from '../export-client'
import fixture from '../fixtures/format-cases.json'
import type { DisplayOptions } from '../format'
import type { RunBody } from '../run-client'
import type { ParamDef } from '../spec'
import type { ResolvedParam } from '../types'

// The file name list is the one the Go suite runs (export_format_test.go), so the
// name the dialog shows and the name of the file the server sends cannot differ (D6).
type FileNameCase = {
    name: string
    title: string
    defs: ParamDef[]
    params: ResolvedParam[]
    today: string
    ext: ExportFormat
    text: string
}

const fileNames = (fixture as unknown as { fileNames: FileNameCase[] }).fileNames

describe('exportFileName', () => {
    it('has cases to run', () => {
        expect(fileNames.map(c => c.ext)).toEqual(expect.arrayContaining(['pdf', 'xlsx', 'csv']))
    })

    it.each(fileNames.map(c => [c.name, c] as const))('%s', (_name, c) => {
        expect(
            exportFileName({
                title: c.title,
                defs: c.defs,
                resolved: c.params,
                today: c.today,
                format: c.ext,
            })
        ).toBe(c.text)
    })
})

describe('exportRequestOf', () => {
    const body: RunBody = {
        reportId: 'rpt000000000001',
        params: {
            period: { from: '2026-01-01', to: '2026-03-31' },
            teams: ['team00000000001'],
        },
        options: {
            expand: [['north']],
            expandAll: true,
            collapse: [['north', 'team00000000001']],
            cursor: 'next-page-cursor',
            limit: 200,
            sort: [{ col: 'amount', dir: 'desc' }],
            today: '2026-11-16',
            timeZone: 'Asia/Tokyo',
        },
    }
    const display: DisplayOptions = {
        divideBy1000: true,
        withoutCents: true,
        negative: 'parens',
        negativeRed: true,
        hidden: ['cost'],
    }

    it('carries the view state, adds the display options and the clock, and drops the paging', () => {
        const deviceHour12 = isTwelveHour(
            new Intl.DateTimeFormat(undefined, { hour: 'numeric' }).resolvedOptions()
        )
        const request = exportRequestOf(body, display)
        expect(request).toEqual({
            reportId: 'rpt000000000001',
            params: {
                period: { from: '2026-01-01', to: '2026-03-31' },
                teams: ['team00000000001'],
            },
            options: {
                expand: [['north']],
                expandAll: true,
                collapse: [['north', 'team00000000001']],
                sort: [{ col: 'amount', dir: 'desc' }],
                today: '2026-11-16',
                timeZone: 'Asia/Tokyo',
                display: {
                    divideBy1000: true,
                    withoutCents: true,
                    negative: 'parens',
                    negativeRed: true,
                    hidden: ['cost'],
                },
            },
            hour12: deviceHour12,
        })
        expect(request.options).not.toHaveProperty('cursor')
        expect(request.options).not.toHaveProperty('limit')
    })
})

describe('ExportRefusal', () => {
    it("takes the server's message", () => {
        const refusal = new ExportRefusal(
            422,
            'This report has more than 5,000 rows. Narrow the period or download CSV.'
        )
        expect(refusal).toBeInstanceOf(Error)
        expect(refusal.status).toBe(422)
        expect(refusal.message).toBe(
            'This report has more than 5,000 rows. Narrow the period or download CSV.'
        )
    })

    it('names the status when the server sent no message', () => {
        const refusal = new ExportRefusal(500, undefined)
        expect(refusal.status).toBe(500)
        expect(refusal.message).toBe('The server answered 500.')
    })
})

describe('canExport', () => {
    it.each([
        ['ready', true],
        ['empty', true],
        ['loading', false],
        ['error', false],
        ['timeout', false],
        ['no-access', false],
        ['incomplete', false],
        ['missing', false],
        ['invalid', false],
    ] as const)('%s → %s', (state, expected) => {
        expect(canExport(state)).toBe(expected)
    })
})

describe('exportOffered', () => {
    it.each([
        ['a saved report, not edited, in the main view', true, false, true, true],
        ['a draft', false, false, true, false],
        ['a report being edited', true, true, true, false],
        ['the drill panel', true, false, false, false],
    ] as const)('%s → %s', (_name, isSaved, isEditing, isMainView, expected) => {
        expect(exportOffered({ isSaved, isEditing, isMainView })).toBe(expected)
    })
})
