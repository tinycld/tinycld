import { Extension } from '@tiptap/core'
import { PluginKey } from '@tiptap/pm/state'
import Suggestion, { type SuggestionOptions } from '@tiptap/suggestion'

// Character-triggered autocomplete for the shared rich editor.
//
// A "trigger" is a character (`@`, `:`, `#`) that opens a picker mid-typing and
// inserts something when an entry is chosen. Cards' @mentions are the first
// consumer; the shape is deliberately generic because emoji and issue-links
// want exactly this and should not each grow their own plugin.
//
// WHY THIS LIVES IN CORE. text/ already has a working trigger framework for its
// slash menu, built on the same @tiptap/suggestion plugin and with both render
// strategies solved. It is not reusable: siblings must not depend on each other
// (CLAUDE.md), it is bound to text's own editor stack rather than
// useRichEditor, and — decisively — the NATIVE editor is a prebuilt WebView
// bundle compiled from core's source, so a package-side ProseMirror plugin
// cannot be injected into it at runtime at all. Any trigger that works on both
// platforms has to be compiled into core. text's slash menu is the proof this
// approach works; this is that idea, generalized and moved to where every
// package can reach it.
//
// RENDER STRATEGY. The plugin owns trigger detection, query parsing and range
// tracking. Where the popover actually renders is the consumer's problem, and
// it differs by platform: on web the popover is a DOM overlay positioned from
// `clientRect`; inside the native WebView there is no host UI, so the page
// posts `show-popover` over the message bus and the host renders it (the
// protocol lives in lib/editor/message-bus/popover-protocol.ts). Web therefore
// gets the default CALLBACK strategy — the caller supplies `onStateChange` —
// while the native page injects its own `render` (webview/source/
// trigger-render-bridge.ts). An injectable rather than a mode flag, so the
// bridge module (which reaches for `window.ReactNativeWebView`) never lands in
// the web bundle.
//
// WHY THE CONFIG IS DECLARATIVE. `allItems` + `insertTemplate` rather than
// `items(query)` + `toInsertText(item)` closures, because the native editor
// page is a PREBUILT BUNDLE: a closure cannot cross into it. The host pushes
// the candidate array over the bridge and the page filters locally, which also
// keeps typing off the WebView round-trip. Both platforms then run the SAME
// `filterTriggerItems`/`renderInsertTemplate`, so web and native cannot rank or
// insert differently — a divergence that would otherwise be invisible until a
// user compared their phone against their laptop.

/** One selectable entry in a trigger's popover. */
export interface TriggerItem {
    /** Stable identity — echoed back to `onSelect`. */
    id: string
    /** Primary line. */
    label: string
    /** Optional secondary line (an email, a role). Falsy values are ignored. */
    secondary?: string
}

/** Where the popover should point, in viewport coordinates. */
export interface TriggerAnchor {
    top: number
    left: number
    bottom: number
    right: number
    width: number
    height: number
}

/** What the popover needs to render itself. */
export interface TriggerState {
    isOpen: boolean
    /** The text typed after the trigger character. */
    query: string
    items: TriggerItem[]
    /** Index of the highlighted row; keyboard nav moves it. */
    selectedIndex: number
    anchor: TriggerAnchor | null
    /**
     * Commit an entry — what a POINTER click calls. Keyboard selection is
     * handled inside the plugin, which owns the keystrokes.
     *
     * This must be the plugin's live command rather than anything the popover
     * assembles itself: the trigger's text range moves as the query grows, and
     * only the plugin knows where it currently is. A popover that inserted text
     * on its own would leave the typed `@query` behind.
     */
    onSelect: (item: TriggerItem) => void
}

export const CLOSED_TRIGGER_STATE: TriggerState = {
    isOpen: false,
    query: '',
    items: [],
    selectedIndex: 0,
    anchor: null,
    onSelect: () => {},
}

/**
 * The part of a trigger that survives JSON — everything the native WebView
 * page needs to run the plugin itself. Kept separate from {@link TriggerConfig}
 * because the page receives exactly this and nothing more.
 */
export interface SerializableTriggerConfig {
    /** Distinguishes this trigger's plugin from any other on the editor. */
    id: string
    /** The character that opens it. */
    char: string
    /** The full candidate pool. Filtering happens per keystroke, locally. */
    allItems: TriggerItem[]
    /** How many rows to offer. Beyond this the user narrows by typing. */
    limit?: number
    /**
     * What replaces `<char><query>`, with `{id}` / `{label}` / `{secondary}`
     * substituted from the chosen item — e.g. `'[[@{id}]] '`. An empty result
     * removes the trigger text and inserts nothing.
     */
    insertTemplate: string
}

export interface TriggerConfig extends SerializableTriggerConfig {
    /** Notifies the consumer whenever the popover should change. */
    onStateChange: (state: TriggerState) => void
    /**
     * Overrides how the popover is presented. Absent → the callback strategy,
     * which is what web uses. The native page supplies a bridge that posts over
     * the message bus instead.
     */
    render?: NonNullable<SuggestionOptions<TriggerItem>['render']>
}

/** How many rows a trigger offers when its config does not say. */
const DEFAULT_TRIGGER_LIMIT = 6

/**
 * Narrow a candidate pool to a query, case-insensitively, across both lines.
 *
 * Shared deliberately: web calls it through {@link createTriggerExtension} and
 * the native page calls it directly, so the two cannot rank differently.
 */
