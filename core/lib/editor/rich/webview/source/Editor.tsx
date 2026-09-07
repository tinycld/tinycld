import type { Editor as TiptapEditor } from '@tiptap/core'
import {
    EditorContent,
    NodeViewWrapper,
    type ReactNodeViewProps,
    ReactNodeViewRenderer,
    useEditor,
} from '@tiptap/react'
import { exitSuggestion } from '@tiptap/suggestion'
import {
    Component,
    type ReactNode,
    useEffect,
    useMemo,
    useState,
    useSyncExternalStore,
} from 'react'
import { Awareness, applyAwarenessUpdate, removeAwarenessStates } from 'y-protocols/awareness'
import * as Y from 'yjs'
import type { EditorMessage } from '../../../message-bus/types'
import { makeMessage } from '../../../message-bus/types'
import { resolveProtectedFileSrc } from '../../authed-image'
import { buildRichEditorExtensions } from '../../extensions'
import { repairMarkdown } from '../../markdown-repair'
import { getFileAuth, setFileAuth, subscribeFileAuth } from './file-auth-store'
import {
    applyKeyboardInset,
    focusEditor,
    readKeyboardInset,
    reapplyKeyboardInset,
} from './host-commands'
import {
    APP_EDITOR_MOUNTED,
    APP_ESCAPE,
    APP_FILE_TOKEN,
    APP_FOCUS,
    APP_KEYBOARD_INSET,
    APP_PAGE_ERROR,
    APP_PARK,
    APP_SUBMIT_SHORTCUT,
    APP_TRIGGER_ITEMS,
    AWARENESS_CURSOR,
    AWARENESS_LEAVE,
    AWARENESS_PEERS,
    type AwarenessLeavePayload,
    type AwarenessPeersPayload,
    decodeUpdate,
    EDITOR_READY,
    encodeUpdate,
    FORMAT_SET_EDITABLE,
    HTML_GET,
    HTML_RESULT,
    HTML_SET,
    type HtmlSetPayload,
    MARKDOWN_GET,
    MARKDOWN_RESULT,
    MARKDOWN_SET,
    type MarkdownSetPayload,
    type RichEditorFileAuth,
    type RichEditorInitCollab,
    type RichEditorInitPayload,
    type TriggerItemsPayload,
    UI_CONTENT_HEIGHT,
    YJS_UPDATE,
    type YjsUpdatePayload,
} from './protocol'
import { deriveWebViewState } from './state'
import { buildEditorCSS } from './styles'
import { getTriggerItems, setTriggerItems } from './trigger-items-store'
import {
    createTriggerBridgeRender,
    defaultNewRequestId,
    defaultPostToHost,
} from './trigger-render-bridge'

declare global {
    interface Window {
        ReactNativeWebView?: { postMessage: (s: string) => void }
        __RICH_EDITOR_INITIALIZED__?: boolean
    }
}

function postToNative(message: unknown): void {
    window.ReactNativeWebView?.postMessage(JSON.stringify(message))
}

/**
 * Report a failure inside the page to the host. The page runs at an opaque
 * origin, so `window.onerror` only ever sees "Script error." — the real
 * message has to be posted from where it is caught.
 */
function reportPageError(where: string, error: unknown): void {
    const message = error instanceof Error ? error.message : String(error)
    const stack = error instanceof Error ? (error.stack ?? '').slice(0, 800) : ''
    postToNative(makeMessage('app', APP_PAGE_ERROR, { message: `${where}: ${message}`, stack }))
}

/** A render or effect error in the editor would otherwise unmount it silently. */
class EditorErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
    state = { failed: false }

    static getDerivedStateFromError() {
        return { failed: true }
    }

    componentDidCatch(error: unknown) {
        reportPageError('render', error)
    }

    render() {
        return this.state.failed ? null : this.props.children
    }
}

/**
 * The page's counterpart of core's AuthedImageView.web: renders an image whose
 * stored src is a tokenless protected-file path, resolved against the
 * credentials the host relays (see APP_FILE_TOKEN). Everything else — data:
 * URIs, external URLs — passes through untouched.
 */
