// Cross-WebView message protocol used by any editor hook backed by
// TenTap's customSource pattern. TenTap's built-in messages keep their
// existing shape under namespaces 'core' and 'format' so TenTap's
// useBridgeState and built-in bridges continue to work unmodified.
// Additional namespaces are reserved for app-specific concerns:
//
//   'app'           - package-specific init/lifecycle (init payload, etc.)
//   'awareness'     - Yjs Awareness cursor/presence reporting
//   'comment'       - comment threads (reserved for v1.1; protected from
//                     future collision now)
//   'core'          - TenTap's CoreMessages (StateUpdate, EditorReady, etc.)
//   'find-replace'  - in-WebView find/replace plugin command + state
//                     channel (used by the text package's FindReplaceBar
//                     on native).
//   'format'        - TenTap's per-bridge format commands (ToggleBold, etc.)
//   'markdown'      - set/get for editors whose document format IS markdown
//                     (card descriptions). The WebView owns the serializer,
//                     so nothing pivots through HTML.
//   'ui'            - the in-WebView editor asks the host to mediate UI
//                     (e.g. show a bottom sheet anchored to the selected
//                     image; an anchored popover for the slash menu).
//   'yjs'           - binary Yjs updates relayed between the host's Y.Doc and
//                     the WebView's. Reserved now, unused today: see below.
//
// Adding a new namespace requires updating this file's union. Adding
// a new message type within an existing namespace is non-breaking.
//
// 'markdown' namespace message types:
//
//   set   (host -> WebView, no response)
//     payload: { markdown: string }
//     Replaces the document. A no-op under collaboration, where the shared
//     document is the source of truth.
//
//   get   (host -> WebView, request)
//     payload: null. Carries a requestId.
//
//   result  (WebView -> host, response)
//     payload: { markdown: string }, requestId echoed.
//     The WebView applies repairMarkdown() before posting — unrepaired
//     @tiptap/markdown output corrupts code spans containing a backtick and
//     drops table-cell pipe escapes, and this value gets persisted.
//
// 'yjs' and 'awareness' namespaces (native collaborative editing):
//
//   Yjs updates and awareness states are binary (Uint8Array) and this channel
//   is a JSON string pipe, so both ride base64-encoded in the payload. 'yjs'
//   carries the document; 'awareness' carries collaborator carets.
//
//   The host keeps the single existing room socket and relays across these
//   channels. The WebView must NOT open its own connection: a second one ships
//   a credential into the page and gives the local user a second awareness
//   identity, so one human shows up as two peers. Both boards and text ride the
//   relay for exactly that reason.
//
//   The awareness relay is deliberately asymmetric — the page sends its cursor
//   POSITION and the host merges it into its own slot, rather than the page
//   sending an awareness state of its own. See
//   `core/lib/editor/rich/awareness-webview-host.ts`.
//
// 'ui' namespace message types:
//
//   The popover messages below are TYPED in ./popover-protocol.ts, which both
//   the in-WebView bridge and the host controller import. What follows is the
//   narrative half — why the protocol has this shape. Change both together.
//
//   selection-changed  (WebView -> host, no response)
//     payload: { kind: 'image' | 'none', image?: ImageSelection }
//     Already in use since Milestone B.
//
//   show-popover  (WebView -> host, request)
//     payload: { kind: 'slash-menu' | string, rect, payload }
//       rect: { top, left, width, height, scrollX, scrollY } in viewport
//         coords + WebView scroll snapshot (matches ImageSelection.rect
//         contract from Milestone B).
//       payload: kind-specific data. For 'slash-menu': { items, query,
//         selectedIndex }.
//     The host renders an anchored overlay and answers via popover-result
//     keyed on requestId. Without requestId the message is malformed.
//
//   popover-result  (host -> WebView, response)
//     requestId echoes the show-popover's requestId.
//     payload: { action: 'select' | 'dismiss', payload?: kind-specific }
//       For 'slash-menu' select: payload is { commandId: string }.
//
//   popover-update  (WebView -> host, no response)
//     Updates the visible overlay's payload (e.g. items as the user types
//     more after the trigger). requestId of the original show-popover.
//     payload: same shape as show-popover.payload minus the kind+rect.
//     If the host has dismissed the popover, this is a no-op.
//
//   popover-exited  (WebView -> host, no response)
//     The WebView's suggestion plugin (or whatever drove the show-popover)
//     has exited on its own — user typed a space that broke the trigger,
//     selected an item, pressed Escape, etc. The host closes any overlay
//     still open for this requestId. If the host already dismissed
//     (backdrop tap), the message is a no-op.
//
//   popover-dismissed  (host -> WebView, no response)
//     Reserved for future use: a host that wants to programmatically
//     dismiss an overlay the WebView didn't initiate the close on (e.g.
//     navigation away, an external trigger). The host posts this so the
//     WebView can clean up its own suggestion-plugin state.
//     Currently unused — the WebView learns of dismissals via
//     popover-result with action='dismiss' instead. Kept here so adding
//     a host-initiated dismissal later doesn't reshuffle the protocol.
//
// 'find-replace' namespace message types:
//
//   host → WebView (no response):
//     set-query         payload: { query: string }
//     clear             payload: null
//     next              payload: null
//     prev              payload: null
//     replace-current   payload: { replacement: string }
//     replace-all       payload: { replacement: string }
//
//   WebView → host (broadcast on every plugin-state change):
//     state-update      payload: { matchCount: number,
//                                   currentIndex: number,
//                                   query: string }
//     The WebView posts this on every transaction whose effect on the
//     find-replace plugin state differs from the prior post (identity
//     skip via serialized payload comparison). The host's bar reads the
//     mirrored state from a Zustand store to render match counts and
//     dispatch commands through useWebViewEditor.postMessage.

export type EditorMessageNamespace =
    | 'app'
    | 'awareness'
    | 'comment'
    | 'core'
    | 'find-replace'
    | 'format'
    | 'markdown'
    | 'suggestion'
    | 'ui'
    | 'yjs'

export interface EditorMessage<TPayload = unknown> {
    namespace: EditorMessageNamespace
    type: string
    // Present iff this is a request expecting a response. Receiver
    // echoes the requestId in its response so the requester can
    // correlate.
    requestId?: string
    payload: TPayload
}

// Helpful constructor - most call sites won't bother with requestId.
export function makeMessage<T>(
    namespace: EditorMessageNamespace,
    type: string,
    payload: T,
    requestId?: string
): EditorMessage<T> {
    return requestId !== undefined
        ? { namespace, type, payload, requestId }
        : { namespace, type, payload }
}

// Discriminant guard for narrowing in handlers.
export function isMessageNamespace<N extends EditorMessageNamespace>(
    message: EditorMessage,
    namespace: N
): message is EditorMessage & { namespace: N } {
    return message.namespace === namespace
}
