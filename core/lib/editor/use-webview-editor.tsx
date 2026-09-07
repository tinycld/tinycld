import {
    destroy,
    EditorWebView,
    postMessage as postToInstance,
    requestFocus,
    subscribe,
} from 'editor-webview'
import type React from 'react'
import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react'
import { View } from 'react-native'
import { captureException } from '../errors'
import { log } from '../logger'
import { deriveToolbarState } from './derive-toolbar-state'
import { createEditorStateStore, type EditorStateStore } from './editor-state-store'
import { createHeightStore, type HeightStore } from './height-store'
import { type EditorMessage, makeMessage } from './message-bus/types'
import { useKeyboardInset } from './native-host/use-keyboard-inset'
import {
    APP_FOCUS,
    APP_INIT,
    EDITOR_READY,
    FORMAT_SET_EDITABLE,
    STATE_UPDATE,
} from './rich/webview/source/protocol'
import type { EditorCommands, EditorHandle, EditorResult } from './types'
import { buildWebViewEditorCommands, type PostEditorMessage } from './webview-editor-commands'

declare const __DEV__: boolean

export interface UseWebViewEditorOptions {
    // The pre-built HTML string that hosts the in-WebView editor.
    // Produced by a package-specific build step calling
    // bundleWebViewEditor. The string contains a TipTap-React instance
    // configured with whatever extensions the package wants.
    editorHtml: string

    // App-specific init payload posted into the WebView once it
    // signals EditorReady. Typed as unknown because each package
    // chooses what to send (auth token, room id, user identity, ...).
    // The in-WebView Editor.tsx parses it via JSON.parse.
    initPayload: unknown

    // Whether the editor accepts user input. Sent to the page as a
    // `format/set-editable` message on change; consumers also use this to
    // disable their toolbar UI.
    editable: boolean

    // Background of the WebView itself, so a page that has not painted yet
    // shows the app's colour rather than white. Optional.
    backgroundColor?: string

    // Pad the document's bottom by the keyboard's height so the caret can
    // scroll clear of it (see native-host/use-keyboard-inset.ts). Defaults to
    // true.
    avoidKeyboard?: boolean

    // Identity of the pooled native WebView this hook owns. Defaults to a
    // fresh id per hook mount; a consumer that already names its editor (the
    // rich editor's editorInstanceId) passes that so logs line up.
    instanceKey?: string

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
    // responsibility to interpret.
    //
    // The callback identity is read through a ref, so the consumer
    // doesn't have to memoize it.
    onUiMessage?: (message: EditorMessage) => void

    // Called when the WebView reports an in-document scroll event from
    // its injected scroll listener. Anchored popovers rendered by the
    // host (slash menu, future image/comment popovers) subscribe to
    // this and dismiss themselves so the overlay doesn't drift away
    // from the anchored element when the user scrolls. A WebView's own
    // scroll events do not fire for in-document scrolling when
    // `scrollEnabled` is false, so the WebView installs a document-level
    // scroll listener that posts a {namespace:'ui', type:'document-scroll'}
    // message; this callback is invoked on every such message.
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
    // channels above, the suggestion bridge posts a flat envelope
    // ({kind: 'suggestion.changed', payload}) so the receiver can route
    // by kind string into the NativeSuggestionBridge's
    // processIncomingMessage(kind, payload) without going through the
    // EditorMessage type. Today the only kind is 'suggestion.changed';
    // additional kinds (e.g. 'suggestion.list-reply') can be added
    // without expanding the EditorMessageNamespace union.
    //
    // The callback identity is read through a ref.
    onSuggestionMessage?: (kind: string, payload: unknown) => void

    // Subscribe to messages the WebView posts on namespaces this hook has no
    // dedicated channel for — today 'markdown', 'html' (the shared rich
    // editor's document channels) and 'app' (submit-shortcut, escape,
    // editor-mounted), plus 'yjs' and 'awareness' for collaboration. A
    // namespace-agnostic fallback rather than another named callback: adding
    // a namespace shouldn't mean adding a prop.
    //
    // Runs after the named channels above, so it never shadows them.
    //
    // The callback identity is read through a ref.
    onMessage?: (message: EditorMessage) => void
}

/** Which init the host last posted, and to which boot of the page. */
export interface PostedInit {
    generation: number
    epoch: number
}

