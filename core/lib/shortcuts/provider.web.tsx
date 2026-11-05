import { topLayer } from '@tinycld/core/ui/overlay/layer-stack'
import { type ReactNode, useEffect, useRef } from 'react'
import { tinykeys } from 'tinykeys'
import { handleKey, type KeyFacts } from './dispatch'
import { atomsForShortcuts, createMatcher } from './matcher'
import { useShortcutRegistry } from './registry'

export interface ShortcutsProviderProps {
    children: ReactNode
}

/**
 * Did this key event come from a text input? The matcher skips most shortcuts
 * when it did (unless `allowInInputs` is set on the shortcut).
 *
 * Decided from the event TARGET, not `document.activeElement`. The two
 * disagree in exactly the case that matters: an editor whose Escape handler
 * blurs or unmounts it. React flushes discrete-event updates synchronously
 * while the event is still propagating, so by the time a plain key reaches
 * the bubble-phase window listener the editor is gone and `activeElement` is
 * BODY — and a shortcut like the card peek's Escape would fire on the very
 * keystroke the editor already handled, closing the panel on the FIRST Escape
 * instead of the second. The target is immutable for the dispatch, so it
 * still names the input the key was actually typed into. (`isContentEditable`
 * is computed from layout and reads false on a node React has already
 * detached, hence the attribute fallback beside it.)
 */
export function isInputKeyEvent(event: KeyboardEvent): boolean {
    const targets = [
        event.target as HTMLElement | null,
        document.activeElement as HTMLElement | null,
    ]
    for (const el of targets) {
        if (!el || typeof el.closest !== 'function') continue
        const tag = el.tagName
        if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true
        if (el.isContentEditable) return true
        if (el.closest('[contenteditable="true"], [contenteditable=""], .ProseMirror')) return true
    }
    return false
}

// One reading per key, taken by the capture listener and reused by the bubble
// listener. A handler between the two can move focus or open a layer; judging
// the key again then could feed it twice or drop it.
const factsByEvent = new WeakMap<KeyboardEvent, KeyFacts>()

function factsOf(event: KeyboardEvent): KeyFacts {
    const known = factsByEvent.get(event)
    if (known) return known
    const layer = topLayer()
    const facts: KeyFacts = {
        key: event.key,
        ctrlKey: event.ctrlKey,
        metaKey: event.metaKey,
        altKey: event.altKey,
        inInput: isInputKeyEvent(event),
        layerTakesEscape: layer?.dismissOnEscape === true,
        menuOpen: layer !== null && !layer.isModal,
    }
    factsByEvent.set(event, facts)
    return facts
}

export function ShortcutsProvider({ children }: ShortcutsProviderProps) {
    const matcherRef = useRef(createMatcher())

    // Rebind tinykeys whenever the set of unique single atoms changes. We
    // diff by the sorted atom list so adding or removing a shortcut that
    // shares atoms with an existing one doesn't force a rebind.
    const atomsKey = useShortcutRegistry(state => {
        return atomsForShortcuts(state.shortcuts.values()).sort().join('|')
    })

    useEffect(() => {
        const atoms = atomsKey ? atomsKey.split('|') : []
        if (atoms.length === 0) return

        // Two listeners, and `handleKey` (dispatch.ts) decides which one
        // offers a key to which shortcuts, so no shortcut is offered a key
        // twice: capture for a key held with a modifier and for a scoped
        // shortcut's key typed in a text field (no menu open), bubble for the
        // rest.
        const capture: Record<string, (event: KeyboardEvent) => void> = {}
        const bubble: Record<string, (event: KeyboardEvent) => void> = {}
        for (const atom of atoms) {
            capture[atom] = event => {
                const matcher = matcherRef.current
                if (!handleKey({ matcher, atom, facts: factsOf(event), phase: 'capture' })) return
                // The focused element must not see a key a shortcut took: a
                // Pressable would press itself, a field would type it.
                event.preventDefault()
                event.stopPropagation()
            }
            bubble[atom] = event => {
                const matcher = matcherRef.current
                if (handleKey({ matcher, atom, facts: factsOf(event), phase: 'bubble' })) {
                    event.preventDefault()
                }
            }
        }

        const unbindCapture = tinykeys(window, capture, { capture: true })
        const unbindBubble = tinykeys(window, bubble)
        return () => {
            unbindCapture()
            unbindBubble()
        }
    }, [atomsKey])

    return <>{children}</>
}
