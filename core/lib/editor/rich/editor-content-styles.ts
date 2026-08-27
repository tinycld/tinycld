/**
 * The rich editor's content stylesheet, shared by web and the native WebView.
 *
 * ONE string, two deliveries. `webview/source/styles.ts` appends it to the page
 * CSS injected into the WebView's isolated document; `use-rich-editor.web.tsx`
 * injects it into `document.head` with the selectors scoped to the editor's
 * wrapper. Keeping a single source is not tidiness — the caret rules previously
 * lived only in the WebView string, so web shipped no caret CSS at all and the
 * carets rendered as an invisible zero-width span.
 *
 * Selectors target a bare `.ProseMirror`. On native that is safe (isolated
 * document); on web the injector prefixes every occurrence with a wrapper class,
 * because an unscoped sheet in the shared `document.head` leaks onto every other
 * ProseMirror on the page — notably mail's compose body, which uses this same
 * editor. `text` hit exactly that and documents it in `use-document-editor.web.tsx`.
 *
 * Colors arrive as CSS custom properties with literal fallbacks rather than
 * interpolated values, so the string is a constant both platforms can share:
 * native resolves them from the init payload, web from `useThemeColor`.
 */
export const EDITOR_SCOPE_CLASS = 'tinycld-rich-editor'

/**
 * Confine the sheet to one editor by prefixing every `.ProseMirror` that STARTS
 * a selector with the wrapper class.
 *
 * Web-only. On native the page owns its document and the bare selectors are
 * already confined; here the sheet lands in the shared `document.head`, where an
 * unscoped `.ProseMirror` rule would restyle every other editor on the page —
 * mail's compose body most of all, since it uses this same editor.
 *
 * Two cases a naive global replace gets wrong, both of which shipped broken
 * before this was extracted and tested:
 *
 *  - `.ProseMirror .ProseMirror-yjs-selection` — a blanket replace rewrites the
 *    SECOND token too, producing a selector that matches nothing.
 *  - `.ProseMirror ul, .ProseMirror ol` — only the first selector in a comma
 *    list starts a line, so the rest would stay unscoped and leak.
 */
export function scopeEditorStyles(css: string, scopeClass = EDITOR_SCOPE_CLASS): string {
    return css.replace(/(^|,)(\s*)\.ProseMirror\b/gm, `$1$2.${scopeClass} .ProseMirror`)
}

/**
 * React Native Web's default `Text` font stack, as the read view resolves it.
 *
 * Kept beside the sheet that uses it rather than imported, because the sheet is
 * a plain string shared with the native WebView page — which has no access to
 * RN's runtime and must be handed the same list literally.
 */
const RN_WEB_FONT_STACK =
    '-apple-system, "system-ui", "Segoe UI", Roboto, Helvetica, Arial, sans-serif, ' +
    '"Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol"'

