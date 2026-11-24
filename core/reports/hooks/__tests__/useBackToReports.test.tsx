// @vitest-environment happy-dom
import { createCollection, localOnlyCollectionOptions } from '@tanstack/db'
import { cleanup, renderHook, waitFor } from '@testing-library/react'
import { router } from 'expo-router'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ReportBaseProvider, useReportBase } from '../../lib/report-base'
import { useBackToReports, useReportBack } from '../useBackToReports'

// A report view drawn on a host's own route goes back to the host's list, not to
// core's hub. The host here is fictional: core names no package.

vi.mock('@tinycld/core/lib/auth', () => ({
    useAuth: () => ({ user: { id: 'u1' }, isLoggedIn: true, isInitializing: false }),
}))
vi.mock('@tinycld/core/lib/pocketbase', () => ({
    useStore: (...names: (keyof typeof stores)[]) => names.map(name => stores[name]),
}))

const SOURCE_ID = 'zoosource000001'

function localCollection<T extends { id: string }>(name: string, initialData: T[]) {
    return createCollection(
        localOnlyCollectionOptions({
            id: `${name}-${Math.random()}`,
            getKey: (row: T) => row.id,
            initialData,
        })
    )
}

// The source report is the reader's own, so useReportRecord reads it as ready.
const stores = {
    reports: localCollection('reports', [
        {
            id: SOURCE_ID,
            title: 'Animals by Pen',
            owner: 'u1',
            published: false,
            shipped_key: '',
            spec: null,
            broken: null,
        },
    ]),
    report_shares: localCollection<{ id: string; report: string; user: string; role: string }>(
        'report_shares',
        []
    ),
    users: localCollection('users', [{ id: 'u1', role: 'member' }]),
}

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

describe('useReportBack', () => {
    it("opens the source report on the host's route when there is no history", async () => {
        vi.mocked(router.canGoBack).mockReturnValue(false)
        const { result } = renderHook(() => useReportBack(SOURCE_ID), { wrapper: zooBase })
        await waitFor(() => expect(result.current.label).toBe('Animals by Pen'))
        result.current.onBack()
        expect(router.replace).toHaveBeenCalledWith(`/a/zoo/reports/${SOURCE_ID}`)
        expect(router.back).not.toHaveBeenCalled()
    })
})
