import { EditorContent, ReactNodeViewRenderer, useEditor } from '@tiptap/react'
import { useEffect, useMemo, useRef } from 'react'
import { View } from 'react-native'
import { log } from '../../logger'
import { useThemeColor } from '../../use-app-theme'
import type { EditorCommands, EditorHandle, EditorResult, EditorToolbarState } from '../types'
import { AuthedImageView } from './AuthedImageView.web'
import {
    EDITOR_CONTENT_STYLES,
    EDITOR_SCOPE_CLASS,
    scopeEditorStyles,
} from './editor-content-styles'
import { editorScaleVars } from './editor-scale'
import { buildRichEditorExtensions } from './extensions'
import { extractImageFilesFromDrop, extractImageFilesFromPaste } from './extract-image-files'
import { repairMarkdown } from './markdown-repair'
import { setMentionLabels } from './mention-node'
import type { UseRichEditorOptions } from './options'
import type { TriggerConfig } from './triggers'

/**
 * Inject the shared content stylesheet once per page.
 *
 * Uniwind/Tailwind preflight strips browser defaults for h1–h6, ul, ol, a and
 * the rest, so without this a card description renders as a wall of flat text
 * with no heading hierarchy or list markers — and remote collaborator carets,
 * which the extension styles only with an inline border-COLOR, render as an
 * invisible zero-width span.
 *
 * EDITOR_CONTENT_STYLES targets a bare `.ProseMirror`. On native it goes into
 * the WebView's isolated document, so that is fine. Here it goes into the shared
 * `document.head`, where an unscoped selector would leak onto every other
 * ProseMirror on the page — notably mail's compose body, which uses this same
 * editor and has its own `.tinycld-mail-editor` rules. Prefixing each selector's
 * LEADING `.ProseMirror` with the wrapper class confines them.
 *
 * Anchored at the start of a selector on purpose: a blanket replace also rewrites
 * the second token of a descendant selector, turning
 * `.ProseMirror .ProseMirror-yjs-selection` into a rule that matches nothing.
 * (`@keyframes` blocks carry no leading `.ProseMirror` and are left alone.)
 */
/** Searched in order, so the first match is the outermost heading at the caret. */
const HEADING_LEVELS = [1, 2, 3, 4, 5, 6]

/**
 * Module-level so its identity never changes: it feeds the extension list,
 * and an unstable value there rebuilds the editor every render (see the
 * extensions memo below).
 */
const AUTHED_IMAGE_NODE_VIEW = ReactNodeViewRenderer(AuthedImageView)

const EDITOR_STYLE_TAG_ID = 'tinycld-rich-editor-styles'
if (typeof document !== 'undefined' && !document.getElementById(EDITOR_STYLE_TAG_ID)) {
    const style = document.createElement('style')
    style.id = EDITOR_STYLE_TAG_ID
    style.textContent = scopeEditorStyles(EDITOR_CONTENT_STYLES)
    document.head.appendChild(style)
}

