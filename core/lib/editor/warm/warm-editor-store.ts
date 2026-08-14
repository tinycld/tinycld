/** Names an editing surface: `composer:<cardId>`, `comment:<id>`, `description:<cardId>`. */
export type SurfaceId = string

export interface WarmSnapshot {
    holder: SurfaceId | null
    generation: number
}

export interface WarmEditorStore {
    /** Take the instance. Returns the generation the caller must post. */
    acquire(surfaceId: SurfaceId): number
    /** Give it back. Returns false if the caller had already been displaced. */
    release(surfaceId: SurfaceId): boolean
    holder(): SurfaceId | null
    generation(): number
    subscribe(listener: () => void): () => void
    getSnapshot(): WarmSnapshot
}

/**
 * Who holds the single warm editor, and which configuration is live.
 *
 * A store rather than component state because the holder changes from event
 * handlers in unrelated subtrees (a comment row, the composer, the description),
 * and useSyncExternalStore lets each of them re-render without a context that
 * re-renders the whole card.
 */
export function createWarmEditorStore(): WarmEditorStore {
    let holder: SurfaceId | null = null
    let generation = 0
    // Rebuilt only on change: useSyncExternalStore compares snapshots by
    // identity and loops forever if a fresh object is returned each call.
    let snapshot: WarmSnapshot = { holder, generation }
    const listeners = new Set<() => void>()

    function commit() {
        snapshot = { holder, generation }
        for (const listener of listeners) listener()
    }

    return {
        acquire(surfaceId) {
            // Unconditional transfer: acquiring while another surface holds the
            // instance is the ordinary handover (tapping a second comment), and
            // the previous holder is dropped outright so its later release
            // cannot evict whoever took over.
            holder = surfaceId
            generation += 1
            commit()
            return generation
        },
        release(surfaceId) {
            // A displaced surface still unmounts and still releases. Honouring
            // that would blank the editor that just took over.
            if (holder !== surfaceId) return false
            holder = null
            commit()
            return true
        },
        holder: () => holder,
        generation: () => generation,
        subscribe(listener) {
            listeners.add(listener)
            return () => {
                listeners.delete(listener)
            }
        },
        getSnapshot: () => snapshot,
    }
}
