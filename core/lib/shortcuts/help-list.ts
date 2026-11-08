import { create } from '@tinycld/core/lib/store'
import { isScopeActive, pickFiring } from './matcher'
import { nextScopeId, topScopeEntry } from './scopes'
import type { Shortcut } from './types'

interface HelpState {
    isOpen: boolean
    /**
     * The first scope id the overlay itself can own, read when it opens. The
     * overlay is a Dialog, whose `LayerEscape` pushes a `modal` scope after
     * this, so judging the list against the stack below this id leaves out the
     * overlay's own entry — which would otherwise shadow every key of the
     * screen the user asked about.
     */
    scopeFloor: number | null
    open: () => void
    close: () => void
    toggle: () => void
}

export const useShortcutHelpStore = create<HelpState>(set => ({
    isOpen: false,
    scopeFloor: null,
    open: () => set({ isOpen: true, scopeFloor: nextScopeId() }),
    close: () => set({ isOpen: false, scopeFloor: null }),
    toggle: () =>
        set(s =>
            s.isOpen
                ? { isOpen: false, scopeFloor: null }
                : { isOpen: true, scopeFloor: nextScopeId() }
        ),
}))

export function useShortcutHelp() {
    return useShortcutHelpStore()
}

/**
 * The shortcuts the help overlay lists, by group. `floor` is the store's
 * `scopeFloor`: activity is judged against the scope stack below it, or the
 * live stack when it is null.
 */
export function helpGroups(
    shortcuts: ReadonlyMap<string, Shortcut>,
    floor: number | null
): Record<string, Shortcut[]> {
    const top = topScopeEntry(floor ?? undefined)
    const byKeys = new Map<string, Shortcut[]>()
    for (const s of shortcuts.values()) {
        // The "Help" group documents how to open/close this very overlay, so
        // showing it inside the overlay is redundant.
        if (s.group === 'Help') continue
        // Only what can actually fire on the screen under the overlay. The
        // registry holds every MOUNTED screen's shortcuts, which is more than
        // the live set two ways: an inner scope shadows the screen underneath
        // (a board's keys cannot fire while a card is open), and
        // `freezeOnBlur` keeps a departed package's screens registered.
        // Listing those advertises keys that do nothing, and duplicates every
        // entry the two screens have in common.
        if (!isScopeActive(s, top)) continue
        const sameKeys = byKeys.get(s.keys)
        if (sameKeys) sameKeys.push(s)
        else byKeys.set(s.keys, [s])
    }
    const groups: Record<string, Shortcut[]> = {}
    for (const sameKeys of byKeys.values()) {
        // A global whose keys an active scoped shortcut also binds never fires
        // (the matcher's rule, `pickFiring`), so it is left out too.
        const scopedWins = pickFiring(sameKeys)?.scope !== 'global'
        for (const s of sameKeys) {
            if (scopedWins && s.scope === 'global') continue
            addToGroup(groups, s)
        }
    }
    for (const key of Object.keys(groups)) {
        groups[key].sort((a, b) => a.description.localeCompare(b.description))
    }
    return groups
}

function addToGroup(groups: Record<string, Shortcut[]>, s: Shortcut) {
    const key = s.group ?? 'General'
    if (!groups[key]) groups[key] = []
    groups[key].push(s)
}
