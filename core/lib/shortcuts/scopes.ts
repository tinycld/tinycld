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

export interface ScopeEntry {
    id: number
    scope: Scope
    /**
     * The newer scopes this entry stays active under (see
     * `activeScopeEntries`). Absent for an entry that did not opt in, which is
     * shadowed by any newer entry.
     */
    activeUnder?: readonly Scope[]
}

export interface ScopeOptions {
    /**
     * Keep this scope's shortcuts live while a newer entry of one of these
     * scopes is on top — a list screen that keeps its own keys while an inline
     * form on it holds the keyboard. On a key both own, the newer scope wins.
     *
     * An entry with this option is never placed above an entry of a scope it
     * names: pushed while such entries are on top, it goes below them. React
     * runs a child's effects first, so a list can be pushed after the `compose`
     * entry of the document it holds, and on top it would shadow that entry.
     */
    activeUnder?: readonly Scope[]
}

const stack: ScopeEntry[] = []
let nextId = 1

// `activeUnder` is set only when given, so an entry that did not opt in has
// the same shape as before the option existed.
function makeEntry(
    id: number,
    scope: Scope,
    activeUnder: readonly Scope[] | undefined
): ScopeEntry {
    return activeUnder === undefined ? { id, scope } : { id, scope, activeUnder }
}

/**
 * Put an entry on the stack. A `modal` entry goes on top. A non-modal entry
 * goes below the run of `modal` entries at the top. Then an entry with
 * `activeUnder` also goes below the run of entries there whose scope it names;
 * an entry without it stays where it is.
 *
 * Why: React runs a child's effects before its parent's, and a nested navigator
 * can focus a parent route and its child route in one commit.
 * - A dialog (`modal`) that mounts in the same commit as its screen is pushed
 *   BEFORE the screen's entry. On top, the screen would shadow the dialog, and
 *   ⌘↵ in the dialog would run the screen's ⌘↵. A dialog is always meant to be
 *   above the screen it opens over.
 * - A list that stays active under `compose` can be pushed AFTER the `compose`
 *   entry of the document it holds. On top, the list would cut the chain
 *   (`compose` did not opt in) and every key of the document would be dead. An
 *   entry of a scope the list names is always meant to be above it.
 */
function insertEntry(entry: ScopeEntry) {
    let index = stack.length
    if (entry.scope !== 'modal') {
        while (index > 0 && stack[index - 1].scope === 'modal') index -= 1
    }
    const activeUnder = entry.activeUnder
    if (activeUnder) {
        while (index > 0 && activeUnder.includes(stack[index - 1].scope)) index -= 1
    }
    stack.splice(index, 0, entry)
}

