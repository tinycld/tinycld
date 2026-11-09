// @vitest-environment happy-dom
//
// On a Mac, Option changes the character a key types (⌥M types µ), and the
// browser reports that character as `event.key`. A shortcut registered as
// `Alt+m` must still fire from the physical key, which `event.code` names.
import { act, cleanup, render } from '@testing-library/react'
import { ShortcutsProvider } from '@tinycld/core/lib/shortcuts/provider.web'
import { useShortcutRegistry } from '@tinycld/core/lib/shortcuts/registry'
import { resetScopes } from '@tinycld/core/lib/shortcuts/scopes'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

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

describe('Option-key shortcuts on a Mac', () => {
    it('fires Alt+m when the key reports its plain letter', () => {
        const run = listen('Alt+m')
        press({ key: 'm', code: 'KeyM', altKey: true })
        expect(run).toHaveBeenCalledTimes(1)
    })

    it('fires Alt+m when Option turned the key into µ', () => {
        const run = listen('Alt+m')
        press({ key: 'µ', code: 'KeyM', altKey: true })
        expect(run).toHaveBeenCalledTimes(1)
    })

    it('fires $mod+Alt+d when Option turned the key into ∂', () => {
        const run = listen('$mod+Alt+d')
        press({ key: '∂', code: 'KeyD', altKey: true, metaKey: true })
        expect(run).toHaveBeenCalledTimes(1)
    })

    it('fires Alt+t and Alt+5 from the physical letter and digit keys', () => {
        const t = listen('Alt+t')
        press({ key: '†', code: 'KeyT', altKey: true })
        expect(t).toHaveBeenCalledTimes(1)
        cleanup()
        clearRegistry()
        const five = listen('Alt+5')
        press({ key: '∞', code: 'Digit5', altKey: true })
        expect(five).toHaveBeenCalledTimes(1)
    })

    it('does not fire Alt+m for another key that types the same character', () => {
        const run = listen('Alt+m')
        press({ key: 'µ', code: 'KeyN', altKey: true })
        expect(run).not.toHaveBeenCalled()
    })
})

/**
 * Safari before 17 (and Chrome before 112) has no RegExp `v` flag. The key
 * library builds a RegExp with the flags "iv" for a binding that ends in a
 * group like `(m|KeyM)`, so in such a browser binding one throws and no
 * shortcut works. This stands in for that browser: a `v` flag throws as it
 * does there.
 */
const RealRegExp = RegExp
function withoutVFlag(pattern: string | RegExp, flags?: string) {
    if (flags?.includes('v'))
        throw new SyntaxError(`Invalid flags supplied to RegExp constructor '${flags}'`)
    return new RealRegExp(pattern, flags)
}
withoutVFlag.prototype = RealRegExp.prototype

describe('Option-key shortcuts in a browser without the RegExp v flag', () => {
    beforeEach(() => {
        globalThis.RegExp = withoutVFlag as unknown as RegExpConstructor
    })
    afterEach(() => {
        globalThis.RegExp = RealRegExp
    })

    it('binds the Option keys without a regex group, so they mount and fire', () => {
        const run = listen('Alt+m')
        press({ key: 'm', code: 'KeyM', altKey: true })
        expect(run).toHaveBeenCalledTimes(1)
    })

    it('fires Alt+m exactly once from one ⌥M press that types µ', () => {
        const run = listen('Alt+m')
        press({ key: 'µ', code: 'KeyM', altKey: true })
        expect(run).toHaveBeenCalledTimes(1)
    })

    it('fires $mod+Alt+d exactly once from one ⌘⌥D press, typed as ∂ or as d', () => {
        const run = listen('$mod+Alt+d')
        press({ key: '∂', code: 'KeyD', altKey: true, metaKey: true })
        expect(run).toHaveBeenCalledTimes(1)
        press({ key: 'd', code: 'KeyD', altKey: true, metaKey: true })
        expect(run).toHaveBeenCalledTimes(2)
    })
})
