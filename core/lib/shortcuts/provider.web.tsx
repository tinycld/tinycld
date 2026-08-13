import { type ReactNode, useEffect, useRef } from 'react'
import { tinykeys } from 'tinykeys'
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
 * while the event is still bubbling, so by the time it reaches this window
 * listener the editor is gone and `activeElement` is BODY — and a shortcut
 * like the card peek's Escape would fire on the very keystroke the editor
 * already handled, closing the panel on the FIRST Escape instead of the
 * second. The target is immutable for the dispatch, so it still names the
 * input the key was actually typed into. (`isContentEditable` is computed
 * from layout and reads false on a node React has already detached, hence the
 * attribute fallback beside it.)
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

        const bindings: Record<string, (event: KeyboardEvent) => void> = {}
        for (const atom of atoms) {
            bindings[atom] = event => {
                const consumed = matcherRef.current.feedAtom(atom, {
                    inInput: isInputKeyEvent(event),
                })
                if (consumed) event.preventDefault()
            }
        }

        return tinykeys(window, bindings)
    }, [atomsKey])

    return <>{children}</>
}