function AuthedImagePageView({ node }: ReactNodeViewProps<HTMLSpanElement>) {
    const auth = useSyncExternalStore(subscribeFileAuth, getFileAuth)
    const src = (node.attrs.src as string | null) ?? ''
    const alt = (node.attrs.alt as string | null) ?? ''
    const title = (node.attrs.title as string | null) ?? ''
    const displaySrc = auth ? resolveProtectedFileSrc(src, auth.baseURL, auth.token) : src

    return (
        <NodeViewWrapper as="span" style={{ display: 'inline-block', lineHeight: 0 }}>
            <img
                src={displaySrc}
                alt={alt || undefined}
                title={title || undefined}
                draggable={false}
                style={{ maxWidth: '100%', height: 'auto' }}
            />
        </NodeViewWrapper>
    )
}

/** Module-level so the extension list keeps a stable identity. */
const AUTHED_IMAGE_NODE_VIEW = ReactNodeViewRenderer(AuthedImagePageView)

/**
 * Decide what an incoming init means for the page's current configuration.
 *
 * Pure so it can be tested without a DOM — the message plumbing around it needs
 * a WebView, but the interesting decisions are here.
 *
 * `null` incoming is a park request: drop to stage one, keeping the booted page.
 */
export function reduceInit(
    current: RichEditorInitPayload | null,
    incoming: RichEditorInitPayload | null
): RichEditorInitPayload | null {
    if (incoming === null) return null
    // A repeat or a late-arriving older payload must not rebuild the editor —
    // that discards whatever has been typed since the current one was applied.
    if (current !== null && incoming.generation <= current.generation) return current
    return incoming
}

/**
 * The rich editor's in-WebView page.
 *
 * This is what makes markdown the editor's native format on mobile. The editor
 * here is a real Tiptap instance built from `buildRichEditorExtensions()` — the
 * same schema the web hook uses, `@tiptap/markdown` included — so markdown is
 * parsed and serialized in place. Nothing pivots through HTML.
 *
 * The host is core's own `editor-webview` native view (see
 * lib/editor/use-webview-editor.tsx). Every instruction it sends arrives as a
 * namespaced message on this page; nothing pivots through a third-party bridge.
 *
 * Mounts in two stages, because the extension set depends on the init payload
 * (placeholder, character limit, and later the collaboration binding): report
 * ready, wait for init, then construct.
 */
export function Editor() {
    const [init, setInit] = useState<RichEditorInitPayload | null>(null)

    useEffect(() => {
        function onMessage(evt: MessageEvent | Event) {
            const data = (evt as MessageEvent).data
            if (typeof data !== 'string') return
            let parsed: EditorMessage
            try {
                parsed = JSON.parse(data) as EditorMessage
            } catch {
                return
            }
            if (parsed.namespace === 'app' && parsed.type === 'init') {
                const incoming = parsed.payload as RichEditorInitPayload
                console.log(`TRACE init received gen ${incoming.generation}`)
                setInit(current => reduceInit(current, incoming))
                return
            }
            if (parsed.namespace === 'app' && parsed.type === APP_PARK) {
                setInit(current => reduceInit(current, null))
            }
        }
        // Some platforms deliver WebView messages on window, others on
        // document; listen to both.
        window.addEventListener('message', onMessage)
        document.addEventListener('message', onMessage)
        // Nothing inside a WebView is visible from the host. A page that
        // throws during a hand-off would otherwise be an empty box.
        function onError(event: ErrorEvent) {
            postToNative(makeMessage('app', APP_PAGE_ERROR, { message: event.message }))
        }
        function onRejection(event: PromiseRejectionEvent) {
            postToNative(makeMessage('app', APP_PAGE_ERROR, { message: String(event.reason) }))
        }
        window.addEventListener('error', onError)
        window.addEventListener('unhandledrejection', onRejection)
        // Posted before Tiptap exists — this is the gate the host waits on
        // before sending init, so it must not depend on the editor.
        postToNative({ type: EDITOR_READY, payload: undefined })
        return () => {
            window.removeEventListener('message', onMessage)
            document.removeEventListener('message', onMessage)
            window.removeEventListener('error', onError)
            window.removeEventListener('unhandledrejection', onRejection)
        }
    }, [])

    if (init == null) return null

    // Keyed on the generation so a handover is a full reconstruction: new
    // Tiptap, new Y.Doc (useCollabDoc's useState initializer reruns), new undo
    // stack, new extension set. Nothing survives the swap, which is what makes
    // it safe to reuse one page across surfaces.
    return (
        <EditorErrorBoundary key={init.generation}>
            <EditorMounted init={init} />
        </EditorErrorBoundary>
    )
}