export function pushScope(scope: Scope, options?: ScopeOptions): number {
    const id = nextId++
    insertEntry(makeEntry(id, scope, options?.activeUnder))
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
 * mounted: mail's list and a Boards board can both hold registered 'list'
 * shortcuts at the same time, and several of them collide (j, k, x). Scope
 * alone cannot separate them; the owning instance can.
 */
export function topScopeId(): number | null {
    return stack.length > 0 ? stack[stack.length - 1].id : null
}

/**
 * The id the next scope instance will be assigned, without taking it. Ids only
 * grow, so every scope pushed after this call has an id at or above it — which
 * is how the shortcut help overlay tells its own `modal` entry apart from the
 * stack of the screen it was opened over.
 */
export function nextScopeId(): number {
    return nextId
}

/**
 * The top entry whose id is below `floor`, or the live top when no floor is
 * given. Null when no such entry is on the stack.
 */
export function topScopeEntry(floor?: number): ScopeEntry | null {
    for (let i = stack.length - 1; i >= 0; i -= 1) {
        const entry = stack[i]
        if (floor === undefined || entry.id < floor) return { ...entry }
    }
    return null
}

/**
 * The scope entries whose shortcuts can fire, newest first, starting at `top`.
 *
 * Walking down the stack from `top`, the next lower entry is active too when
 * its `activeUnder` names the scope of every active entry above it; the walk
 * stops at the first entry that does not. So an entry that did not opt in, or
 * a scope it did not name (a `modal` dialog), shadows everything below it, as
 * when only the top entry was ever active. A `top` that is not on the stack is
 * the whole chain.
 */
export function activeScopeChain(top: ScopeEntry | null): ScopeEntry[] {
    if (!top) return []
    const chain: ScopeEntry[] = [{ ...top }]
    const index = stack.findIndex(e => e.id === top.id)
    for (let i = index - 1; i >= 0; i -= 1) {
        const entry = stack[i]
        const activeUnder = entry.activeUnder
        if (!activeUnder) break
        if (!chain.every(above => activeUnder.includes(above.scope))) break
        chain.push({ ...entry })
    }
    return chain
}

/**
 * The active scope entries, newest first: the top entry below `floor` (the
 * live top when no floor is given) and the entries under it that stay active
 * (`activeScopeChain`). Empty when no entry is below the floor.
 */
export function activeScopeEntries(floor?: number): ScopeEntry[] {
    return activeScopeChain(topScopeEntry(floor))
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
 * fires a shortcut only when its scope === 'global' or it belongs to an active
 * scope entry (the top of the stack, and the entries under it that stay active
 * under it, `activeScopeEntries`).
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
 * landing while a Boards board held the keyboard stamped mail's `j` with the
 * BOARD's id, and — mail's entry being first in the registry's insertion order
 * — the matcher fired mail's handler for a keypress meant for the board, which
 * did nothing visible. That is the bug this hook's split of identity (mount)
 * from stack membership (focus) exists to prevent.
 *
 * `options.activeUnder` keeps this scope active under a newer entry of the
 * named scopes (see `activeScopeEntries`).
 */
export function useShortcutScope(scope: Scope, options?: ScopeOptions): ScopeOwner {
    // The id is assigned ONCE and kept for this instance's whole life. It is
    // read through an accessor rather than returned as a value because no id
    // exists during the render that sets this up; it is assigned on the first
    // READ or on mount, whichever comes first.
    //
    // `useState` (not `useRef`) for the box so its identity is created once and
    // the accessor closes over something stable — the register hooks list the
    // accessor in their effect deps, and a new function per render would
    // re-register on every render.
    const [owner] = useState<{ id: number | null; read: ScopeOwner }>(() => {
        const box: { id: number | null; read: ScopeOwner } = {
            id: null,
            // Assigned on first READ as well as on mount: a descendant that
            // registers through ScopeProvider runs its effect BEFORE this
            // component's own (React runs child effects first), and would
            // otherwise stamp its shortcut null, which never matches.
            read: () => {
                if (box.id === null) box.id = nextId++
                return box.id
            },
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
        owner.read()
        return () => {
            if (owner.id !== null) popScope(owner.id)
            owner.id = null
        }
    }, [owner])

    // Keyed on the option's content, not its identity: a caller that passes a
    // new array literal each render must not pop and re-push its entry (which
    // would move it above a newer entry) on every render.
    const activeUnderKey = activeUnderKeyOf(options?.activeUnder)

    useFocusEffect(
        useCallback(() => {
            const id = owner.read()
            if (id === null) return
            insertEntry(makeEntry(id, scope, activeUnderFromKey(activeUnderKey)))
            return () => popScope(id)
        }, [scope, owner, activeUnderKey])
    )

    return owner.read
}

function activeUnderKeyOf(activeUnder: readonly Scope[] | undefined): string | null {
    if (activeUnder === undefined) return null
    return JSON.stringify([...new Set(activeUnder)].sort())
}

function activeUnderFromKey(key: string | null): readonly Scope[] | undefined {
    if (key === null) return undefined
    return JSON.parse(key) as Scope[]
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
