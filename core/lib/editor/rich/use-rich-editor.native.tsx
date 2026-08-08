import {
    BlockquoteBridge,
    BoldBridge,
    BulletListBridge,
    CodeBridge,
    CoreBridge,
    DropCursorBridge,
    HardBreakBridge,
    HeadingBridge,
    HistoryBridge,
    ItalicBridge,
    LinkBridge,
    OrderedListBridge,
    PlaceholderBridge,
    RichText,
    StrikeBridge,
    TaskListBridge,
    UnderlineBridge,
    useBridgeState,
    useEditorBridge,
} from '@10play/tentap-editor'
import { useMemo, useRef } from 'react'
import { View } from 'react-native'
import { useThemeColor } from '../../use-app-theme'
import type { EditorCommands, EditorHandle, EditorResult, EditorToolbarState } from '../types'
import { htmlToMarkdown, markdownToHTML } from './html-markdown'
import type { UseRichEditorOptions } from './options'

/**
 * Native build of the shared editor: Tiptap running inside a WebView through
 * TenTap's bridges.
 *
 * Markdown is converted on the React Native side rather than in the WebView.
 * TenTap's bridge protocol exchanges HTML, and adding a markdown round-trip
 * inside the WebView would mean shipping a second bundle with its own copy of
 * the schema — the exact duplication that makes text's two extension lists
 * drift. Converting here keeps one schema and one serializer.
 *
 * Collaboration is not yet wired on native: it needs Yjs updates relayed across
 * the bridge, which is a separate piece of work. Passing `collab` here logs in
 * development and renders a plain editor rather than silently pretending to
 * sync.
 */
function buildEditorCSS(colors: { bg: string; fg: string; placeholder: string; primary: string }) {
    return `
        * {
            background-color: ${colors.bg};
            color: ${colors.fg};
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;
        }
        .ProseMirror {
            padding: 0;
            min-height: 100%;
            font-size: 14px;
            line-height: 1.5;
        }
        .ProseMirror:focus {
            outline: none;
        }
        .is-editor-empty:first-child::before {
            color: ${colors.placeholder};
            content: attr(data-placeholder);
            float: left;
            height: 0;
            pointer-events: none;
        }
        blockquote {
            border-left: 3px solid ${colors.placeholder};
            padding-left: 1rem;
            margin-left: 0;
        }
        a {
            color: ${colors.primary};
            text-decoration: underline;
        }
        ul, ol {
            padding-left: 1.5rem;
        }
        code {
            font-family: ui-monospace, Menlo, monospace;
        }
        table {
            border-collapse: collapse;
        }
        td, th {
            border: 1px solid ${colors.placeholder};
            padding: 4px 8px;
        }
    `
}

const baseBridgeExtensions = [
    BoldBridge,
    ItalicBridge,
    UnderlineBridge,
    StrikeBridge,
    CodeBridge,
    HeadingBridge,
    BulletListBridge,
    OrderedListBridge,
    TaskListBridge,
    BlockquoteBridge,
    LinkBridge,
    HistoryBridge,
    HardBreakBridge,
    DropCursorBridge,
]

