import { useShortcutScope } from '@tinycld/core/lib/shortcuts/scopes'
import { useRegisterShortcut } from '@tinycld/core/lib/shortcuts/use-register'
import { useMemo, useRef } from 'react'

let instance = 0

/**
 * Rendered inside an open layer: puts the `modal` shortcut scope on top of
 * the stack, which is what mutes the screen's own shortcuts (a list's j/k, a
 * board's letters) while the layer is up, and registers Escape there for the
 * shortcut help. On web the Escape that actually closes the layer is the
 * engine's capture-phase listener (layer-stack.ts); the shortcut dispatch
 * leaves Escape to it while the layer is open (`keyPhase`,
 * lib/shortcuts/dispatch.ts), in or out of a text field.
 *
 * `useShortcutScope` joins the stack on MOUNT, which is why this is a
 * component the layer renders only while open — a hook in an always-mounted
 * menu would hold the modal scope forever.
 */
export function LayerEscape({ onEscape }: { onEscape: () => void }) {
    const scopeOwner = useShortcutScope('modal')
    const id = useMemo(() => `overlay.escape.${++instance}`, [])
    const onEscapeRef = useRef(onEscape)
    onEscapeRef.current = onEscape
    const shortcut = useMemo(
        () =>
            ({
                id,
                keys: 'Escape',
                scope: 'modal' as const,
                description: 'Close',
                allowInInputs: true,
                run: () => onEscapeRef.current(),
            }) as const,
        [id]
    )
    useRegisterShortcut(shortcut, scopeOwner)
    return null
}
