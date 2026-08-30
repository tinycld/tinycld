import { create } from '../../store'
import type { SurfaceId } from './warm-editor-store'

export interface DraftStore {
    /** Keep uncommitted text. Empty content clears rather than stores. */
    stash(surfaceId: SurfaceId, content: string): void
    /** Read without consuming — re-acquiring twice must yield the same draft. */
    take(surfaceId: SurfaceId): string | null
    clear(surfaceId: SurfaceId): void
    clearAll(): void
}

interface DraftState {
    drafts: Record<string, string>
    stash: (surfaceId: SurfaceId, content: string) => void
    clear: (surfaceId: SurfaceId) => void
    clearAll: () => void
}

/**
 * Uncommitted text belonging to a surface that does not hold the warm editor.
 *
 * Only surfaces with no commit semantics need this. An inline comment edit
 * commits when it hands the editor on, so the write is the record; the composer
 * has no such commit, and before the warm editor its draft survived only because
 * the composer stayed mounted for the life of the open card.
 *
 * REACTIVE, and that is the point. A draft is stashed from an async read — the
 * editor's content cannot be read synchronously, because on native it is a
 * WebView round-trip — so it lands after the render that would have displayed
 * it. A plain Map therefore stored the text correctly and showed the user
 * nothing: the composer read it during render and never re-rendered to see it
 * arrive. `useDraft` below subscribes, so the text appears when it lands.
 *
 * Module-level rather than per-provider. That is not a change of lifetime in
 * practice — EditorSingletonProvider sits above the route tree and is never
 * unmounted — but it does make the growth visible, so it is bounded explicitly:
 * a draft exists only for a card where someone typed a comment, did not send it,
 * and moved on, and `dropSurfaceDraft` drops it when that card's panel closes.
 */
const useDraftState = create<DraftState>()(set => ({
    drafts: {},
    stash: (surfaceId, content) =>
        set(state => {
            // An empty editor is not a draft: re-seeding "" would paint over
            // the placeholder and read as a broken composer.
            if (content.trim() === '') {
                if (state.drafts[surfaceId] === undefined) return state
                const { [surfaceId]: _dropped, ...rest } = state.drafts
                return { drafts: rest }
            }
            if (state.drafts[surfaceId] === content) return state
            return { drafts: { ...state.drafts, [surfaceId]: content } }
        }),
    clear: surfaceId =>
        set(state => {
            if (state.drafts[surfaceId] === undefined) return state
            const { [surfaceId]: _dropped, ...rest } = state.drafts
            return { drafts: rest }
        }),
    clearAll: () => set(state => (Object.keys(state.drafts).length === 0 ? state : { drafts: {} })),
}))

/**
 * Forget a surface's draft.
 *
 * Called when the surface's owner goes away for good — a card panel closing,
 * not the editor merely being handed on. Without it the map only ever grows: a
 * card whose half-typed comment was abandoned would keep that string for the
 * life of the session.
 */
export function dropSurfaceDraft(surfaceId: SurfaceId): void {
    useDraftState.getState().clear(surfaceId)
}

/**
 * Watch the store from outside React.
 *
 * The hook below is how a component reads a draft; this is for a test, or for
 * anything that needs the change signal without a render.
 */
export function subscribeToDrafts(listener: () => void): () => void {
    return useDraftState.subscribe(listener)
}

/**
 * Subscribe to one surface's draft.
 *
 * A selector rather than the whole map, so a comment composer does not re-render
 * because a different card's draft changed.
 */
export function useDraft(surfaceId: SurfaceId): string | null {
    return useDraftState(state => state.drafts[surfaceId] ?? null)
}

/**
 * The imperative face of the same store.
 *
 * Kept because the editor stashes and clears from callbacks and effects, where a
 * hook cannot go — `endSession` reads the editor asynchronously and stashes
 * whenever that resolves. Returns a stable object: the actions never change
 * identity, so a consumer can hold this without re-subscribing.
 */
export function createDraftStore(): DraftStore {
    return {
        stash: (surfaceId, content) => useDraftState.getState().stash(surfaceId, content),
        take: surfaceId => useDraftState.getState().drafts[surfaceId] ?? null,
        clear: surfaceId => useDraftState.getState().clear(surfaceId),
        clearAll: () => useDraftState.getState().clearAll(),
    }
}
