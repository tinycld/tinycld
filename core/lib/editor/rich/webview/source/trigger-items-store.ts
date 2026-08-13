import type { TriggerItem } from '../../triggers'

/**
 * The page's copy of each trigger's candidate pool, keyed by trigger id.
 *
 * A plain module store rather than React state because of WHO reads it: the
 * suggestion plugin's `items` callback runs inside a ProseMirror transaction,
 * not a render, and must see the current roster synchronously. A `useState`
 * value would be a render behind, and the plugin holds no component to
 * subscribe with. `file-auth-store.ts` solves the same problem for the file
 * token; this one needs no listeners, since nothing re-renders on a change —
 * the next keystroke simply reads the new value.
 *
 * Seeded from the init payload and refreshed by APP_TRIGGER_ITEMS whenever the
 * host's roster query re-emits.
 */
const items = new Map<string, TriggerItem[]>()

export function setTriggerItems(triggerId: string, next: TriggerItem[]): void {
    items.set(triggerId, next)
}

export function getTriggerItems(triggerId: string): TriggerItem[] {
    return items.get(triggerId) ?? []
}

/** Test-only. The store is module-global, so suites would otherwise leak. */
export function resetTriggerItems(): void {
    items.clear()
}
