import { handleKey, type KeyFacts, keyPhase } from '@tinycld/core/lib/shortcuts/dispatch'
import { helpGroups, useShortcutHelpStore } from '@tinycld/core/lib/shortcuts/help-list'
import { createMatcher, isScopeActive } from '@tinycld/core/lib/shortcuts/matcher'
import { useShortcutRegistry } from '@tinycld/core/lib/shortcuts/registry'
import {
    currentScopeId,
    nextScopeId,
    popScope,
    pushScope,
    resetScopes,
    topScopeEntry,
} from '@tinycld/core/lib/shortcuts/scopes'
import type { Shortcut } from '@tinycld/core/lib/shortcuts/types'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

function register(shortcut: Shortcut) {
    useShortcutRegistry.getState().register(shortcut)
}

function unregister(id: string) {
    useShortcutRegistry.getState().unregister(id)
}

function clearRegistry() {
    const state = useShortcutRegistry.getState()
    for (const id of Array.from(state.shortcuts.keys())) state.unregister(id)
}

describe('matcher', () => {
    beforeEach(() => {
        vi.useFakeTimers()
    })

    afterEach(() => {
        clearRegistry()
        resetScopes()
        vi.useRealTimers()
    })

    it('fires a single global combo', () => {
        const run = vi.fn()
        register({ id: 't.a', keys: 'a', scope: 'global', description: 'a', run })
        const m = createMatcher()

        const consumed = m.feedAtom('a', { inInput: false })
        expect(consumed).toBe(true)
        expect(run).toHaveBeenCalledTimes(1)
    })

    it('fires a $mod combo atom', () => {
        const run = vi.fn()
        register({
            id: 't.send',
            keys: '$mod+Enter',
            scope: 'compose',
            description: 'send',
            allowInInputs: true,
            run,
        })
        const scopeId = pushScope('compose')
        const m = createMatcher()

        expect(m.feedAtom('$mod+Enter', { inInput: true })).toBe(true)
        expect(run).toHaveBeenCalledTimes(1)
        popScope(scopeId)
    })

    it('fires a "g i" sequence when atoms arrive in order', () => {
        const run = vi.fn()
        register({ id: 't.gi', keys: 'g i', scope: 'global', description: 'go inbox', run })
        const m = createMatcher()

        expect(m.feedAtom('g', { inInput: false })).toBe(true)
        expect(run).not.toHaveBeenCalled()
        expect(m.feedAtom('i', { inInput: false })).toBe(true)
        expect(run).toHaveBeenCalledTimes(1)
    })

    it('resets the sequence after timeout', () => {
        const run = vi.fn()
        register({ id: 't.gi', keys: 'g i', scope: 'global', description: 'go inbox', run })
        const m = createMatcher()

        m.feedAtom('g', { inInput: false })
        // Advance past the 1s sequence window.
        vi.advanceTimersByTime(1500)
        expect(m.getSequence()).toEqual([])
        // A trailing "i" now should not fire.
        m.feedAtom('i', { inInput: false })
        expect(run).not.toHaveBeenCalled()
    })

    it('restarts sequence when an unknown atom interrupts', () => {
        const run = vi.fn()
        register({ id: 't.gi', keys: 'g i', scope: 'global', description: 'go inbox', run })
        const m = createMatcher()

        m.feedAtom('g', { inInput: false })
        // "z" doesn't match the g-prefix, so the sequence resets — but z itself
        // also doesn't match any shortcut, so feedAtom returns false.
        expect(m.feedAtom('z', { inInput: false })).toBe(false)
        expect(m.getSequence()).toEqual([])
        // A fresh "g i" should still fire.
        m.feedAtom('g', { inInput: false })
        m.feedAtom('i', { inInput: false })
        expect(run).toHaveBeenCalledTimes(1)
    })

    it('ignores list-scope shortcuts when modal is on top', () => {
        const run = vi.fn()
        register({ id: 't.j', keys: 'j', scope: 'list', description: 'next', run })
        const listId = pushScope('list')
        const modalId = pushScope('modal')
        const m = createMatcher()

        expect(m.feedAtom('j', { inInput: false })).toBe(false)
        expect(run).not.toHaveBeenCalled()

        popScope(modalId)
        expect(m.feedAtom('j', { inInput: false })).toBe(true)
        expect(run).toHaveBeenCalledTimes(1)
        popScope(listId)
    })

    // A screen frozen by `freezeOnBlur` keeps its shortcuts registered, so two
    // packages can hold the same key at the same scope. Only the one owned by
    // the scope instance on top may fire.
    it('fires the live screen’s shortcut when a frozen screen shares its key', () => {
        const frozen = vi.fn()
        const live = vi.fn()
        const frozenId = pushScope('list')
        register({
            id: 'frozen.j',
            keys: 'j',
            scope: 'list',
            description: 'frozen next',
            scopeId: frozenId,
            run: frozen,
        })
        const liveId = pushScope('list')
        register({
            id: 'live.j',
            keys: 'j',
            scope: 'list',
            description: 'live next',
            scopeId: liveId,
            run: live,
        })
        const m = createMatcher()

        expect(m.feedAtom('j', { inInput: false })).toBe(true)
        expect(live).toHaveBeenCalledTimes(1)
        expect(frozen).not.toHaveBeenCalled()

        // Returning to the other screen hands its own shortcut back.
        popScope(liveId)
        expect(m.feedAtom('j', { inInput: false })).toBe(true)
        expect(frozen).toHaveBeenCalledTimes(1)
        expect(live).toHaveBeenCalledTimes(1)
        popScope(frozenId)
    })

    // The help overlay lists exactly what isScopeActive admits, so a board's
    // keys must disappear from it while a card (a 'modal' scope) is open —
    // otherwise it advertises keys that cannot fire, and duplicates every
    // entry the two screens share.
    it('reports only the scope holding the keyboard as active', () => {
        const boardKey: Shortcut = {
            id: 'board.x',
            keys: 'x',
            scope: 'list',
            description: 'archive',
            run: () => {},
        }
        const jumpKey: Shortcut = {
            id: 'core.jump',
            keys: 't m',
            scope: 'global',
            description: 'jump',
            run: () => {},
        }
        const listId = pushScope('list')
        expect(isScopeActive(boardKey)).toBe(true)

        const modalId = pushScope('modal')
        expect(isScopeActive(boardKey)).toBe(false)
        // A global shortcut stays reachable from inside a modal.
        expect(isScopeActive(jumpKey)).toBe(true)

        popScope(modalId)
        expect(isScopeActive(boardKey)).toBe(true)
        popScope(listId)
    })

    it('skips shortcuts when focus is in an input (default)', () => {
        const run = vi.fn()
        register({ id: 't.j', keys: 'j', scope: 'global', description: 'next', run })
        const m = createMatcher()

        expect(m.feedAtom('j', { inInput: true })).toBe(false)
        expect(run).not.toHaveBeenCalled()
    })

    it('fires input-aware shortcuts regardless of focus', () => {
        const run = vi.fn()
        register({
            id: 't.esc',
            keys: 'Escape',
            scope: 'global',
            description: 'close',
            allowInInputs: true,
            run,
        })
        const m = createMatcher()

        expect(m.feedAtom('Escape', { inInput: true })).toBe(true)
        expect(run).toHaveBeenCalledTimes(1)
    })

    it('honors when() guards', () => {
        const run = vi.fn()
        let allow = false
        register({
            id: 't.g',
            keys: 'g',
            scope: 'global',
            description: 'guarded',
            when: () => allow,
            run,
        })
        const m = createMatcher()

        expect(m.feedAtom('g', { inInput: false })).toBe(false)
        expect(run).not.toHaveBeenCalled()

        allow = true
        expect(m.feedAtom('g', { inInput: false })).toBe(true)
        expect(run).toHaveBeenCalledTimes(1)
    })

    it('unregister removes the shortcut', () => {
        const run = vi.fn()
        register({ id: 't.a', keys: 'a', scope: 'global', description: 'a', run })
        unregister('t.a')
        const m = createMatcher()

        expect(m.feedAtom('a', { inInput: false })).toBe(false)
    })

    // Core's global `/` (search) is registered at app start, before any package
    // screen mounts, so it is first in the registry. A package's scoped `/`
    // must still win while its scope holds the keyboard.
    it('a scoped shortcut beats a global one with the same keys while its scope is active', () => {
        const runGlobal = vi.fn()
        const runScoped = vi.fn()
        register({
            id: 't.search',
            keys: '/',
            scope: 'global',
            description: 'search',
            run: runGlobal,
        })
        const listId = pushScope('list')
        register({
            id: 't.filter',
            keys: '/',
            scope: 'list',
            scopeId: listId,
            description: 'filter',
            run: runScoped,
        })
        const m = createMatcher()

        expect(m.feedAtom('/', { inInput: false })).toBe(true)
        expect(runScoped).toHaveBeenCalledTimes(1)
        expect(runGlobal).not.toHaveBeenCalled()

        popScope(listId)
        expect(m.feedAtom('/', { inInput: false })).toBe(true)
        expect(runGlobal).toHaveBeenCalledTimes(1)
        expect(runScoped).toHaveBeenCalledTimes(1)
    })
})

