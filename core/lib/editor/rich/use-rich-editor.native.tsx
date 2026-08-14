import { CoreBridge, TenTapStartKit } from '@10play/tentap-editor'
import { useFileToken } from '@tinycld/core/file-viewer/use-authed-file-url'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { pb } from '../../pocketbase'
import { useThemeColor } from '../../use-app-theme'
import { releaseEditorFocus, setEditorFocused } from '../editor-focus-state'
import { UI_POPOVER_DISMISS_ON_SCROLL } from '../message-bus/popover-protocol'
import { makeMessage } from '../message-bus/types'
import { publishUiMessage, registerEditorOverlay } from '../overlay'
import type { EditorHandle, EditorResult } from '../types'
import { useWebViewEditor } from '../use-webview-editor'
import { AwarenessWebViewHost } from './awareness-webview-host'
import { MarkdownWebViewHost } from './markdown-webview-host'
import type { UseRichEditorOptions } from './options'
import { TriggerItemsWebViewHost } from './trigger-items-webview-host'
import type { SerializableTriggerConfig, TriggerItem } from './triggers'
import { editorHtml } from './webview/build/editorHtml'
import {
    APP_ESCAPE,
    APP_FILE_TOKEN,
    APP_SUBMIT_SHORTCUT,
    type RichEditorInitPayload,
} from './webview/source/protocol'
import { YjsWebViewHost } from './yjs-webview-host'

/**
 * Native build of the shared editor: Tiptap inside a WebView page we own.
 *
 * Markdown is the editor's native format here, exactly as on web. The page is
 * supplied through TenTap's `customSource`, so it runs
 * `buildRichEditorExtensions()` — `@tiptap/markdown` included — and parses and
 * serializes markdown in place.
 *
 * That replaces the previous arrangement, where markdown pivoted through HTML
 * on every read and write because TenTap's own bridge protocol exchanges HTML
 * strings. The conversion module that existed solely to cross that bridge is
 * gone, along with the parsing work it did on the React Native thread.
 *
 * TenTap remains the WebView host: `RichText`, the bridge lifecycle, and
 * `avoidIosKeyboard` — keyboard avoidance and the focus/scroll handling are the
 * genuinely fiddly part and are worth keeping.
 *
 * Collaboration works here too. The caller's Y.Doc — the room's, already
 * connected on the native side — is relayed to the page over the 'yjs'
 * namespace as base64 updates, and the page's edits come back the same way.
 * Collaborator carets ride the 'awareness' namespace alongside it.
 *
 * The WebView never opens a socket of its own. A second connection would ship a
 * credential into the page and give the local user a second awareness identity,
 * so one human would show up as two peers.
 */
/** Distinguishes concurrently-mounted editors. Never reused within a session. */
let editorInstanceCounter = 0

