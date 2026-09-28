/**
 * Holding the app out of play while a modal layer is open.
 *
 * A dialog sets `aria-modal` on its own surface, but that is a hint to a screen
 * reader and nothing more: the rest of the document stays focusable,
 * hit-testable and queryable behind a backdrop that only covers it visually. A
 * press aimed at something behind the dialog does not reach it — it dismisses
 * the dialog — while anything that targets an element by its content (a screen
 * reader, keyboard navigation, an e2e locator) still finds the page behind and
 * acts on it. `inert` is what actually holds it out.
 *
 * The obvious shape — one `inert` wrapper around the app, with the hosts as its
 * siblings — does not work here. `inert` cascades to the entire subtree and a
 * descendant cannot undo it, and the `sheet` host is deliberately nested deep
 * inside the app content: a Sheet rests at the bottom edge of its PARENT, which
 * is what puts it exactly on the mobile tab bar rather than under it. Wrapping
 * the app therefore made every bottom sheet inert as soon as it opened — the
 * sheet's own content included.
 *
 * So inert the rest of the document instead, the way a native `<dialog>` does:
 * from each live host node, walk up to the document body marking every SIBLING
 * along the path. Everything outside those paths goes inert; every host stays
 * interactive no matter how deeply it is nested, and nothing about a host's
 * position in the layout changes.
 */

const MARKED = new Set<HTMLElement>()

/**
 * Extra nodes that must stay interactive, beyond the overlay hosts.
 *
 * Some always-on-top surfaces render IN PLACE rather than through a host —
 * they are absolutely positioned siblings of the app, not portalled — so the
 * host paths alone do not protect them. A toast raised by a dialog's save is
 * the case that matters: inerting it leaves its Dismiss and Undo dead and
 * hides it from a screen reader, at exactly the moment it has something to
 * say. They register here instead of being moved, so their stacking is
 * untouched.
 */
const EXEMPT = new Set<HTMLElement>()
const exemptListeners = new Set<() => void>()
let exemptEpoch = 0

/** Changes whenever the exempt set does, so a subscriber can re-read it. */
export function inertExemptionEpoch(): number {
    return exemptEpoch
}

/** Registers a node as always-interactive. Returns its unregister. */
export function exemptFromInert(node: HTMLElement | null | undefined): () => void {
    if (!node) return () => {}
    EXEMPT.add(node)
    exemptEpoch += 1
    for (const listener of exemptListeners) listener()
    return () => {
        EXEMPT.delete(node)
        exemptEpoch += 1
        for (const listener of exemptListeners) listener()
    }
}

/** Notified when the exempt set changes, so the gate can re-apply. */
export function subscribeInertExemptions(listener: () => void): () => void {
    exemptListeners.add(listener)
    return () => {
        exemptListeners.delete(listener)
    }
}

function isElement(node: Node | null): node is HTMLElement {
    return node != null && node.nodeType === 1
}

/**
 * The elements that must stay interactive: every ancestor of every host, up to
 * and including the body. A sibling of one of these is outside every host.
 */
function pathsToRoot(hosts: readonly HTMLElement[], body: HTMLElement): Set<HTMLElement> {
    const keep = new Set<HTMLElement>()
    for (const host of hosts) {
        let node: HTMLElement | null = host
        while (node && node !== body.parentElement) {
            keep.add(node)
            node = node.parentElement
        }
    }
    keep.add(body)
    return keep
}

/** Drops `inert` from everything this module set it on. */
export function clearInertSiblings() {
    for (const el of MARKED) el.removeAttribute('inert')
    MARKED.clear()
}

/**
 * Marks everything outside the hosts' ancestor paths `inert`, and clears any
 * marks from a previous call. Safe to run on every change: it recomputes from
 * scratch rather than trying to diff, because the tree moves under it (a host
 * mounts, a screen re-renders) and a stale diff would strand an `inert`.
 *
 * `hosts` are the overlay hosts' DOM nodes. An empty list clears everything —
 * with no host there is no modal surface to protect, and inerting the whole
 * document would lock the user out.
 */
export function applyInertSiblings(hosts: readonly (HTMLElement | undefined)[]) {
    if (typeof document === 'undefined') return
    clearInertSiblings()
    const live = [...hosts, ...EXEMPT].filter((h): h is HTMLElement => h?.isConnected === true)
    if (live.length === 0) return

    const body = document.body
    const keep = pathsToRoot(live, body)
    // A protected node's own contents are the thing being kept interactive, so
    // its subtree is never walked — only the ancestors ABOVE it have siblings
    // to mark.
    const hostNodes = new Set(live)

    for (const node of keep) {
        if (hostNodes.has(node)) continue
        for (const child of Array.from(node.children)) {
            if (!isElement(child) || keep.has(child)) continue
            // Never re-mark something already inert for its own reasons — this
            // module must not clear an attribute it did not set.
            if (child.hasAttribute('inert')) continue
            child.setAttribute('inert', '')
            MARKED.add(child)
        }
    }
}
