// @vitest-environment happy-dom

import { cleanup, render } from '@testing-library/react'
import { createMatcher } from '@tinycld/core/lib/shortcuts/matcher'
import { useShortcutRegistry } from '@tinycld/core/lib/shortcuts/registry'
import { resetScopes } from '@tinycld/core/lib/shortcuts/scopes'
import type { Shortcut } from '@tinycld/core/lib/shortcuts/types'
import { useRegisterShortcut } from '@tinycld/core/lib/shortcuts/use-register'
import { LayerScope } from '@tinycld/core/ui/overlay/escape'
import { type ReactNode, useMemo } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

// A dialog opens inside a route that is already focused, where
// useFocusEffect runs on mount. Model exactly that.
vi.mock('expo-router', async () => {
    const { useEffect } = await import('react')
    return {
        useFocusEffect: (effect: () => undefined | (() => void)) => useEffect(effect, [effect]),
    }
})

function SubmitShortcut({ id, run }: { id: string; run: () => void }) {
    const shortcut = useMemo<Shortcut>(
        () => ({
            id,
            keys: '$mod+Enter',
            scope: 'modal',
            description: 'Submit',
            allowInInputs: true,
            run,
        }),
        [id, run]
    )
    useRegisterShortcut(shortcut)
    return null
}

function clearRegistry() {
    const state = useShortcutRegistry.getState()
    for (const id of Array.from(state.shortcuts.keys())) state.unregister(id)
}

describe('LayerScope', () => {
    afterEach(() => {
        cleanup()
        clearRegistry()
        resetScopes()
    })

    it('stamps a descendant shortcut with the layer id, so it fires', () => {
        const run = vi.fn()
        render(
            <LayerScope onEscape={vi.fn()}>
                <SubmitShortcut id="t.submit" run={run} />
            </LayerScope>
        )

        const stamped = useShortcutRegistry.getState().shortcuts.get('t.submit')
        expect(stamped?.scopeId).toEqual(expect.any(Number))
        expect(createMatcher().feedAtom('$mod+Enter', { inInput: false })).toBe(true)
        expect(run).toHaveBeenCalledTimes(1)
    })

    it('fires only the top layer when two layers are stacked', () => {
        const lower = vi.fn()
        const upper = vi.fn()
        // The second layer opens in a LATER commit, as in the app. Mounted in
        // one commit, the inner scope's effect would push first and end up
        // under the outer one.
        const Lower = ({ children }: { children?: ReactNode }) => (
            <LayerScope onEscape={vi.fn()}>
                <SubmitShortcut id="t.lower" run={lower} />
                {children}
            </LayerScope>
        )
        const { rerender } = render(<Lower />)
        rerender(
            <Lower>
                <LayerScope onEscape={vi.fn()}>
                    <SubmitShortcut id="t.upper" run={upper} />
                </LayerScope>
            </Lower>
        )

        createMatcher().feedAtom('$mod+Enter', { inInput: false })
        expect(upper).toHaveBeenCalledTimes(1)
        expect(lower).not.toHaveBeenCalled()
    })

    it('runs Escape only while isEscapeActive', () => {
        const onEscape = vi.fn()
        const { rerender } = render(<LayerScope onEscape={onEscape} isEscapeActive={false} />)
        createMatcher().feedAtom('Escape', { inInput: false })
        expect(onEscape).not.toHaveBeenCalled()

        rerender(<LayerScope onEscape={onEscape} isEscapeActive />)
        createMatcher().feedAtom('Escape', { inInput: false })
        expect(onEscape).toHaveBeenCalledTimes(1)
    })
})
