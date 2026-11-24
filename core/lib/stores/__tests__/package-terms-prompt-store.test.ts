import { usePackageTermsPromptStore } from '@tinycld/core/lib/stores/package-terms-prompt-store'
import { beforeEach, describe, expect, it } from 'vitest'

describe('usePackageTermsPromptStore', () => {
    beforeEach(() => {
        usePackageTermsPromptStore.setState({ hasPrompted: false })
    })

    it('has not prompted when the app loads', () => {
        expect(usePackageTermsPromptStore.getState().hasPrompted).toBe(false)
    })

    it('stays prompted once marked, with one state change only', () => {
        const states: boolean[] = []
        const stop = usePackageTermsPromptStore.subscribe(s => states.push(s.hasPrompted))
        usePackageTermsPromptStore.getState().markPrompted()
        usePackageTermsPromptStore.getState().markPrompted()
        stop()
        expect(usePackageTermsPromptStore.getState().hasPrompted).toBe(true)
        expect(states).toEqual([true])
    })
})
