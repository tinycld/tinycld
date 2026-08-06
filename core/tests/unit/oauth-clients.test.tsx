// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { OAuthClient } from '../../lib/use-oauth-clients'

// Mock the data hooks directly, same pattern as connected-apps.test.tsx: these
// tests are about how the section presents each state and which mutation it
// fires, not about how the query is fetched.
const useOAuthClients = vi.fn<
    (enabled?: boolean) => {
        data: OAuthClient[] | undefined
        isError: boolean
        isLoading: boolean
    }
>(() => ({ data: [], isError: false, isLoading: false }))
const setDisabledMutate = vi.fn()

vi.mock('@tinycld/core/lib/use-oauth-clients', async importOriginal => {
    const actual = await importOriginal<typeof import('../../lib/use-oauth-clients')>()
    return {
        ...actual,
        useOAuthClients: (enabled?: boolean) => useOAuthClients(enabled),
        useSetClientDisabled: () => ({
            mutate: setDisabledMutate,
            isPending: false,
            variables: undefined,
        }),
    }
})

import {
    describeAccess,
    disableWarning,
    OAuthClientsSection,
    planToggle,
} from '../../components/settings/OAuthClientsSection'

const CLI: OAuthClient = {
    id: 'c1',
    client_id: 'tinycld-cli',
    name: 'TinyCld CLI',
    type: 'public',
    scopes: 'profile mail:read',
    is_first_party: true,
    disabled: false,
    active_grants: 2,
}

afterEach(() => {
    cleanup()
    useOAuthClients.mockReset()
    useOAuthClients.mockReturnValue({ data: [], isError: false, isLoading: false })
    setDisabledMutate.mockReset()
})

function renderSection(isVisible = true) {
    const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
    return render(
        <QueryClientProvider client={client}>
            <OAuthClientsSection isVisible={isVisible} />
        </QueryClientProvider>
    )
}

describe('describeAccess', () => {
    it('counts scopes and reports nothing connected when there are no grants', () => {
        expect(describeAccess({ ...CLI, scopes: 'profile', active_grants: 0 })).toBe(
            '1 scope · nothing connected'
        )
    })

    it('pluralizes scopes and connections', () => {
        expect(describeAccess(CLI)).toBe('2 scopes · 2 connections')
    })

    it('handles an empty scope string without counting a phantom scope', () => {
        // ''.split(/\s+/) yields [''] — length 1 — so an unguarded count would
        // claim a client with no scopes has one.
        expect(describeAccess({ ...CLI, scopes: '', active_grants: 1 })).toBe(
            '0 scopes · 1 connection'
        )
    })
})

describe('disableWarning', () => {
    it('names the blast radius when connections exist', () => {
        expect(disableWarning(CLI)).toContain('2 active connections')
        expect(disableWarning(CLI)).toContain('turn it back on')
    })

    it('does not claim connections will drop when there are none', () => {
        const warning = disableWarning({ ...CLI, active_grants: 0 })
        expect(warning).not.toContain('disconnecting')
        expect(warning).toContain('turn it back on')
    })

    it('falls back to the client_id when the client has no name', () => {
        expect(disableWarning({ ...CLI, name: '' })).toContain('tinycld-cli')
    })
})

describe('OAuthClientsSection — query states', () => {
    // A failed load must not read as a healthy empty registry: that would hide
    // a client that IS registered and possibly compromised.
    it('surfaces a distinct error message when the query fails', () => {
        useOAuthClients.mockReturnValue({ data: undefined, isError: true, isLoading: false })
        const { getByText, queryByText } = renderSection()
        expect(getByText(/couldn't load oauth clients/i)).toBeTruthy()
        expect(queryByText(/no oauth clients are registered/i)).toBeNull()
    })

    it('distinguishes a genuinely empty registry from a failure', () => {
        useOAuthClients.mockReturnValue({ data: [], isError: false, isLoading: false })
        const { getByText, queryByText } = renderSection()
        expect(getByText(/no oauth clients are registered/i)).toBeTruthy()
        expect(queryByText(/couldn't load/i)).toBeNull()
    })

    it('renders nothing at all when the viewer is not an admin', () => {
        // A disabled query reports isLoading forever, so the isVisible check
        // must short-circuit BEFORE the loading branch or a non-admin sees a
        // spinner that never resolves.
        useOAuthClients.mockReturnValue({ data: undefined, isError: false, isLoading: true })
        const { container } = renderSection(false)
        expect(container.textContent).toBe('')
    })

    it('renders the client list with its metadata', () => {
        useOAuthClients.mockReturnValue({ data: [CLI], isError: false, isLoading: false })
        const { getByText } = renderSection()
        expect(getByText('TinyCld CLI')).toBeTruthy()
        expect(getByText(/tinycld-cli · 2 scopes · 2 connections/)).toBeTruthy()
        expect(getByText('first-party')).toBeTruthy()
    })

    it('marks a disabled client as off', () => {
        useOAuthClients.mockReturnValue({
            data: [{ ...CLI, disabled: true }],
            isError: false,
            isLoading: false,
        })
        const { getByText } = renderSection()
        expect(getByText('off')).toBeTruthy()
    })
})

// The switch itself cannot be driven from these tests: react-native's
// Pressable is stubbed as a plain string tag (tests/react-native-stub.cjs), so
// onPress lands as a DOM attribute rather than a handler and no synthetic
// click reaches it. planToggle holds the decision the switch delegates to, so
// the asymmetry is asserted directly instead of through a fake interaction.
describe('planToggle — the kill switch asymmetry', () => {
    it('routes a disable through confirmation rather than mutating', () => {
        expect(planToggle(CLI, true)).toEqual({ action: 'confirm' })
    })

    it('re-enables immediately, with no confirmation step', () => {
        expect(planToggle({ ...CLI, disabled: true }, false)).toEqual({
            action: 'mutate',
            id: 'c1',
            disabled: false,
        })
    })
})

describe('OAuthClientsSection — the confirmation dialog', () => {
    it('does not render a confirmation until one is pending', () => {
        useOAuthClients.mockReturnValue({ data: [CLI], isError: false, isLoading: false })
        const { queryByText } = renderSection()
        expect(queryByText(/turn off this client\?/i)).toBeNull()
    })

    it('renders a switch for each client, labelled by name', () => {
        useOAuthClients.mockReturnValue({ data: [CLI], isError: false, isLoading: false })
        const { container } = renderSection()
        expect(container.querySelector('[accessibilitylabel="TinyCld CLI enabled"]')).toBeTruthy()
    })
})
