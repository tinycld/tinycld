import { ScopeProvider, useShortcutScope } from '@tinycld/core/lib/shortcuts/scopes'
import { useRegisterShortcut } from '@tinycld/core/lib/shortcuts/use-register'
import { type ReactNode, useMemo, useRef } from 'react'

let instance = 0

interface LayerScopeProps {
    onEscape: () => void
    /**
     * False while the layer is still mounted but closing (a sheet's exit
     * animation): the scope stays on top so the screen underneath does not
     * get its keys back mid-animation, but Escape has nothing left to close.
     */
    isEscapeActive?: boolean
    children?: ReactNode
}

/**
 * Rendered around an open layer's content: puts the `modal` shortcut scope on
 * top of the stack, which is what mutes the screen's own shortcuts (a list's
 * j/k, a board's letters) while the layer is up, and registers Escape there
 * for the shortcut help. On web the Escape that actually closes the layer is
 * the engine's capture-phase listener (layer-stack.ts); the shortcut dispatch
 * leaves Escape to it while the layer is open (`keyPhase`,
 * lib/shortcuts/dispatch.ts), in or out of a text field.
 *
 * The content is wrapped in a ScopeProvider so a shortcut registered INSIDE
 * the layer (a ShortcutButton in a dialog footer) is stamped with this
 * layer's id. Without it the button falls back to `currentScopeId`, read in
 * an effect that runs before this one, and is stamped null.
 *
 * `useShortcutScope` joins the stack on MOUNT, which is why this is a
 * component the layer renders only while mounted — a hook in an
 * always-mounted menu would hold the modal scope forever.
 */
export function LayerScope({ onEscape, isEscapeActive = true, children }: LayerScopeProps) {
    const scopeOwner = useShortcutScope('modal')
    const id = useMemo(() => `overlay.escape.${++instance}`, [])
    const latest = useRef({ onEscape, isEscapeActive })
    latest.current = { onEscape, isEscapeActive }
    const shortcut = useMemo(
        () =>
            ({
                id,
                keys: 'Escape',
                scope: 'modal' as const,
                description: 'Close',
                allowInInputs: true,
                when: () => latest.current.isEscapeActive,
                run: () => latest.current.onEscape(),
            }) as const,
        [id]
    )
    useRegisterShortcut(shortcut, scopeOwner)
    return <ScopeProvider owner={scopeOwner}>{children}</ScopeProvider>
}

/** A layer whose content registers no shortcuts of its own (the search palettes). */
export function LayerEscape({ onEscape }: { onEscape: () => void }) {
    return <LayerScope onEscape={onEscape} />
}