function EditorMounted({ init }: { init: RichEditorInitPayload }) {
    console.log(`TRACE render gen ${init.generation}`)
    useEffect(() => {
        const style = document.createElement('style')
        style.id = 'tinycld-rich-editor-styles'
        style.textContent = buildEditorCSS(init.colors, init.scale)
        document.head.appendChild(style)
        return () => style.remove()
    }, [init.colors, init.scale])

    // Seed the credential store before the editor's first paint, so images
    // present in the initial document don't flash a broken frame while the
    // relayed APP_FILE_TOKEN is still in flight.
    useEffect(() => {
        if (init.fileAuth) setFileAuth(init.fileAuth)
    }, [init.fileAuth])

    // Same reasoning for the trigger rosters, but seeded during render rather
    // than in an effect: the suggestion plugin reads the store synchronously
    // from a transaction, and an effect would leave an `@` typed before the
    // first paint looking at an empty roster. Idempotent, so a re-render costs
    // a Map write; a later APP_TRIGGER_ITEMS overwrites it.
    useMemo(() => {
        for (const trigger of init.triggers ?? []) setTriggerItems(trigger.id, trigger.allItems)
    }, [init.triggers])

    const collab = useCollabDoc(init.collab)

    const editor = useEditor({
        editable: init.editable,
        autofocus: init.autofocus ? 'end' : false,
        extensions: buildRichEditorExtensions({
            placeholder: init.placeholder,
            characterLimit: init.characterLimit,
            imageNodeView: AUTHED_IMAGE_NODE_VIEW,
            onSubmitShortcut: () => postToNative(makeMessage('app', APP_SUBMIT_SHORTCUT, null)),
            // Candidates come from the store rather than the init payload's
            // frozen copy, so a roster pushed later is picked up without
            // rebuilding the editor. `onStateChange` is the web callback
            // channel and has nobody to talk to here — the bridge posts to the
            // host instead.
            triggers: (init.triggers ?? []).map(trigger => ({
                ...trigger,
                get allItems() {
                    return getTriggerItems(trigger.id)
                },
                onStateChange: () => {},
                render: createTriggerBridgeRender(trigger, {
                    postToHost: defaultPostToHost,
                    newRequestId: defaultNewRequestId,
                    exitSuggestion,
                    editorInstanceId: init.editorInstanceId,
                }),
            })),
            collab: collab
                ? {
                      document: collab.doc,
                      field: collab.field,
                      awareness: collab.awareness,
                      user: collab.user,
                  }
                : undefined,
        }),
        // Tiptap 3 defaults this false; the toolbar reads active marks off
        // every transaction, so it has to re-render on them.
        shouldRerenderOnTransaction: true,
    })

    useYjsRelay(collab)
    useAwarenessRelay(collab)
    useInitialContent(editor, init, collab != null)
    useStateBroadcast(editor)
    useHostMessages(editor, collab != null)
    useEscapeKey()
    useContentHeight(editor)

    return <EditorContent editor={editor} />
}

/**
 * Apply the starting document once the editor exists.
 *
 * Markdown goes in as markdown — `setContent` routes through the markdown
 * extension's parser. Mail passes 'html' and takes the same path Tiptap would
 * take for an HTML string.
 *
 * Skipped entirely under collaboration: there the document arrives as Yjs
 * state, already applied to the doc before this editor was built. Setting
 * content on top of it appends a second copy of the text — and does so on
 * every client that joins. The web hook skips it for the same reason.
 */
function useInitialContent(
    editor: TiptapEditor | null,
    init: RichEditorInitPayload,
    isCollab: boolean
) {
    useEffect(() => {
        if (!editor) return
        if (isCollab) return
        if (!init.initialContent) return
        editor.commands.setContent(init.initialContent, {
            emitUpdate: false,
            // The markdown extension keys off this to pick its parser; without
            // it a markdown string is treated as HTML and arrives as literal
            // syntax.
            ...(init.contentFormat === 'markdown' ? { contentType: 'markdown' as const } : {}),
        })
    }, [editor, init.initialContent, init.contentFormat, isCollab])
}