// Regression: a blurred-but-mounted screen must keep stamping its OWN scope id.
//
// On web `freezeOnBlur` only sets `display: none`, so a blurred screen's live
// queries keep emitting and its memoised shortcut array keeps changing identity
// — which re-runs its register effect while ANOTHER package holds the keyboard.
// Deriving the stamp from the scope stack at that moment (`currentScopeId`,
// which answers "who holds this scope now") gave the blurred screen the focused
// screen's id. Both then looked live to the matcher, and the one earlier in the
// registry's insertion order won — firing mail's `j` for a keypress meant for a
// cards board, which read to the user as a dropped keystroke.
describe('scope stamping', () => {
    beforeEach(() => {
        resetScopes()
        clearRegistry()
    })

    it('currentScopeId returns the TOP holder, not the caller — why the stamp is passed in', () => {
        const blurredId = pushScope('list')
        const focusedId = pushScope('list')

        // Both screens are mounted at 'list'; the stack cannot distinguish them.
        expect(currentScopeId('list')).toBe(focusedId)
        expect(currentScopeId('list')).not.toBe(blurredId)
    })

    it('a blurred screen re-registering keeps its own id and stays inactive', () => {
        const blurred = vi.fn()
        const focused = vi.fn()
        const blurredId = pushScope('list')
        const focusedId = pushScope('list')

        // Insertion order matters: the blurred screen registered first, so the
        // matcher visits it first and a wrong stamp would win outright.
        register({
            id: 'blurred.j',
            keys: 'j',
            scope: 'list',
            description: 'blurred next',
            scopeId: blurredId,
            run: blurred,
        })
        register({
            id: 'focused.j',
            keys: 'j',
            scope: 'list',
            description: 'focused next',
            scopeId: focusedId,
            run: focused,
        })

        // The blurred screen re-registers (a live query emitted). It stamps with
        // its OWN id — the value its useShortcutScope returned — rather than
        // re-reading the stack, which would hand it `focusedId`.
        register({
            id: 'blurred.j',
            keys: 'j',
            scope: 'list',
            description: 'blurred next',
            scopeId: blurredId,
            run: blurred,
        })

        const m = createMatcher()
        expect(m.feedAtom('j', { inInput: false })).toBe(true)
        expect(focused).toHaveBeenCalledTimes(1)
        expect(blurred).not.toHaveBeenCalled()
    })

    // The three stamp states, which isScopeActive must keep distinct.
    it('treats an undefined stamp as legacy scope-only', () => {
        pushScope('list')
        const legacy: Shortcut = {
            id: 'legacy.j',
            keys: 'j',
            scope: 'list',
            description: 'legacy',
            run: () => {},
        }
        expect(isScopeActive(legacy)).toBe(true)
    })

    // A null stamp means "my owner held no scope entry when I registered", which
    // arises two ways that must NOT be conflated.

    // A dialog mounting inside an already-focused route never re-runs
    // useFocusEffect, so its owner never receives an id and every one of its
    // shortcuts is stamped null — measured, not assumed: all four cards.peek.*
    // shortcuts register with scopeId null on every run. They must still fire,
    // or Escape stops closing dialogs.

    it('re-stamping a blurred screen from the live stack fires the WRONG handler', () => {
        const blurred = vi.fn()
        const focused = vi.fn()
        pushScope('list')
        const focusedId = pushScope('list')

        // The pre-fix behavior, pinned so a regression is unambiguous: the
        // blurred screen re-registers and picks up whatever currentScopeId says.
        register({
            id: 'blurred.j',
            keys: 'j',
            scope: 'list',
            description: 'blurred next',
            scopeId: currentScopeId('list'),
            run: blurred,
        })
        register({
            id: 'focused.j',
            keys: 'j',
            scope: 'list',
            description: 'focused next',
            scopeId: focusedId,
            run: focused,
        })

        const m = createMatcher()
        m.feedAtom('j', { inInput: false })
        expect(blurred).toHaveBeenCalledTimes(1)
        expect(focused).not.toHaveBeenCalled()
    })
})

