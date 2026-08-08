import { EditorContent, useEditor } from '@tiptap/react'
import { useMemo, useRef } from 'react'
import { View } from 'react-native'
import { useThemeColor } from '../../use-app-theme'
import type { EditorCommands, EditorHandle, EditorResult, EditorToolbarState } from '../types'
import { buildRichEditorExtensions } from './extensions'
import { repairMarkdown } from './markdown-repair'
import type { UseRichEditorOptions } from './options'

/**
 * The shared rich-text editor: one schema, one set of commands, used by mail
 * (HTML in and out), cards comments (markdown, single author) and cards
 * descriptions (markdown, collaborative).
 *
 * Returns the same `EditorResult` contract every editor in the app implements,
 * so a consumer can be moved onto this without changing its call sites.
 */
export function useRichEditor(options: UseRichEditorOptions = {}): EditorResult {
    const {
        initialContent,
        contentFormat = 'html',
        placeholder,
        autofocus,
        editable = true,
        containerClassName,
        characterLimit,
        onSubmitShortcut,
        onEscape,
        collab,
    } = options

    const placeholderColor = useThemeColor('field-placeholder')
    const primaryColor = useThemeColor('primary')

    // Callers pass these inline, so their object identity changes every render.
    // Reading them through a ref keeps the extension list — and therefore the
    // editor — stable, while the handlers still call the latest closure.
    const submitRef = useRef(onSubmitShortcut)
    submitRef.current = onSubmitShortcut
    const escapeRef = useRef(onEscape)
    escapeRef.current = onEscape

    // Rebuilding the extension list recreates the editor, and recreating the
    // editor re-renders, so an unstable list here is an infinite loop (React
    // "Maximum update depth exceeded"). The deps are therefore the PRIMITIVE
    // identity of the configuration, never the caller's objects.
    const collabDoc = collab?.document ?? null
    const collabField = collab?.field ?? null
    const collabAwareness = collab?.awareness ?? null
    const collabUserId = collab?.user?.id ?? null
    const collabUserName = collab?.user?.name ?? null
    const collabUserColor = collab?.user?.color ?? null

    const extensions = useMemo(
        () =>
            buildRichEditorExtensions({
                placeholder,
                characterLimit,
                onSubmitShortcut: onSubmitShortcut ? () => submitRef.current?.() : undefined,
                collab:
                    collabDoc && collabField
                        ? {
                              document: collabDoc,
                              field: collabField,
                              awareness: collabAwareness ?? undefined,
                              user:
                                  collabUserId && collabUserName && collabUserColor
                                      ? {
                                            id: collabUserId,
                                            name: collabUserName,
                                            color: collabUserColor,
                                        }
                                      : undefined,
                          }
                        : undefined,
            }),
        [
            placeholder,
            characterLimit,
            // Only whether a handler exists matters; the ref carries the value.
            !!onSubmitShortcut,
            collabDoc,
            collabField,
            collabAwareness,
            collabUserId,
            collabUserName,
            collabUserColor,
        ]
    )

    const tiptapEditor = useEditor(
        {
            extensions,
            // Under collaboration the document arrives over the wire; passing
            // content here would have every client re-apply it on connect.
            content: collab ? undefined : (initialContent ?? ''),
            // Markdown initial content needs the extension to parse it rather
            // than tiptap treating the string as HTML.
            ...(collab || contentFormat !== 'markdown' ? {} : { contentType: 'markdown' as const }),
            editable,
            // Mail declared this option but never applied it on web; honoring
            // it here is why a reply opens with the caret already in the body.
            autofocus: autofocus ? 'end' : false,
            editorProps: {
                handleKeyDown: (_view, event) =>
                    event.key === 'Escape' ? (escapeRef.current?.() ?? false) : false,
            },
        },
        [extensions, editable]
    )

    const editor: EditorHandle = useMemo(() => {
        // tiptap nulls commandManager on destroy, and useEditor can briefly
        // return a destroyed instance during a remount. Touching `.commands` in
        // that window throws, so every imperative call is gated.
        const isLive = () => !!tiptapEditor && !tiptapEditor.isDestroyed
        const warnIfCollab = (method: string) => {
            if (collab && __DEV__) {
                console.warn(
                    `[editor] ${method} is a no-op under collaboration — seed the shared document instead`
                )
            }
            return !!collab
        }
        return {
            getHTML: () => Promise.resolve(isLive() ? (tiptapEditor?.getHTML() ?? '') : ''),
            getText: () => Promise.resolve(isLive() ? (tiptapEditor?.getText() ?? '') : ''),
            // repairMarkdown is not optional: the raw serializer mangles code
            // spans containing a backtick and drops table-cell pipe escapes.
            getMarkdown: () =>
                Promise.resolve(isLive() ? repairMarkdown(tiptapEditor?.getMarkdown() ?? '') : ''),
            setContent: (content: string) => {
                if (!isLive() || warnIfCollab('setContent')) return
                tiptapEditor?.commands.setContent(content)
            },
            setMarkdown: (markdown: string) => {
                if (!isLive() || warnIfCollab('setMarkdown')) return
                tiptapEditor?.commands.setContent(markdown, { contentType: 'markdown' })
            },
            focus: (position?: 'start' | 'end') => {
                if (!isLive()) return
                tiptapEditor
                    ?.chain()
                    .focus(position === 'start' ? 'start' : 'end')
                    .run()
            },
            clear: () => {
                if (!isLive() || warnIfCollab('clear')) return
                tiptapEditor?.commands.clearContent()
            },
            getSelection: () => {
                if (!isLive()) return Promise.resolve(null)
                const selection = tiptapEditor?.state.selection
                if (!selection) return Promise.resolve(null)
                return Promise.resolve({ from: selection.from, to: selection.to })
            },
        }
    }, [tiptapEditor, collab])

    const commands: EditorCommands = useMemo(() => {
        const chain = () =>
            tiptapEditor && !tiptapEditor.isDestroyed ? tiptapEditor.chain().focus() : null
        return {
            toggleBold: () => chain()?.toggleBold().run(),
            toggleItalic: () => chain()?.toggleItalic().run(),
            toggleUnderline: () => chain()?.toggleUnderline().run(),
            toggleBulletList: () => chain()?.toggleBulletList().run(),
            toggleOrderedList: () => chain()?.toggleOrderedList().run(),
            toggleBlockquote: () => chain()?.toggleBlockquote().run(),
            toggleHeading: (level: number) =>
                chain()
                    ?.toggleHeading({ level: level as 1 | 2 | 3 | 4 | 5 | 6 })
                    .run(),
            toggleCode: () => chain()?.toggleCode().run(),
            toggleCodeBlock: () => chain()?.toggleCodeBlock().run(),
            setLink: (url: string) => chain()?.setLink({ href: url }).run(),
            removeLink: () => chain()?.unsetLink().run(),
            undo: () => chain()?.undo().run(),
            redo: () => chain()?.redo().run(),
        }
    }, [tiptapEditor])

    const live = tiptapEditor && !tiptapEditor.isDestroyed ? tiptapEditor : null
    const toolbarState: EditorToolbarState = {
        isBoldActive: live?.isActive('bold') ?? false,
        isItalicActive: live?.isActive('italic') ?? false,
        isUnderlineActive: live?.isActive('underline') ?? false,
        isBulletListActive: live?.isActive('bulletList') ?? false,
        isOrderedListActive: live?.isActive('orderedList') ?? false,
        isBlockquoteActive: live?.isActive('blockquote') ?? false,
        isLinkActive: live?.isActive('link') ?? false,
        isCodeActive: live?.isActive('code') ?? false,
        isCodeBlockActive: live?.isActive('codeBlock') ?? false,
        currentLink: (live?.getAttributes('link')?.href as string) ?? null,
        isEmpty: live?.isEmpty ?? true,
    }

    const EditorComponent = useMemo(
        () =>
            function RichEditorContent() {
                return (
                    <View
                        className={containerClassName}
                        style={{
                            // @ts-expect-error CSS custom properties for web
                            '--editor-placeholder-color': placeholderColor,
                            '--editor-primary-color': primaryColor,
                        }}
                    >
                        <EditorContent editor={tiptapEditor} />
                    </View>
                )
            },
        [tiptapEditor, placeholderColor, primaryColor, containerClassName]
    )

    return { editor, EditorComponent, commands, toolbarState, isReady: !!live }
}
