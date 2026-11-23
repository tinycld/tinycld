// @vitest-environment happy-dom
import { cleanup, renderHook } from '@testing-library/react'
import { router } from 'expo-router'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ReportBaseProvider, useReportBase } from '../../lib/report-base'
import { useBackToReports } from '../useBackToReports'

// A report view drawn on a host's own route goes back to the host's list, not to
// core's hub. The host here is fictional: core names no package.

function zooBase({ children }: { children: ReactNode }) {
    return <ReportBaseProvider base="zoo/reports">{children}</ReportBaseProvider>
}

beforeEach(() => {
    vi.mocked(router.replace).mockClear()
    vi.mocked(router.back).mockClear()
})
afterEach(cleanup)

describe('useReportBase', () => {
    it("is core's own route with no host", () => {
        expect(renderHook(() => useReportBase()).result.current).toBe('reports')
    })

    it('is the base of the host inside its provider', () => {
        const { result } = renderHook(() => useReportBase(), { wrapper: zooBase })
        expect(result.current).toBe('zoo/reports')
    })
})

describe('useBackToReports', () => {
    it('opens the hub when there is no history and no host', () => {
        vi.mocked(router.canGoBack).mockReturnValue(false)
        renderHook(() => useBackToReports()).result.current()
        expect(router.replace).toHaveBeenCalledWith('/a/reports')
    })

    it("opens the host's list when there is no history", () => {
        vi.mocked(router.canGoBack).mockReturnValue(false)
        renderHook(() => useBackToReports(), { wrapper: zooBase }).result.current()
        expect(router.replace).toHaveBeenCalledWith('/a/zoo/reports')
    })

    it('goes back in the history when there is one', () => {
        vi.mocked(router.canGoBack).mockReturnValue(true)
        renderHook(() => useBackToReports(), { wrapper: zooBase }).result.current()
        expect(router.back).toHaveBeenCalledTimes(1)
        expect(router.replace).not.toHaveBeenCalled()
    })
})