// The scope stamping block above resets only before each test, so these blocks
// clear the registry, the scope stack and the help store on both sides.
function resetShortcutState() {
    clearRegistry()
    resetScopes()
    useShortcutHelpStore.getState().close()
}

// Web dispatch runs each key through two window bindings: a capture one that
// sees the key before the focused element, and a bubble one as before. One
// decision per key (keyPhase) says which binding feeds the matcher.
describe('dispatch', () => {
    beforeEach(resetShortcutState)
    afterEach(resetShortcutState)

    it('keyPhase sends modified keys and text-field keys to capture, yields Escape to a layer', () => {
        // No open layer takes Escape.
        expect(
            keyPhase({
                key: 'Enter',
                ctrlKey: false,
                metaKey: true,
                altKey: false,
                inInput: false,
                layerTakesEscape: false,
                menuOpen: false,
            })
        ).toBe('capture')
        expect(
            keyPhase({
                key: 'Enter',
                ctrlKey: true,
                metaKey: false,
                altKey: false,
                inInput: false,
                layerTakesEscape: false,
                menuOpen: false,
            })
        ).toBe('capture')
        expect(
            keyPhase({
                key: 'x',
                ctrlKey: false,
                metaKey: false,
                altKey: true,
                inInput: false,
                layerTakesEscape: false,
                menuOpen: false,
            })
        ).toBe('capture')
        expect(
            keyPhase({
                key: 'Enter',
                ctrlKey: false,
                metaKey: false,
                altKey: false,
                inInput: false,
                layerTakesEscape: false,
                menuOpen: false,
            })
        ).toBe('bubble')
        expect(
            keyPhase({
                key: ' ',
                ctrlKey: false,
                metaKey: false,
                altKey: false,
                inInput: false,
                layerTakesEscape: false,
                menuOpen: false,
            })
        ).toBe('bubble')
        expect(
            keyPhase({
                key: 'j',
                ctrlKey: false,
                metaKey: false,
                altKey: false,
                inInput: false,
                layerTakesEscape: false,
                menuOpen: false,
            })
        ).toBe('bubble')
        expect(
            keyPhase({
                key: '?',
                ctrlKey: false,
                metaKey: false,
                altKey: false,
                inInput: false,
                layerTakesEscape: false,
                menuOpen: false,
            })
        ).toBe('bubble')
        expect(
            keyPhase({
                key: 'Escape',
                ctrlKey: false,
                metaKey: false,
                altKey: false,
                inInput: false,
                layerTakesEscape: false,
                menuOpen: false,
            })
        ).toBe('bubble')
        expect(
            keyPhase({
                key: 'j',
                ctrlKey: false,
                metaKey: false,
                altKey: false,
                inInput: true,
                layerTakesEscape: false,
                menuOpen: false,
            })
        ).toBe('capture')
        expect(
            keyPhase({
                key: 'Enter',
                ctrlKey: false,
                metaKey: false,
                altKey: false,
                inInput: true,
                layerTakesEscape: false,
                menuOpen: false,
            })
        ).toBe('capture')
        expect(
            keyPhase({
                key: 'Escape',
                ctrlKey: false,
                metaKey: false,
                altKey: false,
                inInput: true,
                layerTakesEscape: false,
                menuOpen: false,
            })
        ).toBe('capture')

        // An open layer takes Escape, and only Escape.
        expect(
            keyPhase({
                key: 'Escape',
                ctrlKey: false,
                metaKey: false,
                altKey: false,
                inInput: false,
                layerTakesEscape: true,
                menuOpen: false,
            })
        ).toBeNull()
        expect(
            keyPhase({
                key: 'Escape',
                ctrlKey: false,
                metaKey: false,
                altKey: false,
                inInput: true,
                layerTakesEscape: true,
                menuOpen: false,
            })
        ).toBeNull()
        expect(
            keyPhase({
                key: 'Escape',
                ctrlKey: false,
                metaKey: true,
                altKey: false,
                inInput: false,
                layerTakesEscape: true,
                menuOpen: false,
            })
        ).toBeNull()
        expect(
            keyPhase({
                key: 'Enter',
                ctrlKey: false,
                metaKey: true,
                altKey: false,
                inInput: false,
                layerTakesEscape: true,
                menuOpen: false,
            })
        ).toBe('capture')
        expect(
            keyPhase({
                key: 'j',
                ctrlKey: false,
                metaKey: false,
                altKey: false,
                inInput: false,
                layerTakesEscape: true,
                menuOpen: false,
            })
        ).toBe('bubble')
    })

    it('consumes only a key that an active shortcut matches', () => {
        const post = vi.fn()
        const open = vi.fn()
        const save = vi.fn()
        const next = vi.fn()
        const composeId = pushScope('compose')
        register({
            id: 't.post',
            keys: '$mod+Enter',
            scope: 'compose',
            scopeId: composeId,
            description: 'Post',
            allowInInputs: true,
            run: post,
        })
        register({
            id: 't.open',
            keys: 'Enter',
            scope: 'compose',
            scopeId: composeId,
            description: 'Open',
            run: open,
        })
        register({
            id: 't.save',
            keys: '$mod+s',
            scope: 'compose',
            scopeId: composeId,
            description: 'Save draft',
            allowInInputs: true,
            when: () => false,
            run: save,
        })
        register({ id: 't.next', keys: 'j', scope: 'global', description: 'Next', run: next })
        const matcher = createMatcher()

        const modEnterInField: KeyFacts = {
            key: 'Enter',
            ctrlKey: false,
            metaKey: true,
            altKey: false,
            inInput: true,
            layerTakesEscape: false,
            menuOpen: false,
        }
        expect(
            handleKey({ matcher, atom: '$mod+Enter', facts: modEnterInField, phase: 'capture' })
        ).toBe(true)
        expect(post).toHaveBeenCalledTimes(1)
        expect(
            handleKey({ matcher, atom: '$mod+Enter', facts: modEnterInField, phase: 'bubble' })
        ).toBe(false)
        expect(post).toHaveBeenCalledTimes(1)

        const modEnterOnButton: KeyFacts = {
            key: 'Enter',
            ctrlKey: false,
            metaKey: true,
            altKey: false,
            inInput: false,
            layerTakesEscape: false,
            menuOpen: false,
        }
        expect(
            handleKey({ matcher, atom: '$mod+Enter', facts: modEnterOnButton, phase: 'capture' })
        ).toBe(true)
        expect(post).toHaveBeenCalledTimes(2)

        const modB: KeyFacts = {
            key: 'b',
            ctrlKey: false,
            metaKey: true,
            altKey: false,
            inInput: false,
            layerTakesEscape: false,
            menuOpen: false,
        }
        expect(handleKey({ matcher, atom: '$mod+b', facts: modB, phase: 'capture' })).toBe(false)

        const modS: KeyFacts = {
            key: 's',
            ctrlKey: false,
            metaKey: true,
            altKey: false,
            inInput: true,
            layerTakesEscape: false,
            menuOpen: false,
        }
        expect(handleKey({ matcher, atom: '$mod+s', facts: modS, phase: 'capture' })).toBe(false)
        expect(save).not.toHaveBeenCalled()

        const enterOnButton: KeyFacts = {
            key: 'Enter',
            ctrlKey: false,
            metaKey: false,
            altKey: false,
            inInput: false,
            layerTakesEscape: false,
            menuOpen: false,
        }
        expect(handleKey({ matcher, atom: 'Enter', facts: enterOnButton, phase: 'capture' })).toBe(
            false
        )
        expect(open).not.toHaveBeenCalled()
        expect(handleKey({ matcher, atom: 'Enter', facts: enterOnButton, phase: 'bubble' })).toBe(
            true
        )
        expect(open).toHaveBeenCalledTimes(1)

        const jInField: KeyFacts = {
            key: 'j',
            ctrlKey: false,
            metaKey: false,
            altKey: false,
            inInput: true,
            layerTakesEscape: false,
            menuOpen: false,
        }
        expect(handleKey({ matcher, atom: 'j', facts: jInField, phase: 'capture' })).toBe(false)
        expect(handleKey({ matcher, atom: 'j', facts: jInField, phase: 'bubble' })).toBe(false)
        expect(next).not.toHaveBeenCalled()
    })

    it('leaves Escape to an open layer', () => {
        const cancel = vi.fn()
        const composeId = pushScope('compose')
        register({
            id: 't.cancel',
            keys: 'Escape',
            scope: 'compose',
            scopeId: composeId,
            description: 'Cancel',
            allowInInputs: true,
            run: cancel,
        })
        const matcher = createMatcher()

        const layerEscapeInField: KeyFacts = {
            key: 'Escape',
            ctrlKey: false,
            metaKey: false,
            altKey: false,
            inInput: true,
            layerTakesEscape: true,
            menuOpen: false,
        }
        const layerEscapeOutside: KeyFacts = {
            key: 'Escape',
            ctrlKey: false,
            metaKey: false,
            altKey: false,
            inInput: false,
            layerTakesEscape: true,
            menuOpen: false,
        }
        expect(
            handleKey({ matcher, atom: 'Escape', facts: layerEscapeInField, phase: 'capture' })
        ).toBe(false)
        expect(
            handleKey({ matcher, atom: 'Escape', facts: layerEscapeInField, phase: 'bubble' })
        ).toBe(false)
        expect(
            handleKey({ matcher, atom: 'Escape', facts: layerEscapeOutside, phase: 'capture' })
        ).toBe(false)
        expect(
            handleKey({ matcher, atom: 'Escape', facts: layerEscapeOutside, phase: 'bubble' })
        ).toBe(false)
        expect(cancel).not.toHaveBeenCalled()

        const escapeInField: KeyFacts = {
            key: 'Escape',
            ctrlKey: false,
            metaKey: false,
            altKey: false,
            inInput: true,
            layerTakesEscape: false,
            menuOpen: false,
        }
        expect(handleKey({ matcher, atom: 'Escape', facts: escapeInField, phase: 'capture' })).toBe(
            true
        )
        expect(cancel).toHaveBeenCalledTimes(1)

        const escapeOutside: KeyFacts = {
            key: 'Escape',
            ctrlKey: false,
            metaKey: false,
            altKey: false,
            inInput: false,
            layerTakesEscape: false,
            menuOpen: false,
        }
        expect(handleKey({ matcher, atom: 'Escape', facts: escapeOutside, phase: 'capture' })).toBe(
            false
        )
        expect(handleKey({ matcher, atom: 'Escape', facts: escapeOutside, phase: 'bubble' })).toBe(
            true
        )
        expect(cancel).toHaveBeenCalledTimes(2)
    })

    it('keeps sequences working through the bubble binding', () => {
        const run = vi.fn()
        register({ id: 't.gi', keys: 'g i', scope: 'global', description: 'go inbox', run })
        const gKey: KeyFacts = {
            key: 'g',
            ctrlKey: false,
            metaKey: false,
            altKey: false,
            inInput: false,
            layerTakesEscape: false,
            menuOpen: false,
        }
        const iKey: KeyFacts = {
            key: 'i',
            ctrlKey: false,
            metaKey: false,
            altKey: false,
            inInput: false,
            layerTakesEscape: false,
            menuOpen: false,
        }

        const matcher = createMatcher()
        expect(handleKey({ matcher, atom: 'g', facts: gKey, phase: 'capture' })).toBe(false)
        expect(handleKey({ matcher, atom: 'g', facts: gKey, phase: 'bubble' })).toBe(true)
        expect(run).not.toHaveBeenCalled()
        expect(handleKey({ matcher, atom: 'i', facts: iKey, phase: 'bubble' })).toBe(true)
        expect(run).toHaveBeenCalledTimes(1)

        // The capture call must feed nothing, or this "i" would finish "g i".
        const other = createMatcher()
        expect(handleKey({ matcher: other, atom: 'g', facts: gKey, phase: 'capture' })).toBe(false)
        expect(handleKey({ matcher: other, atom: 'i', facts: iKey, phase: 'bubble' })).toBe(false)
        expect(run).toHaveBeenCalledTimes(1)
    })

    // Core menus and popovers push no shortcut scope, so the screen's scope
    // stays on top under them. Before the capture binding, the menu's search
    // field and its focused trigger stopped ⌘↵; a capture-phase dispatch would
    // run the screen's Post from inside an open menu.
    it('an open menu keeps the old path', () => {
        expect(
            keyPhase({
                key: 'Enter',
                ctrlKey: false,
                metaKey: true,
                altKey: false,
                inInput: false,
                layerTakesEscape: true,
                menuOpen: true,
            })
        ).toBe('bubble')
        expect(
            keyPhase({
                key: 'Enter',
                ctrlKey: false,
                metaKey: true,
                altKey: false,
                inInput: true,
                layerTakesEscape: true,
                menuOpen: true,
            })
        ).toBe('bubble')
        expect(
            keyPhase({
                key: 'j',
                ctrlKey: false,
                metaKey: false,
                altKey: false,
                inInput: true,
                layerTakesEscape: true,
                menuOpen: true,
            })
        ).toBe('bubble')
        expect(
            keyPhase({
                key: 'Escape',
                ctrlKey: false,
                metaKey: false,
                altKey: false,
                inInput: false,
                layerTakesEscape: true,
                menuOpen: true,
            })
        ).toBeNull()

        const post = vi.fn()
        const composeId = pushScope('compose')
        register({
            id: 't.post',
            keys: '$mod+Enter',
            scope: 'compose',
            scopeId: composeId,
            description: 'Post',
            allowInInputs: true,
            run: post,
        })
        const matcher = createMatcher()
        const modEnterInMenuField: KeyFacts = {
            key: 'Enter',
            ctrlKey: false,
            metaKey: true,
            altKey: false,
            inInput: true,
            layerTakesEscape: true,
            menuOpen: true,
        }
        expect(
            handleKey({ matcher, atom: '$mod+Enter', facts: modEnterInMenuField, phase: 'capture' })
        ).toBe(false)
        expect(post).not.toHaveBeenCalled()
    })

    // A react-native-web text field stops every key before the bubble binding,
    // so before the capture binding a global `allowInInputs` shortcut never
    // fired from a field (the field's own handler, such as a find box, kept
    // it). Capture takes only the screen's scoped keys from a field; a global
    // key there stays on the bubble path.
    it('a global shortcut keeps the old path from a text field', () => {
        const find = vi.fn()
        const post = vi.fn()
        register({
            id: 't.find',
            keys: '$mod+f',
            scope: 'global',
            description: 'Find',
            allowInInputs: true,
            run: find,
        })
        const composeId = pushScope('compose')
        register({
            id: 't.post',
            keys: '$mod+Enter',
            scope: 'compose',
            scopeId: composeId,
            description: 'Post',
            allowInInputs: true,
            run: post,
        })
        const matcher = createMatcher()

        const modFInField: KeyFacts = {
            key: 'f',
            ctrlKey: false,
            metaKey: true,
            altKey: false,
            inInput: true,
            layerTakesEscape: false,
            menuOpen: false,
        }
        expect(handleKey({ matcher, atom: '$mod+f', facts: modFInField, phase: 'capture' })).toBe(
            false
        )
        expect(find).not.toHaveBeenCalled()
        expect(handleKey({ matcher, atom: '$mod+f', facts: modFInField, phase: 'bubble' })).toBe(
            true
        )
        // A contenteditable surface lets the key through, as before.
        expect(find).toHaveBeenCalledTimes(1)

        const modEnterInField: KeyFacts = {
            key: 'Enter',
            ctrlKey: false,
            metaKey: true,
            altKey: false,
            inInput: true,
            layerTakesEscape: false,
            menuOpen: false,
        }
        expect(
            handleKey({ matcher, atom: '$mod+Enter', facts: modEnterInField, phase: 'capture' })
        ).toBe(true)
        expect(post).toHaveBeenCalledTimes(1)
        expect(
            handleKey({ matcher, atom: '$mod+Enter', facts: modEnterInField, phase: 'bubble' })
        ).toBe(false)
        expect(post).toHaveBeenCalledTimes(1)

        // Outside a text field a modified key feeds every shortcut in capture.
        const modFOnButton: KeyFacts = {
            key: 'f',
            ctrlKey: false,
            metaKey: true,
            altKey: false,
            inInput: false,
            layerTakesEscape: false,
            menuOpen: false,
        }
        expect(handleKey({ matcher, atom: '$mod+f', facts: modFOnButton, phase: 'capture' })).toBe(
            true
        )
        expect(find).toHaveBeenCalledTimes(2)
        expect(handleKey({ matcher, atom: '$mod+f', facts: modFOnButton, phase: 'bubble' })).toBe(
            false
        )
        expect(find).toHaveBeenCalledTimes(2)

        // The matcher's `only` filter: 'scoped' skips global shortcuts and
        // 'global' skips scoped ones.
        expect(createMatcher().feedAtom('$mod+f', { inInput: true, only: 'scoped' })).toBe(false)
        expect(createMatcher().feedAtom('$mod+Enter', { inInput: true, only: 'global' })).toBe(
            false
        )
        expect(find).toHaveBeenCalledTimes(2)
        expect(post).toHaveBeenCalledTimes(1)
    })
})

