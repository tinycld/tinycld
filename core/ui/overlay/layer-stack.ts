import { useEffect, useRef } from 'react'
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
}

const stack: LayerRecord[] = []
let nextLayerId = 1

/** Test seam. */
export function resetLayers() {
    stack.length = 0
}

export function openLayerCount(): number {
    return stack.length
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

function onPointerDown(event: Event) {
    const layer = layerToDismiss(stack, event.target as Node | null)
    layer?.onDismiss()
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
}

function popLayer(id: number) {
    const index = stack.findIndex(entry => entry.id === id)
    if (index !== -1) stack.splice(index, 1)
    syncListener()
}

interface UseOverlayLayerOptions {
    isOpen: boolean
    /** The surface's node and its anchor (trigger). Read at event time. */
    nodes: () => (Node | null | undefined)[]
    onDismiss: () => void
    dismissOnOutside?: boolean
    dismissOnEscape?: boolean
}

/**
 * Joins the layer stack while `isOpen`. Handles outside pointerdown (web) and
 * the Android back button. Escape is `<LayerEscape>` (escape.tsx), rendered
 * INSIDE the open layer: it joins the shortcut scope stack on mount, so it
 * must not exist while the layer is closed.
 */
export function useOverlayLayer({
    isOpen,
    nodes,
    onDismiss,
    dismissOnOutside = true,
    dismissOnEscape = true,
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
        })
        return () => {
            popLayer(id)
            idRef.current = null
        }
    }, [isOpen, dismissOnOutside, dismissOnEscape])

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
}
