import { mergeAttributes, Node, nodeInputRule } from '@tiptap/core'
import { Plugin, PluginKey } from '@tiptap/pm/state'

/** The attributes a mention node carries. */
interface MentionAttrs {
    userId: string
    name: string | null
}

// The `[[@<userId>|<name>]]` mention token, as an editor node.
//
// The token is the WIRE format: the ID is what cards' Go flush hook parses to
// derive description mentions, and being id-based is what lets a mention
// survive the person renaming themselves. What it must NOT be is what someone
// reads while typing — before this node existed the picker inserted the token
// as literal text, so choosing a colleague put `[[@9rsya4ylg4y2vkp]]` in the
// middle of the sentence.
//
// So: id in the document, name on the screen. The node holds both, renders the
// label, and serializes back to the token.
//
// The NAME half is a display fallback, and it is why a mention stays readable
// when the id cannot be resolved — the person left the board, or the roster has
// not loaded yet. Leaving a board does not un-say the sentence that named you.
// The live roster still wins when it has an answer, so a rename is reflected
// without rewriting stored documents. The name is optional on the wire, so the
// bare `[[@id]]` in already-stored descriptions keeps working.
//
// It is an ATOM. A mention is one indivisible thing — arrow keys step over it,
// backspace removes the whole mention rather than eating the name a letter at a
// time, and no one can edit the label into disagreeing with the id it carries.
//
// Defined in the shared builder, so the schema is identical on web and inside
// the WebView bundle. A node present on one platform and missing on the other
// silently drops its attributes when a document crosses between them, which for
// a mention would mean losing the id and with it the notification.

export interface MentionNodeOptions {
    /**
     * Which trigger's roster names these mentions.
     *
     * A trigger id rather than a resolver function, deliberately. The native
     * editor is a PREBUILT WebView bundle, so a closure cannot cross into it —
     * the same constraint that made TriggerConfig declarative. The roster is
     * already pushed into the page and kept current there, so both platforms
     * look the label up in that one store and cannot disagree about it.
     */
    triggerId?: string
    HTMLAttributes?: Record<string, unknown>
}

/** Shown when an id resolves to nobody. Mirrors renderMentionTokens'. */
const UNKNOWN_LABEL = 'someone'

/**
 * The roster a mention's label comes from, registered per trigger id.
 *
 * A module-global map for the same reason the page's trigger-items store is
 * one: `renderHTML` runs inside a ProseMirror transaction rather than a React
 * render, so it needs the current roster synchronously and has no component to
 * subscribe with. Both platforms populate this from the same roster they
 * already push to the picker, so the name in the document and the name in the
 * list cannot disagree.
 */
const rosters = new Map<string, Map<string, string>>()

/**
 * Editors waiting to redraw when a roster changes.
 *
 * The roster almost always arrives AFTER the document: on native it is pushed
 * over the bridge once the page is ready, and on web it comes from a live query
 * that resolves after mount. By then every mention has already rendered — with
 * an empty roster, so all of them say "@someone" — and ProseMirror will not
 * redraw a node whose attributes have not changed. The label is derived, not
 * stored, so nothing about the node ever does change and the stale text stays
 * on screen for the life of the mount. These listeners are what close that gap.
 */
const rosterListeners = new Set<() => void>()

/** Subscribe to roster changes. Returns the unsubscribe function. */
export function onMentionLabelsChange(listener: () => void): () => void {
    rosterListeners.add(listener)
    return () => {
        rosterListeners.delete(listener)
    }
}

/** Publish the id→label map a trigger's mentions should render with. */
export function setMentionLabels(triggerId: string, items: { id: string; label: string }[]): void {
    const next = new Map(items.map(item => [item.id, item.label]))
    const prev = rosters.get(triggerId)
    // Skip the redraw when nothing a mention renders actually changed — the
    // roster re-emits on unrelated writes, and forcing a redraw per emission
    // would fight the caret.
    if (prev && prev.size === next.size && [...next].every(([k, v]) => prev.get(k) === v)) {
        return
    }
    rosters.set(triggerId, next)
    for (const listener of rosterListeners) listener()
}

/**
 * The name to show for a mention.
 *
 * The roster wins when it has the id — it is live, so a rename shows up
 * immediately. The name CARRIED BY THE TOKEN is the fallback, which is what
 * makes a mention readable when the roster cannot answer: the person left the
 * board, the id belongs to another org's user, or (on native) the roster simply
 * has not arrived yet. Leaving the board does not un-say the sentence someone
 * wrote, so their name stays legible either way.
 *
 * UNKNOWN_LABEL is now a genuine last resort — only a token with no name in it
 * at all, which means a pre-existing `[[@id]]` written before the format
 * carried one.
 */
function lookupLabel(triggerId: string | undefined, userId: string, fallbackName?: string): string {
    const label = triggerId ? rosters.get(triggerId)?.get(userId) : undefined
    return label || fallbackName || UNKNOWN_LABEL
}

/** Test-only. The roster map is module-global and would otherwise leak. */
export function resetMentionLabels(): void {
    rosters.clear()
}

