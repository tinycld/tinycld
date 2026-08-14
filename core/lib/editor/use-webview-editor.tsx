import {
    type BridgeExtension,
    type EditorBridge,
    useBridgeState,
    useEditorBridge,
} from '@10play/tentap-editor'
import type React from 'react'
import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react'
import { View } from 'react-native'
import type { WebViewMessageEvent } from 'react-native-webview'
import { deriveToolbarState } from './derive-toolbar-state'
import { createHeightStore, type HeightStore } from './height-store'
import { type EditorMessage, makeMessage } from './message-bus/types'
import type { EditorCommands, EditorHandle, EditorResult } from './types'
import { buildWebViewEditorCommands } from './webview-editor-commands'

export interface UseWebViewEditorOptions {
    // The pre-built HTML string that hosts the in-WebView editor.
    // Produced by a package-specific build step calling
    // bundleWebViewEditor. The string contains a TipTap-React instance
    // configured with whatever extensions the package wants.
    editorHtml: string

    // TenTap bridges for native<->WebView command routing. These are
    // the standard ones (BoldBridge, ItalicBridge, ...) plus any
    // package-specific bridges. They run on the native side; their
    // counterpart TipTap extensions live inside the WebView (compiled
    // into editorHtml).
    bridgeExtensions: BridgeExtension[]

    // App-specific init payload posted into the WebView once it
    // signals EditorReady. Typed as unknown because each package
    // chooses what to send (auth token, room id, user identity, ...).
    // The in-WebView Editor.tsx parses it via JSON.parse.
    initPayload: unknown

    // Forwarded to TenTap's bridge.setEditable. Toggles whether the
    // editor accepts user input; consumers also use this to disable
    // their toolbar UI.
    editable: boolean

    // Forwarded to useEditorBridge for things like webview
    // background color, etc. Optional.
    theme?: Record<string, unknown>

    // Optional TenTap-level avoidIosKeyboard flag. Defaults to true.
    avoidIosKeyboard?: boolean

    // Optional initial content (HTML). Most consumers won't set this
    // because the editor populates itself from the Y.Doc bootstrap.
    initialContent?: string

    // Whether the WebView contains its own scroll behavior. Pass false
    // when the editor is embedded inside an outer ScrollView (e.g. mail
    // compose); pass true when the editor is the scroll surface (e.g.
    // text document edit). Defaults to true.
    scrollEnabled?: boolean

    // Floor for the WebView's height, in px.
    //
    // Load-bearing whenever the editor sits inside a ScrollView: a
    // `flex-1` child of an unbounded parent resolves to zero, and a
    // zero-height WebView renders nothing at all while reporting no
    // error — the document is there, simply invisible. The default is
    // roughly three lines, enough to read a short description and to
    // make an empty editor look like somewhere to type.
    minHeight?: number

    // Subscribe to messages with the 'ui' namespace from the WebView.
    // Called for every parsable message whose namespace === 'ui'; the
    // payload shape depends on the message type and is the consumer's
    // responsibility to interpret. TenTap's built-in messages (state
    // updates, core action responses) still flow through their own
    // channel via bridgeExtensions — we don't intercept those.
    //
    // The callback identity is read through a ref, so the consumer
    // doesn't have to memoize it.
    onUiMessage?: (message: EditorMessage) => void

    // Called when the WebView reports an in-document scroll event from
    // its injected scroll listener. Anchored popovers rendered by the
    // host (slash menu, future image/comment popovers) subscribe to
    // this and dismiss themselves so the overlay doesn't drift away
    // from the anchored element when the user scrolls. iOS RN-WebView's
    // own `onScroll` does not fire for in-document scrolling when
    // `scrollEnabled` is false (which TenTap sets), so the WebView
    // installs a document-level scroll listener that posts a
    // {namespace:'ui', type:'document-scroll'} message; this callback
    // is invoked on every such message.
    //
    // The callback identity is read through a ref.
    onScroll?: () => void

