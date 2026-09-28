// @vitest-environment happy-dom
import { cleanup, fireEvent, render } from '@testing-library/react'
import { StrictMode } from 'react'
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

import { LoginModal } from '@tinycld/core/components/workspace/LoginModal'
import { useSignInNoticeStore } from '@tinycld/core/lib/stores/sign-in-notice-store'

// The RN test stub passes testID straight through as a literal lowercase
// `testid` attribute (not RTL's default `data-testid`), so queries here go
// through the DOM directly rather than getByTestId — same convention as
// toast-placement.test.tsx.
function byTestId(container: HTMLElement, id: string): HTMLElement | null {
    return container.querySelector(`[testid="${id}"]`)
}

function typeInto(input: HTMLElement | null, value: string) {
    if (!input) throw new Error('input not rendered')
    fireEvent.input(input, { target: { value } })
}

afterEach(() => {
    cleanup()
    login.mockReset()
    useSignInNoticeStore.setState({ notice: null, shownBy: null })
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

        const { container } = render(<LoginModal />)
        expect(byTestId(container, 'sign-in-notice')).toBeTruthy()

        typeInto(byTestId(container, 'identifier'), 'alice@example.com')
        typeInto(byTestId(container, 'login-password'), 'secret')
        fireEvent.click(byTestId(container, 'login-submit') as HTMLElement)

        expect(login).toHaveBeenCalledWith('alice@example.com', 'secret')
        await vi.waitFor(() => {
            expect(useSignInNoticeStore.getState().notice).toBeNull()
        })
    })

    it('keeps the notice through StrictMode double effects', () => {
        useSignInNoticeStore.getState().setNotice('Your widgets invite link has expired.')
        const { container } = render(
            <StrictMode>
                <LoginModal />
            </StrictMode>
        )
        expect(byTestId(container, 'sign-in-notice')).toBeTruthy()
        expect(useSignInNoticeStore.getState().notice).toBe('Your widgets invite link has expired.')
    })

    it('does not show the notice again after the user left sign-in without signing in', () => {
        useSignInNoticeStore.getState().setNotice('Your widgets invite link has expired.')
        const first = render(<LoginModal />)
        expect(byTestId(first.container, 'sign-in-notice')).toBeTruthy()
        first.unmount()

        const { container } = render(<LoginModal />)
        expect(byTestId(container, 'sign-in-notice')).toBeNull()
        expect(useSignInNoticeStore.getState().notice).toBeNull()
    })
})

describe('LoginModal — sign-in error', () => {
    it('shows no error before a sign-in attempt', () => {
        const { container } = render(<LoginModal />)
        expect(byTestId(container, 'sign-in-error')).toBeNull()
    })

    it('shows the error a failed sign-in returns', async () => {
        login.mockResolvedValue({ user: null, error: 'Wrong password.' })
        const { container } = render(<LoginModal />)
        typeInto(byTestId(container, 'identifier'), 'alice@example.com')
        typeInto(byTestId(container, 'login-password'), 'wrong')
        fireEvent.click(byTestId(container, 'login-submit') as HTMLElement)

        await vi.waitFor(() => {
            expect(byTestId(container, 'sign-in-error')?.textContent).toBe('Wrong password.')
        })
    })
})
