import { useMemo, useRef } from 'react'

/**
 * The listeners of one editor's ⌘/Ctrl+K (`EditorCommands.subscribeLinkShortcut`).
 * Both hooks keep them here: the key fires inside the editor (LinkShortcut),
 * and the toolbar that opens the Link dialog is a separate component.
 * Stable for the hook's life, so the editor is not rebuilt for a subscriber.
 *
 * `hasListeners` + `subscribePresence` are a `useSyncExternalStore` source:
 * the native hook tells the WebView page whether to take the key at all.
 */
export function useLinkShortcutListeners() {
    const listenersRef = useRef<Set<() => void> | null>(null)
    if (listenersRef.current === null) listenersRef.current = new Set()
    const listeners = listenersRef.current
    const presenceRef = useRef<Set<() => void> | null>(null)
    if (presenceRef.current === null) presenceRef.current = new Set()
    const presence = presenceRef.current

    return useMemo(() => {
        const notifyPresence = () => {
            for (const onChange of presence) onChange()
        }
        return {
            /** Calls every listener; false when there is none, so the key is not taken. */
            fire: () => {
                for (const listener of listeners) listener()
                return listeners.size > 0
            },
            subscribe: (listener: () => void) => {
                listeners.add(listener)
                notifyPresence()
                return () => {
                    listeners.delete(listener)
                    notifyPresence()
                }
            },
            hasListeners: () => listeners.size > 0,
            subscribePresence: (onChange: () => void) => {
                presence.add(onChange)
                return () => {
                    presence.delete(onChange)
                }
            },
        }
    }, [listeners, presence])
}
