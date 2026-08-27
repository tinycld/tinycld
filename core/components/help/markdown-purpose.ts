// Typographic scale per rendering surface.
//
// One markdown renderer serves three surfaces with genuinely different needs,
// and styling them all as documentation is what made a five-line card comment
// fill a screen: a help topic's h1 is a page title, a comment's is someone
// shouting one word in a chat message.
//
//   documentation — help topics. The original values; nothing about the help
//                   hub changes because this is the default.
//   description   — a card description in its READ state. Must match the
//                   editor that replaces it on tap, to the pixel.
//   compact       — a card comment. A message in a thread, not a document.

/** Which surface is being rendered. */
export type MarkdownPurpose = 'documentation' | 'description' | 'compact'

/**
 * The values a purpose contributes. Everything else — colors, code spans,
 * tables — is shared, because those do not change with the surface.
 */
export interface MarkdownScale {
    bodySize: number
    bodyLineHeight: number
    paragraphSpacing: number
    h1: HeadingScale
    h2: HeadingScale
    h3: HeadingScale
    h4: HeadingScale
    h5: HeadingScale
    h6: HeadingScale
    /** A rule under h1/h2 suits a long document and nothing shorter. */
    headingRule: boolean
    listSpacing: number
    /**
     * How far a list's text sits from the margin, in px.
     *
     * Ported from `.ProseMirror ul, .ProseMirror ol { padding-left: 1.5em }`.
     * The renderer indents nothing by default, so a rendered bullet list hung
     * at the margin while the editor's sat 1.5em in — and the markers landed on
     * a different x the moment anyone tapped to edit.
     */
    listIndent: number
}

interface HeadingScale {
    size: number
    weight: '400' | '500' | '600' | '700'
    marginTop: number
    marginBottom: number
}

/**
 * The editor's own base font size, from the WebView page's `body` rule
 * (rich/webview/source/styles.ts). Every heading below is that base times the
 * `em` multiplier in editor-content-styles.ts, which is what makes the read
 * view and the editor the same size rather than merely similar.
 */
const EDITOR_BASE_PX = 14

/** `.ProseMirror > * + * { margin-top: 0.6em }` — the editor's block rhythm. */
const EDITOR_BLOCK_SPACING = Math.round(EDITOR_BASE_PX * 0.6)

/**
 * `.ProseMirror ul, .ProseMirror ol { padding-left: 1.5em }` — the editor's
 * list indent, as a multiple of the surface's own base size.
 */
export const EDITOR_LIST_INDENT_EM = 1.5

/**
 * Ported from `.ProseMirror h1/h2/h3` in editor-content-styles.ts.
 *
 * The editor spaces blocks with a single `* + *` rule rather than per-element
 * margins, so a heading gets the same gap as a paragraph — hence the uniform
 * marginTop and the zero marginBottom. Giving headings their own larger margins
 * here would reflow the text the instant someone tapped to edit, which is the
 * one thing this scale exists to prevent.
 *
 * CSS margins collapse and React Native's do not, so these are expressed as a
 * top margin only. Setting both would double every gap.
 */
const DESCRIPTION_SCALE: MarkdownScale = {
    bodySize: EDITOR_BASE_PX,
    // 1.5 line-height, matching the page's `body` rule.
    bodyLineHeight: Math.round(EDITOR_BASE_PX * 1.5),
    paragraphSpacing: EDITOR_BLOCK_SPACING,
    h1: {
        size: Math.round(EDITOR_BASE_PX * 1.6),
        weight: '700',
        marginTop: EDITOR_BLOCK_SPACING,
        marginBottom: 0,
    },
    h2: {
        size: Math.round(EDITOR_BASE_PX * 1.35),
        weight: '700',
        marginTop: EDITOR_BLOCK_SPACING,
        marginBottom: 0,
    },
    h3: {
        size: Math.round(EDITOR_BASE_PX * 1.15),
        weight: '600',
        marginTop: EDITOR_BLOCK_SPACING,
        marginBottom: 0,
    },
    // h4-h6 are all `font-size: 1em; font-weight: 600` in the editor.
    h4: { size: EDITOR_BASE_PX, weight: '600', marginTop: EDITOR_BLOCK_SPACING, marginBottom: 0 },
    h5: { size: EDITOR_BASE_PX, weight: '600', marginTop: EDITOR_BLOCK_SPACING, marginBottom: 0 },
    h6: { size: EDITOR_BASE_PX, weight: '600', marginTop: EDITOR_BLOCK_SPACING, marginBottom: 0 },
    // The editor draws no rule under a heading, so neither may this.
    headingRule: false,
    listSpacing: EDITOR_BLOCK_SPACING,
    listIndent: Math.round(EDITOR_BASE_PX * EDITOR_LIST_INDENT_EM),
}

