import { describe, expect, it } from 'vitest'
import { isRowShown, recordPanelOf, recordPanelParams, rowOpensInPanel } from '../record-panel'
import type { ResultRow } from '../types'

// A fictional package: core names no real one.
const PANELS = [
    { slug: 'gizmos', path: 'history' },
    { slug: 'gizmos', path: 'history/deep' },
    { slug: 'acme', path: 'card' },
]

function row(id: string, open?: string): ResultRow {
    return { id, kind: 'detail', depth: 0, label: id, cells: {}, ...(open ? { open } : {}) }
}

describe('recordPanelOf', () => {
    it('names the panel and the segments after <slug>/<path>/', () => {
        expect(recordPanelOf('gizmos/history/c/1', PANELS)).toEqual({
            slug: 'gizmos',
            path: 'history',
            segments: ['c', '1'],
        })
    })

    it('reads a path with one leading slash as the same path', () => {
        expect(recordPanelOf('/gizmos/history/c/1', PANELS)).toEqual({
            slug: 'gizmos',
            path: 'history',
            segments: ['c', '1'],
        })
    })

    it('takes the longer of two nested paths', () => {
        expect(recordPanelOf('gizmos/history/deep/1', PANELS)).toEqual({
            slug: 'gizmos',
            path: 'history/deep',
            segments: ['1'],
        })
    })

    it('opens no panel for a path no registered panel names', () => {
        for (const open of [
            'gizmos/history',
            'gizmos/history/',
            'gizmos/historyx/1',
            'other/history/1',
            'gizmos/history/c//1',
            'gizmos/history/1?tab=x',
            'gizmos/history/1#top',
            '//gizmos/history/1',
            '',
        ]) {
            expect(recordPanelOf(open, PANELS), open).toBeNull()
        }
        expect(recordPanelOf('gizmos/history/1', [])).toBeNull()
    })
})

describe('rowOpensInPanel', () => {
    const wide = {
        open: 'gizmos/history/c/1',
        surface: 'panel' as const,
        hasPanel: true,
        panels: PANELS,
    }

    it('opens a registered path beside the report on a wide window of a host', () => {
        expect(rowOpensInPanel(wide)).toBe(true)
    })

    it('pushes on a page surface, an unregistered path, a view with no host or a row with no open', () => {
        expect(rowOpensInPanel({ ...wide, surface: 'page' })).toBe(false)
        expect(rowOpensInPanel({ ...wide, open: 'gizmos/sale/1' })).toBe(false)
        expect(rowOpensInPanel({ ...wide, hasPanel: false })).toBe(false)
        expect(rowOpensInPanel({ ...wide, open: undefined })).toBe(false)
    })
})

describe('recordPanelParams', () => {
    it('writes dr and clears every drill key (one side surface)', () => {
        // toStrictEqual: toEqual ignores undefined keys, so it would pass a clear that clears nothing.
        expect(
            recordPanelParams('gizmos/history/c/1', {
                d: 'r1',
                'd.p-period': 'thisMonth',
                'd.s': 'amount:asc',
            })
        ).toStrictEqual({
            d: undefined,
            'd.p-period': undefined,
            'd.s': undefined,
            dr: 'gizmos/history/c/1',
        })
    })

    it('replaces an open record and leaves the report keys alone', () => {
        expect(
            recordPanelParams('gizmos/history/c/2', {
                dr: 'gizmos/history/c/1',
                'p-period': 'lastMonth',
            })
        ).toStrictEqual({ dr: 'gizmos/history/c/2' })
    })
})

describe('isRowShown', () => {
    const open = 'gizmos/history/c/1'

    it('marks only the pressed row of many that open one record', () => {
        const search = { dr: open }
        expect(isRowShown(row('d:a', open), 'd:a', search)).toBe(true)
        expect(isRowShown(row('d:b', open), 'd:a', search)).toBe(false)
    })

    it('marks no row when the panel shows another record or no record', () => {
        expect(isRowShown(row('d:a', open), 'd:a', { dr: 'gizmos/history/c/2' })).toBe(false)
        expect(isRowShown(row('d:a', open), 'd:a', {})).toBe(false)
        expect(isRowShown(row('d:a', open), 'd:a', { dr: undefined })).toBe(false)
    })

    it('marks no row after a reload (no pressed row) and never a row with no open', () => {
        expect(isRowShown(row('d:a', open), null, { dr: open })).toBe(false)
        expect(isRowShown(row('d:a'), 'd:a', { dr: open })).toBe(false)
    })

    it('reads the first of a repeated dr', () => {
        expect(isRowShown(row('d:a', open), 'd:a', { dr: [open, 'x'] })).toBe(true)
    })
})
