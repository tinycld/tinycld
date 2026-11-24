// @vitest-environment happy-dom

import { cleanup, render, renderHook } from '@testing-library/react'
import { createMatcher } from '@tinycld/core/lib/shortcuts/matcher'
import { useShortcutRegistry } from '@tinycld/core/lib/shortcuts/registry'
import { resetScopes } from '@tinycld/core/lib/shortcuts/scopes'
import type { Shortcut } from '@tinycld/core/lib/shortcuts/types'
import { useRegisterShortcut } from '@tinycld/core/lib/shortcuts/use-register'
import {
    CoveredProvider,
    coveredInputProps,
    isRouteCovered,
    useOpenUnlessCovered,
} from '@tinycld/core/ui/overlay/covered'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

/**
 * Content under a cover (the package terms cover) stays mounted. Nothing it
 * does by itself may come up over the cover or take keys from it: a dialog or
 * sheet it opens stays closed, and its shortcuts, `global` ones too, do not run.
 */

function clearRegistry() {
    const state = useShortcutRegistry.getState()
    for (const id of Array.from(state.shortcuts.keys())) state.unregister(id)
}

afterEach(() => {
    cleanup()
    clearRegistry()
    resetScopes()
})

function GlobalShortcut({ shortcut }: { shortcut: Shortcut }) {
    useRegisterShortcut(shortcut)
    return null
}

function Content({ isCovered, children }: { isCovered: boolean; children: ReactNode }) {
    return <CoveredProvider value={isCovered}>{children}</CoveredProvider>
}

describe('shortcuts under a cover', () => {
    it('a global shortcut of the covered content does not run while covered', () => {
        const run = vi.fn()
        const shortcut: Shortcut = {
            id: 't.find',
            keys: '$mod+f',
            scope: 'global',
            description: 'Find',
            run,
        }
        const { rerender } = render(
            <Content isCovered>
                <GlobalShortcut shortcut={shortcut} />
            </Content>
        )

        expect(createMatcher().feedAtom('$mod+f', { inInput: false })).toBe(false)
        expect(run).not.toHaveBeenCalled()

        rerender(
            <Content isCovered={false}>
                <GlobalShortcut shortcut={shortcut} />
            </Content>
        )

        expect(createMatcher().feedAtom('$mod+f', { inInput: false })).toBe(true)
        expect(run).toHaveBeenCalledTimes(1)
    })

    it('a shortcut outside any cover runs', () => {
        const run = vi.fn()
        render(
            <GlobalShortcut
                shortcut={{
                    id: 't.search',
                    keys: '/',
                    scope: 'global',
                    description: 'Search',
                    run,
                }}
            />
        )

        expect(createMatcher().feedAtom('/', { inInput: false })).toBe(true)
        expect(run).toHaveBeenCalledTimes(1)
    })
})

describe('isRouteCovered', () => {
    const closed = new Set(['zoo'])

    it('covers the route of a closed package, whichever route is active', () => {
        expect(isRouteCovered('zoo', closed)).toBe(true)
    })

    it('leaves an open package and an app route uncovered', () => {
        expect(isRouteCovered('acme', closed)).toBe(false)
        expect(isRouteCovered('settings', closed)).toBe(false)
    })
})

describe('a closed route under a navigator', () => {
    // Each route gets its own value, so a move to an open route does not
    // register the closed route's shortcuts again.
    function Route({
        name,
        closed,
        shortcut,
    }: {
        name: string
        closed: Set<string>
        shortcut: Shortcut
    }) {
        return (
            <CoveredProvider value={isRouteCovered(name, closed)}>
                <GlobalShortcut shortcut={shortcut} />
            </CoveredProvider>
        )
    }

    it('keeps the global shortcut of the closed route out, and the open route in', () => {
        const zooRun = vi.fn()
        const acmeRun = vi.fn()
        const closed = new Set(['zoo'])
        render(
            <>
                <Route
                    name="zoo"
                    closed={closed}
                    shortcut={{
                        id: 'zoo.find',
                        keys: '$mod+f',
                        scope: 'global',
                        description: 'Find',
                        run: zooRun,
                    }}
                />
                <Route
                    name="acme"
                    closed={closed}
                    shortcut={{
                        id: 'acme.go',
                        keys: '$mod+g',
                        scope: 'global',
                        description: 'Go',
                        run: acmeRun,
                    }}
                />
            </>
        )

        expect(createMatcher().feedAtom('$mod+f', { inInput: false })).toBe(false)
        expect(createMatcher().feedAtom('$mod+g', { inInput: false })).toBe(true)
        expect(zooRun).not.toHaveBeenCalled()
        expect(acmeRun).toHaveBeenCalledTimes(1)
    })
})

describe('coveredInputProps', () => {
    it('makes a covered input read-only and drops its autofocus', () => {
        expect(coveredInputProps(true, { autoFocus: true })).toEqual({
            autoFocus: false,
            editable: false,
        })
        expect(coveredInputProps(true, { editable: true })).toEqual({
            autoFocus: false,
            editable: false,
        })
    })

    it('keeps the caller values of an input under no cover', () => {
        expect(coveredInputProps(false, { autoFocus: true, editable: false })).toEqual({
            autoFocus: true,
            editable: false,
        })
        expect(coveredInputProps(false, {})).toEqual({ autoFocus: undefined, editable: undefined })
    })
})

describe('useOpenUnlessCovered', () => {
    const wrapperFor =
        (isCovered: boolean) =>
        ({ children }: { children: ReactNode }) => (
            <CoveredProvider value={isCovered}>{children}</CoveredProvider>
        )

    it('keeps an open dialog closed while its content is covered', () => {
        const { result } = renderHook(() => useOpenUnlessCovered(true), {
            wrapper: wrapperFor(true),
        })
        expect(result.current).toBe(false)
    })

    it('opens it under no cover, or when the cover goes', () => {
        const { result } = renderHook(() => useOpenUnlessCovered(true), {
            wrapper: wrapperFor(false),
        })
        expect(result.current).toBe(true)

        const outside = renderHook(() => useOpenUnlessCovered(true))
        expect(outside.result.current).toBe(true)
    })

    it('keeps a closed dialog closed', () => {
        const { result } = renderHook(() => useOpenUnlessCovered(false), {
            wrapper: wrapperFor(false),
        })
        expect(result.current).toBe(false)
    })
})
