import { describe, expect, it } from 'vitest'
import { EDITOR_CONTENT_STYLES } from '../../../lib/editor/rich/editor-content-styles'
import { editorScaleFor } from '../../../lib/editor/rich/editor-scale'
import { markdownScale } from '../markdown-purpose'

// The `description` scale exists to make the READ state of a card description
// pixel-identical to the editor that replaces it on tap. Two files, one
// appearance — exactly the shape that drifts silently, so the editor's CSS is
// parsed here and compared rather than trusted to stay in step. (The two
// resolvePopoverPosition copies drifted this way and shipped the same bug
// twice.)
//
// If this fails after an intentional editor restyle, port the new value into
// markdown-purpose.ts. Do not relax the assertion: a mismatch means tapping a
// description visibly reflows the text under the reader's finger.

/** The editor page's own base, from rich/webview/source/styles.ts. */
const EDITOR_BASE_PX = 14

/** Pull `font-size: <n>em` for a selector out of the editor stylesheet. */
/**
 * The `em` a heading falls back to when a surface states no scale of its own.
 *
 * Sizes are CSS variables now, because the surfaces genuinely differ: a comment
 * caps its headings near body text while a description scales them like a
 * document. The fallback in each `var()` is the description's value, which is
 * what these tests pin.
 */
function editorEm(selector: string): number {
    const rule = new RegExp(`\\.ProseMirror ${selector}[^{]*\\{([^}]*)\\}`)
    const body = EDITOR_CONTENT_STYLES.match(rule)?.[1]
    if (!body) throw new Error(`no rule for ${selector} in the editor stylesheet`)
    const size = body.match(/font-size:\s*var\([^,]+,\s*([\d.]+)em\)/)?.[1]
    if (!size) throw new Error(`no em fallback for ${selector}`)
    return Number.parseFloat(size)
}

