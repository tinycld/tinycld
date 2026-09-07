import type { ComponentType } from 'react'
import type { StyleProp, ViewStyle } from 'react-native'

/**
 * The JS-visible contract of the `editor-webview` native module.
 *
 * A pooled WebView is identified by `instanceKey`. Mounting an `EditorWebView`
 * host with that key ATTACHES the pooled native WebView to the host (moving it
 * out of whichever host held it before — last mount wins); unmounting the host
 * detaches without destroying. The page therefore survives React remounting
 * the host in another subtree, which is what lets the app's one warm editor be
 * handed between surfaces without reloading. `destroy` is the only thing that
 * releases the native WebView — the hook that owns the key calls it on unmount.
 *
 * Messages from the page are MODULE events keyed by instance (`subscribe`),
 * not events on the host view: a host comes and goes with every hand-off, and
 * an event addressed to a view React Native is tearing down is dropped.
 *
 * Keep in sync with `tinycld/core/types/editor-webview.d.ts`.
 */
export interface EditorWebViewProps {
    /** Pool key. The hook that owns the page holds one for its lifetime. */
    instanceKey: string
    /** Full HTML of the page. Loaded once per key; later values are ignored. */
    source: string
    /** Whether the WebView scrolls its own content. Default true. */
    scrollEnabled?: boolean
    /** Background of the WebView itself (the host's RN style is separate). */
    webBackgroundColor?: string
    /** Exposes the page to Safari Web Inspector / chrome://inspect. */
    inspectable?: boolean
    style?: StyleProp<ViewStyle>
}

export interface EditorWebViewState {
    exists: boolean
    loaded: boolean
    attached: boolean
}

export interface EditorWebViewListeners {
    /** The page called `window.ReactNativeWebView.postMessage(data)`. */
    onMessage: (data: string) => void
    /**
     * The WebView's content/render process died and the source was reloaded.
     * The page will post `editor-ready` again as it boots.
     */
    onProcessGone?: () => void
}

export interface EditorWebViewModuleType {
    /**
     * Dispatch a `MessageEvent('message', { data })` on the page's `document`.
     * Returns false when no pooled instance exists for the key.
     */
    postMessage(instanceKey: string, data: string): boolean
    /** Make the WebView first responder and show the keyboard. */
    requestFocus(instanceKey: string): Promise<void>
    /** Release the pooled WebView. */
    destroy(instanceKey: string): Promise<void>
    getState(instanceKey: string): EditorWebViewState
    addListener(
        event: 'onMessage' | 'onLoad' | 'onProcessGone',
        listener: (event: { instanceKey: string; data?: string }) => void
    ): { remove(): void }
}

export type EditorWebViewComponent = ComponentType<EditorWebViewProps>
