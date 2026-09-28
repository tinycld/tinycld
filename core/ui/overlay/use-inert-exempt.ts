import { useCallback, useLayoutEffect, useRef } from 'react'
import { Platform, type View } from 'react-native'
import { exemptFromInert } from './inert-siblings'

/**
 * Keeps a surface interactive while a modal layer holds the rest of the app
 * inert.
 *
 * For an always-on-top surface that renders IN PLACE rather than through an
 * overlay host — the toast renderer and the offline overlay, which are
 * absolutely positioned siblings of the app. A surface that portals into a
 * host needs nothing: the host is already protected.
 *
 * Spread the returned ref onto the surface's outermost View.
 */
export function useInertExempt() {
    const cleanupRef = useRef<(() => void) | null>(null)

    const ref = useCallback((node: View | null) => {
        cleanupRef.current?.()
        cleanupRef.current =
            Platform.OS === 'web' ? exemptFromInert(node as unknown as HTMLElement | null) : null
    }, [])

    useLayoutEffect(() => () => cleanupRef.current?.(), [])

    return ref
}
