import { useEffect } from 'react'
import { useShortcutRegistry } from './registry'
import { currentScopeId } from './scopes'
import type { Shortcut } from './types'

/**
 * Stamp the scope instance that owns this shortcut, so the matcher can tell a
 * live screen's 'list' shortcuts from those of a frozen screen that still has
 * the same keys registered. A 'global' shortcut belongs to no instance.
 *
 * Call `useShortcutScope(scope)` ABOVE the register hook in the same component:
 * effects run in call order, so the scope must already be pushed for the
 * shortcut to be stamped with it rather than with an outer screen's instance.
 */
function withScopeId(shortcut: Shortcut): Shortcut {
    if (shortcut.scope === 'global') return shortcut
    return { ...shortcut, scopeId: currentScopeId(shortcut.scope) }
}

/**
 * Register a shortcut for the lifetime of the component. Pass a stable
 * `Shortcut` object (e.g. memoised or module-level) to avoid re-registering
 * on every render.
 */
export function useRegisterShortcut(shortcut: Shortcut | null | false | undefined) {
    const register = useShortcutRegistry(s => s.register)
    const unregister = useShortcutRegistry(s => s.unregister)

    useEffect(() => {
        if (!shortcut) return
        register(withScopeId(shortcut))
        return () => unregister(shortcut.id)
    }, [shortcut, register, unregister])
}

/**
 * Register multiple shortcuts. The array identity is tracked; pass a stable
 * array (module-level or `useMemo`) to avoid thrash.
 */
export function useRegisterShortcuts(shortcuts: Shortcut[]) {
    const register = useShortcutRegistry(s => s.register)
    const unregister = useShortcutRegistry(s => s.unregister)

    useEffect(() => {
        for (const s of shortcuts) register(withScopeId(s))
        return () => {
            for (const s of shortcuts) unregister(s.id)
        }
    }, [shortcuts, register, unregister])
}