describe('the description scale matches the editor', () => {
    const scale = markdownScale('description')

    it('uses the editor page’s base font size for body text', () => {
        expect(scale.bodySize).toBe(EDITOR_BASE_PX)
    })

    // The editor previously stated no size of its own on web, so `.ProseMirror`
    // inherited the app's 16px body while the read view rendered at 14 — every
    // line and every `em`-sized heading grew the moment anyone tapped to edit.
    // The declaration below is what closed that; the fallback in it is the
    // description's base, so a surface that passes no scale still matches.
    it('states a base font size rather than inheriting the page', () => {
        const declared = EDITOR_CONTENT_STYLES.match(
            /\.ProseMirror\s*\{[^}]*font-size:\s*var\(--editor-base-font-size,\s*(\d+)px\)/
        )
        expect(declared, 'the editor stylesheet states no base font size').not.toBeNull()
        expect(Number(declared?.[1])).toBe(EDITOR_BASE_PX)
    })

    // A list's indent is `em` in the editor, so it tracks that surface's own
    // base. The renderer indents nothing by default, so without porting this
    // the rendered bullets hung at the margin while the editor's sat 1.5em in.
    it('indents lists exactly as the editor does', () => {
        const indent = EDITOR_CONTENT_STYLES.match(
            /\.ProseMirror ul, \.ProseMirror ol\s*\{[^}]*padding-left:\s*([\d.]+)em/
        )
        expect(indent, 'the editor stylesheet states no list indent').not.toBeNull()
        expect(scale.listIndent).toBe(
            Math.round(EDITOR_BASE_PX * Number.parseFloat(indent?.[1] ?? '0'))
        )
    })

    it.each([
        ['h1', 'h1'],
        ['h2', 'h2'],
        ['h3', 'h3'],
    ] as const)('sizes %s exactly as the editor does', (key, selector) => {
        const expected = Math.round(EDITOR_BASE_PX * editorEm(selector))
        expect(scale[key].size).toBe(expected)
    })

    // `.ProseMirror > * + * { margin-top: 0.6em }` — one rule for every block,
    // so a heading gets the same gap as a paragraph. Per-heading margins here
    // would reflow the prose the moment someone tapped to edit.
    it('spaces blocks on the editor’s single rhythm', () => {
        const emMatch = EDITOR_CONTENT_STYLES.match(
            /\.ProseMirror > \* \+ \*\s*\{[^}]*margin-top:\s*var\([^,]+,\s*([\d.]+)em\)/
        )
        expect(emMatch).not.toBeNull()
        const expected = Math.round(EDITOR_BASE_PX * Number.parseFloat(emMatch?.[1] ?? '0'))
        expect(scale.paragraphSpacing).toBe(expected)
        expect(scale.h1.marginTop).toBe(expected)
        expect(scale.h2.marginTop).toBe(expected)
        expect(scale.h3.marginTop).toBe(expected)
    })

    // CSS margins collapse; React Native's do not. Setting both would double
    // every gap relative to the editor.
    it('sets a top margin only, because RN margins do not collapse', () => {
        expect(scale.h1.marginBottom).toBe(0)
        expect(scale.h2.marginBottom).toBe(0)
        expect(scale.h3.marginBottom).toBe(0)
    })

    it('draws no rule under a heading, because the editor draws none', () => {
        expect(scale.headingRule).toBe(false)
        expect(EDITOR_CONTENT_STYLES).not.toMatch(/\.ProseMirror h[12][^{]*\{[^}]*border-bottom/)
    })
})

// The gap that let a comment edit at a description's proportions: the parity
// suite only ever checked `description`, so nothing noticed that the editor
// applied one scale to every surface. `editorScaleFor` is now the single source
// both sides read, and these pin it per purpose.
describe('the editor scale is the read scale, for every surface', () => {
    it.each(['description', 'compact'] as const)('matches the %s read scale', purpose => {
        const read = markdownScale(purpose)
        const editor = editorScaleFor(purpose)

        expect(editor.bodySize).toBe(read.bodySize)
        expect(editor.bodyLineHeight).toBe(read.bodyLineHeight)
        expect(editor.blockSpacing).toBe(read.paragraphSpacing)
        expect(editor.h1).toBe(read.h1.size)
        expect(editor.h2).toBe(read.h2.size)
        expect(editor.h3).toBe(read.h3.size)
        expect(editor.h4).toBe(read.h4.size)
    })

    // A comment is a message in a thread: its headings stay near body text and
    // its rhythm is tight. A description is a document. If these ever converged
    // the compact scale would have stopped meaning anything.
    it('keeps a comment visibly tighter than a description', () => {
        const comment = editorScaleFor('compact')
        const description = editorScaleFor('description')
        expect(comment.h1).toBeLessThan(description.h1)
        expect(comment.blockSpacing).toBeLessThan(description.blockSpacing)
    })
})

// RN margins do not collapse, so a bottom margin is ADDED to the next block's
// top one. Every scale with an editor behind it must therefore state the gap
// once, as a top margin — the editor says it once too, as `* + *`. Getting this
// wrong is invisible in isolation and doubles the rhythm of the rendered view.
describe('scales with an editor state their gaps once', () => {
    it.each(['description', 'compact'] as const)('%s sets no bottom margins', purpose => {
        const scale = markdownScale(purpose)
        expect(scale.h1.marginBottom).toBe(0)
        expect(scale.h2.marginBottom).toBe(0)
        expect(scale.h3.marginBottom).toBe(0)
        expect(scale.h4.marginBottom).toBe(0)
        expect(scale.h5.marginBottom).toBe(0)
        expect(scale.h6.marginBottom).toBe(0)
    })
})

// The editor spaces every block with ONE rule — `* + * { margin-top }` — so a
// heading gets exactly the same gap as a paragraph. A scale that gives headings
// their own larger margin renders looser than the editor it swaps with, which
// is what made a comment's headings sit twice as far from the text above them.
describe('scales with an editor space every block alike', () => {
    it.each(['description', 'compact'] as const)('%s spaces headings as blocks', purpose => {
        const scale = markdownScale(purpose)
        for (const key of ['h1', 'h2', 'h3', 'h4', 'h5', 'h6'] as const) {
            expect(scale[key].marginTop).toBe(scale.paragraphSpacing)
        }
    })
})

describe('the other purposes', () => {
    // The help hub must be untouched by this refactor.
    it('leaves documentation on its original values', () => {
        const doc = markdownScale('documentation')
        expect(doc.bodySize).toBe(15)
        expect(doc.h1.size).toBe(24)
        expect(doc.h1.marginTop).toBe(16)
        expect(doc.headingRule).toBe(true)
    })

    it('defaults to documentation, so an existing caller is unaffected', () => {
        expect(markdownScale()).toEqual(markdownScale('documentation'))
    })

    // A `#` in a chat message is emphasis, not a document title.
    it('caps headings in a comment near body size', () => {
        const compact = markdownScale('compact')
        expect(compact.h1.size).toBeLessThanOrEqual(compact.bodySize + 2)
        expect(compact.headingRule).toBe(false)
    })

    it('spaces a comment more tightly than a help topic', () => {
        expect(markdownScale('compact').paragraphSpacing).toBeLessThan(
            markdownScale('documentation').paragraphSpacing
        )
    })
})
