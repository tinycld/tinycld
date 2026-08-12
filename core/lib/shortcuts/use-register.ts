import { useEffect } from 'react'
import { useShortcutRegistry } from './registry'
import { currentScopeId, type ScopeOwner, useScopeOwner } from './scopes'
import type { Shortcut } from './types'

/**
 * Stamp the scope instance that owns this shortcut, so the matcher can tell a
 * live screen's 'list' shortcuts from those of a blurred screen that still has
 * the same keys registered. A 'global' shortcut belongs to no instance.
 *
 * `owner` reads the id the caller's own `useShortcutScope` was assigned on
 * mount. It is preferred over `currentScopeId` because the two disagree exactly
 * when it matters: `currentScopeId` re-reads the mutable stack, so it answers
 * "who holds this scope NOW", while the stamp must record "who is REGISTERING".
 * Those differ whenever a blurred-but-mounted screen re-registers — on web
 * `freezeOnBlur` only hides the subtree, so its live queries keep emitting and
 * its shortcut array keeps changing identity — and the blurred screen would
 * otherwise stamp itself with the FOCUSED screen's id, outranking it in the
 * matcher and firing the wrong package's handler for a colliding key.
 *
 * An owner's id is stable for its instance's whole life, so a blurred screen
 * keeps stamping itself and simply stops matching (its scope entry is off the
 * stack, so it can never be `topScopeId()`). Only the ABSENCE of an owner falls
 * back to `currentScopeId`, preserving behavior for callers that register scoped
 * shortcuts with no `useShortcutScope` above them.
 */
function withScopeId(shortcut: Shortcut, owner: ScopeOwner | null): Shortcut {
    if (shortcut.scope === 'global') return shortcut
    const scopeId = owner ? owner() : currentScopeId(shortcut.scope)
    return { ...shortcut, scopeId }
}

/**
 * Register a shortcut for the lifetime of the component. Pass a stable
 * `Shortcut` object (e.g. memoised or module-level) to avoid re-registering
 * on every render.
 */
export function useRegisterShortcut(
    shortcut: Shortcut | null | false | undefined,
    owner?: ScopeOwner
) {
    const register = useShortcutRegistry(s => s.register)
    const unregister = useShortcutRegistry(s => s.unregister)
    const contextOwner = useScopeOwner()
    const scopeOwner = owner ?? contextOwner

    useEffect(() => {
        if (!shortcut) return
        register(withScopeId(shortcut, scopeOwner))
        return () => unregister(shortcut.id)
    }, [shortcut, scopeOwner, register, unregister])
}

/**
 * Register multiple shortcuts. The array identity is tracked; pass a stable
 * array (module-level or `useMemo`) to avoid thrash.
 */
export function useRegisterShortcuts(shortcuts: Shortcut[], owner?: ScopeOwner) {
    const register = useShortcutRegistry(s => s.register)
    const unregister = useShortcutRegistry(s => s.unregister)
    const contextOwner = useScopeOwner()
    const scopeOwner = owner ?? contextOwner

    useEffect(() => {
        for (const s of shortcuts) register(withScopeId(s, scopeOwner))
        return () => {
            for (const s of shortcuts) unregister(s.id)
        }
    }, [shortcuts, scopeOwner, register, unregister])
}