interface CollabBinding {
    doc: Y.Doc
    awareness: Awareness | undefined
    field: string
    user: { id: string; name: string; color: string } | undefined
}

/** Tags doc transactions whose updates came from the host, so the relay below
 *  doesn't post them straight back and loop. */
const FROM_HOST: unique symbol = Symbol('yjs:from-host')

/** The same guard for awareness: peers' carets applied from the host must not
 *  be posted back as if this page's cursor had moved. */
const FROM_HOST_AWARENESS: unique symbol = Symbol('awareness:from-host')

/**
 * Slack left below the last block when reporting the document's height.
 *
 * Covers the sub-pixel difference between what the page measures and what the
 * native layout rounds to, which is enough on its own to shave the descender
 * off a final line. It also gives the reader somewhere to tap to place the
 * caret at the end.
 */
const TRAILING_SPACE_PX = 24

/**
 * Build the page's Y.Doc once, seeded from the host's state.
 *
 * Constructed lazily in `useState` rather than an effect because Tiptap's
 * Collaboration extension binds to the doc when the editor is CREATED — a doc
 * that appears one render later would leave the editor bound to nothing.
 *
 * This doc keeps its OWN clientID. Adopting the host's was the original plan,
 * but Yjs refuses it — see `RichEditorInitCollab.clientID`. The local user
 * still appears once regardless, because this page's clientID never reaches the
 * wire: the host merges the cursor below into its own awareness slot, and board
 * presence rides the host's socket.
 */
function useCollabDoc(init: RichEditorInitCollab | undefined): CollabBinding | null {
    const [binding] = useState<CollabBinding | null>(() => {
        if (!init) return null
        const doc = new Y.Doc()
        if (init.initialState) {
            // FROM_HOST so seeding doesn't immediately echo the entire
            // document back to the host as if the user had typed it.
            Y.applyUpdate(doc, decodeUpdate(init.initialState), FROM_HOST)
        }
        // This Awareness drives the carets drawn in this page. Only its local
        // slot ever leaves — as a bare cursor position, which the host merges
        // into its own slot — so peers still see one avatar for this human.
        const awareness = init.user ? new Awareness(doc) : undefined
        if (awareness && init.user) {
            awareness.setLocalStateField('user', init.user)
            if (init.peers) {
                // Whoever was already in the room. Seeded FROM_HOST_AWARENESS so
                // it is not mistaken for this page's own cursor moving.
                try {
                    applyAwarenessUpdate(awareness, decodeUpdate(init.peers), FROM_HOST_AWARENESS)
                } catch {
                    // A bad seed costs the initial carets, not the editor; the
                    // next awareness frame from each peer restores them.
                }
            }
        }
        return { doc, awareness, field: init.field, user: init.user }
    })
    return binding
}

/**
 * Pump updates between the page's doc and the host.
 *
 * Outbound is guarded on FROM_HOST so an update we just applied is not posted
 * straight back; the host guards the mirror-image case with its own
 * RELAY_ORIGIN. Without both halves the two docs bounce a single keystroke
 * between them forever.
 */
function useYjsRelay(collab: CollabBinding | null) {
    useEffect(() => {
        if (!collab) return
        const { doc } = collab

        function onLocalUpdate(update: Uint8Array, origin: unknown) {
            if (origin === FROM_HOST) return
            postToNative(makeMessage('yjs', YJS_UPDATE, { update: encodeUpdate(update) }))
        }
        doc.on('update', onLocalUpdate)

        function onMessage(evt: MessageEvent | Event) {
            const data = (evt as MessageEvent).data
            if (typeof data !== 'string') return
            let parsed: EditorMessage
            try {
                parsed = JSON.parse(data) as EditorMessage
            } catch {
                return
            }
            if (parsed.namespace !== 'yjs' || parsed.type !== YJS_UPDATE) return
            const encoded = (parsed.payload as YjsUpdatePayload | undefined)?.update
            if (typeof encoded !== 'string' || encoded.length === 0) return
            try {
                Y.applyUpdate(doc, decodeUpdate(encoded), FROM_HOST)
            } catch {
                // Convergent by construction: a malformed update is dropped
                // and the next one from that peer carries the same state.
            }
        }
        window.addEventListener('message', onMessage)
        document.addEventListener('message', onMessage)

        return () => {
            doc.off('update', onLocalUpdate)
            window.removeEventListener('message', onMessage)
            document.removeEventListener('message', onMessage)
        }
    }, [collab])
}