/**
 * Matches the token in markdown being parsed INTO the editor.
 *
 * The wire format is `[[@<userId>|<name>]]`, and the name half is OPTIONAL so
 * the bare `[[@id]]` written before it existed still parses — those documents
 * are already stored and must keep working.
 *
 * Carrying the name is what keeps a mention readable when the id cannot be
 * resolved: the person left the board, or the roster has not arrived yet. The id
 * stays the identity — it drives notifications and survives a rename — and the
 * name is only a fallback for display, so a stale one is corrected the moment
 * the live roster answers.
 *
 * The name may not contain `]` or `|`, which is what keeps the token
 * unambiguous; the writer percent-encodes them (see escapeMentionName).
 *
 * Accepts the backslash-escaped spelling too. The editor serializes through
 * markdown, where `[` is syntax, so a stored token round-trips as `\[\[@id\]\]`
 * — matching only the bare form would leave escaped tokens as visible text,
 * which is the same trap cards' own TOKEN regex documents.
 */
const TOKEN_PATTERN = /\\?\[\\?\[@([A-Za-z0-9_-]+)(?:\|([^\]|]*))?\\?\]\\?\]/

export const MentionNode = Node.create<MentionNodeOptions>({
    name: 'tinycldMention',
    group: 'inline',
    inline: true,
    atom: true,
    selectable: true,
    // Nothing inside to edit — the label is derived from the id, not stored.
    draggable: false,

    addOptions() {
        return { triggerId: undefined, HTMLAttributes: {} }
    },

    addStorage() {
        return { unsubscribeRoster: undefined as (() => void) | undefined }
    },

    addAttributes() {
        return {
            userId: {
                default: null,
                parseHTML: element => element.getAttribute('data-mention-id'),
                renderHTML: attributes =>
                    attributes.userId ? { 'data-mention-id': attributes.userId } : {},
            },
            // The name as it stood when the mention was written. Display only —
            // the roster overrides it whenever it can, so a rename is reflected
            // without rewriting stored documents. Kept on the node so it
            // survives into the serialized token.
            name: {
                default: null,
                parseHTML: element => element.getAttribute('data-mention-name'),
                renderHTML: attributes =>
                    attributes.name ? { 'data-mention-name': attributes.name } : {},
            },
        }
    },

    parseHTML() {
        return [{ tag: 'span[data-mention-id]' }]
    },

    renderHTML({ node, HTMLAttributes }) {
        const userId = String(node.attrs.userId ?? '')
        const name = node.attrs.name ? String(node.attrs.name) : undefined
        const label = lookupLabel(this.options.triggerId, userId, name)
        return [
            'span',
            mergeAttributes(
                { 'data-mention-id': userId, class: 'tinycld-mention' },
                this.options.HTMLAttributes ?? {},
                HTMLAttributes
            ),
            `@${label}`,
        ]
    },

    // What the editor shows for the node in a plain-text context (copy, the
    // character counter). The name, not the id — the id is not prose.
    renderText({ node }) {
        const name = node.attrs.name ? String(node.attrs.name) : undefined
        return `@${lookupLabel(this.options.triggerId, String(node.attrs.userId ?? ''), name)}`
    },

    /**
     * Redraw this editor's mentions when the roster changes.
     *
     * A no-op transaction is enough: ProseMirror re-renders the nodes it
     * touches, and `renderHTML` reads the label fresh from the roster. Cheap
     * because setMentionLabels only notifies on an actual change.
     *
     * `setMeta('addToHistory', false)` keeps this out of the undo stack — a
     * roster arriving is not an edit the user made, and without it their first
     * undo would be spent on our redraw.
     */
    onCreate() {
        this.storage.unsubscribeRoster = onMentionLabelsChange(() => {
            const { view } = this.editor
            if (!view || view.isDestroyed) return
            view.dispatch(view.state.tr.setMeta('addToHistory', false))
        })
    },

    onDestroy() {
        ;(this.storage.unsubscribeRoster as (() => void) | undefined)?.()
    },

    /**
     * Turn a literal token into a node as it is typed or pasted.
     *
     * This is what makes EXISTING descriptions readable. Stored bodies are full
     * of `[[@id]]`, and they reach the editor by several routes — setContent on
     * open, a Yjs update from a collaborator, a paste — so converting at any one
     * load point would leave the others showing raw tokens. An input rule runs
     * wherever the text appears, so all of them are covered by one rule.
     */
    addInputRules() {
        return [
            nodeInputRule({
                find: new RegExp(`${TOKEN_PATTERN.source}$`),
                type: this.type,
                getAttributes: match => ({
                    userId: match[1],
                    name: unescapeMentionName(match[2]),
                }),
            }),
        ]
    },

    /**
     * Convert token TEXT that arrives in the document into mention nodes.
     *
     * The input rule above only fires while typing, so it covers a mention
     * someone writes and nothing that was LOADED — an existing comment opened
     * showing the raw `[[@id|Name]]`. This plugin closes that gap wherever the
     * text comes from: setContent on open, a Yjs update from a collaborator, a
     * paste.
     *
     * It works on the document rather than on the markdown string, which is the
     * important part. Rewriting the source into HTML before parsing seemed
     * simpler and was actively dangerous: the markdown parsers on the two
     * platforms treat an inline `<span>` differently, so on native the markup
     * survived as literal text, and the next blur SAVED it — turning a comment
     * into `Hi @admin@admin&lt;/span&gt;`. Never round-trip user content through
     * a second syntax to solve a parsing problem.
     */
    addProseMirrorPlugins() {
        const type = this.type
        return [
            new Plugin({
                key: new PluginKey(`${this.name}_textToNode`),
                appendTransaction(_transactions, _oldState, newState) {
                    const replacements: { from: number; to: number; attrs: MentionAttrs }[] = []
                    newState.doc.descendants((node, pos) => {
                        if (!node.isText || !node.text) return
                        const pattern = new RegExp(TOKEN_PATTERN.source, 'g')
                        let match = pattern.exec(node.text)
                        while (match !== null) {
                            replacements.push({
                                from: pos + match.index,
                                to: pos + match.index + match[0].length,
                                attrs: {
                                    userId: match[1],
                                    name: unescapeMentionName(match[2]),
                                },
                            })
                            match = pattern.exec(node.text)
                        }
                    })
                    if (replacements.length === 0) return null

                    const tr = newState.tr
                    // Applied back-to-front so each replacement's positions are
                    // still valid — an earlier one would shift everything after it.
                    for (let i = replacements.length - 1; i >= 0; i--) {
                        const { from, to, attrs } = replacements[i]
                        tr.replaceWith(from, to, type.create(attrs))
                    }
                    // Not an edit the user made: keeping it out of the undo
                    // stack stops their first undo being spent on our rewrite.
                    return tr.setMeta('addToHistory', false)
                },
            }),
        ]
    },

    /**
     * Serialize back to the wire token.
     *
     * This is the half that makes a mention PERSIST. The editor stores markdown,
     * and an atom node the serializer does not recognize contributes nothing —
     * so without this the mention renders correctly, and then silently vanishes
     * the moment the description round-trips through markdown. (It has to be
     * `renderMarkdown` on the extension config: @tiptap/markdown reads
     * `markdownName` / `parseMarkdown` / `renderMarkdown` off the config, not
     * out of `addStorage`.)
     */
    renderMarkdown(node: { attrs?: Record<string, unknown> }) {
        return serializeMentionToken(
            String(node.attrs?.userId ?? ''),
            node.attrs?.name ? String(node.attrs.name) : undefined
        )
    },
})

