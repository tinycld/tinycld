// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { formatLastUsed } from '../../components/settings/ConnectedAppsSection'

describe('formatLastUsed', () => {
    it('reports never for an empty timestamp', () => {
        expect(formatLastUsed('')).toBe('Never used')
    })

    it('reports a relative time for a recent timestamp', () => {
        const oneHourAgo = new Date(Date.now() - 60 * 60 * 1000).toISOString()
        expect(formatLastUsed(oneHourAgo)).toContain('hour')
    })

    it('reports days for an older timestamp', () => {
        const threeDaysAgo = new Date(Date.now() - 3 * 24 * 60 * 60 * 1000).toISOString()
        expect(formatLastUsed(threeDaysAgo)).toContain('day')
    })
})

// Mock the data hook directly, same pattern as about-section.test.tsx — the
// point of these tests is how ConnectedAppsSection presents each query
// state, not how the query is fetched.
interface Grant {
    id: string
    device_label: string
    last_used_at: string
    status: string
}
const useOrgLiveQuery = vi.fn<
    () => { data: Grant[] | undefined; isError: boolean; isLoading: boolean }
>(() => ({ data: [], isError: false, isLoading: false }))
vi.mock('@tinycld/core/lib/use-org-live-query', () => ({
    useOrgLiveQuery: () => useOrgLiveQuery(),
}))

// ConnectedAppsSection only threads the collection through the (mocked)
// useOrgLiveQuery call — a placeholder is enough since the query itself
// never runs.
vi.mock('@tinycld/core/lib/pocketbase', () => ({
    useStore: () => [{}],
    pb: { send: vi.fn() },
}))

import { ConnectedAppsSection } from '../../components/settings/ConnectedAppsSection'

afterEach(() => {
    cleanup()
    useOrgLiveQuery.mockReset()
    useOrgLiveQuery.mockReturnValue({ data: [], isError: false, isLoading: false })
})

const ACTIVE_GRANT: Grant = {
    id: 'g1',
    device_label: "Nathan's laptop",
    last_used_at: '',
    status: 'active',
}

// useRevokeGrant's useMutation needs a real QueryClient in context even
// though no test here triggers a revoke — ConnectedAppsSection calls the
// hook unconditionally on every render.
function renderSection() {
    const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
    return render(
        <QueryClientProvider client={client}>
            <ConnectedAppsSection />
        </QueryClientProvider>
    )
}

describe('ConnectedAppsSection — query states', () => {
    // Regression test for the finding this guards: a FAILED query used to
    // render identically to "no connected apps" (both hit the `if (active
    // .length === 0) return null` branch), hiding a real sync/query error
    // behind what looks like an empty, healthy state.
    it('surfaces a distinct error message when the query fails', () => {
        useOrgLiveQuery.mockReturnValue({ data: undefined, isError: true, isLoading: false })
        const { getByText, queryByText } = renderSection()
        expect(getByText(/couldn't load your connected apps/i)).toBeTruthy()
        // Must not also render as if the list were merely empty.
        expect(queryByText('Devices and integrations')).toBeNull()
    })

    it('renders nothing while still loading, not the error state', () => {
        useOrgLiveQuery.mockReturnValue({ data: undefined, isError: false, isLoading: true })
        const { queryByText } = renderSection()
        expect(queryByText(/couldn't load/i)).toBeNull()
        expect(queryByText('Connected apps')).toBeNull()
    })

    it('renders nothing when the query succeeds with no active grants', () => {
        useOrgLiveQuery.mockReturnValue({ data: [], isError: false, isLoading: false })
        const { queryByText } = renderSection()
        expect(queryByText(/couldn't load/i)).toBeNull()
        expect(queryByText('Connected apps')).toBeNull()
    })

    it('renders the grant list when the query succeeds with active grants', () => {
        useOrgLiveQuery.mockReturnValue({ data: [ACTIVE_GRANT], isError: false, isLoading: false })
        const { getByText } = renderSection()
        expect(getByText('Connected apps')).toBeTruthy()
        expect(getByText("Nathan's laptop")).toBeTruthy()
    })
})
