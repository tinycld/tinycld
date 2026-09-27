// @vitest-environment happy-dom
import { cleanup, fireEvent, render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

// LoginModal only needs login() + the current user from useAuth — mock it
// directly so the test never needs a real pbtsdb/PocketBase stack.
const login = vi.fn()
vi.mock('@tinycld/core/lib/auth', () => ({
    useAuth: () => ({
        login,
        logout: vi.fn(),
        user: null,
        isLoggedIn: false,
        isInitializing: false,
    }),
}))

// Pre-auth branding lookup — irrelevant to the notice behavior under test.
vi.mock('@tinycld/core/lib/use-org-info', () => ({ useOrgInfo: () => ({ org: null }) }))

// The RN TextInput test stub doesn't wire onChangeText to DOM change events
// (react-native-stub.cjs passes it through as an inert prop), so there's no
// way to type into the identifier/password fields directly. Force review-mode
// hints on and use its real "fill demo credentials" button instead — it calls
// the same setIdentifier/setPassword the fields do, driving the form through
// production wiring rather than reaching into component internals.
vi.mock('@tinycld/core/lib/build-mode', () => ({ isReviewBuild: () => true }))
vi.mock('@tinycld/core/lib/core-config', async importOriginal => ({
    ...(await importOriginal<typeof import('@tinycld/core/lib/core-config')>()),
    getCoreConfigOptional: () => ({ demoEmail: 'alice@example.com', demoPassword: 'secret' }),
}))

import { LoginModal } from '@tinycld/core/components/workspace/LoginModal'
import { useSignInNoticeStore } from '@tinycld/core/lib/stores/sign-in-notice-store'

// The RN test stub passes testID straight through as a literal lowercase
// `testid` attribute (not RTL's default `data-testid`), so queries here go
// through the DOM directly rather than getByTestId — same convention as
// toast-placement.test.tsx.
function byTestId(container: HTMLElement, id: string): HTMLElement | null {
    return container.querySelector(`[testid="${id}"]`)
}

afterEach(() => {
    cleanup()
    login.mockReset()
    useSignInNoticeStore.setState({ notice: null })
})

describe('LoginModal — sign-in notice', () => {
    it('renders a notice a package set before routing here', () => {
        useSignInNoticeStore.getState().setNotice('Your widgets invite link has expired.')
        const { container } = render(<LoginModal />)
        expect(byTestId(container, 'sign-in-notice')?.textContent).toBe(
            'Your widgets invite link has expired.'
        )
    })

    it('renders nothing when no notice is set', () => {
        const { container } = render(<LoginModal />)
        expect(byTestId(container, 'sign-in-notice')).toBeNull()
    })

    it('clears the notice after a successful sign-in', async () => {
        useSignInNoticeStore.getState().setNotice('Your widgets invite link has expired.')
        login.mockResolvedValue({ user: { id: 'u1', email: 'a@example.com' }, error: null })

        const { container, getByText } = render(<LoginModal />)
        expect(byTestId(container, 'sign-in-notice')).toBeTruthy()

        // Fills identifier + password via the review-mode "fill demo
        // credentials" button (see the mocks above) — the TextInput stub
        // doesn't wire onChangeText to DOM change events, so this is the
        // only way to reach a non-empty, submittable form.
        fireEvent.click(getByText('Fill demo credentials'))
        fireEvent.click(byTestId(container, 'login-submit') as HTMLElement)

        await vi.waitFor(() => {
            expect(useSignInNoticeStore.getState().notice).toBeNull()
        })
    })
})
