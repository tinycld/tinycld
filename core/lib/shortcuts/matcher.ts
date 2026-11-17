import { getShortcuts } from './registry'
import { activeScopeChain, type ScopeEntry, topScopeEntry } from './scopes'
import type { Shortcut } from './types'

const SEQUENCE_TIMEOUT_MS = 1000

interface MatcherState {
    sequence: string[]
    timer: ReturnType<typeof setTimeout> | null
}

/**
 * Extract the list of unique single atoms from a set of shortcuts.
 * An atom is either a single combo ("$mod+Enter", "?", "g") or one
 * space-separated step of a sequence ("g i" → ["g", "i"]).
 */
export function atomsForShortcuts(shortcuts: Iterable<Shortcut>): string[] {
    const set = new Set<string>()
    for (const s of shortcuts) {
        for (const atom of s.keys.split(/\s+/).filter(Boolean)) {
            set.add(atom)
        }
    }
    return Array.from(set)
}

export interface MatcherContext {
    inInput: boolean
    /**
     * Consider only shortcuts of a scope (`'scoped'`) or only `global` ones
     * (`'global'`). The web provider splits a key typed in a text field this
     * way; see `handleKey` (dispatch.ts).
     */
    only?: 'scoped' | 'global'
}

/**
 * Is this shortcut's scope one currently holding the keyboard? Exported so
 * the help overlay lists exactly what can fire, rather than restating this and
 * drifting from it. `top` is the scope entry to judge against: the live top by
 * default; the overlay passes the top entry below its own. The active entries
 * are `top` and those under it that stay active (`activeScopeChain`).
 */
export function isScopeActive(
    shortcut: Shortcut,
    top: ScopeEntry | null = topScopeEntry()
): boolean {
    if (shortcut.scope === 'global') return true
    return ownerIndex(shortcut, activeScopeChain(top)) !== -1
}

/**
 * The position in `chain` (newest first) of the active entry that owns a
 * scoped shortcut, or -1 when no active entry owns it.
 */
function ownerIndex(shortcut: Shortcut, chain: readonly ScopeEntry[]): number {
    // A blurred screen stays mounted and keeps its shortcuts registered (on web
    // `freezeOnBlur` only hides the subtree), so the live screen is not the only
    // owner of its scope's keys — mail's list and a Boards board both register
    // 'list' j/k/x. Only the instance holding the keyboard may fire.
    //
    // An owner's id is assigned on MOUNT and never changes, so the stamp always
    // names the instance that registered the shortcut — never whoever happened
    // to hold the scope when a re-registration fired. That distinction is the
    // whole point: on web a blurred screen stays mounted (`freezeOnBlur` only
    // hides the subtree), its live queries keep emitting, and it re-registers on
    // its own schedule. Re-deriving the stamp then handed the BLURRED screen the
    // FOCUSED screen's id, and — being earlier in the registry's insertion order
    // — it won the match and swallowed the keypress.
    //
    // `scopeId === undefined` means the shortcut was registered with no scope
    // owner at all; it keeps the original scope-only behavior, and belongs to
    // the newest active entry of its scope. `null` never matches.
    const { scope, scopeId } = shortcut
    if (scopeId === undefined) return chain.findIndex(entry => entry.scope === scope)
    return chain.findIndex(entry => entry.scope === scope && entry.id === scopeId)
}

/**
 * How strongly a shortcut claims its keys: the lower, the stronger. A scoped
 * shortcut ranks by its owning entry's position in the active chain from
 * `top` (0 is the newest), so a newer scope wins a key it shares with an older
 * one that stays active under it. A scoped shortcut no active entry owns ranks
 * after every active one; a `global` ranks last. Exported so the help overlay
 * leaves out the same losers the matcher does.
 */
export function firingRank(shortcut: Shortcut, top: ScopeEntry | null = topScopeEntry()): number {
    return rankIn(shortcut, activeScopeChain(top))
}

function rankIn(shortcut: Shortcut, chain: readonly ScopeEntry[]): number {
    if (shortcut.scope === 'global') return Number.POSITIVE_INFINITY
    const index = ownerIndex(shortcut, chain)
    return index === -1 ? chain.length : index
}

