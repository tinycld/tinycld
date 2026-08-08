import type { Editor } from '@tiptap/core'
import type { RichEditorStatePayload } from './protocol'

/**
 * Derive the toolbar/state snapshot the WebView broadcasts to the host.
 *
 * Pure and separated from the component so it can be tested against a real
 * Tiptap editor under happy-dom — the native editor path has no other
 * mechanical coverage available.
 */
export function deriveWebViewState(editor: Editor): RichEditorStatePayload {
    const { from, to } = editor.state.selection
    const headingLevel = [1, 2, 3, 4, 5, 6].find(level => editor.isActive('heading', { level }))

    // CharacterCount is only registered when a limit was configured, so its
    // storage may be absent. Fall back to the document's plain text.
    const characterCountStorage = editor.storage.characterCount as
        | { characters?: () => number; words?: () => number }
        | undefined
    const text = editor.getText()

    return {
        isBoldActive: editor.isActive('bold'),
        isItalicActive: editor.isActive('italic'),
        isUnderlineActive: editor.isActive('underline'),
        isStrikeActive: editor.isActive('strike'),
        isBulletListActive: editor.isActive('bulletList'),
        isOrderedListActive: editor.isActive('orderedList'),
        isTaskListActive: editor.isActive('taskList'),
        isBlockquoteActive: editor.isActive('blockquote'),
        isCodeActive: editor.isActive('code'),
        isCodeBlockActive: editor.isActive('codeBlock'),
        isLinkActive: editor.isActive('link'),
        activeLink: (editor.getAttributes('link').href as string | undefined) ?? null,
        activeHeadingLevel: headingLevel ?? null,
        isInTable: editor.isActive('table'),
        selectionEmpty: from === to,
        isEmpty: editor.isEmpty,
        characterCount: characterCountStorage?.characters?.() ?? text.length,
        wordCount: characterCountStorage?.words?.() ?? countWords(text),
        canUndo: editor.can().undo(),
        canRedo: editor.can().redo(),
        isReady: true,
    }
}

function countWords(text: string): number {
    const trimmed = text.trim()
    return trimmed === '' ? 0 : trimmed.split(/\s+/).length
}
