import { closeHelp, openHelp, openHelpPackage } from '@tinycld/core/lib/help/open-help'
import { useHelpStore } from '@tinycld/core/lib/help/store'
import { beforeEach, describe, expect, it } from 'vitest'

describe('useHelpStore', () => {
    beforeEach(() => {
        useHelpStore.setState({
            isOpen: false,
            mode: 'topic',
            topicId: null,
            pkgSlug: null,
            cameFrom: null,
        })
    })

    it('starts closed', () => {
        const state = useHelpStore.getState()
        expect(state.isOpen).toBe(false)
        expect(state.topicId).toBeNull()
        expect(state.pkgSlug).toBeNull()
        expect(state.cameFrom).toBeNull()
    })

    it('open() sets isOpen + topicId and leaves cameFrom null (no back arrow)', () => {
        openHelp('core:themes')
        const state = useHelpStore.getState()
        expect(state.isOpen).toBe(true)
        expect(state.mode).toBe('topic')
        expect(state.topicId).toBe('core:themes')
        expect(state.pkgSlug).toBeNull()
        expect(state.cameFrom).toBeNull()
    })

    it('close() preserves topicId so the drawer exit animation does not flash', () => {
        openHelp('core:themes')
        closeHelp()
        const state = useHelpStore.getState()
        expect(state.isOpen).toBe(false)
        // mode/topicId stay so the drawer's ~200 ms slide-out keeps
        // showing the topic title + body until it's fully off-screen.
        // The next open()/openPackage() overwrites these fields.
        expect(state.mode).toBe('topic')
        expect(state.topicId).toBe('core:themes')
    })

    it('open() of a second topic replaces the previous', () => {
        openHelp('core:themes')
        openHelp('core:sharing')
        expect(useHelpStore.getState().topicId).toBe('core:sharing')
    })

    it('openPackage() puts the drawer in package-index mode', () => {
        openHelpPackage('text')
        const state = useHelpStore.getState()
        expect(state.isOpen).toBe(true)
        expect(state.mode).toBe('package')
        expect(state.pkgSlug).toBe('text')
        expect(state.topicId).toBeNull()
        expect(state.cameFrom).toBeNull()
    })

    it('navigateToTopic() after openPackage() records cameFrom for the back arrow', () => {
        openHelpPackage('text')
        useHelpStore.getState().navigateToTopic('text:templates')
        const state = useHelpStore.getState()
        expect(state.mode).toBe('topic')
        expect(state.topicId).toBe('text:templates')
        expect(state.pkgSlug).toBe('text')
        expect(state.cameFrom).toBe('text')
    })

    it('back() returns from topic mode to the originating package list', () => {
        openHelpPackage('text')
        useHelpStore.getState().navigateToTopic('text:templates')
        useHelpStore.getState().back()
        const state = useHelpStore.getState()
        expect(state.mode).toBe('package')
        expect(state.pkgSlug).toBe('text')
        expect(state.topicId).toBeNull()
        expect(state.cameFrom).toBeNull()
    })

    it('open() (e.g. from palette) does NOT set cameFrom — back arrow should not render', () => {
        openHelp('text:templates')
        const state = useHelpStore.getState()
        expect(state.cameFrom).toBeNull()
    })

    it('open() while in package mode clears pkgSlug + cameFrom (e.g. palette mid-browse)', () => {
        openHelpPackage('text')
        openHelp('core:themes')
        const state = useHelpStore.getState()
        expect(state.mode).toBe('topic')
        expect(state.topicId).toBe('core:themes')
        expect(state.pkgSlug).toBeNull()
        expect(state.cameFrom).toBeNull()
    })

    it('close() from package-index mode preserves pkgSlug so the title stays during slide-out', () => {
        openHelpPackage('text')
        closeHelp()
        const state = useHelpStore.getState()
        expect(state.isOpen).toBe(false)
        expect(state.mode).toBe('package')
        expect(state.pkgSlug).toBe('text')
    })

    it('close() from a topic reached via navigateToTopic preserves topicId + cameFrom', () => {
        openHelpPackage('text')
        useHelpStore.getState().navigateToTopic('text:templates')
        closeHelp()
        const state = useHelpStore.getState()
        expect(state.isOpen).toBe(false)
        expect(state.topicId).toBe('text:templates')
        expect(state.cameFrom).toBe('text')
    })

    it('openPackage() after a close clears the previously-preserved topicId', () => {
        openHelp('core:themes')
        closeHelp()
        openHelpPackage('text')
        const state = useHelpStore.getState()
        expect(state.isOpen).toBe(true)
        expect(state.mode).toBe('package')
        expect(state.pkgSlug).toBe('text')
        expect(state.topicId).toBeNull()
    })
})
