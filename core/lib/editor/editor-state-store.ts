/**
 * The last `stateUpdate` the in-WebView editor posted, held for
 * `useSyncExternalStore`.
 *
 * The page broadcasts its whole toolbar state (active marks, heading level,
 * word count, focus, readiness …) as one object on every meaningful
 * transaction. Each snapshot replaces the previous one outright — the page
 * always sends the full picture, so nothing is merged across posts. Readers
 * narrow the fields they need through `deriveToolbarState`.
 *
 * Its own module, like `height-store.ts`, so it can be unit-tested without the
 * native host the hook renders.
 */
export type EditorStateSnapshot = Record<string, unknown>

export interface EditorStateStore {
    get: () => EditorStateSnapshot
    /** Ignores anything that is not a plain object; a malformed post keeps the last state. */
    set: (payload: unknown) => void
    subscribe: (listener: () => void) => () => void
}

const EMPTY: EditorStateSnapshot = {}

export function createEditorStateStore(): EditorStateStore {
    let snapshot = EMPTY
    const listeners = new Set<() => void>()
    return {
        get: () => snapshot,
        set: payload => {
            if (typeof payload !== 'object' || payload === null || Array.isArray(payload)) return
            snapshot = { ...(payload as EditorStateSnapshot) }
            for (const listener of listeners) listener()
        },
        subscribe: listener => {
            listeners.add(listener)
            return () => listeners.delete(listener)
        },
    }
}
