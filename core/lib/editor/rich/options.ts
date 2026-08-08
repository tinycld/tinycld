import type { RichEditorCollabOptions } from './extensions'

export type { RichEditorCollabOptions }

/** How `initialContent` is interpreted and what `setContent` accepts. */
export type EditorContentFormat = 'html' | 'markdown'

export interface UseRichEditorOptions {
    /**
     * Starting content, in `contentFormat`.
     *
     * Ignored under collaboration, where the Yjs document is the source of
     * truth: the markdown extension mutates `options.content` in
     * `onBeforeCreate`, so every joining client would re-apply it and duplicate
     * the document. The server seeds collaborative documents instead.
     */
    initialContent?: string
    contentFormat?: EditorContentFormat
    placeholder?: string
    autofocus?: boolean
    /** False renders a read-only editor — the client mirror of a write gate. */
    editable?: boolean
    /** Extra classes for the wrapping view (web). */
    containerClassName?: string
    /** Hard character ceiling; typing stops here rather than failing on save. */
    characterLimit?: number
    /** ⌘/Ctrl+Enter handler. See SubmitShortcut for why this is not a DOM listener. */
    onSubmitShortcut?: () => void
    /**
     * Escape handler. Return true when handled, to stop the key bubbling to a
     * surrounding dialog — an editor that closes the panel on the first Escape
     * mid-sentence loses the sentence.
     */
    onEscape?: () => boolean
    /** Native WebView chrome color. Web themes through CSS custom properties. */
    theme?: { backgroundColor?: string }
    /** Binds this editor to a shared document. See RichEditorCollabOptions. */
    collab?: RichEditorCollabOptions
}