    // Called when the WebView's editing surface gains or loses focus,
    // on the edge only. Fed by the `isFocused` field of the stateUpdate
    // payload, so a page that doesn't broadcast it simply never fires
    // this. Drives focus-gated host chrome (e.g. a formatting toolbar).
    //
    // The callback identity is read through a ref.
    onFocusChange?: (isFocused: boolean) => void

    // Subscribe to messages with the 'comment' namespace from the
    // WebView. The text package's comment bridge uses this to route
    // tap / removed / selection-response / focus-response messages
    // into host-side resolvers and handler sets.
    //
    // Each call replaces any prior handler — the value is read through
    // a ref, so the consumer doesn't have to memoize it.
    onCommentMessage?: (message: EditorMessage) => void

    // Subscribe to messages with the 'find-replace' namespace from the
    // WebView. The text package's native find-replace controller uses
    // this to push the in-WebView plugin's state-update broadcasts
    // (matchCount / currentIndex / query) into a host-side Zustand
    // store that the FindReplaceBar mirrors.
    //
    // Each call replaces any prior handler — the value is read through
    // a ref, so the consumer doesn't have to memoize it.
    onFindReplaceMessage?: (message: EditorMessage) => void

    // Subscribe to off-protocol {kind, payload} messages emitted by the
    // WebView's suggestion list bridge. Unlike the namespace-based
    // channels above, the Phase 2c suggestion bridge posts a flat
    // envelope ({kind: 'suggestion.changed', payload}) so the receiver
    // can route by kind string into the NativeSuggestionBridge's
    // processIncomingMessage(kind, payload) without going through the
    // EditorMessage type. Today the only kind is 'suggestion.changed';
    // additional kinds (e.g. 'suggestion.list-reply') can be added
    // without expanding the EditorMessageNamespace union.
    //
    // The callback identity is read through a ref.
    onSuggestionMessage?: (kind: string, payload: unknown) => void

    // Subscribe to messages the WebView posts on namespaces this hook has no
    // dedicated channel for — today 'markdown' (the shared rich editor's
    // set/get responses) and 'app' (submit-shortcut, escape). Reserved 'yjs'
    // updates will arrive here too when native collaboration lands, which is
    // why this is a namespace-agnostic fallback rather than another named
    // callback: adding a namespace shouldn't mean adding a prop.
    //
    // Runs after the named channels above, so it never shadows them.
    //
    // The callback identity is read through a ref.
    onMessage?: (message: EditorMessage) => void
}

