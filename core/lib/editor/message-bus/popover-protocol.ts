/**
 * The 'ui' namespace's anchored-popover messages, as types.
 *
 * These shapes were specified in prose in ./types.ts and constructed by hand on
 * both sides — the in-WebView bridge that posts them and the host controller
 * that parses them. Two hand-written spellings of one wire format drift the
 * moment either side gains a field, and the failure is silent: a popover that
 * simply never opens, on a platform with no e2e coverage. Naming them once is
 * what makes that a compile error instead.
 *
 * The prose in ./types.ts remains the narrative explanation of WHY the protocol
 * has this shape; this file is the machine-checkable half.
 *
 * Must stay free of DOM and React Native references — the WebView page and the
 * native host both import it.
 */

/** WebView → host, request. Open an overlay anchored to a document range. */
export const UI_SHOW_POPOVER = 'show-popover'
/** host → WebView, response. Echoes the show-popover's requestId. */
export const UI_POPOVER_RESULT = 'popover-result'
/** WebView → host. Re-renders the open overlay's contents. */
export const UI_POPOVER_UPDATE = 'popover-update'
/** WebView → host. The in-page driver wound down on its own. */
export const UI_POPOVER_EXITED = 'popover-exited'
/**
 * Host-internal, never crosses the WebView boundary. The native editor
 * publishes it into its own bus when the document scrolls, because an overlay
 * anchored to a range that has moved is worse than no overlay.
 */
export const UI_POPOVER_DISMISS_ON_SCROLL = 'popover-dismiss-on-scroll'

/**
 * Where to anchor, in the WebView's viewport coordinates, plus the scroll
 * snapshot the host needs to translate them to screen coordinates.
 *
 * Matches the `ImageSelection.rect` contract the 'ui' namespace already uses.
 * Deliberately not a DOMRect: that is a live object whose values mutate between
 * frames, so it must never be held across a message boundary.
 */
export interface PopoverRect {
    top: number
    left: number
    width: number
    height: number
    scrollX: number
    scrollY: number
}

/** Payload of {@link UI_SHOW_POPOVER}. */
export interface ShowPopoverPayload<TPayload = unknown> {
    /**
     * Which overlay to render. The host keys its registry on this, so two
     * concurrent popover sources must never share a kind.
     */
    kind: string
    rect: PopoverRect
    /** Kind-specific contents. */
    payload: TPayload
    /**
     * Which editor on the screen posted this.
     *
     * Load-bearing wherever more than one editor is mounted at once — a card
     * detail carries a description editor, a comment composer, and possibly an
     * inline comment editor simultaneously. Without it every mounted host
     * controller answers every show-popover, so one `@` opens three overlays,
     * two of them measured against the wrong WebView.
     */
    editorInstanceId?: string
}

/** Payload of {@link UI_POPOVER_UPDATE} — a show-popover minus kind and rect. */
export interface PopoverUpdatePayload<TPayload = unknown> {
    payload: TPayload
    editorInstanceId?: string
}

/** Payload of {@link UI_POPOVER_EXITED}. */
export interface PopoverExitedPayload {
    editorInstanceId?: string
}

/** What the host did with the overlay. */
export type PopoverResultAction = 'select' | 'dismiss'

/** Payload of {@link UI_POPOVER_RESULT}. */
export interface PopoverResultPayload<TPayload = unknown> {
    action: PopoverResultAction
    /** Present on 'select'. Kind-specific. */
    payload?: TPayload
}

/**
 * Contents of a trigger-kind popover — what {@link ShowPopoverPayload.payload}
 * carries for the `trigger:<id>` kinds the rich editor's autocompletes use.
 */
export interface TriggerPopoverPayload {
    items: { id: string; label: string; secondary?: string }[]
    query: string
    selectedIndex: number
}

/** What a trigger popover's 'select' answers with. */
export interface TriggerPopoverSelection {
    itemId: string
}

/** The overlay kind for a rich-editor trigger. Distinct per trigger id. */
export function triggerPopoverKind(triggerId: string): string {
    return `trigger:${triggerId}`
}
