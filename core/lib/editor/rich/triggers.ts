import { Extension } from '@tiptap/core'
import { PluginKey } from '@tiptap/pm/state'
import Suggestion from '@tiptap/suggestion'

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
// protocol is already specified in lib/editor/message-bus/types.ts). This
// module therefore takes CALLBACKS rather than rendering anything — the caller
// supplies `onStateChange`, and web/native each answer it their own way.

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

export interface TriggerConfig {
    /** Distinguishes this trigger's plugin from any other on the editor. */
    id: string
    /** The character that opens it. */
    char: string
    /** Candidates for the current query. Called on every keystroke. */
    items: (query: string) => TriggerItem[]
    /**
     * The text to put in the document in place of `<char><query>`. Returning ''
     * removes the trigger text and inserts nothing.
     */
    toInsertText: (item: TriggerItem) => string
    /** Notifies the consumer whenever the popover should change. */
    onStateChange: (state: TriggerState) => void
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
    const pluginKey = new PluginKey(`tinycldTrigger:${config.id}`)

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
                    items: ({ query: q }) => config.items(q),
                    command: ({ editor, range, props }) => {
                        const text = config.toInsertText(props)
                        const chain = editor.chain().focus().deleteRange(range)
                        if (text) chain.insertContent(text)
                        chain.run()
                    },
                    render: () => ({
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
                    }),
                }),
            ]
        },
    })
}
