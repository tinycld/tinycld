import { describe, expect, it } from 'vitest'
import { EDITOR_CONTENT_STYLES } from '../../../lib/editor/rich/editor-content-styles'
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
function editorEm(selector: string): number {
    const rule = new RegExp(`\\.ProseMirror ${selector}[^{]*\\{([^}]*)\\}`)
    const body = EDITOR_CONTENT_STYLES.match(rule)?.[1]
    if (!body) throw new Error(`no rule for ${selector} in the editor stylesheet`)
    const size = body.match(/font-size:\s*([\d.]+)em/)?.[1]
    if (!size) throw new Error(`no em font-size for ${selector}`)
    return Number.parseFloat(size)
}

describe('the description scale matches the editor', () => {
    const scale = markdownScale('description')

    it('uses the editor page’s base font size for body text', () => {
        expect(scale.bodySize).toBe(EDITOR_BASE_PX)
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
            /\.ProseMirror > \* \+ \*\s*\{[^}]*margin-top:\s*([\d.]+)em/
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
