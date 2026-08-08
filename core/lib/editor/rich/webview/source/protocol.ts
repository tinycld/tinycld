/**
 * Wire protocol between the native host and the rich editor's WebView page.
 *
 * Imported by both sides, so it must stay free of DOM and React Native
 * references — that is also what makes it unit-testable without a device,
 * which matters here because nothing else on this path can be.
 *
 * Message envelopes are the shared `EditorMessage` from lib/editor/message-bus.
 * This file names the types carried on the 'markdown' and 'app' namespaces and
 * types their payloads.
 */
import type { EditorContentFormat } from '../../options'

/** host → WebView. Replaces the document. */
export const MARKDOWN_SET = 'set'
/** host → WebView, carries a requestId. */
export const MARKDOWN_GET = 'get'
/** WebView → host, echoes the get's requestId. */
export const MARKDOWN_RESULT = 'result'

/** WebView → host, posted once the page mounts, before Tiptap is constructed. */
export const EDITOR_READY = 'editor-ready'
/** host → WebView, the one-shot init payload. */
export const APP_INIT = 'init'
/** WebView → host, ⌘/Ctrl+Enter inside the editor. */
export const APP_SUBMIT_SHORTCUT = 'submit-shortcut'
/** WebView → host, Escape inside the editor. */
export const APP_ESCAPE = 'escape'

/**
 * Everything the WebView needs to construct its editor.
 *
 * Sent once per mount, after the page reports ready. The page cannot build its
 * Tiptap instance before this arrives: the extension set depends on the
 * placeholder, character limit, and (later) the collaboration binding.
 */
export interface RichEditorInitPayload {
    /**
     * How `initialContent` is interpreted. Markdown is the native format for
     * card descriptions; mail passes 'html' and never touches the markdown
     * channel.
     */
    contentFormat: EditorContentFormat
    initialContent: string
    placeholder: string
    editable: boolean
    /** Hard ceiling; typing stops here rather than failing on save. */
    characterLimit?: number
    /** Theme colors resolved on the native side, applied as CSS in-page. */
    colors: RichEditorColors
    autofocus: boolean
}

export interface RichEditorColors {
    bg: string
    fg: string
    placeholder: string
    primary: string
    /** Caret + selection color. Falls back to `primary` when absent. */
    accent?: string
}

export interface MarkdownSetPayload {
    markdown: string
}

export interface MarkdownResultPayload {
    markdown: string
}

/**
 * State the WebView broadcasts on every meaningful transaction.
 *
 * Posted under TenTap's own `stateUpdate` message type rather than a namespace
 * of ours, deliberately: that is what `useBridgeState` listens for, so
 * `deriveToolbarState` keeps working unchanged.
 */
export interface RichEditorStatePayload {
    isBoldActive: boolean
    isItalicActive: boolean
    isUnderlineActive: boolean
    isStrikeActive: boolean
    isBulletListActive: boolean
    isOrderedListActive: boolean
    isTaskListActive: boolean
    isBlockquoteActive: boolean
    isCodeActive: boolean
    isCodeBlockActive: boolean
    isLinkActive: boolean
    activeLink: string | null
    activeHeadingLevel: number | null
    isInTable: boolean
    selectionEmpty: boolean
    isEmpty: boolean
    characterCount: number
    wordCount: number
    canUndo: boolean
    canRedo: boolean
    /** TenTap's readiness flag. Consumers gate on it via EditorResult.isReady. */
    isReady: boolean
}

/**
 * Base64 helpers for the reserved 'yjs' namespace.
 *
 * Yjs updates are `Uint8Array` and this channel is a JSON string pipe, so
 * updates have to be encoded. They live here, tested, so that turning
 * collaboration on later is additive rather than a protocol change.
 *
 * `btoa`/`atob` are avoided: they exist in the WebView but not in every host
 * JS runtime, and this module is imported by both sides.
 */
const BASE64_ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'

export function encodeUpdate(bytes: Uint8Array): string {
    let out = ''
    for (let i = 0; i < bytes.length; i += 3) {
        const b0 = bytes[i] as number
        const b1 = bytes[i + 1]
        const b2 = bytes[i + 2]
        out += BASE64_ALPHABET[b0 >> 2]
        out += BASE64_ALPHABET[((b0 & 0x03) << 4) | ((b1 ?? 0) >> 4)]
        out += b1 === undefined ? '=' : BASE64_ALPHABET[((b1 & 0x0f) << 2) | ((b2 ?? 0) >> 6)]
        out += b2 === undefined ? '=' : BASE64_ALPHABET[b2 & 0x3f]
    }
    return out
}

export function decodeUpdate(encoded: string): Uint8Array {
    const clean = encoded.replace(/=+$/, '')
    const bytes = new Uint8Array((clean.length * 3) >> 2)
    let acc = 0
    let bits = 0
    let out = 0
    for (const char of clean) {
        const value = BASE64_ALPHABET.indexOf(char)
        if (value < 0) continue
        acc = (acc << 6) | value
        bits += 6
        if (bits >= 8) {
            bits -= 8
            bytes[out++] = (acc >> bits) & 0xff
        }
    }
    return bytes
}
