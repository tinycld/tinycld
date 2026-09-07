'use strict'

// Stub for the `editor-webview` native view in unit tests. It is Metro-resolved
// (no node_modules entry), so Vite's import-analysis can't resolve the bare
// specifier. Mirrors the web stub (modules/editor-webview/index.web.ts): no
// pooled instance ever exists. Tests that need to observe the host's props or
// the module's calls `vi.mock('editor-webview')` instead.
module.exports = {
    EditorWebView: () => null,
    postMessage: () => false,
    requestFocus: () => {},
    destroy: () => {},
    getState: () => ({ exists: false, loaded: false, attached: false }),
    subscribe: () => () => {},
}