/**
 * Whether the host should post this init payload.
 *
 * Replaces the previous one-shot latch. A warm editor is reconfigured by
 * re-sending init, so the rule is "each generation exactly once, never before
 * the page reports ready" rather than "only ever once".
 *
 * `epoch` counts the page's boots — it starts at 0 (not yet ready) and each
 * `editor-ready` bumps it. A page that boots again has lost everything init
 * built, so the SAME generation must go out again. With the pooled native
 * host a remount of the React component is NOT a boot: the page moves between
 * hosts intact. A second boot means the WebView's content process died and
 * the source was reloaded, or the instance was destroyed and recreated.
 */
export function shouldPostInit(
    lastPosted: PostedInit | null,
    incomingGeneration: number,
    epoch: number
): boolean {
    if (epoch === 0) return false
    if (lastPosted === null) return true
    return incomingGeneration > lastPosted.generation || epoch !== lastPosted.epoch
}

/** Distinguishes concurrently-mounted editors. Never reused within a session. */
let webViewInstanceCounter = 0

// The one place the native WebView is hosted. Encapsulates:
//   - the pooled `editor-webview` host and the instance key that owns it
//   - message routing out of the page (namespaces, state, height, ready)
//   - the EditorReady -> init-payload handshake
//   - the EditorResult contract: handle, commands, toolbar state, poster
//
// Returns the same EditorResult shape consumers expect from any
// useDocumentEditor / useMailEditor variant.
export function useWebViewEditor(options: UseWebViewEditorOptions): EditorResult {
    const {
        editorHtml,
        initPayload,
        editable,
        backgroundColor,
        avoidKeyboard = true,
        instanceKey,
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
    // host. Reading the latest callback off the ref keeps the message
    // handler stable across re-renders.
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
    // shape.
    const onSuggestionMessageRef = useRef(onSuggestionMessage)
    onSuggestionMessageRef.current = onSuggestionMessage

    // Namespace-agnostic fallback for channels without a dedicated ref above.
    // The shared rich editor routes 'markdown', 'html' and 'app' through here.
    const onMessageRef = useRef(onMessage)
    onMessageRef.current = onMessage

    // The pooled WebView this hook owns, for the hook's whole life. The
    // React host component may mount and unmount many times over that life —
    // the warm editor is rendered off-screen while parked and inside whichever
    // surface holds it — and the page survives every one of those, because
    // the native pool only releases it here, when the OWNER goes away.
    const keyRef = useRef('')
    if (!keyRef.current) keyRef.current = instanceKey ?? `webview-${++webViewInstanceCounter}`
    const key = keyRef.current
    useEffect(() => () => destroy(key), [key])

    // Generic message poster. Returns false when no pooled instance exists
    // yet, so callers can choose to swallow, retry, or surface the failure.
    // Stable for the hook's life, which is what lets consumers hold it in
    // memoized bridges without a ref dance.
    const post = useCallback<PostEditorMessage>(
        message => postToInstance(key, JSON.stringify(message)),
        [key]
    )

    // The page's toolbar state, replaced wholesale by each `stateUpdate` it
    // posts. Held in a store rather than React state so a post re-renders
    // only through the subscription below, never the memoized host component.
    const stateStoreRef = useRef<EditorStateStore>(null as unknown as EditorStateStore)
    if (stateStoreRef.current === null) stateStoreRef.current = createEditorStateStore()
    const stateStore = stateStoreRef.current
    const editorState = useSyncExternalStore(stateStore.subscribe, stateStore.get, stateStore.get)

    // Focus edge-detection. The stateUpdate payload carries an `isFocused`
    // flag on every broadcast; consumers care about the transition only.
    const focusChangeRef = useRef(onFocusChange)
    focusChangeRef.current = onFocusChange
    const isWebViewFocused = editorState.isFocused
    const lastFocusRef = useRef<boolean | undefined>(undefined)
    useEffect(() => {
        if (typeof isWebViewFocused !== 'boolean') return
        if (lastFocusRef.current === isWebViewFocused) return
        lastFocusRef.current = isWebViewFocused
        log.debug('core.editor.webview', 'focus', { instanceKey: key, isFocused: isWebViewFocused })
        focusChangeRef.current?.(isWebViewFocused)
    }, [isWebViewFocused])

    // Plumb editable changes through to the page. Before the page has an
    // editor the message is dropped, and the init payload's own `editable`
    // covers the editor that is then built.
    useEffect(() => {
        post(makeMessage('format', FORMAT_SET_EDITABLE, editable))
    }, [post, editable])

    // The WebView's in-page React app posts {type:'editor-ready'} as
    // soon as the top-level <Editor /> mounts — BEFORE it constructs
    // its TipTap instance, because TipTap construction is gated on the
    // init payload from native. So this is the right signal to gate
    // the init post on. Note that we can't use the page's stateUpdate
    // for this: it is only sent after init arrives — chicken-and-egg.
    //
    // A counter rather than a flag, because the page can boot more than once
    // per mount of this hook (see shouldPostInit), and a second boot has to
    // re-trigger the init post.
    const [pageEpoch, setPageEpoch] = useState(0)
    // Mount timestamp for the phase timings below. A WebView editor is an
    // expensive thing to create — a browser cold start plus a bundle — and the
    // three marks (page-ready, init-sent, first-height) are what tell you WHICH
    // of those phases a slow open actually spent its time in.
    const mountAtRef = useRef(Date.now())
    // t0. Logged once per editor so the marks below have a visible origin —
    // without it a slow open is indistinguishable from an editor that mounted
    // late for reasons upstream of this hook.
    const loggedMountRef = useRef(false)
    if (!loggedMountRef.current) {
        loggedMountRef.current = true
        log.debug('core.editor.webview', 'mounted (t0)', { instanceKey: key })
    }

    // Height the page reported for its own content, held in a tiny store
    // rather than state so that a new measurement re-renders ONLY the box
    // wrapping the WebView. Putting it in state here would change
    // EditorComponent's identity and remount the host on every
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

    // Which generation has been posted, and to which page boot, rather than
    // whether ANY init was — the warm editor reconfigures itself by posting a
    // new one, and a rebooted page needs the current one again.
    const lastInitRef = useRef<PostedInit | null>(null)
    useEffect(() => {
        const generation = (initPayload as { generation?: unknown })?.generation
        // Consumers that predate the warm path (mail, text) send no generation;
        // treat those as a single one-shot init, exactly as before.
        const incoming = typeof generation === 'number' ? generation : 0
        if (!shouldPostInit(lastInitRef.current, incoming, pageEpoch)) return
        const message = makeMessage('app', APP_INIT, initPayload)
        try {
            // No instance yet: nothing to post to. The generation is
            // deliberately NOT recorded, so the next epoch or render retries.
            if (!post(message)) return
            log.debug('core.editor.webview', 'init-sent', {
                sinceMountMs: Date.now() - mountAtRef.current,
                generation: incoming,
                epoch: pageEpoch,
            })
            lastInitRef.current = { generation: incoming, epoch: pageEpoch }
        } catch (err) {
            captureException('editor.postInit', err, { generation: incoming })
        }
    }, [post, pageEpoch, initPayload])

    const editor: EditorHandle = useMemo(
        () => ({
            // The document channels are the rich editor's (markdown / html
            // hosts layered on in use-rich-editor.native.tsx); text's page
            // owns its document through Yjs and never reads it this way. What
            // is left here is deliberately inert rather than a request nobody
            // answers.
            getHTML: () => Promise.resolve(''),
            getText: () => Promise.resolve(''),
            setContent: () => {},
            clear: () => {},
            // Two halves: the native view becomes first responder (which is what
            // brings the keyboard up), and the page moves its caret.
            focus: position => {
                requestFocus(key)
                post(makeMessage('app', APP_FOCUS, position ?? 'end'))
            },
            // Native selection query is a request/response round-trip the
            // pages don't answer yet; null is the documented "not ready".
            getSelection: () => Promise.resolve(null),
        }),
        [key, post]
    )

    const commands: EditorCommands = useMemo(() => buildWebViewEditorCommands(post), [post])

    useKeyboardInset(post, avoidKeyboard)

    // The page posts every field of its toolbar state in one object; our
    // pages add fields (isInTable, wordCount, …) beyond the basic marks.
    // deriveToolbarState narrows them all through a loose record view. The
    // helper itself is pure so a unit test can drive it against a synthetic
    // payload.
    const toolbarState = deriveToolbarState(editorState)

    // The page measured itself. A WebView has no intrinsic height, so this is
    // the only way the container can track its content — without it the editor
    // is clipped to a guess (or, inside a ScrollView where flex resolves to
    // zero, invisible).
    const applyContentHeight = useCallback(
        (payload: unknown) => {
            const height = (payload as { height?: unknown } | undefined)?.height
            if (typeof height !== 'number' || height <= 0) return
            log.debug('core.editor.webview', 'first-height', {
                sinceMountMs: Date.now() - mountAtRef.current,
            })
            setContentHeight(height)
        },
        [setContentHeight]
    )

    // Everything the page posts arrives here. Bare messages (`editor-ready`,
    // `stateUpdate`) are the page's lifecycle; the rest carry our
    // {namespace} envelope and fan out to the callbacks above.
    //
    // 'document-scroll' is a special-case 'ui' message that the WebView
    // posts from a window-level scroll listener; we fan it out to
    // onScroll(...) instead of forwarding to onUiMessage so consumers
    // can take it without writing a switch over message.type.
    const onWebViewMessage = useMemo(
        () => (data: string) => {
            let parsed: EditorMessage
            try {
                parsed = JSON.parse(data) as EditorMessage
            } catch {
                return
            }
            // The WebView's bootstrap posts {type:'editor-ready'} as
            // its first message, before TipTap mounts. That's the
            // signal we use to gate the init post — see pageEpoch above.
            if (parsed.type === EDITOR_READY && parsed.namespace === undefined) {
                setPageEpoch(epoch => {
                    log.debug('core.editor.webview', 'page-ready', {
                        sinceMountMs: Date.now() - mountAtRef.current,
                        epoch: epoch + 1,
                    })
                    return epoch + 1
                })
                return
            }
            if (parsed.type === STATE_UPDATE && parsed.namespace === undefined) {
                stateStore.set(parsed.payload)
                return
            }
            if (parsed.namespace === 'ui') {
                if (parsed.type === 'document-scroll') {
                    onScrollRef.current?.()
                    return
                }
                if (parsed.type === 'content-height') {
                    applyContentHeight(parsed.payload)
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
            // subscriber — the shared rich editor's document and 'app'
            // channels, plus 'yjs' and 'awareness' for collaboration.
            if (typeof parsed.namespace === 'string') {
                onMessageRef.current?.(parsed)
            }
        },
        // Everything else is read through a ref; applyContentHeight and the
        // store are created once per mount, so this list never changes in
        // practice.
        [applyContentHeight, stateStore]
    )

    // The page's messages come as module events keyed by instance, for the
    // hook's whole life — not through the host view, which is unmounted and
    // remounted on every hand-off and would lose whatever the page posted in
    // that window.
    useEffect(() => subscribe(key, { onMessage: onWebViewMessage }), [key, onWebViewMessage])

    // The anchor host overlays measure against: the plain host View wrapping
    // the WebView, which is the same box and an ordinary measurable view.
    const measureRef = useRef<View | null>(null)

    const EditorComponent = useMemo(
        () =>
            function WebViewEditorContent() {
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
                        <EditorWebView
                            instanceKey={key}
                            source={editorHtml}
                            scrollEnabled={scrollEnabled}
                            webBackgroundColor={backgroundColor}
                            inspectable={__DEV__}
                            style={{ flex: 1 }}
                        />
                    </EditorHeightBox>
                )
            },
        // contentHeight is deliberately ABSENT: this memo produces a component
        // IDENTITY, and consumers render it as <EditorComponent />. A new
        // identity remounts the host — no longer a page reload, but still a
        // detach and re-attach of the native view — so feeding the measured
        // height in here would churn the host on every measurement. The
        // height is subscribed to inside EditorHeightBox instead.
        // measureRef is deliberately absent: it is a useRef object, stable for
        // the life of the mount.
        [key, editorHtml, scrollEnabled, minHeight, heightStore, backgroundColor]
    )

    // What host overlays POST through. Not a view ref any more — a poster shim
    // keyed on the instance, in a ref-shaped object so the overlay
    // controllers' duck-typing (`ref.current.postMessage(string)`) is
    // untouched. Measuring is `measureRef`'s job.
    const webViewRef = useMemo<React.RefObject<unknown>>(
        () => ({
            current: {
                postMessage: (data: string) => {
                    postToInstance(key, data)
                },
            },
        }),
        [key]
    )

    return {
        editor,
        EditorComponent,
        commands,
        toolbarState,
        webViewRef,
        measureRef,
        postMessage: post,
        isReady: editorState.isReady === true,
        pageEpoch,
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
