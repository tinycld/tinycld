// @vitest-environment happy-dom

import { cleanup, fireEvent, render } from '@testing-library/react'
import { useWindowSizeStore } from '@tinycld/core/lib/stores/window-size-store'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { SidebarItem } from '../SidebarItem'

const PHONE = { width: 390, height: 844 }
const DESKTOP = { width: 1280, height: 800 }

// The stores are module singletons: the phone and desktop tests above must not leak
// their window size or drawer state into the other files of the run.
const initialWindowSize = useWindowSizeStore.getState()
const initialWorkspace = useWorkspaceStore.getState()

function isDrawerOpen() {
    return useWorkspaceStore.getState().isDrawerOpen
}

// What a package's own control does when it opens the drawer (a round New button on a
// phone, for example).
function openDrawer() {
    useWorkspaceStore.getState().setDrawerOpen(true)
}

function renderItem(props: { closesDrawer?: boolean; onPress?: () => void }) {
    const { container } = render(<SidebarItem label="Zoo keepers" testID="zoo-item" {...props} />)
    return container.querySelector('[testid="zoo-item"]') as HTMLElement
}

beforeEach(() => {
    vi.useFakeTimers()
    useWindowSizeStore.setState(PHONE)
    openDrawer()
})

afterEach(() => {
    cleanup()
    vi.useRealTimers()
    useWindowSizeStore.setState(initialWindowSize)
    useWorkspaceStore.setState(initialWorkspace)
})

describe('SidebarItem and the phone drawer', () => {
    it('closes the drawer at once on a press, after the press handler ran', () => {
        const seen: boolean[] = []
        const item = renderItem({ closesDrawer: true, onPress: () => seen.push(isDrawerOpen()) })

        fireEvent.click(item)

        expect(seen).toEqual([true])
        expect(isDrawerOpen()).toBe(false)
    })

    // The defect this guards: the item closed the drawer with a timer 500 ms after the
    // press. A press that changes the route closes the drawer at once (the app layout does
    // that), so the timer then closed a drawer the user had opened again.
    it('leaves a drawer open that is opened again after the press', () => {
        const item = renderItem({ closesDrawer: true })
        fireEvent.click(item)
        // The route change of the press, as the app layout handles it.
        useWorkspaceStore.getState().setDrawerOpen(false)

        openDrawer()
        vi.advanceTimersByTime(1000)

        expect(isDrawerOpen()).toBe(true)
        expect(vi.getTimerCount()).toBe(0)
    })

    it('leaves the drawer open when the item does not close it', () => {
        const item = renderItem({})

        fireEvent.click(item)
        vi.advanceTimersByTime(1000)

        expect(isDrawerOpen()).toBe(true)
    })

    it('does not touch the drawer state above the phone width', () => {
        useWindowSizeStore.setState(DESKTOP)
        const item = renderItem({ closesDrawer: true })

        fireEvent.click(item)
        vi.advanceTimersByTime(1000)

        expect(isDrawerOpen()).toBe(true)
    })
})
