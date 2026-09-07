import { requireNativeModule, requireNativeViewManager } from 'expo-modules-core'
import type {
    EditorWebViewComponent,
    EditorWebViewModuleType,
    EditorWebViewProps,
} from './src/EditorWebView.types'

// Throws at import if the native module is absent. Metro resolves index.web.ts
// for web bundles (see metro.config.cjs), so this file only ever loads on
// iOS/Android where the module is autolinked from modules/.
const nativeModule = requireNativeModule<EditorWebViewModuleType>('EditorWebView')

export const EditorWebView: EditorWebViewComponent =
    requireNativeViewManager<EditorWebViewProps>('EditorWebView')

export function postMessage(instanceKey: string, data: string): boolean {
    return nativeModule.postMessage(instanceKey, data)
}

export function requestFocus(instanceKey: string): void {
    void nativeModule.requestFocus(instanceKey)
}

export function destroy(instanceKey: string): void {
    void nativeModule.destroy(instanceKey)
}

export function getState(instanceKey: string) {
    return nativeModule.getState(instanceKey)
}

export type * from './src/EditorWebView.types'
