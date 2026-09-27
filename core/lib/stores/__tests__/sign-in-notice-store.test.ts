import { useSignInNoticeStore } from '@tinycld/core/lib/stores/sign-in-notice-store'
import { beforeEach, describe, expect, it } from 'vitest'

describe('useSignInNoticeStore', () => {
    beforeEach(() => {
        useSignInNoticeStore.setState({ notice: null, shownBy: null })
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

    it('keeps the notice while the same sign-in screen claims it again', () => {
        const screen = {}
        useSignInNoticeStore.getState().setNotice('Your widgets invite link has expired.')
        useSignInNoticeStore.getState().claim(screen)
        useSignInNoticeStore.getState().claim(screen)
        expect(useSignInNoticeStore.getState().notice).toBe('Your widgets invite link has expired.')
    })

    it('clears a notice that an earlier sign-in screen already showed', () => {
        useSignInNoticeStore.getState().setNotice('Your widgets invite link has expired.')
        useSignInNoticeStore.getState().claim({})
        useSignInNoticeStore.getState().claim({})
        expect(useSignInNoticeStore.getState().notice).toBeNull()
    })

    it('a new notice is shown again after an earlier one was consumed', () => {
        useSignInNoticeStore.getState().setNotice('first')
        useSignInNoticeStore.getState().claim({})
        useSignInNoticeStore.getState().setNotice('second')
        const screen = {}
        useSignInNoticeStore.getState().claim(screen)
        expect(useSignInNoticeStore.getState().notice).toBe('second')
        expect(useSignInNoticeStore.getState().shownBy).toBe(screen)
    })
})
