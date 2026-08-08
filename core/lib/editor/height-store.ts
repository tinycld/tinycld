/**
 * A one-value store for the height the in-WebView editor reports for itself.
 *
 * Exists so a measurement re-renders ONLY the box wrapping the WebView. The
 * obvious `useState` inside useWebViewEditor is what this replaces: it changed
 * the memoized `EditorComponent`'s IDENTITY, and consumers render that as
 * `<EditorComponent />`, so every measurement remounted the WebView — which
 * reset its viewport to `minHeight`, producing another measurement. On a
 * device the editor thrashed between 72px and its real height and never
 * settled.
 *
 * Lives in its own module rather than beside the hook so it can be unit-tested
 * without pulling in the TenTap/react-native-webview require chain.
 */
export interface HeightStore {
    get: () => number | null
    set: (height: number) => void
    subscribe: (listener: () => void) => () => void
}

/**
 * Sub-pixel changes are ignored. A re-render that moves the height by a
 * fraction re-lays-out the WebView, which re-measures and reports again; the
 * threshold is what makes the loop converge. Real content changes (a new
 * paragraph, a wrapped line) move it by far more than this.
 */
const SETTLE_THRESHOLD_PX = 2

export function createHeightStore(): HeightStore {
    let height: number | null = null
    const listeners = new Set<() => void>()
    return {
        get: () => height,
        set: (next: number) => {
            if (height !== null && Math.abs(next - height) < SETTLE_THRESHOLD_PX) return
            height = next
            for (const listener of listeners) listener()
        },
        subscribe: (listener: () => void) => {
            listeners.add(listener)
            return () => listeners.delete(listener)
        },
    }
}
