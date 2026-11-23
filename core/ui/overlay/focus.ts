import { log } from '@tinycld/core/lib/logger'
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
    /**
     * Give focus back to whatever had it before the layer opened. Read when
     * the layer closes: changing it while the layer is open restores nothing.
     */
    restore?: boolean
    /**
     * Moves the focus itself when the layer closes, in place of giving it
     * back to the element that had it before: for a layer whose opener may
     * be gone by then (an action that re-renders the screen).
     *
     * On web it is called one frame after the layer closes or unmounts, only
     * when the focus is then on the body, on nothing, or still inside the
     * closing layer (a Sheet sliding out) — the same rules as the default
     * restore — and also when the opener is no longer in the document. On
     * native, which has no default restore, it is called one frame after the
     * layer closes or unmounts. A change of the other options while the layer
     * stays open does not call it.
     *
     * It also runs when the layer's owner unmounts while the layer is open,
     * with no close (browser Back from a screen with an open Dialog), so the
     * next screen may already be up: focus only refs of your own screen,
     * never an element found by a query. An error it throws is logged
     * (`core.overlay.focus`) and goes no further.
     */
    onRestoreFocus?: () => void
}

const nextFrame = (cb: () => void) => {
    if (typeof requestAnimationFrame === 'function') requestAnimationFrame(cb)
    else setTimeout(cb, 0)
}

// The owner's code runs from a frame callback, where a throw is uncaught (on
// native in a release build that can end the app): report it instead.
function callRestoreFocus(onRestoreFocus: () => void) {
    try {
        onRestoreFocus()
    } catch (err) {
        log.error('core.overlay.focus', err)
    }
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
    // The latest options, read at close. The restore belongs to the close
    // alone: an effect keyed on them would run its cleanup, and so restore
    // the focus, whenever one changed while the layer stayed open (an inline
    // callback on every render, a portal that re-mounts the surface).
    const onRestoreFocusRef = useRef(onRestoreFocus)
    onRestoreFocusRef.current = onRestoreFocus
    const restoreRef = useRef(restore)
    restoreRef.current = restore
    const containerRef = useRef(container)
    containerRef.current = container

    useEffect(() => {
        if (Platform.OS === 'web' || !isActive) return
        return () => {
            if (!restoreRef.current) return
            nextFrame(() => {
                const latest = onRestoreFocusRef.current
                if (latest) callRestoreFocus(latest)
            })
        }
    }, [isActive])

    // Keyed on `isActive` alone, and declared before the focus-in effect so
    // that on the commit that opens the layer it reads the opener before the
    // focus moves in.
    useEffect(() => {
        if (Platform.OS !== 'web' || !isActive || typeof document === 'undefined') return
        const previous = document.activeElement as HTMLElement | null
        return () => {
            if (!restoreRef.current) return
            restoreOnClose(previous, containerRef.current, onRestoreFocusRef)
        }
    }, [isActive])

    useEffect(() => {
        if (Platform.OS !== 'web' || !isActive || typeof document === 'undefined') return
        if (!container) return

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
        return () => container.removeEventListener('keydown', onKeyDown)
    }, [isActive, container, initialFocusRef, trap])
}

/**
 * Web: at the layer's close, hand the focus back to `previous` (or call the
 * owner's `onRestoreFocus`), if nothing else has claimed it.
 */
function restoreOnClose(
    previous: HTMLElement | null,
    container: HTMLElement | null,
    onRestoreFocusRef: RefObject<(() => void) | undefined>
) {
    // A layer whose surface never mounted never took the focus.
    if (!container) return
    const canRestorePrevious =
        previous?.isConnected === true && typeof previous.focus === 'function'
    if (!onRestoreFocusRef.current && !canRestorePrevious) return
    // Hand focus back ONLY if nothing else has claimed it. A menu row
    // may act by moving focus somewhere new — the board header's
    // "Rename board" mounts an autoFocus input — and that focus lands
    // BEFORE this layer unmounts. Restoring unconditionally stole it
    // back, blurring the new field one frame after it appeared; an
    // input that commits on blur then closed itself, so the row looked
    // dead. Focus still inside the closing layer (or dropped to body)
    // means no one else wanted it, which is when restoring is right.
    if (!isUnclaimed(container)) return

    // Deferred a frame: a modal layer holds the rest of the app
    // `inert` while it is open, and the re-render that drops it
    // commits AFTER this cleanup. Focusing an element that is still
    // inert silently does nothing, and focus falls to the body — so
    // wait for the removal to land. Re-check on the way in, because a
    // frame is long enough for something else to claim focus. A Sheet
    // stays in the document while it slides out, with the focus still
    // inside it, so the re-check counts that as unclaimed too.
    nextFrame(() => {
        if (!isUnclaimed(container)) return
        const latest = onRestoreFocusRef.current
        if (latest) callRestoreFocus(latest)
        else if (previous?.isConnected) previous.focus()
    })
}

function isUnclaimed(container: HTMLElement) {
    const active = document.activeElement as HTMLElement | null
    return !active || active === document.body || container.contains(active)
}
