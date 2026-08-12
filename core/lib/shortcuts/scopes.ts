import { useFocusEffect } from 'expo-router'
import {
    createContext,
    createElement,
    type ReactNode,
    useCallback,
    useContext,
    useEffect,
    useState,
} from 'react'
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
 * Reads the id its `useShortcutScope` instance was assigned, at call time.
 *
 * A function rather than the id itself because the assignment happens in an
 * effect: the value does not exist during the render that sets this up, but it
 * always does by the time a register effect runs.
 */
export type ScopeOwner = () => number | null

/**
 * The scope instance a subtree's shortcuts belong to.
 *
 * `null` means "no owning scope in this subtree" — the register hooks then fall
 * back to `currentScopeId`, preserving behavior for the callers that register
 * scoped shortcuts without an accompanying `useShortcutScope` above them.
 */
const ScopeOwnerContext = createContext<ScopeOwner | null>(null)

/**
 * The nearest enclosing `useShortcutScope`'s owner accessor, or null when there
 * is none. Read by the register hooks so a re-registration reuses its OWNER's
 * id instead of re-deriving one from the live stack.
 */
export function useScopeOwner(): ScopeOwner | null {
    return useContext(ScopeOwnerContext)
}

/**
 * Push a scope onto the stack while the owning route is focused. The matcher
 * fires a shortcut only when its scope === 'global' or matches the top of the
 * stack.
 *
 * Stack MEMBERSHIP is keyed on FOCUS, not mount: the package tabs leave a
 * blurred screen mounted, so a mount-keyed push never popped when the user
 * switched packages — the departed screen's scope stayed on top and every
 * shortcut of the package they switched BACK to silently stopped matching.
 * useFocusEffect's cleanup runs on blur and re-runs on refocus, which is what
 * keeps `topScopeId()` naming the screen that actually has the keyboard.
 *
 * Returns an accessor for this instance's id, which the register hooks stamp
 * shortcuts with (and `ScopeProvider` publishes to a descendant subtree).
 * Shortcuts must carry the id of the instance that OWNS them. Re-deriving it on
 * every re-registration (what `currentScopeId` does) makes the stamp depend on
 * WHEN a re-register happens rather than on WHO is registering: on web
 * `freezeOnBlur` only sets `display: none`, so a blurred screen keeps its live
 * queries emitting and re-registers on its own schedule. A mail re-register
 * landing while a cards board held the keyboard stamped mail's `j` with the
 * BOARD's id, and — mail's entry being first in the registry's insertion order
 * — the matcher fired mail's handler for a keypress meant for the board, which
 * did nothing visible. That is the bug this hook's split of identity (mount)
 * from stack membership (focus) exists to prevent.
 */
export function useShortcutScope(scope: Scope): ScopeOwner {
    // The id is assigned ONCE, on mount, and kept for this instance's whole
    // life. It is read through an accessor rather than returned as a value
    // because the assignment happens in an effect: during the render that sets
    // this up there is no id yet, and handing the register effect a null in that
    // same commit is indistinguishable from "this instance owns nothing".
    //
    // `useState` (not `useRef`) for the box so its identity is created once and
    // the accessor closes over something stable — the register hooks list the
    // accessor in their effect deps, and a new function per render would
    // re-register on every render.
    const [owner] = useState<{ id: number | null; read: ScopeOwner }>(() => {
        const box: { id: number | null; read: ScopeOwner } = {
            id: null,
            read: () => box.id,
        }
        return box
    })

    // Mount assigns the identity; focus decides who is on top.
    //
    // These are deliberately separate. The id must exist for the instance's
    // whole life so its shortcuts are always stamped with something stable —
    // `useFocusEffect` fires on ROUTE focus and never re-runs for a dialog
    // opened inside an already-focused route, so keying the identity to focus
    // left every modal shortcut unstamped and unable to fire. Stack MEMBERSHIP
    // still tracks focus, so a blurred screen holds no entry and cannot be on
    // top; its shortcuts keep their id and simply stop matching.
    useEffect(() => {
        owner.id = nextId++
        return () => {
            if (owner.id !== null) popScope(owner.id)
            owner.id = null
        }
    }, [owner])

    useFocusEffect(
        useCallback(() => {
            const id = owner.id
            if (id === null) return
            stack.push({ id, scope })
            return () => popScope(id)
        }, [scope, owner])
    )

    return owner.read
}

/**
 * Publish `useShortcutScope`'s id to the subtree that registers against it.
 *
 * Only needed when the registering component is a DESCENDANT of the one that
 * called `useShortcutScope`; a component doing both itself can pass the
 * returned id straight to the register hook.
 */
export function ScopeProvider({ owner, children }: { owner: ScopeOwner; children: ReactNode }) {
    return createElement(ScopeOwnerContext.Provider, { value: owner }, children)
}
