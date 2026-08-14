import type { SurfaceId } from './warm-editor-store'

export interface DraftStore {
    /** Keep uncommitted text. Empty content clears rather than stores. */
    stash(surfaceId: SurfaceId, content: string): void
    /** Read without consuming — re-acquiring twice must yield the same draft. */
    take(surfaceId: SurfaceId): string | null
    clear(surfaceId: SurfaceId): void
    clearAll(): void
}

/**
 * Uncommitted text belonging to a surface that does not hold the warm editor.
 *
 * Only surfaces with no commit semantics need this. An inline comment edit
 * commits on blur, so handing the editor away writes it; the composer has no
 * such commit, and before the warm editor its draft survived only because the
 * composer stayed mounted for the life of the open card.
 */
export function createDraftStore(): DraftStore {
    const drafts = new Map<SurfaceId, string>()
    return {
        stash(surfaceId, content) {
            // An empty editor is not a draft: re-seeding "" would paint over
            // the placeholder and read as a broken composer.
            if (content.trim() === '') drafts.delete(surfaceId)
            else drafts.set(surfaceId, content)
        },
        take: surfaceId => drafts.get(surfaceId) ?? null,
        clear: surfaceId => {
            drafts.delete(surfaceId)
        },
        clearAll: () => drafts.clear(),
    }
}