/**
 * Pump collaborator carets between this page's Awareness and the host.
 *
 * Asymmetric, unlike the document relay, and deliberately so:
 *
 *  - OUTBOUND we send only this page's own CURSOR POSITION, never an encoded
 *    awareness state. The host merges it into its own slot, so the phone stays
 *    one peer with one avatar. Sending a state would put this page's clientID on
 *    the wire and make one human look like two.
 *  - INBOUND we apply whatever the host relays, which it has already filtered
 *    down to remote peers.
 *
 * The identity skip matters: y-tiptap rewrites the cursor field on every
 * transaction, so without it a burst of typing posts an identical cursor dozens
 * of times.
 */
function useAwarenessRelay(collab: CollabBinding | null) {
    useEffect(() => {
        const awareness = collab?.awareness
        if (!awareness) return

        let lastSent: string | null = null
        function onLocalAwareness(
            { added, updated }: { added: number[]; updated: number[] },
            origin: unknown
        ) {
            if (origin === FROM_HOST_AWARENESS) return
            const local = awareness as Awareness
            if (![...added, ...updated].includes(local.clientID)) return
            const cursor = (local.getLocalState() as { cursor?: unknown } | null)?.cursor ?? null
            const serialized = JSON.stringify(cursor ?? null)
            if (serialized === lastSent) return
            lastSent = serialized
            postToNative(makeMessage('awareness', AWARENESS_CURSOR, { cursor }))
        }
        awareness.on('update', onLocalAwareness)

        function onMessage(evt: MessageEvent | Event) {
            const data = (evt as MessageEvent).data
            if (typeof data !== 'string') return
            let parsed: EditorMessage
            try {
                parsed = JSON.parse(data) as EditorMessage
            } catch {
                return
            }
            if (parsed.namespace !== 'awareness') return
            const local = awareness as Awareness
            try {
                if (parsed.type === AWARENESS_PEERS) {
                    const encoded = (parsed.payload as AwarenessPeersPayload | undefined)?.update
                    if (typeof encoded !== 'string' || encoded.length === 0) return
                    applyAwarenessUpdate(local, decodeUpdate(encoded), FROM_HOST_AWARENESS)
                } else if (parsed.type === AWARENESS_LEAVE) {
                    const ids = (parsed.payload as AwarenessLeavePayload | undefined)?.clientIDs
                    if (!Array.isArray(ids) || ids.length === 0) return
                    removeAwarenessStates(local, ids, FROM_HOST_AWARENESS)
                }
            } catch {
                // A malformed frame costs one repaint of the carets, never the
                // editor; the next frame from that peer restores them.
            }
        }
        window.addEventListener('message', onMessage)
        document.addEventListener('message', onMessage)

        return () => {
            awareness.off('update', onLocalAwareness)
            window.removeEventListener('message', onMessage)
            document.removeEventListener('message', onMessage)
        }
    }, [collab])
}

/**
 * Stream toolbar state to the host on every meaningful transaction.
 *
 * Posted as a bare `stateUpdate` (no namespace), the shape the host's state
 * store reads into deriveToolbarState. Coalesced per frame, with an
 * identity skip so a burst of transactions that doesn't change the toolbar
 * (bulk paste, remote edits) doesn't spam the bridge.
 */
function useStateBroadcast(editor: TiptapEditor | null) {
    useEffect(() => {
        if (!editor) return
        let scheduled = false
        let lastSerialized = ''

        let frame: number | null = null
        function send() {
            frame = null
            // The frame can outlive the effect: a hand-off destroys this editor
            // in the same commit that mounts the next one.
            if (!editor || editor.isDestroyed) return
            try {
                sendUnguarded()
            } catch (error) {
                reportPageError('state-update', error)
            }
        }
        function sendUnguarded() {
            scheduled = false
            if (!editor) return
            const payload = deriveWebViewState(editor)
            const serialized = JSON.stringify(payload)
            if (serialized === lastSerialized) return
            lastSerialized = serialized
            postToNative({ type: 'stateUpdate', payload })
        }
        function schedule() {
            if (scheduled) return
            scheduled = true
            frame = requestAnimationFrame(send)
        }

        editor.on('transaction', schedule)
        editor.on('update', schedule)
        // Focus and blur do not reliably produce a transaction, so without
        // these two the payload's isFocused would only reach the host on the
        // NEXT keystroke — focus-gated chrome would appear a beat late and
        // never hide on blur at all.
        editor.on('focus', schedule)
        editor.on('blur', schedule)
        // Prime the host with a first snapshot so isReady flips without
        // requiring the user to touch the editor.
        schedule()
        return () => {
            editor.off('transaction', schedule)
            editor.off('update', schedule)
            editor.off('focus', schedule)
            editor.off('blur', schedule)
            if (frame !== null) cancelAnimationFrame(frame)
        }
    }, [editor])
}

