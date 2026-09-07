import { useEffect } from 'react'
import { Keyboard, Platform } from 'react-native'
import { makeMessage } from '../message-bus/types'
import { APP_KEYBOARD_INSET } from '../rich/webview/source/protocol'
import type { PostEditorMessage } from '../webview-editor-commands'

/**
 * Keep the caret out from under the keyboard.
 *
 * A port of the keyboard avoidance TenTap's RichText did for us: watch the
 * keyboard, and pad the bottom of the document by its height so the last lines
 * can scroll clear of it. The padding travels as an `app/keyboard-inset`
 * message the page applies (see `rich/webview/source/host-commands.ts`),
 * rather than injected JavaScript — the native host has no script injection,
 * and a message is what every other host → page instruction already is.
 *
 * The numbers are TenTap's. iOS pads by the reported keyboard height plus a
 * small gap. Android's keyboard resizes the window instead, so the page only
 * needs room for the toolbar that sits above the keyboard, and the value is
 * applied after a short delay to let the resize settle.
 */
const IOS_KEYBOARD_GAP_PX = 10
const ANDROID_KEYBOARD_INSET_PX = 44
const ANDROID_SETTLE_MS = 200

export function keyboardInsetFor(os: string, isUp: boolean, height: number): number {
    if (!isUp) return 0
    if (os === 'android') return ANDROID_KEYBOARD_INSET_PX
    return height > 0 ? height + IOS_KEYBOARD_GAP_PX : 0
}

export function useKeyboardInset(post: PostEditorMessage, enabled: boolean): void {
    useEffect(() => {
        if (!enabled) return
        let isUp = false
        let height = 0
        // The page starts with no inset, so a zero is only worth posting after
        // a non-zero one.
        let last = 0
        let timer: ReturnType<typeof setTimeout> | null = null

        const apply = () => {
            timer = null
            const bottom = keyboardInsetFor(Platform.OS, isUp, height)
            if (bottom === last) return
            // Only remember what a page actually received, so an inset posted
            // into nothing is retried by the next keyboard event.
            if (post(makeMessage('app', APP_KEYBOARD_INSET, { bottom }))) last = bottom
        }
        const schedule = () => {
            if (Platform.OS !== 'android') {
                apply()
                return
            }
            if (timer) clearTimeout(timer)
            timer = setTimeout(apply, ANDROID_SETTLE_MS)
        }

        const subscriptions = [
            Keyboard.addListener('keyboardWillShow', () => {
                isUp = true
                schedule()
            }),
            Keyboard.addListener('keyboardDidShow', event => {
                isUp = true
                height = event.endCoordinates.height
                schedule()
            }),
            Keyboard.addListener(
                Platform.OS === 'ios' ? 'keyboardWillHide' : 'keyboardDidHide',
                () => {
                    isUp = false
                    height = 0
                    schedule()
                }
            ),
        ]
        return () => {
            for (const subscription of subscriptions) subscription.remove()
            if (timer) clearTimeout(timer)
        }
    }, [post, enabled])
}
