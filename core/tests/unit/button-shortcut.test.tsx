// @vitest-environment happy-dom

import { cleanup, render } from '@testing-library/react'
import { createMatcher } from '@tinycld/core/lib/shortcuts/matcher'
import { useShortcutRegistry } from '@tinycld/core/lib/shortcuts/registry'
import { resetScopes, type ScopeOwner, useShortcutScope } from '@tinycld/core/lib/shortcuts/scopes'
import type { Scope } from '@tinycld/core/lib/shortcuts/types'
import { LayerScope } from '@tinycld/core/ui/overlay/escape'
import { useButtonShortcut } from '@tinycld/core/ui/shortcut-button/useButtonShortcut'
import { afterEach, describe, expect, it, vi } from 'vitest'

// A dialog or screen mounts inside a route that is already focused, where
// useFocusEffect runs on mount.
vi.mock('expo-router', async () => {
    const { useEffect } = await import('react')
    return {
        useFocusEffect: (effect: () => undefined | (() => void)) => useEffect(effect, [effect]),
    }
})

interface ProbeProps {
    onPress: () => void
    isDisabled?: boolean
    scope?: Scope
    scopeOwner?: ScopeOwner
}

function Probe({ onPress, isDisabled = false, scope = 'modal', scopeOwner }: ProbeProps) {
    useButtonShortcut({
        label: 'Save',
        shortcut: { keys: '$mod+Enter', scope, allowInInputs: true },
        isDisabled,
        onPress,
        scopeOwner,
    })
    return null
}

function Screen({ onPress }: { onPress: () => void }) {
    const owner = useShortcutScope('list')
    return <Probe onPress={onPress} scope="list" scopeOwner={owner} />
}

function clearRegistry() {
    const state = useShortcutRegistry.getState()
    for (const id of Array.from(state.shortcuts.keys())) state.unregister(id)
}

const press = () => createMatcher().feedAtom('$mod+Enter', { inInput: false })

describe('useButtonShortcut', () => {
    afterEach(() => {
        cleanup()
        clearRegistry()
        resetScopes()
    })

    it('presses the button on its keys', () => {
        const onPress = vi.fn()
        render(
            <LayerScope onEscape={vi.fn()}>
                <Probe onPress={onPress} />
            </LayerScope>
        )
        expect(press()).toBe(true)
        expect(onPress).toHaveBeenCalledTimes(1)
    })

    it('leaves the key alone while the button is disabled', () => {
        const onPress = vi.fn()
        render(
            <LayerScope onEscape={vi.fn()}>
                <Probe onPress={onPress} isDisabled />
            </LayerScope>
        )
        expect(press()).toBe(false)
        expect(onPress).not.toHaveBeenCalled()
    })

    // A Sheet keeps its content mounted through its exit animation; a second
    // ⌘↵ then must not run the confirmed action again.
    it('leaves the key alone while its layer is closing', () => {
        const onPress = vi.fn()
        render(
            <LayerScope onEscape={vi.fn()} isActive={false}>
                <Probe onPress={onPress} />
            </LayerScope>
        )
        expect(press()).toBe(false)
        expect(onPress).not.toHaveBeenCalled()
    })

    // A screen's button registers before the screen's own scope effect runs,
    // so without the owner it is stamped null and never fires.
    it('fires on a screen when given the screen scope owner', () => {
        const onPress = vi.fn()
        render(<Screen onPress={onPress} />)
        expect(press()).toBe(true)
        expect(onPress).toHaveBeenCalledTimes(1)
    })
})
