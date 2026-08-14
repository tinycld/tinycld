import { useCallback, useEffect, useRef, useSyncExternalStore } from 'react'
import type { UseRichEditorOptions } from '../rich/options'
import type { WarmEditorLease } from './types'
import { useWarmContext } from './WarmEditorHost.native'
import type { SurfaceId, WarmSnapshot } from './warm-editor-store'

const EMPTY: WarmSnapshot = { holder: null, generation: 0 }
const noopSubscribe = () => () => {}
const emptySnapshot = () => EMPTY

/**
 * Lease the package's single warm editor for one surface.
 *
 * `isWarm` false means no host is mounted; the consumer must mount its own
 * editor and pay the cold start. Warm is an optimization, never a correctness
 * dependency — a fault here degrades to the previous behavior, not a broken
 * editor.
 */
export function useWarmEditor(
    surfaceId: SurfaceId,
    options: UseRichEditorOptions
): WarmEditorLease {
    const warm = useWarmContext()
    const optionsRef = useRef(options)
    optionsRef.current = options

    const snapshot = useSyncExternalStore(
        warm?.store.subscribe ?? noopSubscribe,
        warm?.store.getSnapshot ?? emptySnapshot
    )

    const acquire = useCallback(() => {
        if (!warm) return
        warm.setOptions(optionsRef.current)
        warm.store.acquire(surfaceId)
    }, [warm, surfaceId])

    const release = useCallback(() => {
        warm?.store.release(surfaceId)
    }, [warm, surfaceId])

    // A surface can be unmounted while still holding the instance (the card
    // closes mid-edit). Without this the store would keep a holder that no
    // longer exists and the editor would never park.
    useEffect(() => release, [release])

    const isHolder = warm != null && snapshot.holder === surfaceId
    return {
        isWarm: warm != null,
        acquire,
        release,
        result: isHolder ? warm.result : null,
    }
}
