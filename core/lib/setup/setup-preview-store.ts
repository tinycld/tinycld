import { create } from '@tinycld/core/lib/store'

interface SetupPreviewState {
    /** The workspace name while it is typed, before it is saved. */
    draftName: string | null
    setDraftName: (name: string | null) => void
}

export const useSetupPreviewStore = create<SetupPreviewState>()(set => ({
    draftName: null,
    setDraftName: draftName => set({ draftName }),
}))
