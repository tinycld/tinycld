import type { Scope, Shortcut } from '@tinycld/core/lib/shortcuts/types'
import { useRegisterShortcut } from '@tinycld/core/lib/shortcuts/use-register'
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
}

/**
 * Register the button's shortcut, described by its label in the `?` overlay.
 * The latest `onPress` and disabled state are read through a ref so an inline
 * handler does not re-register on every render. A disabled button's `when`
 * is false, so the matcher does not consume the key and it reaches whatever
 * has focus, as if no shortcut existed.
 */
export function useButtonShortcut({ label, shortcut, isDisabled, onPress }: Options) {
    const instance = useId()
    const latest = useRef({ isDisabled, onPress })
    latest.current = { isDisabled, onPress }
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
    useRegisterShortcut(registered)
}