/**
 * Help topics: the original values, unchanged.
 *
 * A topic is a page someone reads top to bottom, so headings carry real
 * hierarchy and the rule under h1/h2 helps rather than intrudes.
 */
const DOCUMENTATION_SCALE: MarkdownScale = {
    bodySize: 15,
    bodyLineHeight: 22,
    paragraphSpacing: 6,
    h1: { size: 24, weight: '700', marginTop: 16, marginBottom: 8 },
    h2: { size: 20, weight: '600', marginTop: 16, marginBottom: 6 },
    h3: { size: 17, weight: '600', marginTop: 12, marginBottom: 4 },
    h4: { size: 15, weight: '600', marginTop: 10, marginBottom: 4 },
    h5: { size: 14, weight: '600', marginTop: 8, marginBottom: 2 },
    h6: { size: 13, weight: '600', marginTop: 8, marginBottom: 2 },
    headingRule: true,
    listSpacing: 6,
    listIndent: Math.round(15 * EDITOR_LIST_INDENT_EM),
}

/**
 * Comments: a message in a thread.
 *
 * Headings are CAPPED rather than scaled — an `# H1` in a chat message is
 * someone reaching for emphasis, not declaring a document title, so the
 * loudest it gets is a little above body text. Spacing is tight for the same
 * reason: five short lines should read as five short lines, not fill a screen.
 *
 * Top margins only, as in DESCRIPTION_SCALE: RN margins do not collapse, and a
 * bottom margin here would be ADDED to the next block's top one. The 2px bottom
 * these headings used to carry is what made a rendered comment space nearly
 * twice as far apart as the editor that replaces it.
 */
const COMPACT_SCALE: MarkdownScale = {
    bodySize: 15,
    bodyLineHeight: 21,
    paragraphSpacing: 4,
    // marginTop is COMPACT_BLOCK_SPACING for every level, because the editor
    // spaces all blocks with one `* + *` rule and a heading is not special to
    // it. Giving headings a larger gap here made a rendered comment's headings
    // sit twice as far from the text above them as the editor's did.
    h1: { size: 17, weight: '700', marginTop: 4, marginBottom: 0 },
    h2: { size: 16, weight: '700', marginTop: 4, marginBottom: 0 },
    h3: { size: 15, weight: '600', marginTop: 4, marginBottom: 0 },
    h4: { size: 15, weight: '600', marginTop: 4, marginBottom: 0 },
    h5: { size: 15, weight: '600', marginTop: 4, marginBottom: 0 },
    h6: { size: 15, weight: '600', marginTop: 4, marginBottom: 0 },
    headingRule: false,
    listSpacing: 4,
    listIndent: Math.round(15 * EDITOR_LIST_INDENT_EM),
}

const SCALES: Record<MarkdownPurpose, MarkdownScale> = {
    documentation: DOCUMENTATION_SCALE,
    description: DESCRIPTION_SCALE,
    compact: COMPACT_SCALE,
}

export function markdownScale(purpose: MarkdownPurpose = 'documentation'): MarkdownScale {
    return SCALES[purpose]
}

/** Exported for the parity test that keeps `description` matching the editor. */
export const EDITOR_SCALE_SOURCE = {
    basePx: EDITOR_BASE_PX,
    blockSpacing: EDITOR_BLOCK_SPACING,
}
