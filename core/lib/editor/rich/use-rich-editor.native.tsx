import { CoreBridge, TenTapStartKit } from '@10play/tentap-editor'
import { useCallback, useEffect, useMemo, useRef } from 'react'
import { useThemeColor } from '../../use-app-theme'
import type { EditorHandle, EditorResult } from '../types'
import { useWebViewEditor } from '../use-webview-editor'
import { MarkdownWebViewHost } from './markdown-webview-host'
import type { UseRichEditorOptions } from './options'
import { editorHtml } from './webview/build/editorHtml'
import {
    APP_ESCAPE,
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
 * The WebView never opens a socket of its own: that is what text/ does today,
 * and the second connection ships a credential into the page and makes the
 * local user appear twice in presence (TODO(text-native v1.1)).
 */
export function useRichEditor(options: UseRichEditorOptions = {}): EditorResult {
    const {
        initialContent,
        contentFormat = 'html',
        placeholder = '',
        autofocus,
        editable = true,
        characterLimit,
        onSubmitShortcut,
        onEscape,
        theme,
        collab,
    } = options

    const bgColor = useThemeColor('background')
    const fgColor = useThemeColor('foreground')
    const placeholderColor = useThemeColor('field-placeholder')
    const primaryColor = useThemeColor('primary')

    // Callers pass these inline, so their identity changes every render.
    // Reading them through refs keeps the WebView from remounting.
    const submitRef = useRef(onSubmitShortcut)
    submitRef.current = onSubmitShortcut
    const escapeRef = useRef(onEscape)
    escapeRef.current = onEscape

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

    // The init payload is posted once, after the page reports ready. It is
    // deliberately built from primitives so a parent re-render doesn't produce
    // a fresh object and re-trigger the handshake effect — hence the collab
    // identity is spread into its own primitives rather than passed as an
    // object, matching what the web hook does with its six.
    const collabField = collab?.field
    const collabUserId = collab?.user?.id
    const collabUserName = collab?.user?.name
    const collabUserColor = collab?.user?.color
    const initPayload: RichEditorInitPayload = useMemo(
        () => ({
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
            // Snapshotted at handshake time. Anything the doc gains between
            // now and the page mounting arrives as a normal relayed update, so
            // a slightly stale seed is not a lost edit.
            ...(yjsHost && collabField
                ? {
                      collab: {
                          field: collabField,
                          clientID: yjsHost.clientID(),
                          initialState: yjsHost.encodeState(),
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
        }),
        [
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
            collabField,
            collabUserId,
            collabUserName,
            collabUserColor,
        ]
    )

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

    // TenTap's stock bridges still drive the toolbar commands and the
    // getHTML/getText/setContent/focus surface that mail relies on. Their
    // Tiptap counterparts live in our page, which registers the same schema.
    const bridgeExtensions = useMemo(() => [...TenTapStartKit, CoreBridge.configureCSS('')], [])

    const onMessage = useCallback(
        (message: { namespace?: string; type?: string }) => {
            if (yjsHost?.handleMessage(message as never)) return
            if (markdownHost.handleMessage(message as never)) return
            if (message.namespace !== 'app') return
            if (message.type === APP_SUBMIT_SHORTCUT) submitRef.current?.()
            else if (message.type === APP_ESCAPE) escapeRef.current?.()
        },
        [markdownHost, yjsHost]
    )

    const result = useWebViewEditor({
        editorHtml,
        bridgeExtensions,
        initPayload,
        editable,
        theme: { webview: { backgroundColor: theme?.backgroundColor ?? bgColor } },
        avoidIosKeyboard: true,
        // The description editor sits inside the card detail's scroll view;
        // an inner scroll surface would fight it.
        scrollEnabled: false,
        onMessage,
    })

    posterRef.current = result.postMessage ?? null

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
