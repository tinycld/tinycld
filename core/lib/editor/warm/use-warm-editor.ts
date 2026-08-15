import { useCallback, useEffect, useRef, useSyncExternalStore } from 'react'
import type { UseRichEditorOptions } from '../rich/options'
import { useEditorSingleton } from './editor-singleton'
import type { WarmEditorLease } from './types'
import type { SurfaceId, WarmSnapshot } from './warm-editor-store'

const EMPTY: WarmSnapshot = { holder: null, generation: 0, ready: false }
const noopSubscribe = () => () => {}
const emptySnapshot = () => EMPTY

/**
 * Lease the app's single editor for one surface.
 *
 * ONE file for both platforms. The previous `.web` variant was a stub that
 * reported cold and handed back a no-op acquire, so every handover branch —
 * the staleness guard, the refocus, the draft stash — was dead code on the only
 * platform CI runs. Web and native now take the identical path.
 *
 * `result` is non-null only while this surface both HOLDS the instance and that
 * instance is usable. A holder whose editor is still booting reads as null, so
 * "is there an editor to render" is a single null check rather than a second
 * condition every call site could forget.
 */
export function useWarmEditor(
    surfaceId: SurfaceId,
    options: UseRichEditorOptions
): WarmEditorLease {
    const singleton = useEditorSingleton()
    const optionsRef = useRef(options)
    optionsRef.current = options

    const snapshot = useSyncExternalStore(
        singleton?.store.subscribe ?? noopSubscribe,
        singleton?.store.getSnapshot ?? emptySnapshot
    )

    // Keyed on the STORE, not the whole context value. The value's identity
    // changes on every editor rebuild (it carries `result`), and acquiring
    // bumps the generation, which rebuilds the editor — so deriving these from
    // the value would make `acquire` change identity as a RESULT of acquiring,
    // and an effect depending on it would spin. The store and setOptions are
    // both stable for the provider's lifetime, which is what these actually
    // need.
    const store = singleton?.store
    const setOptions = singleton?.setOptions

    const acquire = useCallback(() => {
        if (!store || !setOptions) return
        setOptions(optionsRef.current)
        store.acquire(surfaceId)
    }, [store, setOptions, surfaceId])

    const release = useCallback(() => {
        store?.release(surfaceId)
    }, [store, surfaceId])

    // A surface can be unmounted while still holding the instance (the card
    // closes mid-edit). Without this the store would keep a holder that no
    // longer exists and the editor would never park.
    //
    // Empty deps, reading through a ref, because this must fire on UNMOUNT and
    // nothing else. Depending on `release` re-runs the effect whenever its
    // identity changes — which it does on every handover, and on the boot that
    // swaps the context's null editor for the real one — and the cleanup then
    // releases a lease the surface still legitimately holds. That is what left
    // a just-opened composer holding nothing: it acquired, the boot changed the
    // context, and the stale cleanup immediately took the instance back.
    const releaseRef = useRef(release)
    releaseRef.current = release
    useEffect(() => () => releaseRef.current(), [])

    const isHolder = singleton != null && snapshot.holder === surfaceId
    return {
        isWarm: singleton != null,
        ready: snapshot.ready,
        holder: snapshot.holder,
        acquire,
        release,
        // Gated on readiness as well as holding: acquiring during the boot is
        // ordinary (the user taps as the section opens), and handing back a
        // half-built editor is what would render a dead box.
        result: isHolder && snapshot.ready ? singleton.result : null,
        generation: snapshot.generation,
    }
}