export function useRichEditor(options: UseRichEditorOptions = {}): EditorResult {
    const {
        initialContent,
        contentFormat = 'html',
        placeholder = '',
        autofocus,
        editable = true,
        characterLimit,
        minHeight,
        onSubmitShortcut,
        onEscape,
        onFocus,
        onBlur,
        theme,
        collab,
        triggers,
        overlayKey,
    } = options

    const bgColor = useThemeColor('background')
    const fgColor = useThemeColor('foreground')
    const placeholderColor = useThemeColor('field-placeholder')
    const primaryColor = useThemeColor('primary')

    // The page renders protected images (tokenless stored srcs — see
    // rich/authed-image.ts) but holds no PB client, so the token reaches it
    // from here: seeded in the init payload when it resolved first, and
    // re-posted below on every rotation.
    const { data: fileToken } = useFileToken()

    // Callers pass these inline, so their identity changes every render.
    // Reading them through refs keeps the WebView from remounting.
    const submitRef = useRef(onSubmitShortcut)
    submitRef.current = onSubmitShortcut
    const escapeRef = useRef(onEscape)
    escapeRef.current = onEscape
    const focusRef = useRef(onFocus)
    focusRef.current = onFocus
    const blurRef = useRef(onBlur)
    blurRef.current = onBlur
    // Mirrors the focus flag published to editor-focus-state, so an editor that
    // unmounts while focused can release its claim — no blur is delivered in
    // that case, and the count would stay raised, muting shortcuts app-wide.
    const isFocusedRef = useRef(false)
    useEffect(() => {
        return () => {
            releaseEditorFocus(isFocusedRef.current)
            isFocusedRef.current = false
        }
    }, [])

    // Constructed before the WebView exists, so it holds a poster indirection
    // rather than the poster itself — `postMessage` only becomes available
    // once useWebViewEditor returns.
    const posterRef = useRef<((message: never) => boolean) | null>(null)

    // The relay onto the caller's Y.Doc. Rebuilt only when the doc identity
    // changes (a different card, a reconnected room), because it subscribes to
    // that doc — keeping a stale subscription would relay a dead document's
    // updates into the live editor.
    const collabDoc = collab?.document ?? null
    const yjsHost = useMemo(
        () =>
            collabDoc
                ? new YjsWebViewHost({
                      doc: collabDoc,
                      postMessage: message => posterRef.current?.(message as never) ?? false,
                  })
                : null,
        [collabDoc]
    )
    useEffect(() => () => yjsHost?.destroy(), [yjsHost])

    // The caret relay, alongside the document one. Without it the page's
    // Awareness never leaves the WebView, so the phone sees no remote carets and
    // shows none of its own — the gap that made native co-editing feel dead even
    // though the text was syncing.
    const collabAwareness = collab?.awareness ?? null
    const awarenessHost = useMemo(
        () =>
            collabAwareness
                ? new AwarenessWebViewHost({
                      awareness: collabAwareness,
                      postMessage: message => posterRef.current?.(message as never) ?? false,
                  })
                : null,
        [collabAwareness]
    )
    useEffect(() => () => awarenessHost?.destroy(), [awarenessHost])

    // The init payload is posted once, after the page reports ready. It is
    // deliberately built from primitives so a parent re-render doesn't produce
    // a fresh object and re-trigger the handshake effect — hence the collab
    // identity is spread into its own primitives rather than passed as an
    // object, matching what the web hook does with its six.
    const collabField = collab?.field
    const collabUserId = collab?.user?.id
    const collabUserName = collab?.user?.name
    const collabUserColor = collab?.user?.color
    // Identifies this editor among any others on the same screen, so a host
    // overlay answers only its own popovers. A module counter rather than
    // useId(): the value crosses to the page and back over a JSON pipe, and
    // useId's colons are awkward in that round trip.
    const instanceIdRef = useRef<string>('')
    if (!instanceIdRef.current) instanceIdRef.current = `rich-${++editorInstanceCounter}`
    const editorInstanceId = instanceIdRef.current

    // Only the SHAPE of the triggers belongs in the init payload — the roster
    // is pushed separately, because it changes as members come and go while
    // init is one-shot per mount.
    // Every SerializableTriggerConfig field must be listed here. This mapping is
    // hand-maintained, so a field added to the type but forgotten here reaches
    // web and silently not native — which is how the mention node shipped as a
    // rendered name on web and a raw `[[@id]]` token on the phone.
    const triggerSignature = JSON.stringify(
        (triggers ?? []).map(t => ({
            id: t.id,
            char: t.char,
            limit: t.limit,
            insertTemplate: t.insertTemplate,
            insertsMentionNode: t.insertsMentionNode,
        }))
    )

    const initPayload: RichEditorInitPayload = useMemo(() => {
        const peersAtHandshake = awarenessHost?.encodePeers() ?? null
        return {
            contentFormat,
            initialContent: initialContent ?? '',
            placeholder,
            editable,
            characterLimit,
            autofocus: autofocus ?? false,
            colors: {
                bg: theme?.backgroundColor ?? bgColor,
                fg: fgColor,
                placeholder: placeholderColor,
                primary: primaryColor,
            },
            ...(fileToken ? { fileAuth: { baseURL: pb.baseURL, token: fileToken } } : {}),
            editorInstanceId,
            // Seeded with an EMPTY roster: the real one follows immediately as
            // a push, and baking it in here would rebuild the whole handshake
            // every time someone joined the board.
            ...(triggerSignature === '[]'
                ? {}
                : {
                      triggers: (
                          JSON.parse(triggerSignature) as Omit<
                              SerializableTriggerConfig,
                              'allItems'
                          >[]
                      ).map(t => ({ ...t, allItems: [] })),
                  }),
            // Snapshotted at handshake time. Anything the doc gains between
            // now and the page mounting arrives as a normal relayed update, so
            // a slightly stale seed is not a lost edit.
            ...(yjsHost && collabField
                ? {
                      collab: {
                          field: collabField,
                          clientID: yjsHost.clientID(),
                          initialState: yjsHost.encodeState(),
                          // Peers already in the room. Awareness frames are only
                          // fanned out as they are sent, so without this seed the
                          // page shows no carets until someone happens to move.
                          ...(peersAtHandshake ? { peers: peersAtHandshake } : {}),
                          ...(collabUserId && collabUserName && collabUserColor
                              ? {
                                    user: {
                                        id: collabUserId,
                                        name: collabUserName,
                                        color: collabUserColor,
                                    },
                                }
                              : {}),
                      },
                  }
                : {}),
        }
    }, [
        contentFormat,
        initialContent,
        placeholder,
        editable,
        characterLimit,
        autofocus,
        theme?.backgroundColor,
        bgColor,
        fgColor,
        placeholderColor,
        primaryColor,
        yjsHost,
        awarenessHost,
        collabField,
        collabUserId,
        collabUserName,
        collabUserColor,
        fileToken,
        editorInstanceId,
        triggerSignature,
    ])

    // Token rotations (and a token that resolves after the handshake) reach
    // the page as a message — init is one-shot per mount, so the payload
    // above only covers the token-before-ready ordering.
    useEffect(() => {
        if (!fileToken) return
        posterRef.current?.(
            makeMessage('app', APP_FILE_TOKEN, { baseURL: pb.baseURL, token: fileToken }) as never
        )
    }, [fileToken])

    const markdownHostRef = useRef<MarkdownWebViewHost | null>(null)
    if (markdownHostRef.current === null) {
        markdownHostRef.current = new MarkdownWebViewHost({
            postMessage: message => posterRef.current?.(message as never) ?? false,
        })
        // Seed the fallback so a getMarkdown that times out before the first
        // round-trip returns the document the user opened, not an empty string.
        if (contentFormat === 'markdown' && initialContent) {
            markdownHostRef.current.seed(initialContent)
        }
    }
    const markdownHost = markdownHostRef.current

    useEffect(() => () => markdownHost.destroy(), [markdownHost])

    const triggerItemsHostRef = useRef<TriggerItemsWebViewHost | null>(null)
    if (triggerItemsHostRef.current === null) {
        triggerItemsHostRef.current = new TriggerItemsWebViewHost({
            postMessage: message => posterRef.current?.(message as never) ?? false,
        })
    }
    const triggerItemsHost = triggerItemsHostRef.current
    // Assigned from `result` further down — declared here because the roster
    // effect below is written before `useWebViewEditor` is called, and effects
    // run after the whole body regardless.
    const [isPageReady, setIsPageReady] = useState(false)

    // Push each roster whenever it changes. The effect depends on the
    // SERIALIZED rosters rather than the array: a live query hands back a fresh
    // array on every emission, so an identity dep would post on writes that
    // changed nobody. Parsing the signature back is what keeps the dep honest —
    // the effect reads only what it depends on.
    const rosterSignature = useMemo(
        () => JSON.stringify((triggers ?? []).map(t => [t.id, t.allItems])),
        [triggers]
    )
    // Gated on the page being ready, not just on the roster changing. The
    // editor mounts well before a members query resolves, so the first roster
    // is pushed at a WebView that cannot receive it; and a page reload gives
    // the page a fresh, empty store while the host still remembers sending the
    // roster. Re-running when readiness flips (and clearing the memo first, so
    // an unchanged roster is not skipped as a duplicate) covers both.
    useEffect(() => {
        if (!isPageReady) return
        triggerItemsHost.reset()
        for (const [id, items] of JSON.parse(rosterSignature) as [string, TriggerItem[]][]) {
            triggerItemsHost.push(id, items)
        }
    }, [rosterSignature, triggerItemsHost, isPageReady])

    // TenTap's stock bridges still drive the toolbar commands and the
    // getHTML/getText/setContent/focus surface that mail relies on. Their
    // Tiptap counterparts live in our page, which registers the same schema.
    const bridgeExtensions = useMemo(() => [...TenTapStartKit, CoreBridge.configureCSS('')], [])

    const onDocumentScroll = useCallback(() => {
        publishUiMessage({ namespace: 'ui', type: UI_POPOVER_DISMISS_ON_SCROLL, payload: null })
    }, [])

    const onMessage = useCallback(
        (message: { namespace?: string; type?: string }) => {
            if (awarenessHost?.handleMessage(message as never)) return
            if (yjsHost?.handleMessage(message as never)) return
            if (markdownHost.handleMessage(message as never)) return
            if (message.namespace !== 'app') return
            if (message.type === APP_SUBMIT_SHORTCUT) submitRef.current?.()
            else if (message.type === APP_ESCAPE) escapeRef.current?.()
        },
        [markdownHost, yjsHost, awarenessHost]
    )

    const result = useWebViewEditor({
        editorHtml,
        bridgeExtensions,
        initPayload,
        editable,
        theme: { webview: { backgroundColor: theme?.backgroundColor ?? bgColor } },
        // The floor the WebView is laid out at until the page reports its own
        // height — and the floor it never shrinks below afterwards.
        ...(minHeight === undefined ? {} : { minHeight }),
        avoidIosKeyboard: true,
        // The description editor sits inside the card detail's scroll view;
        // an inner scroll surface would fight it.
        scrollEnabled: false,
        onMessage,
        // Everything on the 'ui' namespace goes to the bus, where a mounted
        // overlay controller for THIS editor picks it up. The hook itself draws
        // nothing — an autocomplete popover is the consumer's UI, not core's.
        onUiMessage: publishUiMessage,
        // An overlay anchored to a range that has scrolled away is worse than
        // no overlay, so a document scroll closes any open popover. Published
        // without an instance id: it is a screen-wide gesture, so every
        // controller should hear it.
        onScroll: onDocumentScroll,
        // Split the host's single edge callback into the same pair of handlers
        // the web variant exposes, so the shared .d.ts contract holds.
        onFocusChange: (isFocused: boolean) => {
            // Tell the shortcut provider a WebView holds the keyboard. It reads
            // TextInput.State, which cannot see into a WebView, so without this
            // every plain letter typed here is offered to the global matcher as
            // a shortcut — see editor-focus-state.ts.
            if (isFocusedRef.current !== isFocused) {
                isFocusedRef.current = isFocused
                setEditorFocused(isFocused)
            }
            if (isFocused) focusRef.current?.()
            else blurRef.current?.()
        },
    })

    posterRef.current = result.postMessage ?? null

    // Mirror the bridge's readiness into state so the roster effect above can
    // depend on it. Compared before setting: this runs on every render, and an
    // unconditional set would loop.
    const isReadyNow = result.isReady === true
    if (isReadyNow !== isPageReady) setIsPageReady(isReadyNow)

    // Publish the handle a host overlay needs to anchor to this editor. The
    // popover is rendered as a sibling (often in another subtree entirely), so
    // context cannot reach it — see editor-overlay-registry.ts.
    // Two refs, two jobs: the popover is MEASURED against the host View
    // (measureRef) because the WebView ref has no measurement methods under
    // Bridgeless, but responses are still POSTED to the WebView ref, which is
    // the only one that can reach the page.
    const webViewRef = result.webViewRef
    const measureRef = result.measureRef
    useEffect(() => {
        if (!overlayKey) return
        return registerEditorOverlay(overlayKey, {
            webViewRef,
            measureRef,
            editorInstanceId,
        })
    }, [overlayKey, webViewRef, measureRef, editorInstanceId])

    // Layer the markdown channel onto the shared handle. Everything else —
    // getHTML, setContent, focus, clear — is TenTap's, unchanged, which is what
    // keeps mail's HTML path working.
    const editor: EditorHandle = useMemo(
        () => ({
            ...result.editor,
            getMarkdown: () => markdownHost.get(),
            setMarkdown: (markdown: string) => markdownHost.set(markdown),
        }),
        [result.editor, markdownHost]
    )

    return { ...result, editor }
}
