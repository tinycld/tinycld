import type { ScopeOwner } from '@tinycld/core/lib/shortcuts/scopes'
import type { Scope, Shortcut } from '@tinycld/core/lib/shortcuts/types'
import { useRegisterShortcut } from '@tinycld/core/lib/shortcuts/use-register'
import { useLayerActive } from '@tinycld/core/ui/overlay/escape'
import { useId, useMemo, useRef } from 'react'

export interface ButtonShortcut {
    keys: string
    /**
     * Required: a `global` shortcut stays live on a screen the user has left
     * (`freezeOnBlur` keeps it mounted), so the caller must choose.
     */
    scope: Scope
    allowInInputs?: boolean
    /** The heading the `?` overlay lists it under. */
    group?: string
}

interface Options {
    label: string
    shortcut: ButtonShortcut
    isDisabled: boolean
    onPress: () => void
    /**
     * The owner `useShortcutScope` returned, for a button on a screen. A
     * dialog's content gets its owner from the dialog; a screen's button
     * registers before the screen's own scope effect runs, so without the
     * owner it is stamped null and never fires.
     */
    scopeOwner?: ScopeOwner
}

/**
 * Register the button's shortcut, described by its label in the `?` overlay.
 * The latest `onPress` and disabled state are read through a ref so an inline
 * handler does not re-register on every render. A disabled button's `when`
 * is false, so the matcher does not consume the key and it reaches whatever
 * has focus, as if no shortcut existed. A closing layer counts as disabled.
 */
export function useButtonShortcut({ label, shortcut, isDisabled, onPress, scopeOwner }: Options) {
    const instance = useId()
    const isLayerActive = useLayerActive()
    const latest = useRef({ isDisabled: isDisabled || !isLayerActive, onPress })
    latest.current = { isDisabled: isDisabled || !isLayerActive, onPress }
    const { keys, scope, allowInInputs, group } = shortcut
    const registered = useMemo<Shortcut>(
        () => ({
            id: `shortcut-button.${instance}`,
            keys,
            scope,
            allowInInputs,
            group,
            description: label,
            when: () => !latest.current.isDisabled,
            run: () => latest.current.onPress(),
        }),
        [instance, keys, scope, allowInInputs, group, label]
    )
    useRegisterShortcut(registered, scopeOwner)
}
