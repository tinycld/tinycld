import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { beforeEach, describe, expect, it } from 'vitest'

describe('useWorkspaceStore — lastPackageHref', () => {
    beforeEach(() => {
        useWorkspaceStore.setState({ lastPackageHref: {} })
    })

    it('starts empty', () => {
        expect(useWorkspaceStore.getState().lastPackageHref).toEqual({})
    })

    it('setLastPackageHref adds a new slug → href entry', () => {
        useWorkspaceStore.getState().setLastPackageHref('trinkets', '/trinkets/abc')
        expect(useWorkspaceStore.getState().lastPackageHref).toEqual({
            trinkets: '/trinkets/abc',
        })
    })

    it('setLastPackageHref merges into existing entries without dropping others', () => {
        const { setLastPackageHref } = useWorkspaceStore.getState()
        setLastPackageHref('trinkets', '/trinkets/abc')
        setLastPackageHref('text', '/text/xyz')
        expect(useWorkspaceStore.getState().lastPackageHref).toEqual({
            trinkets: '/trinkets/abc',
            text: '/text/xyz',
        })
    })

    it('setLastPackageHref overwrites the same slug', () => {
        const { setLastPackageHref } = useWorkspaceStore.getState()
        setLastPackageHref('trinkets', '/trinkets/abc')
        setLastPackageHref('trinkets', '/trinkets/def')
        expect(useWorkspaceStore.getState().lastPackageHref).toEqual({
            trinkets: '/trinkets/def',
        })
    })

    it('clearLastPackageHref removes only the named slug', () => {
        const { setLastPackageHref, clearLastPackageHref } = useWorkspaceStore.getState()
        setLastPackageHref('trinkets', '/trinkets/abc')
        setLastPackageHref('text', '/text/xyz')
        clearLastPackageHref('trinkets')
        expect(useWorkspaceStore.getState().lastPackageHref).toEqual({
            text: '/text/xyz',
        })
    })

    it('clearLastPackageHref is a no-op when the slug was not set', () => {
        const { setLastPackageHref, clearLastPackageHref } = useWorkspaceStore.getState()
        setLastPackageHref('text', '/text/xyz')
        clearLastPackageHref('trinkets')
        expect(useWorkspaceStore.getState().lastPackageHref).toEqual({
            text: '/text/xyz',
        })
    })
})
