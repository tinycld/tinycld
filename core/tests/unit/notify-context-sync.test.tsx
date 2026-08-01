// @vitest-environment happy-dom
import { beforeEach, describe, expect, it, vi } from 'vitest'

// The whole point of this file is to drive the REAL mount path. bell.test.ts
// hand-sets the notify context with a fabricated orgId, so it passes whether or
// not anything ever sets that context in the running app — and nothing did:
// NotifyContextSync gated on an orgId that useOrgInfo() has returned as '' ever
// since the single-org migration, so the context was never set, every bell
// dispatch no-opped, and each one fired captureException('notify.bell.no_context').
// Takeout completion and failure notifications silently went nowhere.
//
// So: mock the hooks the component actually consumes, mount it, and assert on
// the context it produces.

const useAuth = vi.hoisted(() => vi.fn())
const useOrgInfo = vi.hoisted(() => vi.fn())

vi.mock('@tinycld/core/lib/auth', () => ({ useAuth }))
vi.mock('@tinycld/core/lib/use-org-info', () => ({ useOrgInfo }))

import { render } from '@testing-library/react'
import { NotifyContextSync } from '@tinycld/core/components/NotifyContextSync'
import { clearNotifyContext, getNotifyContext } from '@tinycld/core/lib/notify/context'

describe('NotifyContextSync', () => {
    beforeEach(() => {
        clearNotifyContext()
        vi.clearAllMocks()
        // What the shipped useOrgInfo() actually returns post-migration.
        useOrgInfo.mockReturnValue({ orgSlug: '', orgId: '', org: null })
    })

    it('publishes the notify context for a signed-in user', () => {
        useAuth.mockReturnValue({ isLoggedIn: true, user: { id: 'u1' } })

        render(<NotifyContextSync />)

        const ctx = getNotifyContext()
        expect(ctx).not.toBeNull()
        expect(ctx?.userId).toBe('u1')
    })

    it('publishes nothing while signed out', () => {
        useAuth.mockReturnValue({ isLoggedIn: false, user: null })

        render(<NotifyContextSync />)

        expect(getNotifyContext()).toBeNull()
    })

    it('clears the context on unmount, so a sign-out does not leave it stale', () => {
        useAuth.mockReturnValue({ isLoggedIn: true, user: { id: 'u1' } })

        const view = render(<NotifyContextSync />)
        expect(getNotifyContext()).not.toBeNull()

        view.unmount()
        expect(getNotifyContext()).toBeNull()
    })
})
