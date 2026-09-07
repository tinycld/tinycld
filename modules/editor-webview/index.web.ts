import type { EditorWebViewComponent, EditorWebViewState } from './src/EditorWebView.types'

// Web stub for the `editor-webview` native view. On web the shared editor is
// Tiptap in the DOM (use-rich-editor.web.tsx); nothing renders this host or
// calls these functions. The stub exists so the hosting hook's static import
// resolves when bundling for web — Metro picks this file over index.ts.
export const EditorWebView: EditorWebViewComponent = () => null

export function postMessage(): boolean {
    return false
}

export function requestFocus(): void {}

export function destroy(): void {}

export function getState(): EditorWebViewState {
    return { exists: false, loaded: false, attached: false }
}

export function subscribe(): () => void {
    return () => {}
}

export type * from './src/EditorWebView.types'
