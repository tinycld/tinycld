// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, renderHook, waitFor } from '@testing-library/react'
import { router } from 'expo-router'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ReportBaseProvider } from '../../lib/report-base'
import { type ActionReport, useHubActions } from '../useHubActions'

vi.mock('@tinycld/core/lib/auth', () => ({ useAuth: () => ({ user: { id: 'u1' } }) }))
vi.mock('@tinycld/core/lib/pocketbase', () => ({
    useStore: () => [{ insert: () => Promise.resolve() }],
}))

// A copy opens where the report it was made from lives: a host's own route, or core's
// when no host draws the view. The host here is fictional: core names no package.

const report = {
    id: 'r1',
    title: 'Sales',
    description: '',
    spec: {},
} as unknown as ActionReport

function zooBase({ children }: { children: ReactNode }) {
    return (
        <QueryClientProvider client={new QueryClient()}>
            <ReportBaseProvider base="zoo/reports">{children}</ReportBaseProvider>
        </QueryClientProvider>
    )
}

function coreBase({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={new QueryClient()}>{children}</QueryClientProvider>
}

function openedHref() {
    const [href] = vi.mocked(router.push).mock.calls[0]
    const { pathname } = href as { pathname: string }
    return pathname
}

beforeEach(() => vi.mocked(router.push).mockClear())
afterEach(cleanup)

describe('useHubActions saveCopy', () => {
    it("opens the copy on the host's route", async () => {
        const { result } = renderHook(() => useHubActions(), { wrapper: zooBase })
        result.current.saveCopy(report, 'Sales 2')
        await waitFor(() => expect(router.push).toHaveBeenCalledTimes(1))
        expect(openedHref()).toBe('/a/zoo/reports/[id]')
    })

    it("opens the copy on core's route with no host", async () => {
        const { result } = renderHook(() => useHubActions(), { wrapper: coreBase })
        result.current.saveCopy(report, 'Sales 2')
        await waitFor(() => expect(router.push).toHaveBeenCalledTimes(1))
        expect(openedHref()).toBe('/a/reports/[id]')
    })
})