describe('help list', () => {
    beforeEach(resetShortcutState)
    afterEach(resetShortcutState)

    it('the help floor names the stack below a scope that is not pushed yet', () => {
        const composeId = pushScope('compose')
        const floor = nextScopeId()
        expect(nextScopeId()).toBe(floor)
        expect(pushScope('modal')).toBe(floor)

        expect(topScopeEntry()).toEqual({ id: floor, scope: 'modal' })
        expect(topScopeEntry(floor)).toEqual({ id: composeId, scope: 'compose' })
        expect(topScopeEntry(1)).toBeNull()

        const post: Shortcut = {
            id: 't.post',
            keys: '$mod+Enter',
            scope: 'compose',
            scopeId: composeId,
            description: 'Post',
            run: () => {},
        }
        const close: Shortcut = {
            id: 't.close',
            keys: 'Escape',
            scope: 'modal',
            scopeId: floor,
            description: 'Close',
            run: () => {},
        }
        // The live top is the modal; an explicit entry overrides it.
        expect(isScopeActive(post)).toBe(false)
        expect(isScopeActive(post, topScopeEntry(floor))).toBe(true)
        expect(isScopeActive(close)).toBe(true)
        expect(isScopeActive(close, topScopeEntry(floor))).toBe(false)
    })

    it('leaves out a global shortcut that an active scoped one with the same keys shadows', () => {
        register({
            id: 't.search',
            keys: '/',
            scope: 'global',
            description: 'Search',
            group: 'Navigation',
            run: () => {},
        })
        const listId = pushScope('list')
        register({
            id: 't.find',
            keys: '/',
            scope: 'list',
            scopeId: listId,
            description: 'Find in list',
            group: 'List',
            run: () => {},
        })
        const shortcuts = useShortcutRegistry.getState().shortcuts

        const shadowed = helpGroups(shortcuts, null)
        expect(Object.keys(shadowed)).toEqual(['List'])
        expect(shadowed.List.map(s => s.id)).toEqual(['t.find'])

        popScope(listId)
        const unshadowed = helpGroups(shortcuts, null)
        expect(Object.keys(unshadowed)).toEqual(['Navigation'])
        expect(unshadowed.Navigation.map(s => s.id)).toEqual(['t.search'])
    })

    it('lists the shortcuts of the screen under the ⇧? overlay', () => {
        const listId = pushScope('list')
        register({
            id: 't.next',
            keys: 'j',
            scope: 'list',
            scopeId: listId,
            description: 'Next',
            group: 'List',
            run: () => {},
        })
        const composeId = pushScope('compose')
        register({
            id: 't.post',
            keys: '$mod+Enter',
            scope: 'compose',
            scopeId: composeId,
            description: 'Post',
            group: 'Transfer',
            allowInInputs: true,
            run: () => {},
        })
        register({
            id: 't.jump',
            keys: 't m',
            scope: 'global',
            description: 'Jump to mail',
            group: 'Navigation',
            run: () => {},
        })
        register({
            id: 't.help',
            keys: 'Shift+?',
            scope: 'global',
            description: 'Show keyboard shortcuts',
            group: 'Help',
            run: () => {},
        })

        const floorBeforeOpen = nextScopeId()
        useShortcutHelpStore.getState().open()
        const { scopeFloor } = useShortcutHelpStore.getState()
        expect(scopeFloor).toBe(floorBeforeOpen)

        // The overlay's LayerEscape mounts after open() and stamps its own Close.
        const modalId = pushScope('modal')
        register({
            id: 't.close',
            keys: 'Escape',
            scope: 'modal',
            scopeId: modalId,
            description: 'Close',
            allowInInputs: true,
            run: () => {},
        })
        const shortcuts = useShortcutRegistry.getState().shortcuts

        const underOverlay = helpGroups(shortcuts, scopeFloor)
        expect(Object.keys(underOverlay).sort()).toEqual(['Navigation', 'Transfer'])
        expect(underOverlay.Transfer.map(s => s.id)).toEqual(['t.post'])
        expect(underOverlay.Navigation.map(s => s.id)).toEqual(['t.jump'])

        const liveTop = helpGroups(shortcuts, null)
        expect(Object.keys(liveTop).sort()).toEqual(['General', 'Navigation'])
        expect(liveTop.General.map(s => s.id)).toEqual(['t.close'])
        expect(liveTop.Navigation.map(s => s.id)).toEqual(['t.jump'])

        useShortcutHelpStore.getState().close()
        expect(useShortcutHelpStore.getState().scopeFloor).toBeNull()

        const floorBeforeToggle = nextScopeId()
        useShortcutHelpStore.getState().toggle()
        expect(useShortcutHelpStore.getState().isOpen).toBe(true)
        expect(useShortcutHelpStore.getState().scopeFloor).toBe(floorBeforeToggle)
        useShortcutHelpStore.getState().toggle()
        expect(useShortcutHelpStore.getState().isOpen).toBe(false)
        expect(useShortcutHelpStore.getState().scopeFloor).toBeNull()
    })
})