/**
 * Dispatch host → WebView messages.
 *
 * Every message is our own `{namespace, type, payload}` envelope: document
 * channels, file auth, the trigger roster, caret and keyboard instructions,
 * and the toolbar's format commands.
 */
function useHostMessages(editor: TiptapEditor | null, isCollab: boolean) {
    useEffect(() => {
        if (!editor) return
        // A hand-off rebuilds the editor; if the keyboard is still up the new
        // one needs the same room at the bottom.
        try {
            reapplyKeyboardInset(editor)
        } catch (error) {
            reportPageError('keyboard-inset', error)
        }

        function onMessage(evt: MessageEvent | Event) {
            const data = (evt as MessageEvent).data
            if (typeof data !== 'string' || !editor) return
            let parsed: EditorMessage
            try {
                parsed = JSON.parse(data) as EditorMessage
            } catch {
                return
            }
            try {
                dispatchHostMessage(editor, parsed, isCollab)
            } catch (error) {
                reportPageError(`${parsed.namespace}/${parsed.type}`, error)
            }
        }

        window.addEventListener('message', onMessage)
        document.addEventListener('message', onMessage)
        // After subscribing, never before: the host holds document pushes
        // until it hears this, so the ordering is what makes them land.
        postToNative(makeMessage('app', APP_EDITOR_MOUNTED, null))
        return () => {
            window.removeEventListener('message', onMessage)
            document.removeEventListener('message', onMessage)
        }
    }, [editor, isCollab])
}

function dispatchHostMessage(editor: TiptapEditor, parsed: EditorMessage, isCollab: boolean): void {
    // Handled by useYjsRelay; not a format action.
    if (parsed.namespace === 'yjs') return
    // Likewise useAwarenessRelay. Both bails matter: dispatchFormatAction
    // no-ops on an unknown action today, so without them a relay message
    // would fall through and become a latent bug the next time its
    // default branch does something.
    if (parsed.namespace === 'awareness') return
    if (parsed.namespace === 'markdown') {
        handleMarkdownMessage(editor, parsed, isCollab)
        return
    }
    if (parsed.namespace === 'html') {
        handleHtmlMessage(editor, parsed, isCollab)
        return
    }
    if (parsed.namespace === 'app') {
        handleAppMessage(editor, parsed)
        return
    }
    if (parsed.namespace === 'format') dispatchFormatAction(editor, parsed)
}

/**
 * The 'app' namespace once an editor exists: credentials for protected images,
 * the mention roster, and the caret / keyboard instructions. (Init and park
 * are the stage-one Editor's, above.)
 */
function handleAppMessage(editor: TiptapEditor, message: EditorMessage): void {
    switch (message.type) {
        case APP_FILE_TOKEN: {
            const auth = message.payload as RichEditorFileAuth | undefined
            if (auth && typeof auth.token === 'string' && typeof auth.baseURL === 'string') {
                setFileAuth(auth)
            }
            break
        }
        case APP_TRIGGER_ITEMS: {
            const roster = message.payload as TriggerItemsPayload | undefined
            if (roster && typeof roster.triggerId === 'string' && Array.isArray(roster.items)) {
                setTriggerItems(roster.triggerId, roster.items)
            }
            break
        }
        case APP_FOCUS:
            focusEditor(editor, message.payload)
            break
        case APP_KEYBOARD_INSET: {
            const bottom = readKeyboardInset(message.payload)
            if (bottom !== null) applyKeyboardInset(editor, bottom)
            break
        }
        default:
            break
    }
}

