import type { EditorMessage } from '../message-bus/types'

/**
 * A tiny pub/sub routing 'ui' namespace messages from a WebView editor to
 * whatever host code draws overlays for it. The native editor hook publishes
 * everything `useWebViewEditor`'s `onUiMessage` hands it; the anchored-overlay
 * controller subscribes.
 *
 * Not a Zustand store: the bus has no state, only routing. A store would force
 * every subscriber onto a React render path, and this one is also driven from
 * unit tests with no component in sight.
 *
 * SUBSCRIBERS FILTER BY EDITOR INSTANCE, which is the one thing this does that
 * a naive fan-out must not. More than one editor is routinely mounted at once —
 * a card detail carries a description editor, a comment composer, and an inline
 * comment editor while a comment is being edited. Every one of them mounts its
 * own controller, so an unfiltered bus would have all three answer a single
 * `@`: three overlays, two of them measured against the wrong WebView. A
 * message naming no instance is a broadcast (dismiss-on-scroll is screen-wide
 * by nature) and still reaches everyone.
 */
type Handler = (message: EditorMessage) => void

interface Subscription {
    handler: Handler
    /** Undefined → receives every message, filtered or not. */
    instanceId?: string
}

const subscriptions = new Set<Subscription>()

/**
 * Read the editor instance a message belongs to, if it names one.
 *
 * The id rides inside the payload rather than on the envelope because the
 * envelope is TenTap's shared shape — widening it would touch every namespace
 * for the benefit of one.
 */
function messageInstanceId(message: EditorMessage): string | undefined {
    const payload = message.payload as { editorInstanceId?: unknown } | null | undefined
    const id = payload?.editorInstanceId
    return typeof id === 'string' ? id : undefined
}

/**
 * Add a handler, optionally scoped to one editor instance. The returned
 * function removes it. Idempotent against double-unsubscribe.
 */
export function subscribeUiMessage(handler: Handler, instanceId?: string): () => void {
    const subscription: Subscription = { handler, instanceId }
    subscriptions.add(subscription)
    return () => {
        subscriptions.delete(subscription)
    }
}

/**
 * Fan a message out to every subscriber it is addressed to.
 *
 * A throwing handler does not starve the others — this is a router, not a
 * chain — but the error is re-raised afterwards so a test or error boundary
 * still sees it.
 */
export function publishUiMessage(message: EditorMessage): void {
    const target = messageInstanceId(message)
    let thrown: unknown = null
    let didThrow = false
    for (const { handler, instanceId } of subscriptions) {
        // A scoped subscriber ignores other editors' traffic. An unaddressed
        // message is a broadcast and reaches everyone.
        if (target !== undefined && instanceId !== undefined && instanceId !== target) continue
        try {
            handler(message)
        } catch (err) {
            thrown = err
            didThrow = true
        }
    }
    if (didThrow) throw thrown
}

/**
 * Test-only. The bus is module-global, so a suite that publishes into it leaks
 * handlers across files without this in a beforeEach.
 */
export function resetUiMessageBus(): void {
    subscriptions.clear()
}