export const EDITOR_CONTENT_STYLES = `
.ProseMirror {
    padding: 0;
    caret-color: var(--editor-accent-color, currentColor);
    /* The type scale the whole surface is built on.
     *
     * Every heading below is sized in \`em\`, so this ONE value decides how the
     * editor reads — and the read view that swaps places with it derives its
     * own sizes from the same number (components/help/markdown-purpose.ts).
     * Stating it here is what keeps the two identical rather than merely
     * similar: on web the editor previously set no size at all and inherited
     * the app's 16px body, so tapping to edit grew every line and every
     * heading, and the prose visibly reflowed under the caret.
     *
     * A variable rather than a literal because the surfaces differ: a card
     * description is 14px, a comment 15px. The host sets it per surface and
     * the fallback is the description's base. */
    font-size: var(--editor-base-font-size, 14px);
    line-height: var(--editor-base-line-height, 1.5);
    /* The SAME stack React Native Web resolves its default Text to.
     *
     * Stated rather than inherited: on web this sheet lands in a page whose
     * body font is Tailwind's "ui-sans-serif, system-ui, ...", so the editor
     * rendered in a different face from the markdown it replaces. Same size,
     * same line-height, different glyph widths — which shows up as the prose
     * re-wrapping the instant someone taps to edit (a three-line comment
     * became four), and reads as though the line spacing had changed. */
    font-family: ${RN_WEB_FONT_STACK};
    /* Ligatures back ON, matching the rendered markdown.
     *
     * Something in the page CSS (uniwind/Tailwind's preflight) disables them
     * on the editing surface. Disabled ligatures set text measurably WIDER —
     * the same 174 characters fitted 411px in the read view and only 398px
     * here — so a comment that rendered in three lines re-wrapped to four the
     * moment it was tapped, which reads as the line spacing having changed. */
    font-variant-ligatures: normal;
    font-feature-settings: normal;
}
.ProseMirror:focus {
    outline: none;
}
/* The block rhythm. A variable because the surfaces differ: a description is a
   document and spaces on the editor's own 0.6em, while a comment is a message
   in a thread and spaces far more tightly. The read view derives the same
   number from markdownScale(), which is what keeps the swap from reflowing. */
.ProseMirror p {
    /* Reset the browser's default paragraph margins WITHOUT touching the top
       one, which the block-rhythm rule below owns.
    
       A blanket "margin: 0" here always beat that rule: scoping prefixes both
       selectors with the wrapper class, and this one carries an extra element
       token, so it wins on specificity no matter which is declared first. The
       result was that consecutive paragraphs got no gap at all in the editor
       while the rendered markdown gave them one — a multi-paragraph comment
       re-spaced itself the instant it was tapped. */
    margin-right: 0;
    margin-bottom: 0;
    margin-left: 0;
}
.ProseMirror > * + * {
    margin-top: var(--editor-block-spacing, 0.6em);
}
/* The FIRST block has nothing above it to be spaced from. Stated separately
   because the reset above no longer covers margin-top. */
.ProseMirror > :first-child {
    margin-top: 0;
}
/* Placeholder is a decoration on the first empty node, not real text — it must
   not be selectable or it lands in copied content. */
.ProseMirror .is-editor-empty:first-child::before {
    color: var(--editor-placeholder-color, #9ca3af);
    content: attr(data-placeholder);
    float: left;
    height: 0;
    pointer-events: none;
}
/* ProseMirror appends an empty paragraph after a document that ends in a
   BLOCK node — a list, a table, a code block — so the caret has somewhere to go
   to escape it. It is a caret affordance and nothing else: it is never typed,
   never saved (the stored markdown has no trailing blank), and it is recreated
   on load, so it cannot be removed from the document without taking that escape
   route away.

   Collapsing it is what keeps it from costing height. A comment ending in a
   list opened 25px taller than the text it replaced — one line plus its gap —
   which pushed every comment below it down on focus and back up on blur. The
   node still exists and still accepts the caret; it simply reserves no space
   until it does.

   :focus-within so the escape route reappears the moment someone is actually in
   the editor and might need it. */
.ProseMirror > p:last-child:empty,
.ProseMirror > p.is-empty:last-child:not(:first-child) {
    height: 0;
    margin-top: 0;
    overflow: hidden;
}
.ProseMirror:focus-within > p:last-child:empty,
.ProseMirror:focus-within > p.is-empty:last-child:not(:first-child) {
    height: auto;
    margin-top: var(--editor-block-spacing, 0.6em);
    overflow: visible;
}
/* Heading sizes are variables for the same reason as the rhythm above. A
   description SCALES them (a heading is a document heading); a comment CAPS
   them near body size, because an "# H1" in a chat message is someone reaching
   for emphasis, not declaring a title. The fallbacks are the description's. */
.ProseMirror h1 { font-size: var(--editor-h1-size, 1.6em); font-weight: 700; }
.ProseMirror h2 { font-size: var(--editor-h2-size, 1.35em); font-weight: 700; }
.ProseMirror h3 { font-size: var(--editor-h3-size, 1.15em); font-weight: 600; }
.ProseMirror h4, .ProseMirror h5, .ProseMirror h6 {
    font-size: var(--editor-h4-size, 1em);
    font-weight: 600;
}
.ProseMirror blockquote {
    border-left: 3px solid var(--editor-placeholder-color, #9ca3af);
    padding-left: 1rem;
    margin-left: 0;
}
.ProseMirror a {
    color: var(--editor-primary-color, #2563eb);
    text-decoration: underline;
}
/* Indent in \`em\`, not \`rem\`: a rem is the APP's root size, which has nothing
   to do with this surface's type scale, so a 14px description and a 15px
   comment both indented by the same 24px while their markers scaled with the
   text. The read view derives its indent from the same multiple. */
.ProseMirror ul, .ProseMirror ol {
    padding-left: 1.5em;
    margin: 0;
}
.ProseMirror ul { list-style: disc; }
.ProseMirror ol { list-style: decimal; }
.ProseMirror code {
    font-family: ui-monospace, Menlo, monospace;
    font-size: 0.9em;
}
.ProseMirror pre {
    font-family: ui-monospace, Menlo, monospace;
    padding: 0.6rem 0.8rem;
    border-radius: 6px;
    overflow-x: auto;
}
.ProseMirror pre code {
    font-size: 0.9em;
}
/* A mention is one indivisible thing, so it reads as a chip rather than as
   styled prose — the tint and rounding are what tell you the name is a
   reference to a person and not a word someone typed. Uses the accent already
   carrying links so it inherits the theme in both light and dark, rather than
   a hex that would be right in exactly one of them.

   white-space: nowrap keeps a two-word name from breaking across lines and
   splitting the chip in half. */
.ProseMirror .tinycld-mention {
    /* inline-block, not inline: an inline box does not lay out its own
       background, padding or rounding the way a chip needs — the tint either
       does not paint at all or bleeds past the text — so the pill only exists
       once the node is given a box of its own. */
    display: inline-block;
    white-space: nowrap;
    padding: 0.05em 0.35em;
    border-radius: 999px;
    font-size: 0.95em;
    font-weight: 500;
    /* Text in the accent that already carries links, so it follows the theme
       in both light and dark rather than a hex that is right in only one.

       The fill is a mid-grey at low alpha: it reads as a subtle tint against
       both a light and a dark page, so it needs no theme-specific variant and
       no second CSS variable for the host to keep in step. */
    color: var(--editor-primary-color, #2563eb);
    background-color: rgba(127, 127, 127, 0.16);
}
/* Task lists: the checkbox is interactive, so it needs a real hit area on touch,
   and the marker must not double up with the <ul> bullet. */
.ProseMirror ul[data-type='taskList'] {
    list-style: none;
    padding-left: 0;
}
.ProseMirror ul[data-type='taskList'] li {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
}
.ProseMirror ul[data-type='taskList'] li > label {
    flex: 0 0 auto;
    margin-top: 0.15em;
}
.ProseMirror ul[data-type='taskList'] li > div {
    flex: 1 1 auto;
    min-width: 0;
}
.ProseMirror table {
    border-collapse: collapse;
    width: 100%;
    table-layout: fixed;
}
.ProseMirror td, .ProseMirror th {
    border: 1px solid var(--editor-placeholder-color, #9ca3af);
    padding: 4px 8px;
    vertical-align: top;
}
.ProseMirror th {
    font-weight: 600;
}
.ProseMirror img {
    max-width: 100%;
    height: auto;
}
.ProseMirror hr {
    border: none;
    border-top: 1px solid var(--editor-placeholder-color, #9ca3af);
}

/* ── Remote collaborator carets (CollaborationCaret v3) ────────────────────
   @tiptap/extension-collaboration-caret v3 renamed the v2 classes to the
   PLURAL "carets" form. This file previously carried the singular v2 names,
   so the rules matched nothing and every caret rendered invisibly. The
   class-name parity test pins these against the installed package.

   The extension injects:

     <span class="collaboration-carets__caret" style="border-color: $color">
         <div class="collaboration-carets__label" style="background-color: $color">
             $userName
         </div>
     </span>

   Only border-COLOR is set inline, so without a border-style here the caret
   resolves to nothing visible; and the label, a block-level child of an inline
   span, balloons to fill the line unless positioned. Both footguns are why
   these rules exist rather than relying on the extension's defaults.

   A peer's non-empty SELECTION uses a different naming scheme entirely — see
   the ProseMirror-yjs-selection rule at the end. */
.ProseMirror .collaboration-carets__caret {
    position: relative;
    display: inline-block;
    width: 0;
    /* An EMPTY inline-block with no height collapses to a zero-area box, and a
       border on a zero-height box draws nothing at all. The extension gives this
       span no text of its own, and the label below is absolutely positioned so
       it contributes no height either — so the line must be sized HERE or the
       caret is invisible however correct the border is. 1em tracks the
       surrounding font size; vertical-align keeps it on the text baseline
       instead of hanging below it. */
    height: 1em;
    vertical-align: text-bottom;
    margin-left: -1px;
    border-left-style: solid;
    border-left-width: 2px;
    pointer-events: none;
    word-break: normal;
    box-sizing: content-box;
    /* Zero letter-spacing keeps the 2px line from picking up word-break
       adjustments inside justified paragraphs. */
    letter-spacing: 0;
}

/* The label rides ABOVE the caret as a small pill, clear of the line of text,
   so the caret line itself stays visible end to end. bottom:100% anchors it to
   the caret span's top edge; left:-2px compensates for the caret border so
   label and caret share a vertical edge. Background color is set inline by the
   extension to the user's color. */
.ProseMirror .collaboration-carets__label {
    position: absolute;
    bottom: 100%;
    left: -2px;
    margin-bottom: 4px;
    z-index: 20;
    padding: 1px 6px 2px;
    border-radius: 4px 4px 4px 0;
    font: 600 11px/1.3 -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    letter-spacing: 0.01em;
    color: #fff;
    white-space: nowrap;
    user-select: none;
    pointer-events: none;
    box-shadow:
        0 1px 2px rgba(0, 0, 0, 0.12),
        0 2px 6px rgba(0, 0, 0, 0.08);
}

/* A remote peer's non-empty selection. Note the class here is NOT one of the
   collaboration-carets__* pair — y-tiptap's defaultSelectionBuilder emits
   ProseMirror-yjs-selection with the background color inline, so this rule only
   has to keep the highlight from eating pointer events. */
.ProseMirror .ProseMirror-yjs-selection {
    pointer-events: none;
}
`
