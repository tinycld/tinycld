import { type RefObject, useEffect } from 'react'
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
}: UseFocusTrapOptions) {
    useEffect(() => {
        if (Platform.OS !== 'web' || !isActive || typeof document === 'undefined') return
        if (!container) return
        const previous = document.activeElement as HTMLElement | null

        const initial = initialFocusRef?.current
        if (initial) initial.focus()
        else if (!container.contains(document.activeElement)) {
            const first = focusables(container)[0]
            if (first) first.focus()
            else container.focus()
        }

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
            if (!restore || !previous?.isConnected || typeof previous.focus !== 'function') return
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
            if (isUnclaimed) previous.focus()
        }
    }, [isActive, container, initialFocusRef, trap, restore])
}
