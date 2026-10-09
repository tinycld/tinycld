import { create } from '@tinycld/core/lib/store'

/** Whether the connection options sheet (opened from the connection notice) is open. */
interface ConnectionOptionsState {
    isOpen: boolean
    open: () => void
    close: () => void
}

export const useConnectionOptionsStore = create<ConnectionOptionsState>()(set => ({
    isOpen: false,
    open: () => set({ isOpen: true }),
    close: () => set({ isOpen: false }),
}))
