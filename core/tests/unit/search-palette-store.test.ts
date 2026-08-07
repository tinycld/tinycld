import { useSearchPaletteStore } from '@tinycld/core/lib/search/search-palette-store'
import { beforeEach, describe, expect, it } from 'vitest'

describe('useSearchPaletteStore', () => {
    beforeEach(() => {
        useSearchPaletteStore.getState().close()
    })

    it('opens seeded with the current package as a chip', () => {
        useSearchPaletteStore.getState().open('mail')
        const state = useSearchPaletteStore.getState()
        expect(state.isOpen).toBe(true)
        expect(state.text).toBe('mail: ')
    })

    it('opens with empty text when no package is active', () => {
        useSearchPaletteStore.getState().open(null)
        expect(useSearchPaletteStore.getState().text).toBe('')
    })

    it('resets text and selection on close', () => {
        const store = useSearchPaletteStore.getState()
        store.open('mail')
        store.setText('mail: budget')
        store.setSelectedRowId('m1')
        store.close()
        const state = useSearchPaletteStore.getState()
        expect(state.isOpen).toBe(false)
        expect(state.text).toBe('')
        expect(state.selectedRowId).toBeNull()
    })

    it('clears the selection when the text changes', () => {
        const store = useSearchPaletteStore.getState()
        store.open(null)
        store.setSelectedRowId('m1')
        store.setText('budget')
        expect(useSearchPaletteStore.getState().selectedRowId).toBeNull()
    })

    it('keeps the selection when set explicitly', () => {
        const store = useSearchPaletteStore.getState()
        store.open(null)
        store.setText('budget')
        store.setSelectedRowId('d1')
        expect(useSearchPaletteStore.getState().selectedRowId).toBe('d1')
    })
})
