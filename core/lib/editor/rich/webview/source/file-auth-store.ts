import type { RichEditorFileAuth } from './protocol'

/**
 * The page's copy of the host's file credentials — a hand-rolled external
 * store rather than React state because it is written from a window message
 * listener that outlives any one component, and read by every image node view
 * via useSyncExternalStore.
 */
let current: RichEditorFileAuth | null = null
const listeners = new Set<() => void>()

export function setFileAuth(next: RichEditorFileAuth | null): void {
    if (next?.baseURL === current?.baseURL && next?.token === current?.token) return
    current = next
    for (const listener of listeners) listener()
}

export function getFileAuth(): RichEditorFileAuth | null {
    return current
}

export function subscribeFileAuth(listener: () => void): () => void {
    listeners.add(listener)
    return () => listeners.delete(listener)
}
