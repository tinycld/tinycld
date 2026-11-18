// @vitest-environment happy-dom
import { act, cleanup, renderHook } from '@testing-library/react'
import { createElement, type ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ScreenStub } from '../../../../tests/expo-router-stub'
import { useLeaveGuard } from '../use-leave-guard'

/**
 * The in-app half of the leave guard: what Discard dispatches for a held
 * navigation action. A screen's `navigation.dispatch` in expo-router
 * (useNavigationCache) sends `{ source: <the screen's route key>, ...action }`,
 * so the fake screen's dispatch does the same. A parent stack's REPLACE or
 * pop has no `source` and a `target` of the parent navigator; a replay that
 * picks up the guarded screen's key names a route the parent does not know,
 * and the parent then does nothing.
 *
 * `usePreventRemove` is faked as in expo-router 58: its callback gets the
 * held action and `repeat` (which turns prevention off, then dispatches the
 * action through the screen's navigation), and it returns `disablePrevention`.
 */

const SCREEN_KEY = 'edit-screen-key'

type Action = { type: string; payload?: unknown; source?: string; target?: string }
type HeldAction = { data: { action: Action } }

/** Every call the guard makes to prevention and dispatch, in order. */
const calls = vi.hoisted(() => ({ log: [] as string[] }))

const preventRemove = vi.hoisted(() => ({
    onPrevent: null as null | ((event: HeldAction) => void),
    disablePrevention: () => {
        calls.log.push('disablePrevention')
    },
}))

vi.mock('expo-router/react-navigation', async () => {
    const { useNavigation } = await import('expo-router')
    return {
        usePreventRemove: (
            isPreventing: boolean,
            onPrevent: (event: HeldAction & { repeat: () => void }) => void
        ) => {
            const navigation = useNavigation() as { dispatch: (action: unknown) => void }
            preventRemove.onPrevent = isPreventing
                ? event =>
                      onPrevent({
                          data: event.data,
                          repeat: () => {
                              preventRemove.disablePrevention()
                              navigation.dispatch(event.data.action)
                          },
                      })
                : null
            return preventRemove.disablePrevention
        },
    }
})

/** The actions the screen's navigation sent, as expo-router stamps them. */
let dispatched: Action[] = []

function guardedScreen() {
    return {
        pathname: '/stub/guard/edit',
        navigation: {
            isFocused: () => true,
            dispatch: (action: unknown) => {
                calls.log.push('dispatch')
                dispatched.push({ source: SCREEN_KEY, ...(action as Action) })
            },
        },
    }
}

function renderGuard() {
    const screen = guardedScreen()
    const wrapper = ({ children }: { children: ReactNode }) =>
        createElement(ScreenStub, { value: screen }, children)
    return renderHook(() => useLeaveGuard(true), { wrapper })
}

function hold(action: Action) {
    act(() => {
        preventRemove.onPrevent?.({ data: { action } })
    })
}

beforeEach(() => {
    dispatched = []
    calls.log = []
    preventRemove.onPrevent = null
})

afterEach(() => {
    cleanup()
})

describe('useLeaveGuard Discard replay of a held in-app action', () => {
    it('replays a parent stack move that had no source with no source and the same target', () => {
        const { result } = renderGuard()
        hold({
            type: 'REPLACE',
            payload: { name: 'index', params: {} },
            target: 'parent-stack-key',
        })
        expect(result.current.isConfirming).toBe(true)

        act(() => result.current.discard())

        expect(dispatched).toHaveLength(1)
        expect(dispatched[0]?.source).toBeUndefined()
        expect(dispatched[0]?.target).toBe('parent-stack-key')
        expect(dispatched[0]?.type).toBe('REPLACE')
        expect(dispatched[0]?.payload).toEqual({ name: 'index', params: {} })
        expect(result.current.isConfirming).toBe(false)
    })

    it('replays a held action that had a source with that same source', () => {
        const { result } = renderGuard()
        hold({
            type: 'POP',
            payload: { count: 1 },
            source: 'other-route-key',
            target: 'own-stack-key',
        })

        act(() => result.current.discard())

        expect(dispatched).toHaveLength(1)
        expect(dispatched[0]?.source).toBe('other-route-key')
        expect(dispatched[0]?.target).toBe('own-stack-key')
    })

    it('turns removal prevention off before it replays the held action', () => {
        const { result } = renderGuard()
        hold({ type: 'REPLACE', payload: { name: 'index' }, target: 'parent-stack-key' })

        act(() => result.current.discard())

        expect(calls.log).toEqual(['disablePrevention', 'dispatch'])
    })

    it('dispatches nothing and keeps prevention on when the user keeps editing', () => {
        const { result } = renderGuard()
        hold({ type: 'REPLACE', payload: { name: 'index' }, target: 'parent-stack-key' })

        act(() => result.current.cancel())

        expect(result.current.isConfirming).toBe(false)
        expect(dispatched).toEqual([])
        expect(calls.log).toEqual([])
        expect(preventRemove.onPrevent).not.toBeNull()
    })
})
