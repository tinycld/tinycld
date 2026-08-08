import type { Extensions } from '@tiptap/core'
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
import { SubmitShortcut } from './submit-shortcut'

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
    const { placeholder, characterLimit, onSubmitShortcut, collab } = options

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
        Image,
        Placeholder.configure({ placeholder: placeholder ?? '' }),
        Markdown.configure({ markedOptions: { gfm: true } }),
    ]

    if (typeof characterLimit === 'number') {
        extensions.push(CharacterCount.configure({ limit: characterLimit }))
    }
    if (onSubmitShortcut) {
        extensions.push(SubmitShortcut.configure({ onSubmit: onSubmitShortcut }))
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
