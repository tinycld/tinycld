import { type RefObject, useEffect, useRef } from 'react'
import { Platform } from 'react-native'

const FOCUSABLE =
    'a[href], button:not([disabled]), input:not([disabled]), textarea:not([disabled]), ' +
    'select:not([disabled]), [tabindex]:not([tabindex="-1"]), [role="menuitem"]'

function focusables(container: HTMLElement): HTMLElement[] {
    return Array.from(container.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
        element => element.getAttribute('aria-disabled') !== 'true'
    )
}

interface UseFocusTrapOptions {
    isActive: boolean
    /**
     * The layer's node, as STATE from a callback ref rather than a ref
     * object: a layer portals into a host and may mount a commit after its
     * component does, and an effect keyed on a ref object would run once,
     * see null, and never come back.
     */
    container: HTMLElement | null
    /** Focused on open instead of the first focusable. */
    initialFocusRef?: RefObject<{ focus: () => void } | null>
    /** Keep Tab inside the container. Off for a menu, whose focus is roving. */
    trap?: boolean
    /** Give focus back to whatever had it before the layer opened. */
    restore?: boolean
    /**
     * Moves the focus itself when the layer closes, in place of giving it
     * back to the element that had it before: for a layer whose opener may
     * be gone by then (an action that re-renders the screen). Called at the
     * same point and under the same rules as the default restore — one frame
     * after close, only while nothing else holds the focus — but also when
     * the opener is no longer in the document. Native has no focus restore:
     * there it is called one frame after the layer closes.
     */
    onRestoreFocus?: () => void
}

const nextFrame = (cb: () => void) => {
    if (typeof requestAnimationFrame === 'function') requestAnimationFrame(cb)
    else setTimeout(cb, 0)
}

const AUTOFOCUS_SELECTOR = '[data-autofocus="true"]'

/**
 * The `dataSet` for an input that takes `autoFocus`: the caller's own, plus
 * the `data-autofocus` marker when `autoFocus` is set. react-native-web hands
 * `autoFocus` to React DOM, which focuses the input at mount and writes no
 * attribute; a popover surface is still hidden at that mount, so the focus is
 * lost, and the marker is how the surface finds the input when it shows.
 */
export function autoFocusDataSet({
    autoFocus,
    dataSet,
}: {
    autoFocus?: boolean | null
    dataSet?: Record<string, string | number | undefined>
}) {
    if (!autoFocus) return dataSet
    return { ...dataSet, autofocus: 'true' }
}

/**
 * Focus `initial` if given, else — unless the focus is already inside — the
 * input that asked for focus (`autoFocusDataSet`), else the container's first
 * focusable, else the container itself. Web only.
 */
export function moveFocusIn(container: HTMLElement, initial?: { focus: () => void } | null) {
    if (Platform.OS !== 'web') return
    if (initial) initial.focus()
    else if (!container.contains(document.activeElement)) {
        const asked = container.querySelector<HTMLElement>(AUTOFOCUS_SELECTOR)
        const target = asked ?? focusables(container)[0]
        if (target) target.focus()
        else container.focus()
    }
}

/**
 * Focus for a layer on web: move it in on open, keep it in while open, hand
 * it back on close. Native has no focus ring to manage.
 *
 * Runs in an effect AFTER the layer has painted, which is what the
 * `requestAnimationFrame` workaround in the old PromptDialog was for: an
 * `autoFocus` prop fired before gluestack's trap had settled and lost.
 */
export function useLayerFocus({
    isActive,
    container,
    initialFocusRef,
    trap = true,
    restore = true,
    onRestoreFocus,
}: UseFocusTrapOptions) {
    // The latest callback, read at close: an inline function must not re-run
    // the focus effect (and so restore focus) on every render.
    const onRestoreFocusRef = useRef(onRestoreFocus)
    onRestoreFocusRef.current = onRestoreFocus

    useEffect(() => {
        if (Platform.OS === 'web' || !isActive || !restore) return
        return () => nextFrame(() => onRestoreFocusRef.current?.())
    }, [isActive, restore])

    useEffect(() => {
        if (Platform.OS !== 'web' || !isActive || typeof document === 'undefined') return
        if (!container) return
        const previous = document.activeElement as HTMLElement | null

        moveFocusIn(container, initialFocusRef?.current)

        const onKeyDown = (event: KeyboardEvent) => {
            if (!trap || event.key !== 'Tab') return
            const items = focusables(container)
            if (items.length === 0) {
                event.preventDefault()
                return
            }
            const first = items[0]
            const last = items[items.length - 1]
            const active = document.activeElement
            if (event.shiftKey && (active === first || !container.contains(active))) {
                event.preventDefault()
                last.focus()
            } else if (!event.shiftKey && (active === last || !container.contains(active))) {
                event.preventDefault()
                first.focus()
            }
        }
        container.addEventListener('keydown', onKeyDown)

        return () => {
            container.removeEventListener('keydown', onKeyDown)
            if (!restore) return
            const moveFocus = onRestoreFocusRef.current
            const canRestorePrevious =
                previous?.isConnected === true && typeof previous.focus === 'function'
            if (!moveFocus && !canRestorePrevious) return
            // Hand focus back ONLY if nothing else has claimed it. A menu row
            // may act by moving focus somewhere new — the board header's
            // "Rename board" mounts an autoFocus input — and that focus lands
            // BEFORE this layer unmounts. Restoring unconditionally stole it
            // back, blurring the new field one frame after it appeared; an
            // input that commits on blur then closed itself, so the row looked
            // dead. Focus still inside the closing layer (or dropped to body)
            // means no one else wanted it, which is when restoring is right.
            const active = document.activeElement as HTMLElement | null
            const isUnclaimed = !active || active === document.body || container.contains(active)
            if (!isUnclaimed) return

            // Deferred a frame: a modal layer holds the rest of the app
            // `inert` while it is open, and the re-render that drops it
            // commits AFTER this cleanup. Focusing an element that is still
            // inert silently does nothing, and focus falls to the body — so
            // wait for the removal to land. Re-check on the way in, because a
            // frame is long enough for something else to claim focus.
            nextFrame(() => {
                const now = document.activeElement as HTMLElement | null
                const latest = onRestoreFocusRef.current
                // A Sheet stays in the document while it slides out, with the
                // focus still inside it: for the callback that is unclaimed too.
                const isStillInLayer = !!latest && !!now && container.contains(now)
                if (now && now !== document.body && !isStillInLayer) return
                if (latest) latest()
                else if (previous?.isConnected) previous.focus()
            })
        }
    }, [isActive, container, initialFocusRef, trap, restore])
}
