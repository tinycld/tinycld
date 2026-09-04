import { create } from '@tinycld/core/lib/store'

export type PickerSource = 'photoLibrary' | 'camera' | 'documents'

export interface PickerSheetRequest {
    sources: PickerSource[]
    /**
     * Called with the chosen source, or null on dismissal. Returns a promise
     * so the host can keep the sheet open until the underlying picker has
     * launched (matching the pre-hoist inline behavior).
     */
    onSelect: (source: PickerSource | null) => Promise<void>
}

interface PickerSheetState {
    request: PickerSheetRequest | null
    open: (request: PickerSheetRequest) => void
    close: () => void
}

/**
 * Drives the native source-chooser sheet (photo library / camera / documents)
 * that usePickFiles opens when more than one source is offered.
 *
 * The sheet itself renders in FilePickerSheetHost, mounted once at the layout
 * level. It used to be an element returned by usePickFiles and mounted inline
 * beside whichever button triggered it — but a BottomDrawer rests at the
 * bottom of its PARENT, so from inside an absolutely-positioned panel (e.g.
 * boards' peek, zIndex 20) it sat at the panel's bottom edge, trapped in that
 * stacking context, instead of on the tab bar.
 */
export const usePickerSheetStore = create<PickerSheetState>()(set => ({
    request: null,
    open: request =>
        set(prev => {
            // A second pick while one is showing replaces it; resolve the old
            // awaiter as dismissed so its promise doesn't hang forever.
            prev.request?.onSelect(null)
            return { request }
        }),
    close: () => set({ request: null }),
}))
