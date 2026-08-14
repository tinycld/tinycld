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

export const EDITOR_CONTENT_STYLES = `
.ProseMirror {
    padding: 0;
    caret-color: var(--editor-accent-color, currentColor);
}
.ProseMirror:focus {
    outline: none;
}
.ProseMirror > * + * {
    margin-top: 0.6em;
}
.ProseMirror p {
    margin: 0;
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
.ProseMirror h1 { font-size: 1.6em; font-weight: 700; }
.ProseMirror h2 { font-size: 1.35em; font-weight: 700; }
.ProseMirror h3 { font-size: 1.15em; font-weight: 600; }
.ProseMirror h4, .ProseMirror h5, .ProseMirror h6 {
    font-size: 1em;
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
.ProseMirror ul, .ProseMirror ol {
    padding-left: 1.5rem;
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
