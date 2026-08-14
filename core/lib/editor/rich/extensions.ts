import type { Extensions, NodeViewRenderer } from '@tiptap/core'
import Collaboration from '@tiptap/extension-collaboration'
import CollaborationCaret from '@tiptap/extension-collaboration-caret'
import { Image } from '@tiptap/extension-image'
import { TaskItem, TaskList } from '@tiptap/extension-list'
import Placeholder from '@tiptap/extension-placeholder'
import { Table, TableCell, TableHeader, TableRow } from '@tiptap/extension-table'
import { CharacterCount } from '@tiptap/extensions'
import { Markdown } from '@tiptap/markdown'
import StarterKit from '@tiptap/starter-kit'
import type { Awareness } from 'y-protocols/awareness'
import type * as Y from 'yjs'
import { MentionNode, setMentionLabels } from './mention-node'
import { SubmitShortcut } from './submit-shortcut'
import { createTriggerExtension, type TriggerConfig } from './triggers'

export interface RichEditorCollabOptions {
    /** The Y.Doc to bind to. Never construct one per editor — pass the room's. */
    document: Y.Doc
    /** Which top-level fragment of that doc this editor owns, e.g. `card:<id>`. */
    field: string
    /** Room awareness. Supplying it (with `user`) turns on collaborator carets. */
    awareness?: Awareness
    /**
     * Caret identity. Must be the same {id,name,color} shape presence publishes:
     * CollaborationCaret overwrites `awareness.user` on mount, so a different
     * shape here makes the local user vanish from every avatar row.
     */
    user?: { id: string; name: string; color: string }
}

export interface BuildRichEditorExtensionsOptions {
    placeholder?: string
    characterLimit?: number
    onSubmitShortcut?: () => void
    collab?: RichEditorCollabOptions
    /**
     * Custom renderer for image nodes. An option rather than baked in because
     * the renderer is environment-specific (React node view on web, another
     * inside the WebView page) and this builder must stay importable from both.
     */
    imageNodeView?: NodeViewRenderer
    /**
     * Character-triggered autocompletes (`@` mentions, and whatever wants the
     * same shape later). Built here rather than added by the caller so BOTH
     * platforms get them from the one schema — the native editor is a prebuilt
     * WebView bundle, so a plugin a package adds at runtime would exist on web
     * and silently not on native. See triggers.ts.
     */
    triggers?: TriggerConfig[]
}

/**
 * The single source of the editor schema, shared by the web hook and the
 * WebView bundle.
 *
 * Sharing one builder is not tidiness — text/ keeps two hand-maintained
 * extension lists and needs a test to diff them, because a node present on one
 * platform and absent on the other silently drops attributes when a document
 * crosses between them.
 *
 * Every node type here is load-bearing for data preservation, verified by
 * round-tripping real content: with a bare StarterKit, task lists lose their
 * checkboxes, GFM tables vanish entirely, and images degrade to their alt text.
 * Table and image editing UI is deliberately absent for now — the schema must
 * still carry them so existing descriptions survive being opened.
 */
export function buildRichEditorExtensions(
    options: BuildRichEditorExtensionsOptions = {}
): Extensions {
    const { placeholder, characterLimit, onSubmitShortcut, collab, imageNodeView, triggers } =
        options

    const extensions: Extensions = [
        StarterKit.configure({
            link: { openOnClick: false },
            // Yjs owns history under collaboration; StarterKit's would undo
            // other people's edits.
            undoRedo: collab ? false : undefined,
        }),
        TaskList,
        TaskItem.configure({ nested: true }),
        Table,
        TableRow,
        TableHeader,
        TableCell,
        imageNodeView ? Image.extend({ addNodeView: () => imageNodeView }) : Image,
        Placeholder.configure({ placeholder: placeholder ?? '' }),
        Markdown.configure({ markedOptions: { gfm: true } }),
    ]

    if (typeof characterLimit === 'number') {
        extensions.push(CharacterCount.configure({ limit: characterLimit }))
    }
    if (onSubmitShortcut) {
        extensions.push(SubmitShortcut.configure({ onSubmit: onSubmitShortcut }))
    }
    for (const trigger of triggers ?? []) {
        extensions.push(createTriggerExtension(trigger))
        // One node instance per mention trigger, bound to that trigger's
        // roster, so two triggers on one editor cannot resolve each other's ids.
        if (trigger.insertsMentionNode) {
            extensions.push(MentionNode.configure({ triggerId: trigger.id }))
            setMentionLabels(trigger.id, trigger.allItems)
        }
    }
    if (collab) {
        extensions.push(Collaboration.configure({ document: collab.document, field: collab.field }))
        if (collab.awareness && collab.user) {
            extensions.push(
                CollaborationCaret.configure({
                    provider: { awareness: collab.awareness },
                    user: collab.user,
                })
            )
        }
    }
    return extensions
}