/**
 * Build the stored token for a mention.
 *
 * The name rides along so the mention stays readable when the roster cannot
 * name the id — see TOKEN_PATTERN. It is omitted entirely when unknown, which
 * produces the original `[[@id]]` spelling and keeps the two formats one.
 */
export function serializeMentionToken(userId: string, name?: string): string {
    const trimmed = name?.trim()
    if (!trimmed) return `[[@${userId}]]`
    return `[[@${userId}|${escapeMentionName(trimmed)}]]`
}

/**
 * `]` and `|` would end the token early, so they are PERCENT-encoded.
 *
 * Not backslash-escaped, which is the obvious choice and does not survive:
 * markdown owns the backslash, so the parser strips it before the token is ever
 * matched — `a\]b` arrives as `a]b`, the bracket then closes the token, and the
 * rest of the name spills into the document as visible text. Percent-encoding
 * passes through markdown untouched.
 *
 * `%` itself is encoded first so decoding cannot turn a literal `%5D` a user
 * typed into a bracket.
 */
function escapeMentionName(name: string): string {
    return name.replace(/%/g, '%25').replace(/\|/g, '%7C').replace(/\]/g, '%5D')
}

function unescapeMentionName(raw: string | undefined): string | null {
    if (!raw) return null
    return raw.replace(/%5D/gi, ']').replace(/%7C/gi, '|').replace(/%25/gi, '%') || null
}

/**
 * Rewrite the wire tokens in a markdown string into the HTML the node parses.
 *
 * Content reaches the editor as markdown, and `[[@id]]` means nothing to a
 * markdown parser — it would arrive as literal text and the node would never be
 * created. Converting first is what makes an existing description open with its
 * mentions already rendered as names.
 */
export function mentionTokensToHtml(markdown: string, triggerId?: string): string {
    if (!markdown || markdown.indexOf('[@') === -1) return markdown
    return markdown.replace(
        new RegExp(TOKEN_PATTERN, 'g'),
        (_match, userId: string, rawName: string | undefined) => {
            const name = unescapeMentionName(rawName)
            const label = lookupLabel(triggerId, userId, name ?? undefined)
            const nameAttr = name ? ` data-mention-name="${escapeHtmlAttr(name)}"` : ''
            return `<span data-mention-id="${userId}"${nameAttr}>@${escapeHtmlAttr(label)}</span>`
        }
    )
}

/** The name is user-controlled, so it must not be able to close the attribute. */
function escapeHtmlAttr(value: string): string {
    return value
        .replace(/&/g, '&amp;')
        .replace(/"/g, '&quot;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
}
