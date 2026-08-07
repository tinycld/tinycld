import { create } from '@tinycld/core/lib/store'

interface SearchPaletteState {
    isOpen: boolean
    text: string
    /**
     * The selected row's id rather than its index: the flat list re-sorts as
     * slower packages resolve, and an index would leave the cursor pointing at
     * whatever row happened to land in that slot.
     */
    selectedRowId: string | null
    /** Open, seeding the active package as a chip so the common case is free. */
    open: (seedSlug: string | null) => void
    close: () => void
    setText: (value: string) => void
    setSelectedRowId: (id: string | null) => void
}

// Not persisted: a restored palette would open a dialog the user did not ask
// for, and a restored query would run against data that has since changed.
export const useSearchPaletteStore = create<SearchPaletteState>()(set => ({
    isOpen: false,
    text: '',
    selectedRowId: null,
    open: seedSlug => set({ isOpen: true, text: seedSlug ? `${seedSlug}: ` : '', selectedRowId: null }),
    close: () => set({ isOpen: false, text: '', selectedRowId: null }),
    // A new query invalidates the old selection — the row may no longer exist.
    setText: value => set({ text: value, selectedRowId: null }),
    setSelectedRowId: id => set({ selectedRowId: id }),
}))
