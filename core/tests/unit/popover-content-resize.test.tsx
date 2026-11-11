// @vitest-environment happy-dom

import { act, cleanup, fireEvent, render as renderBare } from '@testing-library/react'
import { useWindowSizeStore } from '@tinycld/core/lib/stores/window-size-store'
import { OverlayProvider } from '@tinycld/core/ui/overlay'
import { Popover, placePopover, type Size } from '@tinycld/core/ui/popover'
import type { ReactElement } from 'react'
import { Text } from 'react-native'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

/**
 * On web, react-native-web reports `onLayout` late (its ResizeObserver callback
 * is batched), so content that grew after the open was painted for a frame at
 * the top placed for its old size. In the picker that broke, that frame covered
 * the resting pointer and an option stole the focus from the search field.
 *
 * A browser delivers ResizeObserver notifications after layout and before
 * paint, so a popover that measures its content with its own observer and
 * commits the new placement inside that callback never paints the stale frame.
 * This test holds the part jsdom can express: when the content's new size is
 * delivered to the observer, the surface's committed style already holds the
 * placement `placePopover` gives for that size; nothing waits for `onLayout`
 * or a later task. A popover that measures in a layout effect instead commits
 * during the re-render itself, which this test accepts too.
 */

const render = (ui: ReactElement) => renderBare(<OverlayProvider>{ui}</OverlayProvider>)

const VIEWPORT: Size = { width: 1024, height: 768 }
// Near the bottom: the short content fits below, the tall content must flip above.
const ANCHOR = { x: 100, y: 700 }
const SHORT: Size = { width: 240, height: 40 }
const TALL: Size = { width: 240, height: 300 }

function expectedPlacement(size: Size) {
    return placePopover({
        anchor: { ...ANCHOR, width: 0, height: 0 },
        size,
        viewport: VIEWPORT,
        placement: 'bottom-start',
    })
}

// The browser API the popover may measure with; happy-dom's own never fires.
// Stubbing a missing browser behavior, not one of our components.
class StubResizeObserver {
    static live = new Set<StubResizeObserver>()
    readonly targets = new Set<Element>()
    constructor(private readonly callback: ResizeObserverCallback) {
        StubResizeObserver.live.add(this)
    }
    observe(target: Element) {
        this.targets.add(target)
    }
    unobserve(target: Element) {
        this.targets.delete(target)
    }
    disconnect() {
        this.targets.clear()
        StubResizeObserver.live.delete(this)
    }
    /** Delivers `size` for every observed node inside `scope`, as the browser does after layout. */
    static deliver(scope: Element, size: Size) {
        for (const observer of StubResizeObserver.live) {
            const entries = [...observer.targets]
                .filter(target => scope.contains(target))
                .map(target => resizeEntry(target, size))
            if (entries.length > 0) {
                observer.callback(entries, observer as unknown as ResizeObserver)
            }
        }
    }
}

function resizeEntry(target: Element, { width, height }: Size): ResizeObserverEntry {
    const box = [{ inlineSize: width, blockSize: height }]
    return {
        target,
        contentRect: rect({ width, height }),
        borderBoxSize: box,
        contentBoxSize: box,
        devicePixelContentBoxSize: box,
    } as unknown as ResizeObserverEntry
}

function rect({ width, height }: Size): DOMRect {
    return {
        x: 0,
        y: 0,
        top: 0,
        left: 0,
        width,
        height,
        right: width,
        bottom: height,
        toJSON: () => ({}),
    } as DOMRect
}

/** Makes a DOM node measure as `size` by every reading a popover may take. */
function setMeasuredSize(node: Element, size: Size) {
    const define = (key: string, value: unknown) =>
        Object.defineProperty(node, key, { configurable: true, get: () => value })
    define('offsetWidth', size.width)
    define('offsetHeight', size.height)
    define('clientWidth', size.width)
    define('clientHeight', size.height)
    define('scrollWidth', size.width)
    define('scrollHeight', size.height)
    Object.defineProperty(node, 'getBoundingClientRect', {
        configurable: true,
        value: () => rect(size),
    })
}

function surface(): HTMLElement {
    const node = document.querySelector<HTMLElement>('[testid="surf"]')
    if (!node) throw new Error('no popover surface rendered')
    return node
}

// The content view: the scroll region's child, the node the popover measures.
function content(): HTMLElement {
    const node = surface().querySelector<HTMLElement>('rn-scrollview > *')
    if (!node) throw new Error('no popover content rendered')
    return node
}

function Harness({ rows }: { rows: number }) {
    const items = Array.from({ length: rows }, (_, i) => `Option ${i + 1}`)
    return (
        <Popover anchor={ANCHOR} isOpen testID="surf" placement="bottom-start">
            {items.map(label => (
                <Text key={label}>{label}</Text>
            ))}
        </Popover>
    )
}

/** Delivers a size outside act, so only a synchronous commit shows in the DOM. */
function deliverOutsideAct(scope: Element, size: Size) {
    const env = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    const previous = env.IS_REACT_ACT_ENVIRONMENT
    env.IS_REACT_ACT_ENVIRONMENT = false
    try {
        StubResizeObserver.deliver(scope, size)
    } finally {
        env.IS_REACT_ACT_ENVIRONMENT = previous
    }
}

describe('Popover content that grows after the open (web)', () => {
    beforeEach(() => {
        vi.stubGlobal('ResizeObserver', StubResizeObserver)
        useWindowSizeStore.getState().setSize(VIEWPORT.width, VIEWPORT.height)
    })
    afterEach(() => {
        cleanup()
        StubResizeObserver.live.clear()
        vi.unstubAllGlobals()
    })

    it('re-places the surface for the new size when the observer delivers it, not on a later onLayout', () => {
        const { rerender } = render(<Harness rows={1} />)

        // First placement: as today, from the first measurement.
        setMeasuredSize(content(), SHORT)
        act(() => {
            fireEvent(content(), new CustomEvent('rn-layout', { detail: { x: 0, y: 0, ...SHORT } }))
            StubResizeObserver.deliver(surface(), SHORT)
        })
        const short = expectedPlacement(SHORT)
        expect(short.side).toBe('bottom')
        expect(surface().style.top).toBe(`${short.top}px`)
        expect(surface().style.opacity).toBe('1')

        // The options arrive: the content grows. Its new size reaches the
        // popover the way a browser reports it before paint; onLayout never
        // fires here, because on web it comes too late.
        setMeasuredSize(content(), TALL)
        act(() =>
            rerender(
                <OverlayProvider>
                    <Harness rows={10} />
                </OverlayProvider>
            )
        )
        deliverOutsideAct(surface(), TALL)

        const tall = expectedPlacement(TALL)
        expect(tall.side).toBe('top')
        expect(surface().style.top).toBe(`${tall.top}px`)
        expect(surface().style.maxHeight).toBe(`${tall.maxHeight}px`)
    })
})
