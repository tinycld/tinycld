import { useSignInNoticeStore } from '@tinycld/core/lib/stores/sign-in-notice-store'
import { beforeEach, describe, expect, it } from 'vitest'

describe('useSignInNoticeStore', () => {
    beforeEach(() => {
        useSignInNoticeStore.setState({ notice: null })
    })

    it('starts with no notice', () => {
        expect(useSignInNoticeStore.getState().notice).toBeNull()
    })

    it('setNotice stores the text', () => {
        // A package (e.g. "widgets") sets this before routing an unauthenticated
        // user to sign-in, so the login form can explain why they landed there.
        useSignInNoticeStore.getState().setNotice('Your widgets invite link has expired.')
        expect(useSignInNoticeStore.getState().notice).toBe('Your widgets invite link has expired.')
    })

    it('clear resets the notice to null', () => {
        useSignInNoticeStore.getState().setNotice('Your widgets invite link has expired.')
        useSignInNoticeStore.getState().clear()
        expect(useSignInNoticeStore.getState().notice).toBeNull()
    })
})
