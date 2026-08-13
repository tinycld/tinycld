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
import type { SerializableTriggerConfig, TriggerItem } from '../../triggers'

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
 * host → WebView: the server base URL and a fresh PocketBase file token.
 *
 * The page has no PocketBase client and must never hold a credential beyond
 * this — but an image in a description points at a PROTECTED file (the stored
 * src is tokenless, see rich/authed-image.ts), so without a token every <img>
 * in the WebView 404s. The host owns the token lifecycle (useFileToken, ~55min
 * cache) and re-posts on every rotation; init carries the same shape for the
 * common case where the token exists before the handshake.
 */
export const APP_FILE_TOKEN = 'file-auth'

/**
 * host → WebView: replace one trigger's candidate pool.
 *
 * The page holds a SNAPSHOT of the candidates rather than asking per keystroke.
 * A round-trip per character would cross the bridge on a JS thread already
 * carrying the Yjs relay, and would need sequence numbers to stop a late `@na`
 * response rendering under a typed `@nath`. The cost is that a member added
 * while the popover is open appears on the next push rather than the next
 * keystroke — bounded, and invisible at human speed.
 */
export const APP_TRIGGER_ITEMS = 'trigger-items'

/**
 * Yjs document update, base64-encoded. Sent in BOTH directions: the host
 * relays what arrives on the room socket, and the WebView relays what the
 * local user types.
 *
 * The WebView never opens its own connection: a second one would make the local
 * user appear twice in presence, as well as shipping a credential into the page.
 */
export const YJS_UPDATE = 'update'

/**
 * Page → host, on the 'awareness' namespace: the page's OWN cursor position, as
 * the JSON relative-position pair y-tiptap keeps in its awareness slot — or null
 * when it has none (blur, or the editor going away).
 *
 * Deliberately NOT an encoded awareness update. The page's clientID must never
 * reach the wire: the realtime client only ever encodes its own slot
 * (`realtime/client.ts`, `encodeAwarenessUpdate(awareness, [localID])`), and the
 * broker announces one awareness id per connection, so a second slot would
 * strand a ghost caret on every peer until y-protocols' reaper cleared it. The
 * host merges this cursor into its own slot instead — one peer, one avatar, one
 * caret.
 *
 * A relative position survives the trip because it names ITEMS IN THE DOCUMENT,
 * which are identical across every replica. No translation is needed.
 */
export const AWARENESS_CURSOR = 'cursor'

/**
 * Host → page, on the 'awareness' namespace: the REMOTE peers' awareness states,
 * base64 of `encodeAwarenessUpdate` over their client ids.
 *
 * The host's own slot is excluded. It carries the cursor the page just sent, and
 * relaying it back would arrive under a different clientID than the page's own —
 * so y-tiptap's "don't draw my own caret" filter would not catch it and the user
 * would watch a ghost caret with their own name trail their typing.
 */
export const AWARENESS_PEERS = 'peers'

/**
 * Host → page: peers who left, so the page drops their carets.
 *
 * Separate from `AWARENESS_PEERS` because a departed client is usually gone from
 * the awareness `meta` map too, and `encodeAwarenessUpdate` reads
 * `meta.get(clientID).clock` with no guard — encoding one throws.
 */
export const AWARENESS_LEAVE = 'leave'

/** Payload for {@link AWARENESS_CURSOR}. */
export interface AwarenessCursorPayload {
    /** `{anchor, head}` relative positions as JSON, or null for no cursor. */
    cursor: { anchor: unknown; head: unknown } | null
}

/** Payload for {@link AWARENESS_PEERS}. */
export interface AwarenessPeersPayload {
    /** Base64 `encodeAwarenessUpdate(awareness, remoteIDs)`. */
    update: string
}

/** Payload for {@link AWARENESS_LEAVE}. */
export interface AwarenessLeavePayload {
    clientIDs: number[]
}

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
    /**
     * File-token seed for protected images, when the host already holds one at
     * handshake time. Later rotations arrive as {@link APP_FILE_TOKEN}.
     */
    fileAuth?: RichEditorFileAuth
    /**
     * Character-triggered autocompletes, in their serializable form. Absent →
     * the page installs none. Rosters are refreshed by {@link APP_TRIGGER_ITEMS}.
     */
    triggers?: SerializableTriggerConfig[]
    /**
     * Identifies this editor among any others mounted on the same screen, so a
     * host overlay can tell whose popover it is drawing. See
     * {@link ShowPopoverPayload.editorInstanceId}.
     */
    editorInstanceId?: string
}

/** Payload for {@link APP_TRIGGER_ITEMS}. */
export interface TriggerItemsPayload {
    triggerId: string
    items: TriggerItem[]
}

/** Payload for {@link APP_FILE_TOKEN} and `RichEditorInitPayload.fileAuth`. */
export interface RichEditorFileAuth {
    /** The server origin image srcs resolve against (host-side `pb.baseURL`). */
    baseURL: string
    /** Short-lived per-user PocketBase file token. */
    token: string
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
     * Double presence is avoided a different way: the page's clientID never
     * reaches the wire. The relay carries document updates, remote peers'
     * awareness states inbound, and the page's cursor POSITION outbound — and
     * the host merges that cursor into its own awareness slot rather than
     * opening a second one. Board presence therefore still rides the host's
     * single socket, which is the only thing peers ever see.
     *
     * Two clientIDs on the phone, one on the wire: one avatar, one caret.
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
    /**
     * Base64 `encodeAwarenessUpdate` of the REMOTE peers' slots at init, absent
     * when nobody else is in the room.
     *
     * Awareness frames are only fanned out as they are sent, so a joining client
     * is told nothing about who is already present — the same gap cards' presence
     * hook closes by republishing when someone arrives. Without this seed a phone
     * would see no carets until a peer happened to move.
     */
    peers?: string
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
    /**
     * Whether the editing surface itself holds focus. The host turns changes in
     * this into the onFocus/onBlur callbacks, so focus-gated chrome behaves the
     * same on native as on web.
     */
    isFocused: boolean
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
