import { beforeEach, describe, expect, it } from 'vitest'
import { recordTitleOf, useReportUIStore } from '../report-ui-store'

const state = () => useReportUIStore.getState()

describe('the report UI store per view', () => {
    beforeEach(() => state().startVisit())

    it('opens a surface in the drill view and leaves the main view alone', () => {
        state().openSurface({ kind: 'param', paramId: 'period' }, 'drill')
        state().openCustomize('drill')
        expect(state().scopes.drill).toEqual({
            surface: { kind: 'param', paramId: 'period' },
            isCustomizeOpen: true,
        })
        expect(state().scopes.main).toEqual({ surface: null, isCustomizeOpen: false })
    })

    it('opens a surface in the main view and leaves the drill view alone', () => {
        state().openSurface({ kind: 'sort' })
        state().openCustomize()
        expect(state().scopes.main).toEqual({ surface: { kind: 'sort' }, isCustomizeOpen: true })
        expect(state().scopes.drill).toEqual({ surface: null, isCustomizeOpen: false })
    })

    it('closes a surface of a kind only in its own view', () => {
        state().openSurface({ kind: 'sort' })
        state().openSurface({ kind: 'sort' }, 'drill')
        state().closeSurfaceOf('sort', 'drill')
        expect(state().scopes.main.surface).toEqual({ kind: 'sort' })
        expect(state().scopes.drill.surface).toBeNull()
    })

    it('counts drill opens and keeps the count across a visit start', () => {
        const before = state().drillOpens
        state().noteDrillOpen()
        state().noteDrillOpen()
        state().startVisit()
        expect(state().drillOpens).toBe(before + 2)
    })

    it('marks a drill open as focused only once, until the next press', () => {
        state().noteDrillOpen()
        expect(state().drillOpens).toBeGreaterThan(state().drillFocused)
        state().noteDrillFocused()
        expect(state().drillFocused).toBe(state().drillOpens)
        state().noteDrillOpen()
        expect(state().drillOpens).toBe(state().drillFocused + 1)
    })

    it('keeps the figure that opened the drill panel until the next visit', () => {
        state().setDrillOrigin('amount|g:east')
        expect(state().drillOrigin).toBe('amount|g:east')
        state().startVisit()
        expect(state().drillOrigin).toBeNull()
    })

    // A fictional record path: core names no real package.
    const OPEN = 'gizmos/history/c/1'

    it('opens a record: marks its row and counts the press', () => {
        const before = state().recordOpens
        state().openRecord('d:row1')
        expect(state().openedRow).toBe('d:row1')
        expect(state().recordOpens).toBe(before + 1)
    })

    it("closes the report's Customize when a record opens, and leaves the drill view's", () => {
        state().openCustomize()
        state().openCustomize('drill')
        state().openRecord('d:row1')
        expect(state().scopes.main.isCustomizeOpen).toBe(false)
        expect(state().scopes.drill.isCustomizeOpen).toBe(true)
    })

    it("keeps a record's newest title", () => {
        state().openRecord('d:row1')
        state().setRecordTitle(OPEN, 'Gizmo 1001')
        state().setRecordTitle(OPEN, 'Gizmo 1001 (paid)')
        expect(recordTitleOf(state().recordTitle, OPEN)).toBe('Gizmo 1001 (paid)')
    })

    it('shows no title for another record until its panel names it', () => {
        state().setRecordTitle(OPEN, 'Gizmo 1001')
        expect(recordTitleOf(state().recordTitle, 'gizmos/history/c/2')).toBe('')
        expect(recordTitleOf(null, OPEN)).toBe('')
    })

    // Many rows open one record. A press on another of them leaves dr and the
    // panel's component as they are (no remount, so usePanelTitle does not run
    // again): the title stays and the mark moves (C6).
    it('keeps the title and moves the mark when another row opens the same record', () => {
        state().openRecord('d:a')
        state().setRecordTitle(OPEN, 'Gizmo 1001')
        state().openRecord('d:b')
        expect(state().openedRow).toBe('d:b')
        expect(recordTitleOf(state().recordTitle, OPEN)).toBe('Gizmo 1001')
        // A second press on the opened row keeps it too.
        state().openRecord('d:b')
        expect(recordTitleOf(state().recordTitle, OPEN)).toBe('Gizmo 1001')
    })

    it('marks a record open as focused only once, until the next press', () => {
        state().openRecord('d:row1')
        expect(state().recordOpens).toBeGreaterThan(state().recordFocused)
        state().noteRecordFocused()
        expect(state().recordFocused).toBe(state().recordOpens)
        state().openRecord('d:row2')
        expect(state().recordOpens).toBe(state().recordFocused + 1)
    })

    it('starts a visit with no record opened', () => {
        state().openRecord('d:row1')
        state().setRecordTitle(OPEN, 'Gizmo 1001')
        state().noteRecordFocused()
        state().startVisit()
        expect(state().openedRow).toBeNull()
        expect(state().recordTitle).toBeNull()
        expect(state().recordOpens).toBe(0)
        expect(state().recordFocused).toBe(0)
    })

    it('opens the email dialog for one report and closes it, leaving the share dialog alone', () => {
        state().openShare('rpt000000000002')
        state().openEmail('rpt000000000001')
        expect(state().emailReportId).toBe('rpt000000000001')
        expect(state().shareReportId).toBe('rpt000000000002')
        state().closeEmail()
        expect(state().emailReportId).toBeNull()
        expect(state().shareReportId).toBe('rpt000000000002')
        state().closeShare()
    })

    it('starts a visit with both views closed', () => {
        state().openCustomize()
        state().openSurface({ kind: 'add-filter' }, 'drill')
        state().startVisit()
        expect(state().scopes).toEqual({
            main: { surface: null, isCustomizeOpen: false },
            drill: { surface: null, isCustomizeOpen: false },
        })
    })
})
