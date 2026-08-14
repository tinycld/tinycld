import { useCallback, useSyncExternalStore } from 'react'

/**
 * Where a host overlay finds the editor it must anchor to.
 *
 * The problem this solves: an anchored overlay needs the editor's `webViewRef`
 * to translate the page's viewport coordinates into screen coordinates, but the
 * popover is rendered as a SIBLING of the editor, not a descendant of it —
 * often in a different subtree entirely (a dialog host, say). React context
 * cannot cross that, and moving the popover under the editor would break the
 * web variant, which deliberately portals to the document body so the card
 * detail's scroll container cannot clip it.
 *
 * So the editor publishes its handle under a caller-supplied key and the
 * overlay looks it up. The key must be unique PER EDITOR, not per screen or per
 * document — a card detail mounts a description editor and a comment composer
 * against the same board, and a shared key would anchor one's popover to the
 * other's WebView.
 */
export interface EditorOverlayHandle {
    /** Where responses are POSTED. Only the WebView ref can reach the page. */
    webViewRef: unknown
    /**
     * What the overlay is MEASURED against — the host View wrapping the
     * WebView. Separate from webViewRef because under the New Architecture
     * that ref carries native WebView commands and no measurement methods, so
     * measuring it returns nothing and the popover is dismissed before it can
     * be drawn. Optional: a caller that has no host view still positions
     * nothing, which is the pre-existing fail-closed behavior.
     */
    measureRef?: unknown
    editorInstanceId: string
}

const handles = new Map<string, EditorOverlayHandle>()
const listeners = new Set<() => void>()

function emit(): void {
    for (const listener of listeners) listener()
}

/** Publish an editor's handle. Returns the deregistration function. */
export function registerEditorOverlay(key: string, handle: EditorOverlayHandle): () => void {
    handles.set(key, handle)
    emit()
    return () => {
        // Only clear if this registration is still the live one — a remount can
        // register the replacement before the outgoing effect cleans up.
        if (handles.get(key) === handle) {
            handles.delete(key)
            emit()
        }
    }
}

function subscribe(listener: () => void): () => void {
    listeners.add(listener)
    return () => {
        listeners.delete(listener)
    }
}

/**
 * Read the editor registered under a key, re-rendering when it appears.
 *
 * Returns null until the editor mounts, which is the normal first-render state:
 * the overlay renders nothing, and the editor's registration effect wakes it.
 */
export function useEditorOverlay(key: string | undefined): EditorOverlayHandle | null {
    const getSnapshot = useCallback(() => (key ? (handles.get(key) ?? null) : null), [key])
    return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

/** Test-only. The registry is module-global and would otherwise leak. */
export function resetEditorOverlayRegistry(): void {
    handles.clear()
    listeners.clear()
}
