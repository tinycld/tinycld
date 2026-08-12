import CollaborationCaret from '@tiptap/extension-collaboration-caret'
import { defaultSelectionBuilder } from '@tiptap/y-tiptap'
import { afterEach, describe, expect, it } from 'vitest'
import {
    EDITOR_CONTENT_STYLES,
    EDITOR_SCOPE_CLASS,
    scopeEditorStyles,
} from '../editor-content-styles'

/**
 * Pins the caret CSS to the class names the INSTALLED extension actually emits.
 *
 * This is the regression that made remote carets invisible on every platform:
 * tiptap v3 renamed the v2 classes from `collaboration-caret__*` to the plural
 * `collaboration-carets__*`, our stylesheet kept the singular form, and the
 * rules silently matched nothing. Nothing failed — the spans still rendered,
 * with no width and no border-style, so they were simply never visible.
 *
 * Asserting the strings alone would only pin the CSS to what someone typed. We
 * drive the extension's real `render` instead, so a future tiptap rename fails
 * here rather than blanking carets in production.
 */

/**
 * The subset of the DOM the extension's default `render` touches: it creates a
 * span and a div, adds one class to each, sets a style attribute, and nests
 * them. A full jsdom environment would work too, but this vitest project runs
 * on `node` and the surface is this small.
 */
class FakeElement {
    readonly classes: string[] = []
    readonly children: FakeElement[] = []
    readonly classList = { add: (name: string) => this.classes.push(name) }
    setAttribute() {}
    insertBefore(child: FakeElement) {
        this.children.push(child)
    }
}

afterEach(() => {
    ;(globalThis as { document?: unknown }).document = undefined
})

/** Every class the caret decorations carry, straight from the extension. */
function renderedCaretClasses(): string[] {
    ;(globalThis as { document?: unknown }).document = {
        createElement: () => new FakeElement(),
        createTextNode: () => new FakeElement(),
    }
    // The real signature returns an HTMLElement; the fake document above hands
    // back a FakeElement instead, which is all the render actually touches.
    const render = CollaborationCaret.options.render as unknown as (user: {
        name: string
        color: string
    }) => FakeElement
    const caret = render({ name: 'Ada', color: '#ff0000' })
    return [caret.classes, ...caret.children.map(child => child.classes)].flat()
}

describe('editor caret styles', () => {
    it('styles every class the installed extension emits', () => {
        for (const className of renderedCaretClasses()) {
            expect(EDITOR_CONTENT_STYLES).toContain(`.${className}`)
        }
    })

    it('styles the remote selection class, which is named differently', () => {
        // The selection comes from y-tiptap, not the caret extension, and uses
        // `ProseMirror-yjs-selection` rather than a `collaboration-carets__`
        // name. Easy to miss when adding the caret rules by hand.
        const { class: selectionClass } = defaultSelectionBuilder({ color: '#ff0000' })
        expect(EDITOR_CONTENT_STYLES).toContain(`.${selectionClass}`)
    })

    it('carries the plural v3 caret names, not the dead v2 singulars', () => {
        expect(EDITOR_CONTENT_STYLES).toContain('.collaboration-carets__caret')
        expect(EDITOR_CONTENT_STYLES).toContain('.collaboration-carets__label')
        // The exact drift that shipped: `collaboration-caret__` matches nothing.
        expect(EDITOR_CONTENT_STYLES).not.toContain('.collaboration-caret__')
    })

    it('gives the caret a border-style, since only the color is set inline', () => {
        // The extension sets `border-color` inline and nothing else, so a rule
        // without a style and width resolves to no visible border at all.
        const caretRule = EDITOR_CONTENT_STYLES.match(
            /\.collaboration-carets__caret\s*\{[^}]*\}/
        )?.[0]
        expect(caretRule).toBeDefined()
        expect(caretRule).toContain('border-left-style')
        expect(caretRule).toContain('border-left-width')
    })

    it('gives the caret a height, or its border draws nothing', () => {
        // The extension's span carries no text of its own and its label is
        // absolutely positioned, so an unsized inline-block collapses to a
        // zero-AREA box — and a border on a zero-height box is invisible. This
        // shipped: the caret was in the DOM, correctly classed, and 0px tall.
        const caretRule = EDITOR_CONTENT_STYLES.match(
            /\.collaboration-carets__caret\s*\{[^}]*\}/
        )?.[0]
        expect(caretRule).toMatch(/height:\s*[^0\s]/)
    })

    it('positions the label, which would otherwise fill the line', () => {
        // The label is a block-level <div> inside an inline <span>; unpositioned
        // it balloons across the whole line ("the giant colored block").
        const labelRule = EDITOR_CONTENT_STYLES.match(
            /\.collaboration-carets__label\s*\{[^}]*\}/
        )?.[0]
        expect(labelRule).toContain('position: absolute')
    })
})

describe('scopeEditorStyles', () => {
    // Web injects this sheet into the shared document.head, so a selector that
    // escapes the scope restyles mail's compose body — and one that gets
    // over-rewritten silently styles nothing. Both failure modes are invisible
    // without a test: the page simply looks a bit wrong somewhere else.

    it('scopes a plain rule', () => {
        expect(scopeEditorStyles('.ProseMirror p { margin: 0; }')).toBe(
            '.tinycld-rich-editor .ProseMirror p { margin: 0; }'
        )
    })

    it('leaves a descendant .ProseMirror-* class alone', () => {
        // A blanket replace rewrites this second token too, producing
        // `.scope .ProseMirror .scope .ProseMirror-yjs-selection` — a selector
        // that matches nothing, so remote selections lose their styling.
        expect(scopeEditorStyles('.ProseMirror .ProseMirror-yjs-selection { x: y; }')).toBe(
            '.tinycld-rich-editor .ProseMirror .ProseMirror-yjs-selection { x: y; }'
        )
    })

    it('scopes every selector in a comma list', () => {
        // Anchoring on line starts alone leaves `.ProseMirror ol` unscoped, and
        // an unscoped rule leaks onto every other editor on the page.
        expect(scopeEditorStyles('.ProseMirror ul, .ProseMirror ol { padding: 0; }')).toBe(
            '.tinycld-rich-editor .ProseMirror ul, .tinycld-rich-editor .ProseMirror ol { padding: 0; }'
        )
    })

    it('scopes every rule in the real stylesheet, leaking none', () => {
        const scoped = scopeEditorStyles(EDITOR_CONTENT_STYLES)
        for (const selector of scoped.split('{').slice(0, -1)) {
            const last = selector.split(',').at(-1) ?? ''
            if (!last.includes('.ProseMirror')) continue
            expect(last).toContain(`.${EDITOR_SCOPE_CLASS} .ProseMirror`)
        }
    })
})
