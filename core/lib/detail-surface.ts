export const DETAIL_PANEL_WIDTH = 480
// Below this window width a 480 px panel leaves the list too narrow, so the record is a page.
export const DETAIL_PANEL_MIN_WINDOW = 1360

export type DetailSurface = 'panel' | 'page'

export function detailSurfaceFor(windowWidth: number): DetailSurface {
    return windowWidth >= DETAIL_PANEL_MIN_WINDOW ? 'panel' : 'page'
}