function matches(shortcut: Shortcut, ctx: MatcherContext): boolean {
    if (ctx.only === 'scoped' && shortcut.scope === 'global') return false
    if (ctx.only === 'global' && shortcut.scope !== 'global') return false
    if (!isScopeActive(shortcut)) return false
    if (ctx.inInput && !shortcut.allowInInputs) return false
    if (shortcut.when && !shortcut.when()) return false
    return true
}

/**
 * Of shortcuts with the same keys that can all fire, the one that does. A
 * shortcut of an active scope wins over a `global` one: core registers its
 * globals (`/` search) at app start, before any screen mounts, so registry
 * order alone would let them shadow a screen's own key. Among scoped
 * shortcuts of different active entries, the newest entry's wins
 * (`firingRank`), whatever the registry order: the newer scope is the one the
 * user just opened. Within one entry, and among globals, the first in order
 * wins. Exported so the help overlay lists the same winner the matcher fires.
 */
export function pickFiring(
    candidates: Iterable<Shortcut>,
    top: ScopeEntry | null = topScopeEntry()
): Shortcut | null {
    const chain = activeScopeChain(top)
    let best: Shortcut | null = null
    let bestRank = Number.POSITIVE_INFINITY
    for (const shortcut of candidates) {
        const rank = rankIn(shortcut, chain)
        if (best === null || rank < bestRank) {
            best = shortcut
            bestRank = rank
        }
    }
    return best
}

function* exactCandidates(keys: string, ctx: MatcherContext): Generator<Shortcut> {
    for (const shortcut of getShortcuts().values()) {
        if (shortcut.keys === keys && matches(shortcut, ctx)) yield shortcut
    }
}

function findExactMatch(keys: string, ctx: MatcherContext): Shortcut | null {
    return pickFiring(exactCandidates(keys, ctx))
}

/**
 * Does any registered shortcut's keys string *start with* the given sequence,
 * with more atoms to come? Used to decide whether to keep buffering.
 */
function hasPrefixMatch(prefix: string, ctx: MatcherContext): boolean {
    const prefixAtoms = prefix.split(/\s+/).filter(Boolean)
    for (const shortcut of getShortcuts().values()) {
        const keyAtoms = shortcut.keys.split(/\s+/).filter(Boolean)
        if (keyAtoms.length <= prefixAtoms.length) continue
        if (!matches(shortcut, ctx)) continue
        let ok = true
        for (let i = 0; i < prefixAtoms.length; i++) {
            if (keyAtoms[i] !== prefixAtoms[i]) {
                ok = false
                break
            }
        }
        if (ok) return true
    }
    return false
}

export function createMatcher() {
    const state: MatcherState = { sequence: [], timer: null }

    function reset() {
        state.sequence = []
        if (state.timer) {
            clearTimeout(state.timer)
            state.timer = null
        }
    }

    function armTimer() {
        if (state.timer) clearTimeout(state.timer)
        state.timer = setTimeout(reset, SEQUENCE_TIMEOUT_MS)
    }

    /**
     * Feed a single key atom into the matcher. Returns true if the atom was
     * consumed (either fired a shortcut or is building toward a sequence).
     * Callers should preventDefault when true.
     */
    function feedAtom(atom: string, ctx: MatcherContext): boolean {
        const candidate = [...state.sequence, atom].join(' ')

        const exact = findExactMatch(candidate, ctx)
        if (exact) {
            reset()
            exact.run({ keys: candidate })
            return true
        }

        if (hasPrefixMatch(candidate, ctx)) {
            state.sequence.push(atom)
            armTimer()
            return true
        }

        // The atom didn't match any exact or prefix — if we were mid-sequence,
        // reset and try the atom again as a fresh first atom.
        if (state.sequence.length > 0) {
            reset()
            return feedAtom(atom, ctx)
        }

        return false
    }

    return { feedAtom, reset, getSequence: () => [...state.sequence] }
}

export type Matcher = ReturnType<typeof createMatcher>
