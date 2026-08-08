import type { Editor as TiptapEditor } from '@tiptap/core'
import { EditorContent, useEditor } from '@tiptap/react'
import { useEffect, useState } from 'react'
import type { EditorMessage } from '../../../message-bus/types'
import { makeMessage } from '../../../message-bus/types'
import { buildRichEditorExtensions } from '../../extensions'
import { repairMarkdown } from '../../markdown-repair'
import {
    APP_ESCAPE,
    APP_SUBMIT_SHORTCUT,
    EDITOR_READY,
    MARKDOWN_GET,
    MARKDOWN_RESULT,
    MARKDOWN_SET,
    type MarkdownSetPayload,
    type RichEditorInitPayload,
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

    const editor = useEditor({
        editable: init.editable,
        autofocus: init.autofocus ? 'end' : false,
        extensions: buildRichEditorExtensions({
            placeholder: init.placeholder,
            characterLimit: init.characterLimit,
            onSubmitShortcut: () => postToNative(makeMessage('app', APP_SUBMIT_SHORTCUT, null)),
            // Collaboration is not enabled yet. Routing through the same
            // builder call from the start is what keeps turning it on additive
            // — the option already exists on the builder.
            collab: undefined,
        }),
        // Tiptap 3 defaults this false; the toolbar reads active marks off
        // every transaction, so it has to re-render on them.
        shouldRerenderOnTransaction: true,
    })

    useInitialContent(editor, init)
    useStateBroadcast(editor)
    useHostMessages(editor)
    useEscapeKey()

    return <EditorContent editor={editor} />
}

/**
 * Apply the starting document once the editor exists.
 *
 * Markdown goes in as markdown — `setContent` routes through the markdown
 * extension's parser. Mail passes 'html' and takes the same path Tiptap would
 * take for an HTML string.
 */
function useInitialContent(editor: TiptapEditor | null, init: RichEditorInitPayload) {
    useEffect(() => {
        if (!editor) return
        if (!init.initialContent) return
        editor.commands.setContent(init.initialContent, {
            emitUpdate: false,
            // The markdown extension keys off this to pick its parser; without
            // it a markdown string is treated as HTML and arrives as literal
            // syntax.
            ...(init.contentFormat === 'markdown' ? { contentType: 'markdown' as const } : {}),
        })
    }, [editor, init.initialContent, init.contentFormat])
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
function useHostMessages(editor: TiptapEditor | null) {
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

            if (parsed.namespace === 'markdown') {
                handleMarkdownMessage(editor, parsed)
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
    }, [editor])
}

function handleMarkdownMessage(editor: TiptapEditor, message: EditorMessage): void {
    if (message.type === MARKDOWN_SET) {
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
