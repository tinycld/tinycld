// Vitest stub for @10play/tentap-editor.
//
// The package's `react-native` export condition points at src/index.tsx — raw
// TypeScript source, which re-exports RichText and the bridge modules. Those
// reach react-native internals carrying Flow syntax (`import typeof`), so Vite's
// node environment fails to parse them and any test whose import chain touches
// use-webview-editor.tsx dies at collect time with "Unexpected token 'typeof'".
// Same failure mode as the react-native-svg and uniwind stubs beside this file.
//
// Only the surface use-webview-editor.tsx imports is needed: the hooks return
// inert values, since the pure host logic (shouldPostInit, deriveToolbarState)
// is what unit tests exercise — driving a real bridge needs a WebView.
'use strict'

function useEditorBridge() {
    return {
        webviewRef: { current: null },
        getHTML: () => Promise.resolve(''),
        getText: () => Promise.resolve(''),
        setContent: () => {},
        focus: () => {},
        setEditable: () => {},
    }
}

function useBridgeState() {
    return {}
}

function RichText() {
    return null
}

class BridgeExtension {}

module.exports = {
    useEditorBridge,
    useBridgeState,
    RichText,
    BridgeExtension,
    TenTapStartKit: [],
    CoreBridge: {},
    defaultEditorTheme: {},
}
