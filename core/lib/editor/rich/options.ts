import type { RichEditorCollabOptions } from './extensions'
import type { TriggerConfig } from './triggers'

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
    /**
     * Focus/blur of the editing surface itself, not the container view.
     *
     * Drives chrome that should only appear while someone is writing — a
     * formatting toolbar, a focused border. On native these fire from the
     * WebView's own focus state relayed over the bridge, so both platforms
     * agree on what "focused" means.
     */
    onFocus?: () => void
    onBlur?: () => void
    /**
     * Receives image files dropped or pasted onto the editor, with the
     * ProseMirror position they should land at (the drop point, or the caret
     * for a paste). The caller owns upload + insertion — this hook must not
     * know where files go. Web only: OS drops don't exist on native, and the
     * WebView intercepts its own paste. Non-image drops are left to bubble so
     * a surrounding DropZone can attach them.
     */
    onImageDrop?: (files: File[], pos: number) => void
    /** Native WebView chrome color. Web themes through CSS custom properties. */
    theme?: { backgroundColor?: string }
    /** Binds this editor to a shared document. See RichEditorCollabOptions. */
    collab?: RichEditorCollabOptions
    /**
     * Character-triggered autocompletes (e.g. `@` mentions). Handled inside the
     * shared extension builder so web and the native WebView behave identically
     * — see rich/triggers.ts.
     */
    triggers?: TriggerConfig[]
    /**
     * Publishes this editor's handle so a host-drawn overlay (a native trigger
     * popover) can anchor to it. Native-only; web popovers position from the
     * DOM and ignore it.
     *
     * Must be unique PER EDITOR. A card detail mounts a description editor and
     * a comment composer against the same board, so anything derived from the
     * board alone would anchor one's popover to the other's WebView.
     */
    overlayKey?: string
}
