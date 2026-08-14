import { useEffect, useReducer, useState } from 'react'
import { Dimensions, Platform, StyleSheet, View } from 'react-native'
import { FullWindowOverlay } from 'react-native-screens'
import { UI_POPOVER_RESULT } from '../message-bus/popover-protocol'
import { makeMessage } from '../message-bus/types'
import {
    type AnchoredOverlayAction,
    type AnchoredOverlayRequest,
    type AnchoredOverlayResponseAction,
    anchoredOverlayReducer,
    decodeUiMessage,
    initialAnchoredOverlayState,
    type PopoverPosition,
    resolvePopoverPosition,
} from './anchored-overlay-state'
import { subscribeUiMessage } from './ui-message-bus'

// Vertical gap between the anchor's bottom and the popover. Mirrors the
// web's POPOVER_GAP_PX so the two platforms feel visually consistent.
const POPOVER_GAP_PX = 4
// Estimated popover height for the flip-above-the-caret decision. The
// web variant uses the same value; we don't try to do a measure-then-
// reflow pass on native because the flicker would be more disruptive
// than a slightly-imprecise flip threshold.
const POPOVER_HEIGHT_ESTIMATE_PX = 320
const POPOVER_WIDTH_PX = 260

// Minimal duck-typed shape we use off the WebView ref. The TenTap
// webviewRef points at the underlying react-native-webview instance,
// which exposes both .measure (a RN View method) and .postMessage
// (an RN-WebView method). We don't depend on the full type — every
// callsite narrows on `typeof ... === 'function'` before invoking.
interface WebViewMeasurable {
    measure(
        cb: (
            x: number,
            y: number,
            width: number,
            height: number,
            pageX: number,
            pageY: number
        ) => void
    ): void
    /**
     * The PRIMARY measurement, despite the name order here.
     *
     * It reports WINDOW coordinates, which is the space the popover is
     * positioned in — `measure` reports parent-relative ones, so an editor low
     * on the screen anchored its popover far above the caret. It is also the
     * one that fires at all for a WebView under the New Architecture
     * (Bridgeless), where `measure` silently never invokes its callback.
     */
    measureInWindow(cb: (x: number, y: number, width: number, height: number) => void): void
    postMessage(message: string): void
}

// Subset of a WebView ref that the controller needs.
type WebViewRef = React.RefObject<unknown> | null

// Props accepted by every overlay component the registry knows about.
// payload comes verbatim from the WebView's show-popover.payload (kind-
// scoped); respond closes the popover by posting popover-result back.
//
// The controller hands the body a `respond` closure rather than
// surfacing the protocol; each body just calls respond('select', ...)
// or respond('dismiss') on user input. Future overlay kinds plug in
// the same way.
export interface AnchoredOverlayProps {
    payload: unknown
    respond: (action: AnchoredOverlayResponseAction, payload?: unknown) => void
}

export interface AnchoredOverlayRegistry {
    [kind: string]: (props: AnchoredOverlayProps) => React.ReactNode
}

export interface AnchoredOverlayControllerProps {
    webViewRef: WebViewRef
    /**
     * The view to measure for positioning. Falls back to `webViewRef`, which
     * only works on the old architecture — under Bridgeless the WebView ref
     * exposes no measurement methods, so a caller that wants a popover to
     * actually appear must pass the host view wrapping the editor.
     */
    measureRef?: WebViewRef
    registry: AnchoredOverlayRegistry
    /**
     * Which editor's popovers this controller answers. Omitted → it answers
     * every editor's, which is only safe when exactly one is mounted. Pass it
     * whenever a screen can carry two (a card detail carries three).
     */
    editorInstanceId?: string
}

// Promise wrapper around .measure(). Resolves to null when the ref is
// missing, the current value isn't a measurable, or measure returns
// without invoking the callback (the RN runtime can drop measure calls
// when a view isn't laid out yet). On null the controller fails closed —
// dismisses the request and posts popover-result so the WebView clears
// its trigger — rather than rendering the popover at (0, 0) with no
// relationship to the caret.
async function measureWebView(
    ref: WebViewRef
): Promise<{ pageX: number; pageY: number; width: number; height: number } | null> {
    // measureInWindow FIRST, and it is the one that is actually correct here.
    //
    // The popover is positioned in screen space, and only measureInWindow
    // reports screen coordinates. `measure`'s pageX/pageY are relative to the
    // view's PARENT, which for an editor low on the screen — the comment
    // composer pinned at the bottom of a card — is a far smaller Y than the
    // caret's real position, so the popover drew hundreds of points above the
    // text it belonged to.
    //
    // `measure` stays as the fallback rather than being dropped: it is the one
    // that answers when the view is not yet attached to a window.
    const inWindow = await runMeasure(ref, 'measureInWindow')
    if (inWindow && inWindow.width > 0) return inWindow
    return runMeasure(ref, 'measure')
}

