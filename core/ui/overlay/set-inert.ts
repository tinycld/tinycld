/**
 * Web: sets or clears `inert` on a node that a cover holds out of play.
 *
 * `inert` stops new focus inside the node, but an element that already holds
 * the focus (an input that autofocused before the cover came up) keeps it and
 * still takes typed keys, so it loses the focus here.
 */
export function setInert(node: HTMLElement, isInert: boolean) {
    if (!isInert) {
        node.removeAttribute('inert')
        return
    }
    node.setAttribute('inert', '')
    const active = node.ownerDocument.activeElement
    if (active instanceof HTMLElement && node.contains(active)) active.blur()
}
