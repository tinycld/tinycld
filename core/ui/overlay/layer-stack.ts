import { useCallback, useEffect, useRef, useSyncExternalStore } from 'react'
import { BackHandler, Platform } from 'react-native'

/**
 * The stack of open layers, and how a layer leaves it.
 *
 * One rule covers every surface: a pointer that lands OUTSIDE the topmost
 * layer dismisses that layer and nothing else. A click inside a dialog but
 * outside the menu open on top of it closes the menu; the next click, outside
 * the dialog, closes the dialog. A layer's own anchor counts as inside, so a
 * trigger toggles its menu closed itself rather than being dismissed and
 * reopened by the same press.
 *
 * Escape and the Android back button dismiss the topmost layer the same way.
 */
export interface LayerRecord {
    id: number
    /** The DOM nodes that count as inside this layer: its surface and its anchor. */
    nodes: () => (Node | null | undefined)[]
    onDismiss: () => void
    dismissOnOutside: boolean
    dismissOnEscape: boolean
    /**
     * Whether this layer takes the rest of the page out of play — a dialog or
     * a sheet, not a menu or a popover. `aria-modal` on the surface is only a
     * hint to a reader; what actually holds is the app subtree being `inert`
     * while this is set. See `useIsModalLayerOpen`.
     */
    isModal: boolean
}

const stack: LayerRecord[] = []
let nextLayerId = 1
const modalListeners = new Set<() => void>()

/** Test seam. */
export function resetLayers() {
    stack.length = 0
    currentPointerId = null
    currentPointerConsumed = false
    currentPointerAt = 0
    notifyModalListeners()
}

export function openLayerCount(): number {
    return stack.length
}

export function modalLayerCount(): number {
    return stack.reduce((count, layer) => (layer.isModal ? count + 1 : count), 0)
}

function notifyModalListeners() {
    for (const listener of modalListeners) listener()
}

export function subscribeModalLayers(listener: () => void): () => void {
    modalListeners.add(listener)
    return () => {
        modalListeners.delete(listener)
    }
}

/**
 * Which layer a pointerdown on `target` dismisses, or null. Pure, so the
 * rule above is pinned by a unit test rather than by an e2e run.
 */
export function layerToDismiss(
    layers: readonly LayerRecord[],
    target: Node | null
): LayerRecord | null {
    const top = layers[layers.length - 1]
    if (!top?.dismissOnOutside) return null
    if (target && top.nodes().some(node => node?.contains(target))) return null
    return top
}

export function topLayer(): LayerRecord | null {
    return stack[stack.length - 1] ?? null
}

let listenerInstalled = false

// The most recent pointerdown, and whether it dismissed a layer. A press that
// closes a menu must not ALSO be read as a press on whatever the menu was drawn
// over — see `wasConsumedByLayerDismissal`.
let currentPointerId: number | null = null
let currentPointerConsumed = false
// When that pointerdown arrived, for the id-less-click fallback below.
let currentPointerAt = 0

/**
 * How long after a dismissing press an id-LESS click may still be attributed
 * to it. Long enough to cover the delay a touch platform puts between
 * `pointerdown` and the `click` it synthesizes, short enough that a separate,
 * deliberate second press cannot fall inside it.
 */
const IDLESS_CLICK_WINDOW_MS = 300

function onPointerDown(event: Event) {
    // Recorded for EVERY press, not only a dismissing one. A mouse reuses the
    // same pointerId (1) for its whole life, so a flag that only ever got set
    // would stay true forever and the backdrop would never close the dialog
    // again after the first menu dismissal.
    const pointer = event as PointerEvent
    currentPointerId = typeof pointer.pointerId === 'number' ? pointer.pointerId : null
    currentPointerConsumed = false
    currentPointerAt = now()

    const layer = layerToDismiss(stack, event.target as Node | null)
    if (!layer) return
    currentPointerConsumed = true
    layer.onDismiss()
}

/**
 * Whether this press already dismissed a layer, and so has been spent.
 *
 * A menu opened from inside a dialog is drawn over the dialog's backdrop, so
 * one press outside the menu produces BOTH: a `pointerdown` in the capture
 * phase, which dismisses the menu, and then a `click` on the backdrop, by
 * which time the dialog is back on top. Acting on that click closes the
 * dialog too — two layers dismissed by one press, and the user's work in the
 * dialog discarded by a press that was only ever meant to close the menu.
 *
 * Reading it CONSUMES it: the one click that belongs to that press is
 * swallowed, and the next press on the backdrop closes the dialog normally,
 * even from the same mouse reporting the same pointerId.
 *
 * A click with NO pointerId is the iOS Safari case, and it is why the id match
 * alone is not enough. iOS Safari fires the `pointerdown` (with a real
 * pointerId) but dispatches the `click` it synthesizes afterwards as a plain
 * `MouseEvent`, which carries no `pointerId` at all. On the id match alone that
 * click never looks consumed, so a press that only meant to close a menu drawn
 * inside a Sheet still reaches the backdrop underneath and closes the Sheet
 * too — the exact double dismissal this guard exists to stop, on the one
 * platform where a Sheet is the primary modal.
 *
 * So an id-less event falls back to time: it counts as consumed when the
 * dismissing press is still within `IDLESS_CLICK_WINDOW_MS`. This is safe
 * because the flag is still single-use and still only set by a press that
 * actually dismissed something, so the fallback can swallow at most one click,
 * and only inside a window that opens on that press. The residual risk is a
 * genuine second press landing under 300 ms after a dismissing one: it is
 * swallowed, and the user presses again. The alternative — the id-less click
 * always winning — discards the user's work in the dialog instead.
 *
 * The fallback needs a dismissing press that HAD an id. An id-less
 * `pointerdown` followed by an id-less click cannot be told apart from two
 * unrelated synthetic events, so that pairing is never consumed: treating it as
 * a match would be the permanently-dead-backdrop bug by another route.
 */