/**
 * Run one of the two measurement methods, resolving null if it does not answer.
 *
 * Both are callback-based and neither reports failure, so the timeout is the
 * only way to notice that a call was dropped — which is the normal outcome for
 * `measure` on a WebView under Bridgeless.
 */
function runMeasure(
    ref: WebViewRef,
    method: 'measure' | 'measureInWindow'
): Promise<{ pageX: number; pageY: number; width: number; height: number } | null> {
    const r = ref?.current as Partial<WebViewMeasurable> | null | undefined
    if (!r || typeof r[method] !== 'function') return Promise.resolve(null)
    return new Promise(resolve => {
        let resolved = false
        const settle = (
            value: { pageX: number; pageY: number; width: number; height: number } | null
        ) => {
            if (resolved) return
            resolved = true
            clearTimeout(fallback)
            resolve(value)
        }
        const fallback = setTimeout(() => settle(null), 250)
        try {
            if (method === 'measureInWindow') {
                ;(r as WebViewMeasurable).measureInWindow((x, y, width, height) => {
                    settle({ pageX: x, pageY: y, width, height })
                })
            } else {
                ;(r as WebViewMeasurable).measure((_x, _y, width, height, pageX, pageY) => {
                    settle({ pageX, pageY, width, height })
                })
            }
        } catch {
            settle(null)
        }
    })
}

// Send a 'ui' namespace message into the WebView. Used to deliver
// popover-result responses (and any future host→WebView UI message).
// Safe to call when the ref is detached — the message is silently
// dropped because the WebView is gone, which matches the "fire and
// forget" semantics of the bus.
function postUiToWebView(ref: WebViewRef, type: string, payload: unknown, requestId?: string) {
    const r = ref?.current as Partial<WebViewMeasurable> | null | undefined
    if (!r || typeof r.postMessage !== 'function') return
    try {
        ;(r as WebViewMeasurable).postMessage(
            JSON.stringify(makeMessage('ui', type, payload, requestId))
        )
    } catch {
        // postMessage throws if the WebView's content world hasn't
        // initialized yet (very early in the mount, before the bridge
        // is up). Swallow — the popover protocol is best-effort.
    }
}

