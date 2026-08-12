import { createMatcher, isScopeActive } from '@tinycld/core/lib/shortcuts/matcher'
import { useShortcutRegistry } from '@tinycld/core/lib/shortcuts/registry'
import {
    currentScopeId,
    popScope,
    pushScope,
    resetScopes,
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
