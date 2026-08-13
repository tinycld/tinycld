import type { exitSuggestion, SuggestionOptions } from '@tiptap/suggestion'
import {
    type PopoverRect,
    type PopoverResultPayload,
    type TriggerPopoverSelection,
    triggerPopoverKind,
    UI_POPOVER_EXITED,
    UI_POPOVER_RESULT,
    UI_POPOVER_UPDATE,
    UI_SHOW_POPOVER,
} from '../../../message-bus/popover-protocol'
import { type SerializableTriggerConfig, type TriggerItem, triggerPluginKey } from '../../triggers'

/**
 * The bridge render strategy for a trigger, running INSIDE the WebView page.
 *
 * The page has no host UI of its own, so instead of drawing a popover it posts
 * `show-popover` and lets the native side render one, answering over
 * `popover-result`. Selections and dismissals arrive on a 'message' listener
 * held for the lifetime of the open popover.
 *
 * What stays in the page, deliberately: item filtering, the selected index, and
 * the insert itself. Only the page holds the suggestion plugin's live `range`,
 * which moves as the query grows — a host that assembled its own insert would
 * write at the ORIGINAL trigger position and leave the typed query behind. The
 * host's job is to draw pixels and report taps.
 *
 * Keyboard handling stays here too: the overlay is never focused, so a hardware
 * or Android soft keyboard's arrows would otherwise move the text caret.
 *
 * Deps are injected so the wire format can be asserted in a unit test without a
 * device — nothing else on this path is testable, which is exactly why it is
 * worth the indirection.
 */

/**
 * Post a 'ui' message out of the WebView. Silently no-ops on the host, where
 * `window.ReactNativeWebView` is undefined.
 */
export function defaultPostToHost(message: object) {
    const target = (
        globalThis as { window?: { ReactNativeWebView?: { postMessage: (s: string) => void } } }
    ).window
    target?.ReactNativeWebView?.postMessage(JSON.stringify(message))
}

/**
 * Correlation id for a show-popover. `crypto.randomUUID` is not guaranteed in
 * the WebView's content world; a timestamp plus a random tail disambiguates
 * plenty within one editing session.
 */
export function defaultNewRequestId(triggerId: string): string {
    return `${triggerId}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`
}

/** Translate the plugin's live DOMRect into the serializable wire shape. */
export function toPopoverRect(rect: DOMRect | null | undefined): PopoverRect | null {
    if (!rect) return null
    return {
        top: rect.top,
        left: rect.left,
        width: rect.width,
        height: rect.height,
        scrollX: typeof window === 'undefined' ? 0 : window.scrollX,
        scrollY: typeof window === 'undefined' ? 0 : window.scrollY,
    }
}

export interface TriggerBridgeDeps {
    postToHost: (message: object) => void
    newRequestId: (triggerId: string) => string
    exitSuggestion: typeof exitSuggestion
    /**
     * Which editor this page is. Rides on every posted message so a host with
     * several editors mounted answers only its own — see
     * `ShowPopoverPayload.editorInstanceId`.
     */
    editorInstanceId?: string
}

