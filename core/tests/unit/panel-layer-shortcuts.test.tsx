// @vitest-environment happy-dom
//
// A panel layer (an overlay Drawer) holds the screen's own content and pushes
// no shortcut scope, so the shortcut system reads a key in it as if no layer
// were open: a scoped ⌘↵ typed in a field in the panel still fires, and a
// screen's Escape in a field runs before the layer stack closes the panel.
import { act, cleanup, render } from '@testing-library/react'
import { ShortcutsProvider } from '@tinycld/core/lib/shortcuts/provider.web'
import { useShortcutRegistry } from '@tinycld/core/lib/shortcuts/registry'
import { pushScope, resetScopes } from '@tinycld/core/lib/shortcuts/scopes'
import { resetLayers, useOverlayLayer } from '@tinycld/core/ui/overlay/layer-stack'
import { afterEach, describe, expect, it, vi } from 'vitest'

// `$mod` is read from the platform when the key library loads: an Apple one here.
vi.hoisted(() => {
    Object.defineProperty(navigator, 'platform', { value: 'MacIntel', configurable: true })
})

function clearRegistry() {
    const state = useShortcutRegistry.getState()
    for (const id of Array.from(state.shortcuts.keys())) state.unregister(id)
}

afterEach(() => {
    cleanup()
    clearRegistry()
    resetScopes()
    resetLayers()
})

const NO_NODES = () => []

function Panel({ onDismiss }: { onDismiss: () => void }) {
    useOverlayLayer({
        isOpen: true,
        nodes: NO_NODES,
        onDismiss,
        dismissOnOutside: false,
        dismissOnEscape: true,
        isModal: false,
        isPanel: true,
    })
    // react-native-web's TextInput stops every keydown in its own handler, so
    // the key never reaches a bubble-phase window listener.
    return <input data-testid="panel-field" onKeyDown={event => event.stopPropagation()} />
}

function screenShortcut(keys: string, run: () => void) {
    // The screen that owns the shortcut holds the keyboard.
    pushScope('compose')
    act(() => {
        useShortcutRegistry.getState().register({
            id: `test.${keys}`,
            keys,
            scope: 'compose',
            description: keys,
            allowInInputs: true,
            run,
        })
    })
}

function typeInField(field: HTMLElement, init: KeyboardEventInit) {
    field.focus()
    act(() => {
        field.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, ...init }))
    })
}

describe('Shortcuts with a panel layer open', () => {
    it('fires an allowInInputs ⌘↵ typed in a field inside the panel', () => {
        const run = vi.fn()
        screenShortcut('$mod+Enter', run)
        const { getByTestId } = render(
            <ShortcutsProvider>
                <Panel onDismiss={() => {}} />
            </ShortcutsProvider>
        )

        typeInField(getByTestId('panel-field'), { key: 'Enter', metaKey: true })
        expect(run).toHaveBeenCalledTimes(1)
    })

    it("runs the screen's allowInInputs Escape before the stack closes the panel", () => {
        const order: string[] = []
        screenShortcut('Escape', () => order.push('screen'))
        const { getByTestId } = render(
            <ShortcutsProvider>
                <Panel onDismiss={() => order.push('panel')} />
            </ShortcutsProvider>
        )

        typeInField(getByTestId('panel-field'), { key: 'Escape' })
        expect(order[0]).toBe('screen')
    })
})
