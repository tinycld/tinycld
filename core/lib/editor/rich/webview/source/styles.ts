import { EDITOR_CONTENT_STYLES } from '../../editor-content-styles'
import type { EditorTypeScale } from '../../options'
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
export function buildEditorCSS(colors: RichEditorColors, scale?: EditorTypeScale): string {
    const accent = colors.accent ?? colors.primary
    // The page's own defaults, kept as the fallback so a host that says nothing
    // renders exactly as it always has (the description's proportions).
    const baseFontSize = scale?.bodySize ?? 14
    const baseLineHeight = scale ? scale.bodyLineHeight / scale.bodySize : 1.5
    return `
        :root {
            --editor-placeholder-color: ${colors.placeholder};
            --editor-primary-color: ${colors.primary};
            --editor-accent-color: ${accent};
            /* The scale the shared sheet reads. Set here as well as on body so
               the .ProseMirror rules resolve it. Headings and block spacing are
               included because the surfaces genuinely differ in more than size:
               a comment caps its headings and spaces tightly. */
            --editor-base-font-size: ${baseFontSize}px;
            --editor-base-line-height: ${baseLineHeight};
            ${scale ? `--editor-block-spacing: ${scale.blockSpacing}px;` : ''}
            ${scale ? `--editor-h1-size: ${scale.h1}px;` : ''}
            ${scale ? `--editor-h2-size: ${scale.h2}px;` : ''}
            ${scale ? `--editor-h3-size: ${scale.h3}px;` : ''}
            ${scale ? `--editor-h4-size: ${scale.h4}px;` : ''}
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
            /* The shared sheet states the same stack on .ProseMirror — see
               RN_WEB_FONT_STACK there. Repeated on body so chrome outside the
               editing surface (the placeholder) matches too. */
            font-family: -apple-system, "system-ui", 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;
            font-size: ${baseFontSize}px;
            line-height: ${baseLineHeight};
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
