import { EDITOR_CONTENT_STYLES } from '../../editor-content-styles'
import type { RichEditorColors } from './protocol'

/**
 * The WebView page's CSS: the shared content stylesheet, plus the html/body
 * rules only a page that owns its own document needs.
 *
 * Relocated from use-rich-editor.native.tsx, where it was handed to TenTap via
 * `CoreBridge.configureCSS`. Now that we own the page it is injected directly,
 * which also lets it cover nodes TenTap's bridges never styled (task lists,
 * code blocks).
 *
 * The content rules themselves live in `editor-content-styles.ts` and are shared
 * with web — they used to live here, which is how web ended up shipping no caret
 * CSS at all. Colors arrive resolved rather than as CSS custom properties (the
 * native side reads them from the theme hook and the WebView has no access to
 * that), so they are mapped onto the custom properties the shared sheet reads.
 */
export function buildEditorCSS(colors: RichEditorColors): string {
    const accent = colors.accent ?? colors.primary
    return `
        :root {
            --editor-placeholder-color: ${colors.placeholder};
            --editor-primary-color: ${colors.primary};
            --editor-accent-color: ${accent};
        }
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
        /* WebView only: the page fills the host-measured height so a tap
           anywhere below the last line still lands in the editor. On web the
           surrounding layout owns the height, and this rule would fight the
           measurement — see core/lib/editor/height-store.ts. */
        .ProseMirror {
            min-height: 100%;
        }
${EDITOR_CONTENT_STYLES}
    `
}