// Shared TenTap-customSource wrapper. Encapsulates:
//   - useEditorBridge with the package's editorHtml + bridges
//   - useBridgeState subscription
//   - the EditorReady -> init-payload handshake
//   - adapting TenTap's command surface to the EditorResult contract
//
// Returns the same EditorResult shape consumers expect from any
// useDocumentEditor / useMailEditor variant.
export function useWebViewEditor(options: UseWebViewEditorOptions): EditorResult {
    const {
        editorHtml,
        bridgeExtensions,
        initPayload,
        editable,
        theme,
        avoidIosKeyboard = true,
        initialContent,
        scrollEnabled = true,
        minHeight = 72,
        onUiMessage,
        onScroll,
        onFocusChange,
        onCommentMessage,
        onFindReplaceMessage,
        onSuggestionMessage,
        onMessage,
    } = options

    // Pin onUiMessage behind a ref so the consumer can pass an
    // identity-fresh closure on each render without remounting the
    // WebView. The RichText component is recreated when EditorComponent
    // does its useMemo dance below; reading the latest callback off
    // the ref keeps the message bridge stable across re-renders.
    const onUiMessageRef = useRef(onUiMessage)
    onUiMessageRef.current = onUiMessage

    // Same indirection for onScroll. The 'ui' namespace fan-out below
    // recognizes 'document-scroll' and routes it to this ref. Keeping
    // it separate from onUiMessage means consumers don't have to write
    // a switch over message.type just to react to scroll, and the
    // event shape stays an implementation detail of the WebView.
    const onScrollRef = useRef(onScroll)
    onScrollRef.current = onScroll

    // Mirrors onUiMessageRef — the 'comment' namespace fan-out routes
    // every parsable comment message through this ref. The text
    // package's native comment bridge is the sole consumer today.
    const onCommentMessageRef = useRef(onCommentMessage)
    onCommentMessageRef.current = onCommentMessage

    // Same ref-backed pattern for the 'find-replace' namespace. The
    // text package's native FindReplaceController routes state-update
    // broadcasts from the in-WebView plugin into its Zustand mirror
    // through this hook.
    const onFindReplaceMessageRef = useRef(onFindReplaceMessage)
    onFindReplaceMessageRef.current = onFindReplaceMessage

    // Same ref-backed pattern for the off-protocol suggestion-bridge
    // messages. The handler is keyed on parsed.kind (not namespace) so
    // the WebView's list-bridge can keep its simpler {kind, payload}
    // shape from Phase 2c Task 12.
    const onSuggestionMessageRef = useRef(onSuggestionMessage)
    onSuggestionMessageRef.current = onSuggestionMessage

    // Namespace-agnostic fallback for channels without a dedicated ref above.
    // The shared rich editor routes 'markdown' and 'app' through here.
    const onMessageRef = useRef(onMessage)
    onMessageRef.current = onMessage

    const liveBridge = useEditorBridge({
        initialContent,
        bridgeExtensions,
        theme,
        autofocus: false,
        avoidIosKeyboard,
        customSource: editorHtml,
    })

    // useEditorBridge returns a fresh wrapper object every render even
    // though its underlying refs are stable. Pinning the first wrapper
    // prevents the WebView from remounting on every parent re-render.
    // Mirrors mail's useMailEditor pattern.
    const bridgeRef = useRef<EditorBridge>(liveBridge)
    const bridge = bridgeRef.current

    const bridgeState = useBridgeState(bridge)

    // Fire onFocusChange on the EDGE, not on every state update: the
    // WebView rebroadcasts its whole payload on each transaction, so
    // calling on every render would invoke the consumer once per
    // keystroke. Undefined (a page that doesn't broadcast the field)
    // never produces an edge, so non-rich WebView editors are unaffected.
    const isWebViewFocused = (bridgeState as unknown as Record<string, unknown>).isFocused
    const focusChangeRef = useRef(onFocusChange)
    focusChangeRef.current = onFocusChange
    const lastFocusRef = useRef<boolean | undefined>(undefined)
    useEffect(() => {
        if (typeof isWebViewFocused !== 'boolean') return
        if (lastFocusRef.current === isWebViewFocused) return
        lastFocusRef.current = isWebViewFocused
        focusChangeRef.current?.(isWebViewFocused)
    }, [isWebViewFocused])

    // Plumb editable changes through to the WebView. setEditable is
    // safe to call before EditorReady; TenTap queues it.
    useEffect(() => {
        bridge.setEditable(editable)
    }, [bridge, editable])

    // The WebView's in-page React app posts {type:'editor-ready'} as
    // soon as the top-level <Editor /> mounts — BEFORE it constructs
    // its TipTap instance, because TipTap construction is gated on the
    // init payload from native. So this is the right signal to gate
    // the init post on. Note that we can't use bridgeState.isReady
    // (TenTap's StateUpdate-driven flag) for this: that one only flips
    // when the WebView sends a `stateUpdate`, which our custom Editor
    // only sends after init arrives — chicken-and-egg.
    const [webviewReady, setWebviewReady] = useState(false)
    const mountAtRef = useRef(Date.now())

    // Height the page reported for its own content, held in a tiny store
    // rather than state so that a new measurement re-renders ONLY the box
    // wrapping the WebView. Putting it in state here would change
    // EditorComponent's identity and remount the WebView on every
    // measurement — see the memo below.
    // Created once and never reassigned, so both the store and its setter are
    // stable for the life of the hook. Read into locals rather than through
    // `.current` at each use site: the memos below genuinely depend on them, and
    // a `ref.current` in a dep array is neither honest nor something the linter
    // can reason about.
    const heightStoreRef = useRef<HeightStore>(null as unknown as HeightStore)
    if (heightStoreRef.current === null) heightStoreRef.current = createHeightStore()
    const heightStore = heightStoreRef.current
    const setContentHeight = heightStore.set

    // Post the package's init payload once the WebView signals ready.
    // Idempotent guard prevents double-init on hot-reload edge cases.
    // Intentionally one-shot per mount; if the in-WebView page reloads
    // itself (transient disconnect, in-WebView crash), it must re-fetch
    // state from its own bootstrap rather than rely on a re-init
    // payload from native.
    const initSentRef = useRef(false)
    useEffect(() => {
        if (initSentRef.current) return
        if (!webviewReady) return
        const webview = bridge.webviewRef?.current
        if (!webview) return
        const message = makeMessage('app', 'init', initPayload)
        try {
            console.log('[LAG] init-sent at', Date.now() - mountAtRef.current, 'ms')
            webview.postMessage(JSON.stringify(message))
            initSentRef.current = true
        } catch {
            // postMessage can fail mid-handshake; the next render's
            // webviewReady or bridge identity change will retry.
        }
    }, [bridge, webviewReady, initPayload])

    const editor: EditorHandle = useMemo(
        () => ({
            getHTML: () => bridge.getHTML(),
            getText: () => bridge.getText(),
            setContent: (html: string) => bridge.setContent(html),
            focus: (position?: 'start' | 'end') => bridge.focus(position ?? 'end'),
            clear: () => bridge.setContent(''),
            // Native selection query is a request/response round-trip.
            // The in-WebView editor responds to {app,getSelection,reqId}
            // with {app,selectionResult,reqId}. Each call generates a
            // fresh requestId and waits on a one-shot resolver. v1
            // stub: return null until the Phase 4 work wires the
            // selection-query bridge. Phase 5 (awareness cursor) is the
            // first consumer.
            getSelection: () => Promise.resolve(null),
        }),
        [bridge]
    )

    const commands: EditorCommands = useMemo(() => buildWebViewEditorCommands(bridge), [bridge])

    // bridgeState carries every field posted in the WebView's
    // stateUpdate payload, but TenTap only types the fields registered
    // via bridge extensions. Our customSource Editor posts
    // isInTable/selectionEmpty/wordCount/etc. too — deriveToolbarState
    // reads them through a loose record view to avoid a declaration-
    // merging ceremony per consumer. The helper itself is pure so a
    // unit test can drive it against a synthetic bridgeState shape.
    const toolbarState = deriveToolbarState(bridgeState as unknown as Record<string, unknown>)

    // Wraps the WebView's onMessage so TenTap's bridge-extension dispatch
    // (state updates, core action responses) still runs while we layer
    // on 'ui' / 'comment' namespace observation. exclusivelyUseCustomOnMessage
    // is explicitly false so RichText's own handler keeps firing —
    // passing true would silence every TenTap bridge, including the
    // StateUpdate that powers useBridgeState. The handler ignores any
    // message that doesn't carry our explicit { namespace } envelope
    // (TenTap's own messages are typed { type, payload } without one).
    //
    // 'document-scroll' is a special-case 'ui' message that the WebView
    // posts from a window-level scroll listener; we fan it out to
    // onScroll(...) instead of forwarding to onUiMessage so consumers
    // can take it without writing a switch over message.type.
    const onWebViewMessage = useMemo(
        () => (event: WebViewMessageEvent) => {
            const data = event?.nativeEvent?.data
            if (typeof data !== 'string') return
            let parsed: EditorMessage
            try {
                parsed = JSON.parse(data) as EditorMessage
            } catch {
                return
            }
            // The WebView's bootstrap posts {type:'editor-ready'} as
            // its first message, before TipTap mounts. That's the
            // signal we use to gate the init post — see webviewReady
            // above. TenTap's onMessage also sees this (we run
            // alongside its dispatch), but it doesn't flip any
            // bridgeState flag from it.
            if (parsed.type === 'editor-ready') {
                console.log('[LAG] page-ready at', Date.now() - mountAtRef.current, 'ms')
                setWebviewReady(true)
                return
            }
            if (parsed.namespace === 'ui') {
                if (parsed.type === 'document-scroll') {
                    onScrollRef.current?.()
                    return
                }
                // The page measured itself. A WebView has no intrinsic
                // height, so this is the only way the container can track
                // its content — without it the editor is clipped to a
                // guess (or, inside a ScrollView where flex resolves to
                // zero, invisible).
                if (parsed.type === 'content-height') {
                    const height = (parsed.payload as { height?: unknown } | undefined)?.height
                    if (typeof height === 'number' && height > 0) {
                        console.log('[LAG] first-height at', Date.now() - mountAtRef.current, 'ms')
                        setContentHeight(height)
                    }
                    return
                }
                onUiMessageRef.current?.(parsed)
                return
            }
            if (parsed.namespace === 'comment') {
                onCommentMessageRef.current?.(parsed)
                return
            }
            if (parsed.namespace === 'find-replace') {
                onFindReplaceMessageRef.current?.(parsed)
                return
            }
            // Off-protocol {kind, payload} envelope used by the
            // suggestion list bridge. Falls through the namespace
            // checks above because the bridge intentionally keeps the
            // simpler shape — there's no requestId correlation or
            // other namespace-grade machinery needed for the one-way
            // snapshot push.
            const kind = (parsed as { kind?: unknown }).kind
            if (typeof kind === 'string' && kind === 'suggestion.changed') {
                onSuggestionMessageRef.current?.(kind, (parsed as { payload?: unknown }).payload)
                return
            }
            // Anything left that carries a namespace goes to the generic
            // subscriber — the shared rich editor's 'markdown' and 'app'
            // channels, plus 'yjs' and 'awareness' for native collaboration.
            if (typeof parsed.namespace === 'string') {
                onMessageRef.current?.(parsed)
            }
        },
        // Everything else is read through a ref; setContentHeight comes from a
        // store created once per mount, so this list never changes in practice.
        [setContentHeight]
    )

    // The anchor host overlays measure against.
    //
    // NOT the WebView ref. Under the New Architecture (Bridgeless) TenTap's
    // `webviewRef.current` is a Fabric native-command handle exposing only
    // WebView commands — goForward, reload, postMessage, injectJavaScript and
    // friends — with no `measure` or `measureInWindow` on it or its prototype.
    // Every anchored popover therefore measured null, and the controller's
    // fail-closed path dismissed the request before drawing anything: on
    // device the mention picker never appeared at all.
    //
    // This ref points at the plain host View wrapping the WebView, which is an
    // ordinary RN view with the usual measurement methods. It is the same box,
    // so its origin is the WebView's origin — exactly what the popover math
    // wants — and it keeps working regardless of what TenTap's ref becomes.
    const measureRef = useRef<View | null>(null)

    // RichText is loaded lazily inside the EditorComponent because this
    // hook is a single non-platform file (not a .native.tsx split). A
    // top-level import of RichText would force web bundles to resolve
    // react-native-webview, which has no web shim. The lazy require runs
    // only when EditorComponent renders, which only happens on native.
    const EditorComponent = useMemo(
        () =>
            function WebViewEditorContent() {
                const { RichText } = require('@10play/tentap-editor')
                return (
                    // Height is the page's to report, not ours to guess. A
                    // WebView has no intrinsic height, and `flex-1` resolves
                    // to ZERO inside a ScrollView (nothing bounded to fill),
                    // so the editor renders invisibly — no error, just a gap.
                    // Once the page measures itself we take that height
                    // exactly; `minHeight` covers the frames before the first
                    // measurement.
                    //
                    // When the editor IS the scroll surface (scrollEnabled),
                    // it owns a bounded viewport and should keep filling it
                    // rather than growing with its content.
                    <EditorHeightBox
                        heightStore={heightStore}
                        minHeight={minHeight}
                        grows={!scrollEnabled}
                        measureRef={measureRef}
                    >
                        <RichText
                            editor={bridge}
                            scrollEnabled={scrollEnabled}
                            onMessage={onWebViewMessage}
                            exclusivelyUseCustomOnMessage={false}
                        />
                    </EditorHeightBox>
                )
            },
        // contentHeight is deliberately ABSENT: this memo produces a component
        // IDENTITY, and consumers render it as <EditorComponent />. A new
        // identity unmounts and remounts the WebView, which resets its
        // viewport to minHeight — so feeding the measured height in here
        // makes the editor thrash between 72px and its real height and never
        // settle. The height is subscribed to inside EditorHeightBox instead.
        // measureRef is deliberately absent: it is a useRef object, stable for
        // the life of the mount. Listing it would be a no-op at best, and this
        // memo produces a component IDENTITY — a new one remounts the WebView.
        [bridge, scrollEnabled, onWebViewMessage, minHeight, heightStore]
    )

    // Surface the WebView ref through the EditorResult so host code can
    // call .measure(...) to translate the WebView's viewport coords
    // into screen coords (for anchored popovers) and .postMessage(...)
    // to send 'ui' namespace responses back. The ref's `current` is
    // the underlying react-native-webview instance — opaque to us, the
    // anchored-overlay controller narrows it at the call site.
    const webViewRef = bridge.webviewRef as React.RefObject<unknown>

    // Generic message poster. Native consumers (e.g. the text package's
    // comment bridge) use this to drive WebView-side handlers that
    // don't have a first-class command surface on `commands`. Returns
    // false when the WebView isn't mounted yet so callers can choose
    // to swallow or surface the failure. Web variants of consuming
    // hooks return `() => false` instead because there's no WebView.
    const postMessage = useCallback(
        (message: EditorMessage): boolean => {
            const webview = bridge.webviewRef?.current as
                | { postMessage?: (s: string) => void }
                | null
                | undefined
            if (!webview || typeof webview.postMessage !== 'function') return false
            webview.postMessage(JSON.stringify(message))
            return true
        },
        [bridge]
    )

    return {
        editor,
        EditorComponent,
        commands,
        toolbarState,
        webViewRef,
        measureRef,
        postMessage,
        isReady: bridgeState.isReady === true,
    }
}

