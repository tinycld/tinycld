import { generateHTML, generateJSON } from '@tiptap/core'
import { MarkdownManager } from '@tiptap/markdown'
import { buildRichEditorExtensions } from './extensions'
import { repairMarkdown } from './markdown-repair'

/**
 * HTML ↔ Markdown conversion without mounting an editor.
 *
 * The native editor speaks HTML across TenTap's bridge, but cards store
 * markdown, so the two formats have to meet somewhere. Doing it here — on the
 * React Native side, through the same extension set the web editor uses —
 * keeps a single schema. The alternative, converting inside the WebView, would
 * mean a second bundle carrying its own copy of the schema, which is precisely
 * how text/ ended up with two extension lists that must be diffed by a test.
 *
 * `marked` (the parser behind `@tiptap/markdown`) is already proven on Hermes:
 * `react-native-marked` ships it in this app for help topics. The Hermes hazard
 * documented elsewhere in the repo is specific to markdown-it's ESM build.
 */

// One manager and one extension set for the process. Building either per call
// would re-register every extension on what can be a per-keystroke path.
let manager: MarkdownManager | null = null
const extensions = buildRichEditorExtensions()

function markdownManager(): MarkdownManager {
    if (!manager) {
        manager = new MarkdownManager({ extensions })
    }
    return manager
}

/** Parse markdown into the HTML the WebView editor expects. */
export function markdownToHTML(markdown: string): string {
    const json = markdownManager().parse(markdown)
    return generateHTML(json, extensions)
}

/**
 * Serialize editor HTML to markdown.
 *
 * Applies the same repair pass as the web variant, so both platforms produce
 * the same spelling of a document — otherwise switching devices would rewrite
 * the row.
 */
export function htmlToMarkdown(html: string): string {
    const json = generateJSON(html, extensions)
    return repairMarkdown(markdownManager().serialize(json))
}
