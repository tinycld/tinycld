import { type MarkdownPurpose, markdownScale } from '../../../components/help/markdown-purpose'
import type { EditorTypeScale } from './options'

/**
 * The editor's type scale for a surface, derived from the SAME entry the read
 * view renders at.
 *
 * The editor and the rendered markdown swap places on a tap, so every number
 * they disagree on is prose that moves under the reader's caret at the moment
 * they start editing. Deriving both from `markdownScale(purpose)` is what makes
 * that impossible to get wrong — the alternative, restating the values in the
 * stylesheet, is exactly how a comment ended up editing at a description's
 * proportions: headings half again too large and blocks spaced twice as far
 * apart as the comment they replaced.
 *
 * Returned in px rather than em because the caller writes them into CSS custom
 * properties, and an em there would compound against the base this same object
 * sets.
 */
export function editorScaleFor(purpose: MarkdownPurpose): EditorTypeScale {
    const scale = markdownScale(purpose)
    return {
        bodySize: scale.bodySize,
        bodyLineHeight: scale.bodyLineHeight,
        // The renderer expresses the gap as a top margin on the following block
        // (RN margins do not collapse); `.ProseMirror > * + *` is the same rule
        // said in CSS, so the number carries over unchanged.
        blockSpacing: scale.paragraphSpacing,
        h1: scale.h1.size,
        h2: scale.h2.size,
        h3: scale.h3.size,
        h4: scale.h4.size,
    }
}

/**
 * The scale as CSS custom properties for the web wrapper.
 *
 * Returns nothing when no scale was given, so the stylesheet's own fallbacks
 * (the description's values) stay in force — writing `undefined` into a custom
 * property breaks the cascade rather than falling back.
 */
export function editorScaleVars(scale?: EditorTypeScale): Record<string, string> {
    if (!scale) return {}
    return {
        '--editor-base-font-size': `${scale.bodySize}px`,
        // Unitless, so it inherits proportionally the way a line-height should.
        '--editor-base-line-height': `${scale.bodyLineHeight / scale.bodySize}`,
        '--editor-block-spacing': `${scale.blockSpacing}px`,
        '--editor-h1-size': `${scale.h1}px`,
        '--editor-h2-size': `${scale.h2}px`,
        '--editor-h3-size': `${scale.h3}px`,
        '--editor-h4-size': `${scale.h4}px`,
    }
}
