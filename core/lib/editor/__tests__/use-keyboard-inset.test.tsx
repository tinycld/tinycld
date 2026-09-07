// @vitest-environment happy-dom
import { renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { EditorMessage } from '../message-bus/types'

type KeyboardListener = (event: { endCoordinates: { height: number } }) => void
const listeners = new Map<string, KeyboardListener>()
const platform = { OS: 'ios' }

vi.mock('react-native', () => ({
    Platform: platform,
    Keyboard: {
        addListener: (name: string, listener: KeyboardListener) => {
            listeners.set(name, listener)
            return { remove: () => listeners.delete(name) }
        },
    },
}))

const { keyboardInsetFor, useKeyboardInset } = await import('../native-host/use-keyboard-inset')

afterEach(() => {
    listeners.clear()
    platform.OS = 'ios'
    vi.useRealTimers()
})

function fire(name: string, height = 0) {
    listeners.get(name)?.({ endCoordinates: { height } })
}

describe('keyboardInsetFor', () => {
    it.each([
        ['ios', true, 300, 310],
        ['ios', true, 0, 0],
        ['ios', false, 300, 0],
        ['android', true, 300, 44],
        ['android', false, 300, 0],
    ])('%s up=%s height=%s → %s', (os, isUp, height, expected) => {
        expect(keyboardInsetFor(os, isUp, height)).toBe(expected)
    })
})

describe('useKeyboardInset', () => {
    it('posts the inset when the keyboard shows and clears it when it hides', () => {
        const sent: EditorMessage[] = []
        const post = (message: EditorMessage) => {
            sent.push(message)
            return true
        }
        renderHook(() => useKeyboardInset(post, true))
        fire('keyboardWillShow')
        fire('keyboardDidShow', 300)
        fire('keyboardWillHide')
        expect(sent).toEqual([
            { namespace: 'app', type: 'keyboard-inset', payload: { bottom: 310 } },
            { namespace: 'app', type: 'keyboard-inset', payload: { bottom: 0 } },
        ])
    })

    it('does not repeat an unchanged inset', () => {
        const post = vi.fn(() => true)
        renderHook(() => useKeyboardInset(post, true))
        fire('keyboardDidShow', 300)
        fire('keyboardDidShow', 300)
        expect(post).toHaveBeenCalledTimes(1)
    })

    it('retries an inset the page never received', () => {
        const post = vi.fn().mockReturnValueOnce(false).mockReturnValue(true)
        renderHook(() => useKeyboardInset(post, true))
        fire('keyboardDidShow', 300)
        fire('keyboardDidShow', 300)
        expect(post).toHaveBeenCalledTimes(2)
    })

    it('does nothing when disabled, and unsubscribes on unmount', () => {
        const post = vi.fn(() => true)
        const { unmount } = renderHook(() => useKeyboardInset(post, false))
        expect(listeners.size).toBe(0)
        unmount()
        const { unmount: unmountEnabled } = renderHook(() => useKeyboardInset(post, true))
        expect(listeners.size).toBe(3)
        unmountEnabled()
        expect(listeners.size).toBe(0)
    })

    it('waits for the window to settle on Android', () => {
        vi.useFakeTimers()
        platform.OS = 'android'
        const post = vi.fn(() => true)
        renderHook(() => useKeyboardInset(post, true))
        fire('keyboardDidShow', 300)
        expect(post).not.toHaveBeenCalled()
        vi.advanceTimersByTime(200)
        expect(post).toHaveBeenCalledWith({
            namespace: 'app',
            type: 'keyboard-inset',
            payload: { bottom: 44 },
        })
    })
})
