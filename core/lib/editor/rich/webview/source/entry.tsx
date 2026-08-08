import { createRoot } from 'react-dom/client'
import { Editor } from './Editor'

// One-shot mount. The reentry guard stops a hot reload from initializing a
// second React root inside the WebView. Styles are injected by the editor
// itself rather than here, because they are themed from the init payload,
// which has not arrived at this point.
if (!window.__RICH_EDITOR_INITIALIZED__) {
    window.__RICH_EDITOR_INITIALIZED__ = true
    const root = document.getElementById('root')
    if (root) createRoot(root).render(<Editor />)
}
