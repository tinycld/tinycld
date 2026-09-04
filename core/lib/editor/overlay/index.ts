/**
 * Host-side anchored overlays for WebView editors.
 *
 * The native editor is a WebView, so anything that must be drawn by the host —
 * an autocomplete popover, a bottom sheet anchored to a selection — cannot be
 * rendered by the page that decided to show it. The page posts over the 'ui'
 * namespace; this module routes those messages, measures the WebView, and
 * renders a native Modal at the right screen coordinates.
 *
 * Promoted from the text package, which proved the approach on its slash menu.
 * It moved here when boards' @-mentions became the second consumer: siblings
 * cannot import each other, so the alternative was a second copy of a
 * non-trivial state machine and its geometry.
 */

export {
    AnchoredOverlayController,
    type AnchoredOverlayControllerProps,
    type AnchoredOverlayProps,
    type AnchoredOverlayRegistry,
} from './anchored-overlay-controller'
export {
    type AnchoredOverlayAction,
    type AnchoredOverlayRect,
    type AnchoredOverlayRequest,
    type AnchoredOverlayResponseAction,
    type AnchoredOverlayState,
    anchoredOverlayReducer,
    decodeUiMessage,
    initialAnchoredOverlayState,
    resolvePopoverPosition,
} from './anchored-overlay-state'
export {
    type EditorOverlayHandle,
    registerEditorOverlay,
    resetEditorOverlayRegistry,
    useEditorOverlay,
} from './editor-overlay-registry'
export { publishUiMessage, resetUiMessageBus, subscribeUiMessage } from './ui-message-bus'