function handleMarkdownMessage(
    editor: TiptapEditor,
    message: EditorMessage,
    isCollab: boolean
): void {
    if (message.type === MARKDOWN_SET) {
        // A no-op under collaboration, by design: the shared doc is the source
        // of truth, and replacing the whole document would delete every peer's
        // concurrent text and re-insert this copy as new content.
        if (isCollab) return
        const { markdown } = message.payload as MarkdownSetPayload
        editor.commands.setContent(markdown, { emitUpdate: false, contentType: 'markdown' })
        return
    }
    if (message.type === MARKDOWN_GET) {
        // Repair before it leaves the WebView. Raw @tiptap/markdown output
        // corrupts code spans containing a backtick and drops table-cell pipe
        // escapes, and this value is what gets persisted.
        const markdown = repairMarkdown(editor.getMarkdown())
        postToNative(makeMessage('markdown', MARKDOWN_RESULT, { markdown }, message.requestId))
    }
}

/**
 * The html channel: the same two operations for the editor's other wire
 * format. Exported so the round-trip can be tested against a real Tiptap
 * instance without a WebView.
 */
export function handleHtmlMessage(
    editor: TiptapEditor,
    message: EditorMessage,
    isCollab: boolean
): void {
    if (message.type === HTML_SET) {
        // Same no-op as the markdown set: under collaboration the shared doc is
        // the source of truth.
        if (isCollab) return
        const { html } = message.payload as HtmlSetPayload
        editor.commands.setContent(html, { emitUpdate: false })
        return
    }
    if (message.type === HTML_GET) {
        postToNative(
            makeMessage(
                'html',
                HTML_RESULT,
                { html: editor.getHTML(), text: editor.getText() },
                message.requestId
            )
        )
    }
}

interface FormatAction {
    type?: string
    payload?: unknown
}

/**
 * One toolbar command, as `buildWebViewEditorCommands` sends it: a flat
 * `{namespace:'format', type, payload}`. Exported so the round-trip can be
 * tested against a real Tiptap instance without a WebView.
 */
export function dispatchFormatAction(editor: TiptapEditor, action: FormatAction): void {
    const chain = () => editor.chain().focus()
    switch (action.type) {
        case 'toggle-bold':
            chain().toggleBold().run()
            break
        case 'toggle-italic':
            chain().toggleItalic().run()
            break
        case 'toggle-underline':
            chain().toggleUnderline().run()
            break
        case 'toggle-strike':
            chain().toggleStrike().run()
            break
        case 'toggle-code':
            chain().toggleCode().run()
            break
        case 'toggle-code-block':
            chain().toggleCodeBlock().run()
            break
        // The list types are camelCase on the wire (a spelling inherited from
        // the bridge library this page once ran under, and kept: both pages
        // and the command builder agree on it).
        case 'toggle-bulletList':
            chain().toggleBulletList().run()
            break
        case 'toggle-orderedList':
            chain().toggleOrderedList().run()
            break
        case 'toggle-taskList':
            chain().toggleList('taskList', 'taskItem').run()
            break
        case 'toggle-blockquote':
            chain().toggleBlockquote().run()
            break
        case 'toggle-heading': {
            const level = readHeadingLevel(action.payload)
            if (level) chain().toggleHeading({ level }).run()
            break
        }
        case 'set-link': {
            const href = readLinkHref(action.payload)
            if (href) chain().setLink({ href }).run()
            else chain().unsetLink().run()
            break
        }
        case 'insert-image': {
            const image = readImagePayload(action.payload)
            // chain().focus() restores the selection the page held before the
            // RN-side picker dialog took focus, so the image lands at the
            // caret the user left.
            if (image) chain().setImage(image).run()
            break
        }
        case 'undo':
            chain().undo().run()
            break
        case 'redo':
            chain().redo().run()
            break
        case FORMAT_SET_EDITABLE:
            if (typeof action.payload === 'boolean') editor.setEditable(action.payload)
            break
        default:
            break
    }
}

type HeadingLevel = 1 | 2 | 3 | 4 | 5 | 6

// The level travels bare (`toggle-heading`'s payload is the number); an
// `{level}` object is accepted too so a caller cannot get it wrong.
function readHeadingLevel(payload: unknown): HeadingLevel | null {
    const raw =
        typeof payload === 'number'
            ? payload
            : typeof payload === 'object' && payload !== null
              ? (payload as { level?: unknown }).level
              : undefined
    return typeof raw === 'number' && raw >= 1 && raw <= 6 ? (raw as HeadingLevel) : null
}