export function useRichEditor(options: UseRichEditorOptions = {}): EditorResult {
    const {
        initialContent,
        contentFormat = 'html',
        placeholder = '',
        autofocus,
        editable = true,
        theme,
        collab,
    } = options

    const bgColor = useThemeColor('background')
    const fgColor = useThemeColor('foreground')
    const placeholderColor = useThemeColor('field-placeholder')
    const primaryColor = useThemeColor('primary')

    if (collab && __DEV__) {
        console.warn(
            '[editor] collaborative editing is not yet supported on native; rendering a local editor'
        )
    }

    const bridgeExtensions = useMemo(() => {
        const css = buildEditorCSS({
            bg: theme?.backgroundColor ?? bgColor,
            fg: fgColor,
            placeholder: placeholderColor,
            primary: primaryColor,
        })
        return [
            CoreBridge.configureCSS(css),
            ...baseBridgeExtensions,
            PlaceholderBridge.configureExtension({ placeholder }),
        ]
    }, [theme?.backgroundColor, bgColor, fgColor, placeholderColor, primaryColor, placeholder])

    const editorTheme = useMemo(
        () => ({ webview: { backgroundColor: theme?.backgroundColor ?? bgColor } }),
        [theme?.backgroundColor, bgColor]
    )

    const liveBridge = useEditorBridge({
        initialContent:
            contentFormat === 'markdown' && initialContent
                ? markdownToHTML(initialContent)
                : initialContent,
        bridgeExtensions,
        theme: editorTheme,
        autofocus: autofocus ?? false,
        avoidIosKeyboard: true,
        editable,
    })

    // useEditorBridge returns a fresh wrapper every render even though its
    // underlying refs are stable. Passing that fresh object to <RichText>
    // remounts the WebView on every re-render, which steals focus on every
    // keystroke. Pin the first wrapper; the refs still route correctly.
    const bridgeRef = useRef(liveBridge)
    const bridge = bridgeRef.current

    const bridgeState = useBridgeState(bridge)

    const editor: EditorHandle = useMemo(
        () => ({
            getHTML: () => bridge.getHTML(),
            getText: () => bridge.getText(),
            // The WebView speaks HTML, so markdown is produced here. The
            // conversion applies the same repair pass as the web variant.
            getMarkdown: async () => htmlToMarkdown(await bridge.getHTML()),
            setContent: (html: string) => bridge.setContent(html),
            setMarkdown: (markdown: string) => bridge.setContent(markdownToHTML(markdown)),
            focus: (position?: 'start' | 'end') => bridge.focus(position ?? 'end'),
            clear: () => bridge.setContent(''),
            // TenTap exposes no selection query. Callers must tolerate null —
            // the contract marks getSelection as best-effort for this reason.
            getSelection: () => Promise.resolve(null),
        }),
        [bridge]
    )

    const commands: EditorCommands = useMemo(
        () => ({
            toggleBold: () => bridge.toggleBold(),
            toggleItalic: () => bridge.toggleItalic(),
            toggleUnderline: () => bridge.toggleUnderline(),
            toggleBulletList: () => bridge.toggleBulletList(),
            toggleOrderedList: () => bridge.toggleOrderedList(),
            toggleBlockquote: () => bridge.toggleBlockquote(),
            toggleHeading: (level: number) => bridge.toggleHeading(level as 1 | 2 | 3 | 4 | 5 | 6),
            toggleCode: () => bridge.toggleCode?.(),
            setLink: (url: string) => bridge.setLink(url),
            removeLink: () => bridge.setLink(''),
            undo: () => bridge.undo(),
            redo: () => bridge.redo(),
        }),
        [bridge]
    )

    const toolbarState: EditorToolbarState = {
        isBoldActive: bridgeState.isBoldActive,
        isItalicActive: bridgeState.isItalicActive,
        isUnderlineActive: bridgeState.isUnderlineActive,
        isBulletListActive: bridgeState.isBulletListActive,
        isOrderedListActive: bridgeState.isOrderedListActive,
        isBlockquoteActive: bridgeState.isBlockquoteActive,
        isLinkActive: bridgeState.isLinkActive,
        isCodeActive: bridgeState.isCodeActive,
        currentLink: bridgeState.activeLink ?? null,
        isEmpty: bridgeState.empty,
    }

    const EditorComponent = useMemo(
        () =>
            function RichEditorContent() {
                return (
                    <View className="flex-1">
                        <RichText editor={bridge} scrollEnabled={false} />
                    </View>
                )
            },
        [bridge]
    )

    return {
        editor,
        EditorComponent,
        commands,
        toolbarState,
        webViewRef: bridge.webviewRef ?? null,
        isReady: bridgeState.isReady ?? true,
    }
}
