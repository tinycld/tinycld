import type { RichEditorColors } from './protocol'

/**
 * Editor CSS, themed from the init payload.
 *
 * Relocated from use-rich-editor.native.tsx, where it was handed to TenTap via
 * `CoreBridge.configureCSS`. Now that we own the page it is injected directly,
 * which also lets it cover nodes TenTap's bridges never styled (task lists,
 * code blocks).
 *
 * Colors arrive resolved rather than as CSS custom properties: the native side
 * reads them from the theme hook, and the WebView has no access to that.
 */
export function buildEditorCSS(colors: RichEditorColors): string {
    const accent = colors.accent ?? colors.primary
    return `
        * {
            -webkit-tap-highlight-color: transparent;
            box-sizing: border-box;
        }
        html, body {
            margin: 0;
            padding: 0;
            background-color: ${colors.bg};
            color: ${colors.fg};
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;
            font-size: 14px;
            line-height: 1.5;
            /* The host sizes the WebView; the page must never scroll itself. */
            overflow-wrap: break-word;
        }
        .ProseMirror {
            padding: 0;
            min-height: 100%;
            caret-color: ${accent};
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
        /* Placeholder is a decoration on the first empty node, not real text —
           it must not be selectable or it lands in copied content. */
        .ProseMirror .is-editor-empty:first-child::before {
            color: ${colors.placeholder};
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
            border-left: 3px solid ${colors.placeholder};
            padding-left: 1rem;
            margin-left: 0;
        }
        .ProseMirror a {
            color: ${colors.primary};
            text-decoration: underline;
        }
        .ProseMirror ul, .ProseMirror ol {
            padding-left: 1.5rem;
            margin: 0;
        }
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
        /* Task lists: the checkbox is interactive, so it needs a real hit area
           on touch, and the marker must not double up with the <ul> bullet. */
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
            border: 1px solid ${colors.placeholder};
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
            border-top: 1px solid ${colors.placeholder};
        }
        /* Remote collaborator carets. Unused until native collaboration lands,
           but the styles ship now so enabling it needs no CSS change. */
        .collaboration-caret__caret {
            border-left: 1px solid;
            border-right: 1px solid;
            margin-left: -1px;
            margin-right: -1px;
            pointer-events: none;
            position: relative;
            word-break: normal;
        }
        .collaboration-caret__label {
            border-radius: 3px 3px 3px 0;
            color: ${colors.bg};
            font-size: 11px;
            font-style: normal;
            font-weight: 600;
            left: -1px;
            line-height: normal;
            padding: 0.1rem 0.3rem;
            position: absolute;
            top: -1.4em;
            user-select: none;
            white-space: nowrap;
        }
    `
}
