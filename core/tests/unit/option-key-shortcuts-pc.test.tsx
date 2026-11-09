// @vitest-environment happy-dom
//
// Off a Mac, Alt does not change the character a key types: Alt+M reports
// `key: 'm'` and `code: 'KeyM'`. A shortcut bound by both the letter and the
// physical key must still run once for that one press.
import { act, cleanup, render } from '@testing-library/react'
import { ShortcutsProvider } from '@tinycld/core/lib/shortcuts/provider.web'
import { useShortcutRegistry } from '@tinycld/core/lib/shortcuts/registry'
import { resetScopes } from '@tinycld/core/lib/shortcuts/scopes'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// `$mod` is read from the platform when the key library loads: a PC one here.
vi.hoisted(() => {
    Object.defineProperty(navigator, 'platform', { value: 'Linux x86_64', configurable: true })
})

/**
 * Chrome before 112 has no RegExp `v` flag; the key library builds a binding
 * that ends in a group with the flags "iv", which throws there. A `v` flag
 * throws here as it does in that browser.
 */
const RealRegExp = RegExp
function withoutVFlag(pattern: string | RegExp, flags?: string) {
    if (flags?.includes('v'))
        throw new SyntaxError(`Invalid flags supplied to RegExp constructor '${flags}'`)
    return new RealRegExp(pattern, flags)
}
withoutVFlag.prototype = RealRegExp.prototype

function clearRegistry() {
    const state = useShortcutRegistry.getState()
    for (const id of Array.from(state.shortcuts.keys())) state.unregister(id)
}

beforeEach(() => {
    globalThis.RegExp = withoutVFlag as unknown as RegExpConstructor
})

afterEach(() => {
    cleanup()
    clearRegistry()
    resetScopes()
    globalThis.RegExp = RealRegExp
})

function listen(keys: string) {
    const run = vi.fn()
    act(() => {
        useShortcutRegistry.getState().register({
            id: `test.${keys}`,
            keys,
            scope: 'global',
            description: keys,
            run,
        })
    })
    render(
        <ShortcutsProvider>
            <div />
        </ShortcutsProvider>
    )
    return run
}

function press(init: KeyboardEventInit) {
    act(() => {
        window.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, ...init }))
    })
}

describe('Alt-key shortcuts off a Mac', () => {
    it('fires Alt+m exactly once from one Alt+M press that reports its plain letter', () => {
        const run = listen('Alt+m')
        press({ key: 'm', code: 'KeyM', altKey: true })
        expect(run).toHaveBeenCalledTimes(1)
    })

    it('fires Alt+5 exactly once from one Alt+5 press', () => {
        const run = listen('Alt+5')
        press({ key: '5', code: 'Digit5', altKey: true })
        expect(run).toHaveBeenCalledTimes(1)
    })
})
