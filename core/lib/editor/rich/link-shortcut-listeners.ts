import { useMemo, useRef } from 'react'

/**
 * The listeners of one editor's ⌘/Ctrl+K (`EditorCommands.subscribeLinkShortcut`).
 * Both hooks keep them here: the key fires inside the editor (LinkShortcut),
 * and the toolbar that opens the Link dialog is a separate component.
 * Stable for the hook's life, so the editor is not rebuilt for a subscriber.
 */
export function useLinkShortcutListeners() {
    const listenersRef = useRef<Set<() => void> | null>(null)
    if (listenersRef.current === null) listenersRef.current = new Set()
    const listeners = listenersRef.current

    return useMemo(
        () => ({
            /** Calls every listener; false when there is none, so the key is not taken. */
            fire: () => {
                for (const listener of listeners) listener()
                return listeners.size > 0
            },
            subscribe: (listener: () => void) => {
                listeners.add(listener)
                return () => {
                    listeners.delete(listener)
                }
            },
        }),
        [listeners]
    )
}