export function wasConsumedByLayerDismissal(event: { pointerId?: number } | undefined): boolean {
    if (!currentPointerConsumed) return false
    const id = event?.pointerId
    if (typeof id === 'number') {
        if (id !== currentPointerId) return false
    } else {
        if (currentPointerId === null) return false
        if (now() - currentPointerAt > IDLESS_CLICK_WINDOW_MS) return false
    }
    currentPointerConsumed = false
    return true
}

/** Monotonic where available; `Date.now` is the fallback for a test env. */
function now(): number {
    return typeof performance !== 'undefined' ? performance.now() : Date.now()
}

/**
 * Escape, at the document in the capture phase. Not through the shortcut
 * system: react-native-web's TextInput swallows Escape in its own bubble
 * handler, so a key pressed with a field focused — the search box in a
 * picker, the rule name in a dialog — never reached a bubble-phase listener
 * and the layer stayed open. Stopping propagation here keeps the key from
 * reaching the shortcut system, which would otherwise close the next layer
 * down with the same press.
 */
function onKeyDown(event: KeyboardEvent) {
    if (event.key !== 'Escape') return
    const top = stack[stack.length - 1]
    if (!top?.dismissOnEscape) return
    event.preventDefault()
    event.stopPropagation()
    top.onDismiss()
}

function syncListener() {
    if (Platform.OS !== 'web' || typeof document === 'undefined') return
    const wanted = stack.length > 0
    if (wanted && !listenerInstalled) {
        // Capture phase, and pointerdown rather than click: a layer must close
        // on the press that STARTS an outside interaction, before that press
        // is delivered to whatever sits underneath.
        document.addEventListener('pointerdown', onPointerDown, true)
        document.addEventListener('keydown', onKeyDown, true)
        listenerInstalled = true
    } else if (!wanted && listenerInstalled) {
        document.removeEventListener('pointerdown', onPointerDown, true)
        document.removeEventListener('keydown', onKeyDown, true)
        listenerInstalled = false
    }
}

function pushLayer(record: LayerRecord) {
    stack.push(record)
    syncListener()
    if (record.isModal) notifyModalListeners()
}

function popLayer(id: number) {
    const index = stack.findIndex(entry => entry.id === id)
    if (index === -1) return
    const [removed] = stack.splice(index, 1)
    syncListener()
    if (removed?.isModal) notifyModalListeners()
}

interface UseOverlayLayerOptions {
    isOpen: boolean
    /** The surface's node and its anchor (trigger). Read at event time. */
    nodes: () => (Node | null | undefined)[]
    onDismiss: () => void
    dismissOnOutside?: boolean
    dismissOnEscape?: boolean
    /** See LayerRecord.isModal. Default false: a menu or popover leaves the page usable. */
    isModal?: boolean
}

/**
 * Joins the layer stack while `isOpen`. Handles outside pointerdown (web) and
 * the Android back button. Escape is `<LayerEscape>` (escape.tsx), rendered
 * INSIDE the open layer: it joins the shortcut scope stack on mount, so it
 * must not exist while the layer is closed.
 *
 * Returns `isTopLayer()`, read at event time. A layer that dismisses itself
 * from its own surface — a dialog's backdrop — must consult it first, or it
 * breaks the one rule the stack exists to enforce: a press dismisses at most
 * the TOP layer. A menu open above a dialog draws inside the dialog's
 * backdrop, so a press that misses the menu lands on that backdrop and would
 * otherwise close BOTH, discarding the user's work in the dialog on the press
 * that was only meant to close the menu.
 */
export function useOverlayLayer({
    isOpen,
    nodes,
    onDismiss,
    dismissOnOutside = true,
    dismissOnEscape = true,
    isModal = false,
}: UseOverlayLayerOptions) {
    const idRef = useRef<number | null>(null)
    const nodesRef = useRef(nodes)
    nodesRef.current = nodes
    const onDismissRef = useRef(onDismiss)
    onDismissRef.current = onDismiss

    useEffect(() => {
        if (!isOpen) return
        const id = nextLayerId++
        idRef.current = id
        pushLayer({
            id,
            nodes: () => nodesRef.current(),
            onDismiss: () => onDismissRef.current(),
            dismissOnOutside,
            dismissOnEscape,
            isModal,
        })
        return () => {
            popLayer(id)
            idRef.current = null
        }
    }, [isOpen, dismissOnOutside, dismissOnEscape, isModal])

    // A function, not a boolean: the stack changes without re-rendering this
    // component, so the answer has to be read when the press happens.
    const isTopLayer = useCallback(() => topLayer()?.id === idRef.current, [])

    // Android back: only the topmost layer answers, and it swallows the event
    // so the navigator does not also pop a screen.
    useEffect(() => {
        if (!isOpen || Platform.OS !== 'android' || !dismissOnEscape) return
        const subscription = BackHandler.addEventListener('hardwareBackPress', () => {
            if (topLayer()?.id !== idRef.current) return false
            onDismissRef.current()
            return true
        })
        return () => subscription.remove()
    }, [isOpen, dismissOnEscape])

    return { isTopLayer }
}

/**
 * Whether a modal layer is open right now, re-rendering the caller when that
 * changes. The app root subscribes so it can go `inert` — see OverlayProvider.
 */
export function useIsModalLayerOpen(): boolean {
    return useSyncExternalStore(subscribeModalLayers, modalLayerIsOpen, alwaysFalse)
}

function modalLayerIsOpen(): boolean {
    return modalLayerCount() > 0
}

// Server snapshot: nothing is open before hydration.
function alwaysFalse(): boolean {
    return false
}