export function createTriggerBridgeRender(
    config: SerializableTriggerConfig,
    deps: TriggerBridgeDeps
): NonNullable<SuggestionOptions<TriggerItem>['render']> {
    const kind = triggerPopoverKind(config.id)
    // The same instance the plugin registered under — exitSuggestion resolves
    // its state by key, so a fresh PluginKey here would silently find nothing.
    const pluginKey = triggerPluginKey(config.id)

    return () => {
        let currentRequestId: string | null = null
        // `SuggestionProps.command` is the WRAPPED form — the plugin captures
        // editor and range internally and hands back a closure taking only the
        // item. It is rebuilt every transaction, so holding the one from
        // onStart would insert at a stale position.
        let currentCommand: ((item: TriggerItem) => void) | null = null
        let currentItems: TriggerItem[] = []
        let currentQuery = ''
        let selectedIndex = 0
        let editorView: import('@tiptap/pm/view').EditorView | null = null

        const dismissPlugin = () => {
            if (!editorView) return
            try {
                deps.exitSuggestion(editorView, pluginKey)
            } catch {
                // The plugin state can already be cleared by an intervening
                // transaction; there is nothing left to exit.
            }
        }

        const post = (type: string, payload: unknown) => {
            if (!currentRequestId) return
            deps.postToHost({
                namespace: 'ui',
                type,
                requestId: currentRequestId,
                payload,
            })
        }

        const postUpdate = () => {
            post(UI_POPOVER_UPDATE, {
                payload: { items: currentItems, query: currentQuery, selectedIndex },
                editorInstanceId: deps.editorInstanceId,
            })
        }

        const listen = (add: boolean) => {
            if (typeof window === 'undefined') return
            // Android delivers on `document`, iOS on `window`. Both, always.
            if (add) {
                window.addEventListener('message', onHostMessage)
                document.addEventListener('message', onHostMessage as EventListener)
            } else {
                window.removeEventListener('message', onHostMessage)
                document.removeEventListener('message', onHostMessage as EventListener)
            }
        }

        function onHostMessage(evt: MessageEvent) {
            if (typeof evt.data !== 'string') return
            let parsed: { namespace?: string; type?: string; requestId?: string; payload?: unknown }
            try {
                parsed = JSON.parse(evt.data)
            } catch {
                return
            }
            if (parsed.namespace !== 'ui' || parsed.type !== UI_POPOVER_RESULT) return
            if (!currentRequestId || parsed.requestId !== currentRequestId) return

            const payload = parsed.payload as
                | PopoverResultPayload<TriggerPopoverSelection>
                | null
                | undefined
            if (payload?.action === 'select') {
                const picked = currentItems.find(item => item.id === payload.payload?.itemId)
                if (picked) currentCommand?.(picked)
                currentRequestId = null
            } else if (payload?.action === 'dismiss') {
                dismissPlugin()
                currentRequestId = null
            }
        }

        return {
            onStart: props => {
                currentCommand = props.command
                currentItems = props.items
                currentQuery = props.query
                selectedIndex = 0
                editorView = props.editor.view

                const rect = toPopoverRect(props.clientRect?.())
                // No anchor means nowhere to put the overlay. Staying silent
                // beats asking the host to draw at (0,0).
                if (!rect) return

                currentRequestId = deps.newRequestId(config.id)
                listen(true)
                post(UI_SHOW_POPOVER, {
                    kind,
                    rect,
                    payload: { items: currentItems, query: currentQuery, selectedIndex },
                    editorInstanceId: deps.editorInstanceId,
                })
            },

            onUpdate: props => {
                currentCommand = props.command
                currentItems = props.items
                currentQuery = props.query
                // Clamp rather than reset: re-filtering as the query grows must
                // not throw the highlight back to the top on every keystroke.
                if (selectedIndex >= currentItems.length) {
                    selectedIndex = Math.max(0, currentItems.length - 1)
                }
                postUpdate()
            },

            onKeyDown: ({ event }) => {
                if (!currentRequestId) return false
                switch (event.key) {
                    case 'ArrowDown':
                        if (currentItems.length === 0) return false
                        selectedIndex = (selectedIndex + 1) % currentItems.length
                        postUpdate()
                        event.preventDefault()
                        return true
                    case 'ArrowUp':
                        if (currentItems.length === 0) return false
                        selectedIndex =
                            (selectedIndex - 1 + currentItems.length) % currentItems.length
                        postUpdate()
                        event.preventDefault()
                        return true
                    case 'Enter':
                    case 'Tab': {
                        const item = currentItems[selectedIndex]
                        if (!item) return false
                        currentCommand?.(item)
                        event.preventDefault()
                        return true
                    }
                    case 'Escape':
                        dismissPlugin()
                        event.preventDefault()
                        return true
                    default:
                        return false
                }
            },

            onExit: () => {
                // The plugin wound down on its own — a space broke the trigger,
                // an item was picked, Escape. Tell the host so it closes any
                // overlay still open for this request.
                post(UI_POPOVER_EXITED, { editorInstanceId: deps.editorInstanceId })
                listen(false)
                currentRequestId = null
                currentCommand = null
                currentItems = []
                currentQuery = ''
                selectedIndex = 0
                editorView = null
            },
        }
    }
}
