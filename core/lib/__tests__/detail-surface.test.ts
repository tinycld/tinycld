import { describe, expect, it } from 'vitest'
import { DETAIL_PANEL_MIN_WINDOW, DETAIL_PANEL_WIDTH, detailSurfaceFor } from '../detail-surface'

describe('detailSurfaceFor', () => {
    it('shows a record beside the list from 1360 px, as a page below', () => {
        expect(detailSurfaceFor(1359)).toBe('page')
        expect(detailSurfaceFor(1360)).toBe('panel')
        expect([DETAIL_PANEL_MIN_WINDOW, DETAIL_PANEL_WIDTH]).toEqual([1360, 480])
    })
})
