import { type DetailSurface, detailSurfaceFor } from './detail-surface'
import { useWindowSizeStore } from './stores/window-size-store'

// A primitive selector, so a reader re-renders only when the surface flips.
export function useDetailSurface(): DetailSurface {
    return useWindowSizeStore(s => detailSurfaceFor(s.width))
}
