// @vitest-environment happy-dom

import { cleanup, render as renderBare } from '@testing-library/react'
import { log } from '@tinycld/core/lib/logger'
import { OverlayProvider, useLayerFocus } from '@tinycld/core/ui/overlay'
import { Sheet } from '@tinycld/core/ui/sheet'
import { useState } from 'react'
import { Platform, Text } from 'react-native'
import { afterEach, describe, expect, it, vi } from 'vitest'

const byId = <T extends HTMLElement>(id: string) => document.getElementById(id) as T
const nextFrame = () => new Promise<void>(resolve => requestAnimationFrame(() => resolve()))
const originalOS = Platform.OS

afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
    Platform.OS = originalOS
})

function SheetHarness({
    isOpen,
    onRestoreFocus,
}: {
    isOpen: boolean
    onRestoreFocus?: () => void
}) {
    return (
        <OverlayProvider>
            <button type="button" id="opener">
                Opener
            </button>
            <Sheet isOpen={isOpen} onClose={() => {}} title="Sheet" onRestoreFocus={onRestoreFocus}>
                <Text>body</Text>
            </Sheet>
        </OverlayProvider>
    )
}

// The workspace reanimated stub reports motion on, so the Sheet keeps its
// surface (and the focus in it) for its exit animation after `isOpen` goes false.
describe('Sheet — default restore after an animated close', () => {
    it('gives the focus back to the opener one frame after close', async () => {
        const { rerender } = renderBare(<SheetHarness isOpen={false} />)
        byId('opener').focus()
        rerender(<SheetHarness isOpen />)
        await nextFrame()
        expect(document.activeElement).not.toBe(byId('opener'))
        expect(document.activeElement).not.toBe(document.body)

        rerender(<SheetHarness isOpen={false} />)
        await nextFrame()
        expect(document.activeElement).toBe(byId('opener'))
    })
})

describe('useLayerFocus — a throwing onRestoreFocus', () => {
    it('is logged and does not throw out of the frame', async () => {
        const logError = vi.spyOn(log, 'error').mockImplementation(() => {})
        const uncaught: unknown[] = []
        const onError = (event: ErrorEvent) => {
            uncaught.push(event.error ?? event.message)
            event.preventDefault()
        }
        window.addEventListener('error', onError)
        const boom = new Error('boom')
        const onRestoreFocus = () => {
            throw boom
        }
        const { rerender } = renderBare(
            <SheetHarness isOpen={false} onRestoreFocus={onRestoreFocus} />
        )
        rerender(<SheetHarness isOpen onRestoreFocus={onRestoreFocus} />)
        await nextFrame()
        rerender(<SheetHarness isOpen={false} onRestoreFocus={onRestoreFocus} />)
        await nextFrame()
        await nextFrame()
        window.removeEventListener('error', onError)

        expect(uncaught).toEqual([])
        expect(logError).toHaveBeenCalledWith('core.overlay.focus', boom)
    })
})

function Layer({
    isActive,
    trap,
    restore,
    onRestoreFocus,
}: {
    isActive: boolean
    trap: boolean
    restore: boolean
    onRestoreFocus?: () => void
}) {
    const [container, setContainer] = useState<HTMLElement | null>(null)
    useLayerFocus({ isActive, container, trap, restore, onRestoreFocus })
    if (!isActive) return null
    return (
        <div ref={setContainer as (el: HTMLDivElement | null) => void}>
            <button type="button" id="row">
                Row
            </button>
        </div>
    )
}

function Harness({
    isActive,
    trap = true,
    restore = true,
    onRestoreFocus,
}: {
    isActive: boolean
    trap?: boolean
    restore?: boolean
    onRestoreFocus?: () => void
}) {
    return (
        <>
            <button type="button" id="opener">
                Opener
            </button>
            <Layer
                isActive={isActive}
                trap={trap}
                restore={restore}
                onRestoreFocus={onRestoreFocus}
            />
        </>
    )
}

// A layer whose options change while it stays open (a caller that toggles
// `trap`, or a portal that re-mounts the surface) has not closed: neither the
// restore nor the owner's callback may run then.
describe('useLayerFocus — restore only on a real close', () => {
    it('does not call onRestoreFocus when an option changes while open, and does on close', async () => {
        const onRestoreFocus = vi.fn()
        const { rerender } = renderBare(
            <Harness isActive={false} onRestoreFocus={onRestoreFocus} />
        )
        byId('opener').focus()
        rerender(<Harness isActive onRestoreFocus={onRestoreFocus} />)
        expect(document.activeElement).toBe(byId('row'))

        rerender(<Harness isActive trap={false} onRestoreFocus={onRestoreFocus} />)
        await nextFrame()
        expect(onRestoreFocus).not.toHaveBeenCalled()

        rerender(<Harness isActive={false} trap={false} onRestoreFocus={onRestoreFocus} />)
        await nextFrame()
        expect(onRestoreFocus).toHaveBeenCalledTimes(1)
    })

    it('does not move the focus to the opener when an option changes while open', async () => {
        const { rerender } = renderBare(<Harness isActive={false} />)
        byId('opener').focus()
        rerender(<Harness isActive />)
        expect(document.activeElement).toBe(byId('row'))

        rerender(<Harness isActive trap={false} />)
        await nextFrame()
        expect(document.activeElement).toBe(byId('row'))

        rerender(<Harness isActive={false} trap={false} />)
        await nextFrame()
        expect(document.activeElement).toBe(byId('opener'))
    })

    it('does not call onRestoreFocus on native when restore changes while open', async () => {
        Platform.OS = 'ios'
        const onRestoreFocus = vi.fn()
        const { rerender } = renderBare(
            <Harness isActive={false} onRestoreFocus={onRestoreFocus} />
        )
        rerender(<Harness isActive onRestoreFocus={onRestoreFocus} />)

        rerender(<Harness isActive restore={false} onRestoreFocus={onRestoreFocus} />)
        await nextFrame()
        expect(onRestoreFocus).not.toHaveBeenCalled()

        rerender(<Harness isActive={false} onRestoreFocus={onRestoreFocus} />)
        await nextFrame()
        expect(onRestoreFocus).toHaveBeenCalledTimes(1)
    })
})
