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
 * Yjs document update, base64-encoded. Sent in BOTH directions: the host
 * relays what arrives on the room socket, and the WebView relays what the
 * local user types.
 *
 * The WebView never opens its own connection. text/ does that today and the
 * second connection makes the local user appear twice in presence
 * (TODO(text-native v1.1)) as well as shipping a credential into the page.
 */
export const YJS_UPDATE = 'update'

/**
 * WebView → host, on the 'ui' namespace: the document's height in CSS px.
 *
 * A WebView has no intrinsic height, so the host has to be told. Inside a
 * ScrollView there is nothing to flex against either, which leaves the editor
 * clipped to whatever the host guessed — or collapsed to zero. Posted on mount
 * and whenever the content resizes.
 *
 * The host matches this string literally (like `document-scroll`), since
 * use-webview-editor is package-agnostic and does not import from `rich/`.
 */
export const UI_CONTENT_HEIGHT = 'content-height'

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
    /** Present iff this editor is collaborative. Absent → a local editor. */
    collab?: RichEditorInitCollab
}

/**
 * The collaboration binding, handed to the page at init.
 *
 * Everything here is a primitive or a plain object: it crosses a JSON pipe, so
 * the Y.Doc itself cannot. The page builds its OWN Y.Doc from `initialState`
 * and keeps it in sync by relaying updates through the host.
 */
export interface RichEditorInitCollab {
    /** Which top-level fragment this editor owns, e.g. `card:<id>`. */
    field: string
    /**
     * The host doc's clientID, for correlation only — the page does NOT adopt
     * it.
     *
     * Adopting it was the original plan (TODO(cards M9): "the WebView reuses
     * the native client's clientID so the local user does not appear twice").
     * Yjs forbids it: two docs sharing a clientID would collide on item
     * identity, and it defends itself — assigning the id and then applying the
     * host's state logs "Changed the client-id because another client seems to
     * be using it" and reassigns a random one. It sticks only while the host
     * doc is empty, which is exactly the case that doesn't matter.
     *
     * Double presence is avoided a different way, and the reason it works is
     * that this relay carries DOCUMENT UPDATES ONLY. The page's Awareness is
     * local to the WebView and drives just the carets rendered in it; board
     * presence stays on the host's single socket, which is the only thing
     * peers ever see. Two clientIDs, one avatar.
     */
    clientID: number
    /**
     * Base64 `Y.encodeStateAsUpdate` of the host doc at init.
     *
     * The page applies this instead of `setContent`. Under collaboration the
     * document arrives as Yjs state, and setting content on top of it would
     * duplicate the text on every client that joins.
     */
    initialState: string
    /** Caret identity — same {id,name,color} shape presence publishes. */
    user?: { id: string; name: string; color: string }
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

export interface YjsUpdatePayload {
    /** Base64 of a Yjs update — see encodeUpdate/decodeUpdate below. */
    update: string
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
