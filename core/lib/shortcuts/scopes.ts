import { useFocusEffect } from 'expo-router'
import { useCallback, useRef } from 'react'
import type { Scope } from './types'

const stack: { id: number; scope: Scope }[] = []
let nextId = 1

export function pushScope(scope: Scope): number {
    const id = nextId++
    stack.push({ id, scope })
    return id
}

export function popScope(id: number) {
    const idx = stack.findIndex(e => e.id === id)
    if (idx !== -1) stack.splice(idx, 1)
}

export function topScope(): Scope | null {
    return stack.length > 0 ? stack[stack.length - 1].scope : null
}

/**
 * The id of the scope entry currently on top — which screen owns the keyboard
 * right now, as opposed to merely which KIND of screen it is.
 *
 * The distinction matters because `freezeOnBlur` leaves a departed screen
 * mounted: mail's list and a cards board can both hold registered 'list'
 * shortcuts at the same time, and several of them collide (j, k, x). Scope
 * alone cannot separate them; the owning instance can.
 */
export function topScopeId(): number | null {
    return stack.length > 0 ? stack[stack.length - 1].id : null
}

/** The id a scope-bound shortcut registered under, for `Shortcut.scopeId`. */
export function currentScopeId(scope: Scope): number | null {
    for (let i = stack.length - 1; i >= 0; i -= 1) {
        if (stack[i].scope === scope) return stack[i].id
    }
    return null
}

/** Reset all scope state — for use in tests only. */
export function resetScopes() {
    stack.length = 0
    nextId = 1
}

/**
 * Push a scope onto the stack while the owning route is focused. The matcher
 * fires a shortcut only when its scope === 'global' or matches the top of the
 * stack.
 *
 * Keyed on FOCUS, not mount: the package tabs render with `freezeOnBlur`, which
 * leaves a blurred screen mounted. A mount-keyed push therefore never popped
 * when the user switched packages — the departed screen's scope stayed on top,
 * and every shortcut belonging to the package they switched BACK to silently
 * stopped matching (resuming a frozen screen remounts nothing, so it pushed
 * nothing). useFocusEffect's cleanup runs on blur, including the blur that
 * precedes a freeze, and re-runs on refocus.
 */
export function useShortcutScope(scope: Scope) {
    const idRef = useRef<number | null>(null)
    useFocusEffect(
        useCallback(() => {
            idRef.current = pushScope(scope)
            return () => {
                if (idRef.current !== null) popScope(idRef.current)
                idRef.current = null
            }
        }, [scope])
    )
}
