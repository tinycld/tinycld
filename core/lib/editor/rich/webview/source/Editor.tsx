import type { Editor as TiptapEditor } from '@tiptap/core'
import { EditorContent, useEditor } from '@tiptap/react'
import { useEffect, useState } from 'react'
import { Awareness } from 'y-protocols/awareness'
import * as Y from 'yjs'
import type { EditorMessage } from '../../../message-bus/types'
import { makeMessage } from '../../../message-bus/types'
import { buildRichEditorExtensions } from '../../extensions'
import { repairMarkdown } from '../../markdown-repair'
import {
    APP_ESCAPE,
    APP_SUBMIT_SHORTCUT,
    decodeUpdate,
    EDITOR_READY,
    encodeUpdate,
    MARKDOWN_GET,
    MARKDOWN_RESULT,
    MARKDOWN_SET,
    type MarkdownSetPayload,
    type RichEditorInitCollab,
    type RichEditorInitPayload,
    UI_CONTENT_HEIGHT,
    YJS_UPDATE,
    type YjsUpdatePayload,
} from './protocol'
import { deriveWebViewState } from './state'
import { buildEditorCSS } from './styles'

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
 * The rich editor's in-WebView page.
 *
 * This is what makes markdown the editor's native format on mobile. The editor
 * here is a real Tiptap instance built from `buildRichEditorExtensions()` — the
 * same schema the web hook uses, `@tiptap/markdown` included — so markdown is
 * parsed and serialized in place. Nothing pivots through HTML.
 *
 * TenTap is still the WebView host (it supplies `RichText` and, importantly,
 * `avoidIosKeyboard`), but its bridge protocol is bypassed: we own the page via
 * `customSource`, so `useTenTap` and the `BridgeExtension` system are unused.
 * Their channel exchanges HTML strings, which is exactly the constraint being
 * removed.
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
                setInit(parsed.payload as RichEditorInitPayload)
            }
        }
        // Some platforms deliver WebView messages on window, others on
        // document; listen to both.
        window.addEventListener('message', onMessage)
        document.addEventListener('message', onMessage)
        // Posted before Tiptap exists — this is the gate the host waits on
        // before sending init, so it must not depend on the editor.
        postToNative({ type: EDITOR_READY, payload: undefined })
        return () => {
            window.removeEventListener('message', onMessage)
            document.removeEventListener('message', onMessage)
        }
    }, [])

    if (init == null) return null

    return <EditorMounted init={init} />
}

function EditorMounted({ init }: { init: RichEditorInitPayload }) {
    useEffect(() => {
        const style = document.createElement('style')
        style.id = 'tinycld-rich-editor-styles'
        style.textContent = buildEditorCSS(init.colors)
        document.head.appendChild(style)
        return () => style.remove()
    }, [init.colors])

    const collab = useCollabDoc(init.collab)

    const editor = useEditor({
        editable: init.editable,
        autofocus: init.autofocus ? 'end' : false,
        extensions: buildRichEditorExtensions({
            placeholder: init.placeholder,
            characterLimit: init.characterLimit,
            onSubmitShortcut: () => postToNative(makeMessage('app', APP_SUBMIT_SHORTCUT, null)),
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
 * still appears once, because this relay carries document updates only: the
 * Awareness below never leaves the WebView, and board presence rides the
 * host's socket.
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
        // Awareness is page-local and drives only the carets rendered in this
        // WebView. Board-level presence stays on the host's socket, which is
        // where the avatar row reads from.
        const awareness = init.user ? new Awareness(doc) : undefined
        if (awareness && init.user) awareness.setLocalStateField('user', init.user)
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
 * Stream toolbar state to the host on every meaningful transaction.
 *
 * Posted under TenTap's own `stateUpdate` type so `useBridgeState` on the
 * native side keeps consuming it unchanged. Coalesced per frame, with an
 * identity skip so a burst of transactions that doesn't change the toolbar
 * (bulk paste, remote edits) doesn't spam the bridge.
 */
function useStateBroadcast(editor: TiptapEditor | null) {
    useEffect(() => {
        if (!editor) return
        let scheduled = false
        let lastSerialized = ''

        function send() {
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
            requestAnimationFrame(send)
        }

        editor.on('transaction', schedule)
        editor.on('update', schedule)
        // Prime the host with a first snapshot so isReady flips without
        // requiring the user to touch the editor.
        schedule()
        return () => {
            editor.off('transaction', schedule)
            editor.off('update', schedule)
        }
    }, [editor])
}

/**
 * Dispatch host → WebView messages.
 *
 * Two envelope shapes arrive: our own namespaced messages, and TenTap's bridge
 * actions, which wrap the real action as `{type:'action', payload:{type,...}}`.
 * Unwrapping that is load-bearing — without it every native toolbar button
 * reads as type 'action', matches nothing, and silently no-ops.
 */
function useHostMessages(editor: TiptapEditor | null, isCollab: boolean) {
    useEffect(() => {
        if (!editor) return

        function onMessage(evt: MessageEvent | Event) {
            const data = (evt as MessageEvent).data
            if (typeof data !== 'string' || !editor) return
            let parsed: EditorMessage
            try {
                parsed = JSON.parse(data) as EditorMessage
            } catch {
                return
            }

            // Handled by useYjsRelay; not a format action.
            if (parsed.namespace === 'yjs') return
            if (parsed.namespace === 'markdown') {
                handleMarkdownMessage(editor, parsed, isCollab)
                return
            }
            dispatchFormatAction(editor, unwrapTenTapAction(parsed))
        }

        window.addEventListener('message', onMessage)
        document.addEventListener('message', onMessage)
        return () => {
            window.removeEventListener('message', onMessage)
            document.removeEventListener('message', onMessage)
        }
    }, [editor, isCollab])
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

interface IncomingAction {
    namespace?: string
    type?: string
    payload?: unknown
}

// TenTap's useEditorBridge wraps every bridge command as
// {type:'action', payload:{type:'toggle-bold'}}. Our own 'format' messages are
// already flat and pass through untouched.
function unwrapTenTapAction(parsed: IncomingAction): IncomingAction {
    if (parsed.type !== 'action') return parsed
    const inner = parsed.payload
    if (inner === null || typeof inner !== 'object') return parsed
    if (typeof (inner as IncomingAction).type !== 'string') return parsed
    return inner as IncomingAction
}

function dispatchFormatAction(editor: TiptapEditor, action: IncomingAction): void {
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
        // TenTap's list bridges emit camelCase action strings, not kebab-case.
        // The literal has to match exactly or the message is dropped.
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
        case 'undo':
            chain().undo().run()
            break
        case 'redo':
            chain().redo().run()
            break
        case 'focus':
            editor.commands.focus('end')
            break
        default:
            break
    }
}

type HeadingLevel = 1 | 2 | 3 | 4 | 5 | 6

// TenTap's HeadingBridge emits the level bare; our own format messages wrap it
// in {level}. Accept both rather than depending on which side sent it.
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
