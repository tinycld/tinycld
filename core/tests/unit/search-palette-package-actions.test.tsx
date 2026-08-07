// @vitest-environment happy-dom
//
// Regression test for a Rules of Hooks violation: PackageActions must call
// `useSearchActions` unconditionally once the adapter module resolves, not
// through `adapter?.useSearchActions()`. That form calls zero hooks on the
// first render (while the dynamic import is pending, `useQuery` returns
// `data: undefined`) and one hook once it resolves — a different hook count
// between renders of the SAME component instance, which React detects and
// throws on ("Rendered more hooks than during the previous render").
//
// This is exercised here rather than through the full SearchPalette because
// SearchPalette.web.tsx also imports the icon map, which transitively pulls
// in the generated lucide-react-native deep imports that this Vite-based
// unit environment cannot resolve (see chip-text.ts extraction for the same
// constraint). Stubbing the icon map import lets PackageActions render on
// its own without touching that unrelated resolution problem.
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { useRef } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

vi.mock('@tinycld/core/components/workspace/package-icon-map', () => ({
    getIcon: () => () => null,
}))

// Simulates the async dynamic import: the adapter resolves only after
// `resolveAdapter()` is called from within a test, so the component is
// guaranteed to render at least once with the query still pending.
let resolveAdapter: (() => void) | undefined
// This mock MUST call a real hook. React only counts hooks it actually sees,
// so a plain `vi.fn(() => ({ onSelect }))` leaves the hook count unchanged
// between renders and the conditional-call bug goes undetected — verified:
// the test passed against the original buggy code until this used useRef.
// Real adapters call useRouter/useOrgHref/useStore, so this mirrors them.
const searchActions = vi.fn(() => {
    const onSelect = useRef(vi.fn()).current
    return { onSelect }
})
const adapterModule = {
    toRow: () => null,
    useSearchActions: searchActions,
}

vi.mock('@tinycld/core/lib/search/registry', () => ({
    loadSearchAdapter: () =>
        new Promise(resolve => {
            resolveAdapter = () => resolve(adapterModule)
        }),
    searchPackages: [],
}))

import { PackageActions } from '@tinycld/core/components/search-palette/SearchPalette.web'

function Providers({ children }: { children: ReactNode }) {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

afterEach(() => {
    cleanup()
    resolveAdapter = undefined
    searchActions.mockClear()
})

test('does not throw, and registers the handler, when the adapter resolves after the first render', async () => {
    const onReady = vi.fn()

    // First render: loadSearchAdapter's promise is still pending, so
    // useAdapterModule's useQuery returns `data: undefined` and
    // PackageActions renders null. If this were the version that calls
    // `adapter?.useSearchActions()` inline, this render already differs in
    // hook count from the one that follows adapter resolution.
    expect(() => {
        render(
            <Providers>
                <PackageActions slug="mail" onReady={onReady} />
            </Providers>
        )
    }).not.toThrow()

    expect(onReady).not.toHaveBeenCalled()

    // Resolve the adapter — triggers the re-render where the adapter becomes
    // available and ResolvedPackageActions mounts fresh, calling
    // useSearchActions for the first time in ITS lifetime (never a second
    // hook count for the outer PackageActions instance).
    resolveAdapter?.()

    await waitFor(() => {
        expect(onReady).toHaveBeenCalledWith('mail', expect.any(Function))
    })
    expect(searchActions).toHaveBeenCalledTimes(1)
})
