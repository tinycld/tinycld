import { beforeEach, describe, expect, it } from 'vitest'
import { useReportUIStore } from '../report-ui-store'

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
