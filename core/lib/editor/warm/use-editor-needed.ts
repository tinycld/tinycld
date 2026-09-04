import { useEffect } from 'react'
import { useEditorSingleton } from './editor-singleton'

/**
 * Declare that this section may edit, so the one app-wide editor boots.
 *
 * A DECLARATION, not a mount: the caller gets nothing back and renders nothing.
 * A package layout calls it to say "someone in here might start editing", and
 * the singleton above the route tree — which outlives this section — does the
 * rest. That indirection is the point. A package that MOUNTED its own host, as
 * boards did, destroyed and re-booted the editor every time the user left the
 * section and came back.
 *
 * Idempotent, and it never disposes: the first call flips the latch and every
 * later call from any package is a no-op. Nothing here tears the editor down on
 * unmount.
 *
 * In an effect rather than during render because it flips shared state other
 * components subscribe to.
 */
export function useEditorNeeded(): void {
    const singleton = useEditorSingleton()
    const declareNeed = singleton?.declareNeed
    useEffect(() => {
        declareNeed?.()
    }, [declareNeed])
}