function readLinkHref(payload: unknown): string {
    if (typeof payload === 'string') return payload
    if (typeof payload === 'object' && payload !== null) {
        const href = (payload as { href?: unknown }).href
        if (typeof href === 'string') return href
    }
    return ''
}

function readImagePayload(payload: unknown): { src: string; alt?: string } | null {
    if (typeof payload !== 'object' || payload === null) return null
    const { src, alt } = payload as { src?: unknown; alt?: unknown }
    if (typeof src !== 'string' || src.length === 0) return null
    return { src, ...(typeof alt === 'string' ? { alt } : {}) }
}

/**
 * Tell the host how tall the document is.
 *
 * A WebView has no intrinsic height, and inside a ScrollView it has nothing to
 * flex against either, so the host cannot work this out for itself — the
 * editor ends up clipped to whatever fixed height was guessed. Measuring the
 * page and reporting it is the only way the container can track content.
 *
 * `ResizeObserver` rather than an editor `update` listener: height changes for
 * reasons ProseMirror never emits an update for — a font finishing loading, an
 * image decoding, the keyboard changing the viewport width and reflowing a
 * paragraph onto another line.
 */
function useContentHeight(editor: TiptapEditor | null) {
    useEffect(() => {
        if (!editor) return
        let last = -1
        function report() {
            if (!editor || editor.isDestroyed) return
            try {
                reportUnguarded()
            } catch (error) {
                reportPageError('content-height', error)
            }
        }
        function reportUnguarded() {
            const node = editor?.view.dom as HTMLElement | undefined
            if (!node) return
            const children = Array.from(node.children) as HTMLElement[]
            if (children.length === 0) return
            // Measure from the TOP OF THE PAGE to the last block's bottom,
            // not first-child-top to last-child-bottom. The earlier version
            // dropped whatever sits above the first block and, worse, both
            // outer margins — which clipped the final line of a long
            // description, since a collapsed bottom margin falls outside
            // every child's bounding box.
            //
            // Deliberately NOT `node.getBoundingClientRect()` or
            // `documentElement.scrollHeight`: `.ProseMirror` carries
            // `min-height: 100%`, so both report the viewport back — the very
            // value being set — and the loop can only grow.
            const last_ = children[children.length - 1]
            const bottom = last_.getBoundingClientRect().bottom + window.scrollY
            const marginBottom = Number.parseFloat(getComputedStyle(last_).marginBottom) || 0
            // Trailing breathing room. A description that ends flush against
            // the container edge reads as cut off even when it isn't, and it
            // leaves nowhere comfortable to tap to put the caret at the end.
            const height = Math.ceil(bottom + marginBottom + TRAILING_SPACE_PX)
            if (height <= 0 || Math.abs(height - last) < 2) return
            last = height
            postToNative(makeMessage('ui', UI_CONTENT_HEIGHT, { height }))
        }

        const observer = new ResizeObserver(report)
        // Observe the BODY, not documentElement: the latter's size is the
        // viewport, which is what the host is being told to set, so observing
        // it would fire on our own resize. The body wraps the editor and grows
        // with it — including a reflow inside a block, which resizing the
        // editor node alone does not report.
        observer.observe(document.body)
        // ResizeObserver fires on observe, but the first frame can measure
        // before styles land; an editor update is the other trigger that
        // matters and costs nothing to also listen for.
        editor.on('update', report)
        report()

        return () => {
            observer.disconnect()
            editor.off('update', report)
        }
    }, [editor])
}

/**
 * Report Escape to the host.
 *
 * Handled at the document level rather than as a ProseMirror keymap: the host
 * needs it even when focus has moved to a nested input inside the page, and it
 * must reach the surrounding dialog, which lives outside the WebView.
 */
function useEscapeKey() {
    useEffect(() => {
        function onKeyDown(evt: KeyboardEvent) {
            if (evt.key === 'Escape') postToNative(makeMessage('app', APP_ESCAPE, null))
        }
        document.addEventListener('keydown', onKeyDown)
        return () => document.removeEventListener('keydown', onKeyDown)
    }, [])
}
