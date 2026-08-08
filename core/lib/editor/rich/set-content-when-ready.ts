import type { EditorHandle } from '../types'

/**
 * Push content into an editor as soon as it can accept it.
 *
 * The web editor is ready synchronously, so this is a direct call. The native
 * variant resolves through the WebView's ready handshake, which is why the
 * signature is a subscription returning a cleanup function rather than a plain
 * setter — a caller that unmounts before the WebView reports ready must be able
 * to cancel the pending write.
 *
 * Promoted from mail, which is still the main caller: restoring a draft into a
 * compose window goes through here.
 */
export function setContentWhenReady(editor: EditorHandle, content: string): () => void {
    editor.setContent(content)
    return () => {}
}