// AnchoredOverlayController — host-side overlay router. Subscribes to
// the ui-message-bus, opens an absolutely-positioned Modal with the
// kind-specific body, and responds back to the WebView. Web mounts of
// this component are no-ops (we render through a portal there, not a
// Modal), which keeps screen-level mounting platform-agnostic.
export function AnchoredOverlayController({
    webViewRef,
    measureRef,
    registry,
    editorInstanceId,
}: AnchoredOverlayControllerProps): React.ReactElement | null {
    const [state, dispatch] = useReducer(anchoredOverlayReducer, initialAnchoredOverlayState)
    const [screenPos, setScreenPos] = useState<PopoverPosition | null>(null)

    // Subscribe to the bus so 'show-popover' / 'popover-update' /
    // 'popover-dismiss-on-scroll' / 'popover-exited' messages get
    // reduced into state. The native variant of useDocumentEditor is
    // responsible for publishing every 'ui' message into the bus; we
    // just consume.
    // Scoped to this editor: several editors can be mounted at once, and an
    // unscoped subscription would answer their popovers too — see
    // ui-message-bus.ts.
    useEffect(() => {
        const unsubscribe = subscribeUiMessage(message => {
            const action: AnchoredOverlayAction | null = decodeUiMessage(message)
            if (action) dispatch(action)
        }, editorInstanceId)
        return unsubscribe
    }, [editorInstanceId])

    // On show, .measure() the WebView to translate viewport coords to
    // screen coords, then compute the popover anchor. We re-run only on
    // a fresh request (requestId change) so popover-update messages
    // don't trigger a remeasure — the anchor is fixed once the popover
    // opens, and an in-flight scroll already dismisses.
    //
    // webViewRef is intentionally omitted from the dep array: the
    // useEditorBridge hook returns a stable RefObject whose identity
    // doesn't change across renders. Including it would invite a
    // re-measure on a hypothetical ref swap that the surrounding code
    // doesn't actually perform.
    // biome-ignore lint/correctness/useExhaustiveDependencies: intentional narrow dep — remeasure only on a fresh requestId (not on popover-update); webViewRef is a stable RefObject (see comment above)
    useEffect(() => {
        if (!state.open) {
            setScreenPos(null)
            return
        }
        let cancelled = false
        const open = state.open
        ;(async () => {
            const m = await measureWebView(measureRef ?? webViewRef)
            if (cancelled) return
            if (!m) {
                // Measure failed (no ref, timed out, threw). Falling
                // back to (0, 0) would render the popover in the
                // top-left of the screen with no visual relationship
                // to the caret — worse than showing nothing. Fail
                // closed: dismiss the request and tell the WebView we
                // gave up so the suggestion plugin clears its trigger.
                postUiToWebView(
                    webViewRef,
                    'popover-result',
                    { action: 'dismiss' as AnchoredOverlayResponseAction },
                    open.requestId
                )
                dispatch({ type: 'dismiss-external' })
                return
            }
            const { width: vw, height: vh } = Dimensions.get('window')
            const pos = resolvePopoverPosition({
                rect: open.rect,
                webViewOriginX: m.pageX,
                webViewOriginY: m.pageY,
                viewportWidth: vw,
                viewportHeight: vh,
                popoverWidth: POPOVER_WIDTH_PX,
                popoverHeightEstimate: POPOVER_HEIGHT_ESTIMATE_PX,
                gap: POPOVER_GAP_PX,
            })
            setScreenPos(pos)
        })()
        return () => {
            cancelled = true
        }
    }, [state.open?.requestId])

    if (Platform.OS === 'web') return null
    if (!state.open || !screenPos) return null

    const open: AnchoredOverlayRequest = state.open
    const Body = registry[open.kind]
    if (!Body) return null

    const respond = (action: AnchoredOverlayResponseAction, payload?: unknown) => {
        postUiToWebView(
            webViewRef,
            UI_POPOVER_RESULT,
            { action, payload: payload ?? undefined },
            open.requestId
        )
        dispatch({ type: 'respond', requestId: open.requestId })
    }

    // Deliberately NOT a <Modal>. On iOS a Modal is a separate window that
    // takes first responder the moment it appears, which blurs the WebView and
    // dismisses the soft keyboard — so the caret vanished mid-word and the user
    // could not keep typing to narrow the list, which is the whole point of an
    // autocomplete. (Worse, with the keyboard gone the editor no longer held
    // the keys, so on iOS the next letter reached the global shortcut matcher.)
    //
    // An absolutely-positioned sibling in the SAME window has no first-responder
    // of its own, so the WebView keeps focus and the keyboard stays up. The
    // coordinates are already screen-space — resolvePopoverPosition translated
    // them through the WebView's measured origin — so the layer this renders
    // into must also be screen-space: it is pinned to the full window rather
    // than laid out in flow, which is what `position: absolute` with all four
    // insets at 0 gives us inside a parent that does not clip.
    //
    // pointerEvents="box-none" on the wrapper is load-bearing: it lets touches
    // pass through the transparent full-screen layer to the editor underneath,
    // so tapping outside the popover still moves the caret rather than being
    // swallowed by an invisible backdrop.
    return (
        <ScreenOverlayLayer>
            {/* There is no backdrop catcher on purpose. Tapping away moves the
                caret in the still-focused editor, which breaks the trigger and
                makes the page post popover-exited — the overlay closes on its
                own. A full-screen catcher would have to swallow that tap to
                report it, so the tap would dismiss the popover WITHOUT moving
                the caret, and the user would have to tap twice. */}
            {/* Exactly one of top/bottom is set: below the caret the popover
                hangs from its top edge, flipped above it the BOTTOM edge is
                pinned to the caret instead. Anchoring the flipped case by its
                bottom is what lets the box be whatever height it turns out to
                be — the estimate below only caps it and decides which way to
                open, and is never used to compute a position. */}
            <View
                style={{
                    position: 'absolute',
                    ...(screenPos.top === undefined ? {} : { top: screenPos.top }),
                    ...(screenPos.bottom === undefined ? {} : { bottom: screenPos.bottom }),
                    left: screenPos.left,
                    width: POPOVER_WIDTH_PX,
                    maxHeight: POPOVER_HEIGHT_ESTIMATE_PX,
                }}
            >
                <Body payload={open.payload} respond={respond} />
            </View>
        </ScreenOverlayLayer>
    )
}

/**
 * The screen-space layer the popover is positioned into.
 *
 * The coordinates handed to it are absolute screen coordinates
 * (resolvePopoverPosition already translated them through the WebView's
 * measured origin), so this layer must span the window and must not be clipped
 * or scrolled by whatever subtree the controller happens to be mounted in — in
 * a card detail that is a `View` several levels inside a `ScrollView`.
 *
 * On iOS `FullWindowOverlay` gives exactly that: a sibling UIView in the window
 * above the app's content. Crucially it is NOT a UIWindow and has no first
 * responder of its own, so — unlike the `Modal` this replaced — presenting it
 * does not blur the WebView or dismiss the keyboard, and the user can keep
 * typing to filter.
 *
 * Elsewhere it is a plain absolutely-positioned layer. Android has no
 * equivalent primitive (FullWindowOverlay warns and degrades to a View there),
 * so the popover relies on its ancestors staying overflow-visible; it is
 * `box-none` so the transparent layer never swallows a touch meant for the
 * editor beneath it.
 */
function ScreenOverlayLayer({ children }: { children: React.ReactNode }) {
    if (Platform.OS === 'ios') {
        return <FullWindowOverlay>{children}</FullWindowOverlay>
    }
    return (
        <View style={StyleSheet.absoluteFill} pointerEvents="box-none">
            {children}
        </View>
    )
}