/**
 * Sizes the WebView to the height the page reported.
 *
 * A WebView has no intrinsic height, and `flex-1` resolves to zero inside a
 * ScrollView (nothing bounded to fill), so without this the editor is either
 * invisible or clipped to a guess. `grows` is false when the editor IS the
 * scroll surface — there it owns a bounded viewport and should fill it rather
 * than growing with its content.
 */
function EditorHeightBox({
    heightStore,
    minHeight,
    grows,
    measureRef,
    children,
}: {
    heightStore: HeightStore
    minHeight: number
    grows: boolean
    /** Anchor for host overlays — see `measureRef` on the hook's result. */
    measureRef?: React.RefObject<View | null>
    children: React.ReactNode
}) {
    const height = useSyncExternalStore(heightStore.subscribe, heightStore.get, heightStore.get)
    const resolved =
        grows && height != null ? { height: Math.max(height, minHeight) } : { minHeight }
    return (
        <View
            ref={measureRef}
            // `flex-1` ONLY when the editor is the scroll surface and should
            // fill its parent. When it grows with its content the height above
            // is the answer, and flex-1 actively fights it: it carries
            // flexShrink:1, so a tight parent shrinks the box below that height
            // — the comment composer inside the card peek collapsed to a
            // one-line sliver while its own toolbar and Send button, which do
            // not shrink, kept their size. flexShrink:0 pins the measured
            // height as a floor the layout cannot claw back.
            className={grows ? undefined : 'flex-1'}
            style={grows ? { ...resolved, flexShrink: 0 } : resolved}
        >
            {children}
        </View>
    )
}
