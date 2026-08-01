// @vitest-environment happy-dom
import { cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

// usePkgAccess existed for a full release with ZERO callers — the review's
// "unenforced dead code" finding — so nothing ever exercised its level
// resolution. Now that PackageAccessDenied gates the package content area on
// it, the resolution rules are load-bearing: guests default to 'none' (they
// get access only via an org_pkg_access row), members default to 'full'
// (rows only restrict them), owners/admins are always 'full'.

const h = vi.hoisted(() => ({
    scenario: 'owner' as
        | 'owner'
        | 'memberDefault'
        | 'memberNone'
        | 'memberReadonly'
        | 'guestDefault'
        | 'guestReadonly',
}))

vi.mock('@tinycld/core/lib/auth', () => ({
    useAuth: () => ({ user: { id: 'u1', email: 'me@example.com' } }),
}))

vi.mock('@tinycld/core/lib/pocketbase', async () => {
    const { createCollection, localOnlyCollectionOptions } = await import('@tanstack/db')
    const mk = (id: string, initialData: { id: string }[]) =>
        createCollection(
            localOnlyCollectionOptions({ id, getKey: (r: { id: string }) => r.id, initialData })
        )
    const me = (role: string) => [{ id: 'u1', role }]
    const grant = (access: string) => [{ id: 'a1', user: 'u1', pkg: 'mail', access }]
    const registry: Record<string, Record<string, unknown>> = {
        owner: { users: mk('u-owner', me('owner')), org_pkg_access: mk('a-owner', []) },
        memberDefault: {
            users: mk('u-member-d', me('member')),
            org_pkg_access: mk('a-member-d', []),
        },
        memberNone: {
            users: mk('u-member-n', me('member')),
            org_pkg_access: mk('a-member-n', grant('none')),
        },
        memberReadonly: {
            users: mk('u-member-r', me('member')),
            org_pkg_access: mk('a-member-r', grant('readonly')),
        },
        guestDefault: {
            users: mk('u-guest-d', me('guest')),
            org_pkg_access: mk('a-guest-d', []),
        },
        guestReadonly: {
            users: mk('u-guest-r', me('guest')),
            org_pkg_access: mk('a-guest-r', grant('readonly')),
        },
    }
    return {
        useStore: (...names: string[]) => names.map(n => registry[h.scenario][n]),
    }
})

import { usePkgAccessResult } from '../../lib/use-pkg-access'

afterEach(() => cleanup())

async function resolveAccess(scenario: typeof h.scenario, slug = 'mail') {
    h.scenario = scenario
    const { result } = renderHook(() => usePkgAccessResult(slug))
    await waitFor(() => expect(result.current.isReady).toBe(true))
    return result.current.access
}

describe('usePkgAccessResult', () => {
    it('owners and admins are always full', async () => {
        expect(await resolveAccess('owner')).toBe('full')
    })

    it('members default to full without an override row', async () => {
        expect(await resolveAccess('memberDefault')).toBe('full')
    })

    it("a member's 'none' override denies", async () => {
        expect(await resolveAccess('memberNone')).toBe('none')
    })

    it("a member's 'readonly' override is reported as readonly", async () => {
        expect(await resolveAccess('memberReadonly')).toBe('readonly')
    })

    it('guests default to none — access only via an explicit grant', async () => {
        expect(await resolveAccess('guestDefault')).toBe('none')
    })

    it("a guest's grant opens exactly the granted level", async () => {
        expect(await resolveAccess('guestReadonly')).toBe('readonly')
    })

    it('a grant for another package does not leak: the guest is still denied elsewhere', async () => {
        expect(await resolveAccess('guestReadonly', 'drive')).toBe('none')
    })
})