/**
 * The shared rich-text editor: one schema, one set of commands, used by mail
 * (HTML in and out), boards comments (markdown, single author) and cards
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
        minHeight,
        characterLimit,
        generation = 0,
        onSubmitShortcut,
        onEscape,
        onFocus,
        onBlur,
        onImageDrop,
        collab,
        triggers,
        scale,
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
    const focusRef = useRef(onFocus)
    focusRef.current = onFocus
    const blurRef = useRef(onBlur)
    blurRef.current = onBlur
    // Triggers carry callbacks (`items`, `onStateChange`) that close over
    // fresh state, and callers build the array inline — so its identity churns
    // every render. Rebuilding the extension list on that identity would
    // recreate the whole editor on every keystroke. Instead the array is read
    // through a ref and each trigger is wrapped ONCE in a stable config that
    // forwards to the current one, keyed by trigger id. Same reasoning as the
    // handler refs above; the extra step is that the wrapper must survive too.
    const triggersRef = useRef(triggers)
    triggersRef.current = triggers

    // What actually warrants a rebuild: which triggers exist, not the identity
    // of the callbacks they carry. Derived into a plain string so the memo
    // below depends on a VALUE rather than an array literal — a dependency the
    // linter can check, instead of one it has to be told to ignore.
    const triggerSignature = (triggers ?? []).map(t => `${t.id}:${t.char}`).join(',')

    // Each trigger is wrapped once in a stable config that forwards to whatever
    // is current, looked up by id through the ref. Without this the editor
    // would be recreated on every keystroke — and, since `allItems` is a fresh
    // array whenever the roster query re-emits, on every membership change too.
    const stableTriggers = useMemo(
        () =>
            triggerSignature
                .split(',')
                .filter(Boolean)
                .map((entry): TriggerConfig => {
                    const id = entry.slice(0, entry.lastIndexOf(':'))
                    const char = entry.slice(entry.lastIndexOf(':') + 1)
                    const current = () => triggersRef.current?.find(x => x.id === id)
                    // `allItems` is a getter rather than a value: the roster
                    // changes as members join, and baking the array in here
                    // would freeze the candidate list at editor-creation time.
                    return {
                        id,
                        char,
                        get allItems() {
                            return current()?.allItems ?? []
                        },
                        get limit() {
                            return current()?.limit
                        },
                        get insertTemplate() {
                            return current()?.insertTemplate ?? ''
                        },
                        get insertsMentionNode() {
                            return current()?.insertsMentionNode
                        },
                        onStateChange: state => current()?.onStateChange(state),
                    }
                }),
        [triggerSignature]
    )

    // Keep each mention trigger's label roster current.
    //
    // The extension list is deliberately NOT rebuilt when the roster changes
    // (that would recreate the editor on every membership change), so a node
    // registered only at build time would resolve names against whatever the
    // query had returned at mount — usually nothing, since it resolves after.
    // Publishing on every change is what makes a mention render as a name
    // rather than "@someone".
    const mentionRosterSignature = JSON.stringify(
        (triggers ?? [])
            .filter(t => t.insertsMentionNode)
            .map(t => [t.id, t.allItems.map(i => [i.id, i.label])])
    )
    useEffect(() => {
        for (const [id, pairs] of JSON.parse(mentionRosterSignature) as [
            string,
            [string, string][],
        ][]) {
            setMentionLabels(
                id,
                pairs.map(([itemId, label]) => ({ id: itemId, label }))
            )
        }
    }, [mentionRosterSignature])

    const imageDropRef = useRef(onImageDrop)
    imageDropRef.current = onImageDrop

    // Rebuilding the extension list recreates the editor, and recreating the
    // editor re-renders, so an unstable list here is an infinite loop (React
    // "Maximum update depth exceeded"). The deps are therefore the PRIMITIVE
    // identity of the configuration, never the caller's objects.
    const collabDoc = collab?.document ?? null
    const collabField = collab?.field ?? null
    // Read through a ref, and NOT in the deps below.
    //
    // Only the Yjs binding — the document and its field — genuinely needs a
    // rebuild; the caret identity is read once when the extension is
    // configured. Both of these arrive ASYNCHRONOUSLY though (awareness with
    // the room, the user with the auth query), so depending on them destroyed
    // and recreated the editor a beat after it mounted. That takes the
    // ProseMirror node out of the document, which blurs it — the caret someone
    // had just placed by clicking a description vanished a few hundred
    // milliseconds later.
    const collabIdentityRef = useRef({ awareness: collab?.awareness, user: collab?.user })
    collabIdentityRef.current = { awareness: collab?.awareness, user: collab?.user }

    // Only WHETHER a submit handler exists belongs in the deps below — the ref
    // carries the value, and depending on the caller's inline closure would
    // rebuild the extension list (and so the editor) on every render. Hoisted to
    // a named boolean because an inline `!!onSubmitShortcut` in the dep array
    // reads to the linter as a missing dependency.
    const hasSubmitShortcut = !!onSubmitShortcut

    const extensions = useMemo(
        () =>
            buildRichEditorExtensions({
                placeholder,
                characterLimit,
                triggers: stableTriggers,
                imageNodeView: AUTHED_IMAGE_NODE_VIEW,
                onSubmitShortcut: hasSubmitShortcut ? () => submitRef.current?.() : undefined,
                collab:
                    collabDoc && collabField
                        ? {
                              document: collabDoc,
                              field: collabField,
                              // The LIVE values at configure time, rather than
                              // the ones captured when this memo last ran — see
                              // the ref's note above for why they are not deps.
                              awareness: collabIdentityRef.current.awareness,
                              user: collabIdentityRef.current.user,
                          }
                        : undefined,
            }),
        [placeholder, characterLimit, stableTriggers, hasSubmitShortcut, collabDoc, collabField]
    )

    const tiptapEditor = useEditor(
        {
            extensions,
            // Tiptap v3's useEditor does NOT re-render on transactions by
            // default, and `toolbarState` below is recomputed per render — so
            // without this every active flag freezes at its mount-time value
            // and a toolbar reads as permanently "not bold". The other two
            // editors in the ecosystem already set it: text's
            // use-document-editor.web.tsx and our own WebView page
            // (rich/webview/source/Editor.tsx).
            shouldRerenderOnTransaction: true,
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
            // Read through refs for the same reason as the handlers above: the
            // caller's inline closure would otherwise have to join the dep
            // array, rebuilding the editor on every render.
            onFocus: () => focusRef.current?.(),
            onBlur: () => blurRef.current?.(),
            editorProps: {
                handleKeyDown: (_view, event) =>
                    event.key === 'Escape' ? (escapeRef.current?.() ?? false) : false,
                handleDrop: (view, event) => {
                    const handler = imageDropRef.current
                    if (!handler) return false
                    const files = extractImageFilesFromDrop(event)
                    // Non-image drops fall through (return false, no
                    // preventDefault) so a surrounding DropZone still sees
                    // them and attaches the file the ordinary way.
                    if (files.length === 0) return false
                    event.preventDefault()
                    // Without this the same drop bubbles to the DropZone
                    // wrapping the card detail, which would ALSO upload the
                    // image as a plain attachment — every dropped image
                    // attached twice.
                    event.stopPropagation()
                    const pos =
                        view.posAtCoords({ left: event.clientX, top: event.clientY })?.pos ??
                        view.state.selection.from
                    handler(files, pos)
                    return true
                },
                handlePaste: (view, event) => {
                    const handler = imageDropRef.current
                    if (!handler) return false
                    const files = extractImageFilesFromPaste(event)
                    if (files.length === 0) return false
                    handler(files, view.state.selection.from)
                    return true
                },
            },
        },
        // `generation` is what makes a handover a full reconstruction rather
        // than a mutation: tiptap tears the editor down and rebuilds it on a dep
        // change, so the incoming surface gets a new schema, a new undo stack
        // and a fresh selection, and `content` above is re-read from the new
        // surface's initialContent. Without it the previous surface's history
        // and caret would leak into the next one — the native page has always
        // done this by keying its mount on the same value (webview/source/
        // Editor.tsx, `key={init.generation}`).
        //
        // `editable` is deliberately NOT here. A dep change DESTROYS the editor,
        // which takes its DOM node out of the document and blurs it — and this
        // value resolves asynchronously (a card's write gate waits on the room's
        // serverHello, which is null again after every disconnect). Rebuilding on
        // it meant the editor was torn down a beat after someone clicked into it,
        // losing the caret they had just placed. It is applied as a command
        // below, which is what tiptap provides setEditable for.
        [extensions, generation]
    )

    // Applied rather than rebuilt — see the dep note above.
    useEffect(() => {
        if (!tiptapEditor || tiptapEditor.isDestroyed) return
        if (tiptapEditor.isEditable !== editable) tiptapEditor.setEditable(editable)
    }, [tiptapEditor, editable])

    const editor: EditorHandle = useMemo(() => {
        // tiptap nulls commandManager on destroy, and useEditor can briefly
        // return a destroyed instance during a remount. Touching `.commands` in
        // that window throws, so every imperative call is gated.
        const isLive = () => !!tiptapEditor && !tiptapEditor.isDestroyed
        // debug, not warn: this catches a developer calling the wrong API, which
        // is a coding mistake to read in the console — not a production incident
        // worth an event.
        const warnIfCollab = (method: string) => {
            if (collab) {
                log.debug(
                    'core.editor.rich',
                    `${method} is a no-op under collaboration — seed the shared document instead`
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
            focus: (position?: 'start' | 'end' | { x: number; y: number }) => {
                if (!isLive() || !tiptapEditor) return
                if (position && typeof position === 'object') {
                    // ProseMirror resolves a viewport point to a document
                    // position itself, which is more accurate than anything
                    // derived from the read view's text nodes — it knows the
                    // editor's own layout, including nodes with no text.
                    const at = tiptapEditor.view.posAtCoords({
                        left: position.x,
                        top: position.y,
                    })?.pos
                    // A press that lands outside any text (the padding below a
                    // short description) resolves to nothing. Falling through
                    // to 'end' puts the caret where a reader who clicked past
                    // the prose would expect it.
                    if (at != null) {
                        // The DOM node first. Tiptap's `.focus()` chain command
                        // schedules its own focus and does NOT reliably take DOM
                        // focus when the view has just been mounted — the chain
                        // reports success while `document.activeElement` stays
                        // on <body>, which is the state that left a clicked
                        // description showing an editor with no caret.
                        tiptapEditor.view.dom.focus({ preventScroll: true })
                        tiptapEditor.chain().focus().setTextSelection(at).run()
                        return
                    }
                }
                // Same reason as above: take DOM focus explicitly, then let the
                // chain place the selection.
                tiptapEditor.view.dom.focus({ preventScroll: true })
                tiptapEditor
                    .chain()
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
            insertImage: (src: string, alt?: string) => chain()?.setImage({ src, alt }).run(),
            insertImageAt: (src: string, pos: number, alt?: string) => {
                if (!tiptapEditor || tiptapEditor.isDestroyed) return
                // Clamped at CALL time, not capture time: the position was
                // measured before an async upload, and collab peers may have
                // shrunk the document since.
                const max = tiptapEditor.state.doc.content.size
                const at = Math.min(Math.max(pos, 0), max)
                tiptapEditor
                    .chain()
                    .focus()
                    .insertContentAt(at, { type: 'image', attrs: { src, alt } })
                    .run()
            },
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
        // Mirrors deriveWebViewState's derivation so a heading button reads the
        // same on both platforms; the WebView already broadcast this and web
        // was the side left returning undefined.
        activeHeadingLevel:
            HEADING_LEVELS.find(level => live?.isActive('heading', { level })) ?? null,
        isEmpty: live?.isEmpty ?? true,
    }

    const EditorComponent = useMemo(
        () =>
            function RichEditorContent() {
                return (
                    <View
                        // The scope class is what makes the injected stylesheet
                        // above apply; without it the editor is unstyled.
                        className={
                            containerClassName
                                ? `${EDITOR_SCOPE_CLASS} ${containerClassName}`
                                : EDITOR_SCOPE_CLASS
                        }
                        style={{
                            // @ts-expect-error CSS custom properties for web
                            '--editor-placeholder-color': placeholderColor,
                            '--editor-primary-color': primaryColor,
                            // The floor, honoured on BOTH platforms as this
                            // option's contract promises. Web used to ignore it
                            // and callers duplicated the number as a
                            // `min-h-[72px]` class — which is a second source of
                            // truth, and silently produces no rule at all if
                            // written as a template literal, since Tailwind
                            // compiles by scanning source text.
                            ...(minHeight == null ? {} : { minHeight }),
                            // The surface's type scale. Spread rather than set
                            // one by one so an absent scale leaves the sheet's
                            // own fallbacks in place — writing "undefined" into
                            // a custom property breaks the cascade.
                            ...editorScaleVars(scale),
                        }}
                    >
                        <EditorContent editor={tiptapEditor} />
                    </View>
                )
            },
        [tiptapEditor, placeholderColor, primaryColor, containerClassName, minHeight, scale]
    )

    return { editor, EditorComponent, commands, toolbarState, isReady: !!live }
}
