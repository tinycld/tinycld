/**
 * Ambient declaration for the app-shell-provided `editor-webview` native view.
 *
 * `editor-webview` is a local Expo native module in the runnable app shell
 * (`tinycld/modules/editor-webview/`), autolinked into the binary at build time.
 * Core hosts the shared editor's WebView page through it
 * (`lib/editor/use-webview-editor.tsx`), but the implementation is only present
 * in the app shell — so this shape lets core, and every feature that imports the
 * hook, typecheck without the module on disk. Metro maps the bare specifier to
 * the module (or its inert web stub); vitest maps it to a stub.
 *
 * Keep in sync with `tinycld/modules/editor-webview/src/EditorWebView.types.ts`.
 */
declare module 'editor-webview' {
    import type { ComponentType } from 'react'
    import type { StyleProp, ViewStyle } from 'react-native'

    export interface EditorWebViewMessageEvent {
        nativeEvent: { data: string }
    }

    export interface EditorWebViewInstanceEvent {
        nativeEvent: { instanceKey: string }
    }

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
        /** The page called `window.ReactNativeWebView.postMessage(data)`. */
        onMessage?: (event: EditorWebViewMessageEvent) => void
        /** The page finished loading its source. */
        onLoad?: (event: EditorWebViewInstanceEvent) => void
        /**
         * The WebView's content/render process died and the source was reloaded.
         * The page will post `editor-ready` again as it boots.
         */
        onProcessGone?: (event: EditorWebViewInstanceEvent) => void
        style?: StyleProp<ViewStyle>
    }

    export interface EditorWebViewState {
        exists: boolean
        loaded: boolean
        attached: boolean
    }

    /**
     * The host view. Mounting it ATTACHES the pooled native WebView for
     * `instanceKey` (moving it out of any host that held it — last mount wins);
     * unmounting detaches without destroying, so the page survives a remount in
     * another subtree.
     */
    export const EditorWebView: ComponentType<EditorWebViewProps>

    /**
     * Dispatch a `MessageEvent('message', { data })` on the page's `document`.
     * Returns false when no pooled instance exists for the key.
     */
    export function postMessage(instanceKey: string, data: string): boolean
    /** Make the WebView first responder and show the keyboard. */
    export function requestFocus(instanceKey: string): void
    /** Release the pooled WebView. The owning hook calls this on unmount. */
    export function destroy(instanceKey: string): void
    export function getState(instanceKey: string): EditorWebViewState
}
