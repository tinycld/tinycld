import { menuBarRegistryId } from '@tinycld/core/ui/menubar/menubar-store'
import { useOpenMenuStore } from '@tinycld/core/ui/menubar/open-menu-store'
import { beforeEach, describe, expect, it } from 'vitest'

describe('open-menu-store', () => {
    beforeEach(() => {
        useOpenMenuStore.setState({ openId: null })
    })

    it('open / close toggle a single id', () => {
        useOpenMenuStore.getState().open('alpha')
        expect(useOpenMenuStore.getState().openId).toBe('alpha')
        useOpenMenuStore.getState().close()
        expect(useOpenMenuStore.getState().openId).toBeNull()
    })

    it('open of a second id implicitly closes the first', () => {
        useOpenMenuStore.getState().open('alpha')
        useOpenMenuStore.getState().open('beta')
        expect(useOpenMenuStore.getState().openId).toBe('beta')
    })
})

describe('menuBarRegistryId', () => {
    it('namespaces ids under the menubar: prefix', () => {
        expect(menuBarRegistryId('file')).toBe('menubar::file')
        expect(menuBarRegistryId('whatever')).toBe('menubar::whatever')
    })

    it('folds the scope into the id', () => {
        expect(menuBarRegistryId('format', 'trinkets')).toBe('menubar:trinkets:format')
        expect(menuBarRegistryId('format', 'notepads')).toBe('menubar:notepads:format')
    })

    it('keeps the same menuId distinct across scopes', () => {
        // The bug this fixes: a frozen trinkets menubar and an active notepads menubar
        // both have a "format" menu; without scoping they shared one registry
        // key and both opened at once.
        expect(menuBarRegistryId('format', 'trinkets')).not.toBe(
            menuBarRegistryId('format', 'notepads')
        )
    })

    it('produces ids that do not collide with non-menubar registry entries', () => {
        const menubarFile = menuBarRegistryId('file', 'trinkets')
        const toolbarFile = 'toolbar:file'
        expect(menubarFile).not.toBe(toolbarFile)
        expect(menubarFile.startsWith('menubar:')).toBe(true)
        expect(toolbarFile.startsWith('menubar:')).toBe(false)
    })
})