export function filterTriggerItems(
    items: TriggerItem[],
    query: string,
    limit: number = DEFAULT_TRIGGER_LIMIT
): TriggerItem[] {
    const q = query.trim().toLowerCase()
    const matches = q
        ? items.filter(
              item =>
                  item.label.toLowerCase().includes(q) ||
                  (item.secondary?.toLowerCase().includes(q) ?? false)
          )
        : items
    return matches.slice(0, limit)
}

/**
 * Fill a trigger's insert template from the chosen item.
 *
 * Unknown placeholders are left verbatim rather than blanked: a template is
 * author-supplied, and silently swallowing a typo would produce a token that
 * looks right in the source and parses as nothing downstream.
 */
export function renderInsertTemplate(template: string, item: TriggerItem): string {
    return template.replace(/\{(id|label|secondary)\}/g, (whole, field: string) => {
        const value = item[field as keyof TriggerItem]
        return typeof value === 'string' ? value : whole
    })
}

/**
 * The plugin key for a trigger id.
 *
 * Stable per id, because the native bridge needs the SAME instance the plugin
 * registered under in order to call `exitSuggestion` when the host dismisses.
 */
const pluginKeys = new Map<string, PluginKey>()
export function triggerPluginKey(id: string): PluginKey {
    const existing = pluginKeys.get(id)
    if (existing) return existing
    const created = new PluginKey(`tinycldTrigger:${id}`)
    pluginKeys.set(id, created)
    return created
}

// Translate the plugin's DOMRect into the serializable anchor shape. A DOMRect
// is a live object whose values mutate between frames, so it must not be held.
function toAnchor(rect: DOMRect | null | undefined): TriggerAnchor | null {
    if (!rect) return null
    return {
        top: rect.top,
        left: rect.left,
        bottom: rect.bottom,
        right: rect.right,
        width: rect.width,
        height: rect.height,
    }
}

/**
 * Build the ProseMirror extension for one trigger.
 *
 * Keyboard handling lives here rather than in the popover because the editor
 * owns the keystrokes: the popover is not focused, so ArrowUp/Down/Enter/Escape
 * would otherwise move the text caret instead of the selection.
 */
export function createTriggerExtension(config: TriggerConfig): Extension {
    const pluginKey = triggerPluginKey(config.id)

    return Extension.create({
        name: `tinycldTrigger_${config.id}`,

        addProseMirrorPlugins() {
            // Selection index is owned here — it survives `onUpdate` (the list
            // re-filters as the query grows) and resets when the popover opens.
            let selectedIndex = 0
            let items: TriggerItem[] = []
            let query = ''
            let anchor: TriggerAnchor | null = null

            // The plugin rebuilds `command` on every transaction with an
            // updated range. Holding the one from `onStart` would insert at the
            // ORIGINAL trigger position, dropping whatever was typed after it.
            let currentCommand: ((item: TriggerItem) => void) | null = null

            const select = (item: TriggerItem | undefined) => {
                if (!item) return false
                currentCommand?.(item)
                return true
            }

            const publish = (isOpen: boolean) => {
                config.onStateChange(
                    isOpen
                        ? {
                              isOpen,
                              query,
                              items,
                              selectedIndex,
                              anchor,
                              onSelect: item => {
                                  select(item)
                              },
                          }
                        : CLOSED_TRIGGER_STATE
                )
            }

            return [
                Suggestion<TriggerItem>({
                    editor: this.editor,
                    pluginKey,
                    char: config.char,
                    // Fire mid-line too, not only at the start of a block.
                    startOfLine: false,
                    // A mention query is one token; allowing spaces would keep
                    // the popover open across the rest of the sentence.
                    allowSpaces: false,
                    items: ({ query: q }) => filterTriggerItems(config.allItems, q, config.limit),
                    command: ({ editor, range, props }) => {
                        const text = renderInsertTemplate(config.insertTemplate, props)
                        const chain = editor.chain().focus().deleteRange(range)
                        if (text) chain.insertContent(text)
                        chain.run()
                    },
                    render:
                        config.render ??
                        (() => ({
                            onStart: props => {
                                currentCommand = props.command
                                items = props.items
                                query = props.query
                                selectedIndex = 0
                                anchor = toAnchor(props.clientRect?.())
                                publish(true)
                            },
                            onUpdate: props => {
                                currentCommand = props.command
                                items = props.items
                                query = props.query
                                // Clamp rather than reset: re-filtering must not
                                // throw the highlight back to the top on every key.
                                if (selectedIndex >= items.length) {
                                    selectedIndex = Math.max(0, items.length - 1)
                                }
                                anchor = toAnchor(props.clientRect?.())
                                publish(true)
                            },
                            onKeyDown: ({ event }) => {
                                if (items.length === 0) return false
                                switch (event.key) {
                                    case 'ArrowDown':
                                        selectedIndex = (selectedIndex + 1) % items.length
                                        publish(true)
                                        event.preventDefault()
                                        return true
                                    case 'ArrowUp':
                                        selectedIndex =
                                            (selectedIndex - 1 + items.length) % items.length
                                        publish(true)
                                        event.preventDefault()
                                        return true
                                    case 'Enter':
                                    case 'Tab':
                                        if (!select(items[selectedIndex])) return false
                                        event.preventDefault()
                                        return true
                                    case 'Escape':
                                        publish(false)
                                        event.preventDefault()
                                        return true
                                    default:
                                        return false
                                }
                            },
                            onExit: () => {
                                currentCommand = null
                                items = []
                                query = ''
                                anchor = null
                                publish(false)
                            },
                        })),
                }),
            ]
        },
    })
}
