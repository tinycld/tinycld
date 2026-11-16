// @vitest-environment happy-dom

import { act, cleanup, fireEvent, render as renderBare } from '@testing-library/react'
import { useWindowSizeStore } from '@tinycld/core/lib/stores/window-size-store'
import { OverlayProvider } from '@tinycld/core/ui/overlay'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { Popover, type Size } from '@tinycld/core/ui/popover'
import type { ReactElement } from 'react'
import { Pressable, Text } from 'react-native'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

/**
 * On web an unplaced popover surface is `visibility: hidden`, so the mount
 * focus of an `autoFocus` input inside it does nothing in a browser. When the
 * surface shows, the popover moves the focus in again; it must pick the input
 * that asked for focus, not the first focusable. happy-dom does not refuse
 * focus to a hidden element, so the test takes the focus away again after the
 * mount, which is the state a browser leaves.
 */

const render = (ui: ReactElement) => renderBare(<OverlayProvider>{ui}</OverlayProvider>)

const VIEWPORT: Size = { width: 1024, height: 768 }
const ANCHOR = { x: 100, y: 100 }
const SIZE: Size = { width: 240, height: 80 }

// The browser API the popover measures with; happy-dom's own never fires.
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
    static deliver(scope: Element, { width, height }: Size) {
        const box = [{ inlineSize: width, blockSize: height }]
        for (const observer of StubResizeObserver.live) {
            const entries = [...observer.targets]
                .filter(target => scope.contains(target))
                .map(
                    target =>
                        ({
                            target,
                            contentRect: rect(SIZE),
                            borderBoxSize: box,
                            contentBoxSize: box,
                            devicePixelContentBoxSize: box,
                        }) as unknown as ResizeObserverEntry
                )
            if (entries.length > 0)
                observer.callback(entries, observer as unknown as ResizeObserver)
        }
    }
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

function content(): HTMLElement {
    const node = surface().querySelector<HTMLElement>('rn-scrollview > *')
    if (!node) throw new Error('no popover content rendered')
    return node
}

function byTestId(id: string): HTMLElement {
    const node = document.querySelector<HTMLElement>(`[testid="${id}"]`)
    if (!node) throw new Error(`no ${id} rendered`)
    return node
}

// tabIndex: the react-native stub renders custom elements, which take focus
// only with one (react-native-web renders a focusable button and input).
function Harness() {
    return (
        <>
            <Pressable testID="outside" accessibilityRole="button" tabIndex={0}>
                <Text>Outside</Text>
            </Pressable>
            <Popover anchor={ANCHOR} isOpen testID="surf" placement="bottom-start">
                <Pressable testID="first" accessibilityRole="button" tabIndex={0}>
                    <Text>First</Text>
                </Pressable>
                <PlainInput testID="search" autoFocus tabIndex={0} />
            </Popover>
        </>
    )
}

describe('Popover content with an autoFocus input that is not the first focusable (web)', () => {
    beforeEach(() => {
        vi.stubGlobal('ResizeObserver', StubResizeObserver)
        useWindowSizeStore.getState().setSize(VIEWPORT.width, VIEWPORT.height)
    })
    afterEach(() => {
        cleanup()
        StubResizeObserver.live.clear()
        vi.unstubAllGlobals()
    })

    it('marks the input and focuses it, not the first focusable, when the surface shows', () => {
        render(<Harness />)
        expect(byTestId('search').getAttribute('data-autofocus')).toBe('true')

        // What a browser leaves: the mount focus hit a hidden surface.
        act(() => byTestId('outside').focus())
        expect(surface().style.opacity).toBe('0')

        setMeasuredSize(content(), SIZE)
        act(() => {
            fireEvent(content(), new CustomEvent('rn-layout', { detail: { x: 0, y: 0, ...SIZE } }))
            StubResizeObserver.deliver(surface(), SIZE)
        })

        expect(surface().style.opacity).toBe('1')
        // By test id: a failing toBe on two elements prints the whole tree.
        expect(document.activeElement?.getAttribute('testid')).toBe('search')
    })
})
