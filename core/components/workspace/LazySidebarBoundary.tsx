import {
    Component,
    type ReactNode,
    Suspense,
    useCallback,
    useEffect,
    useRef,
    useState,
} from 'react'
import { nextAttempt, shouldRetryOnTimeout } from './lazy-sidebar-retry'

interface LazySidebarBoundaryProps {
    /** Skeleton shown while the lazy chunk loads (Suspense fallback). */
    fallback: ReactNode
    /** The lazy sidebar subtree. */
    children: ReactNode
    /** How long to wait in the fallback before forcing a remount (ms). */
    stuckTimeoutMs?: number
    /** Max remount attempts before giving up and surfacing the error/skeleton. */
    maxRetries?: number
}

interface ErrorState {
    hasError: boolean
}

/**
 * Internal error boundary: a lazy `import()` that REJECTS (Metro chunk fetch /
 * module-eval failure) makes React.lazy throw on render. Without a boundary that
 * error propagates and the sidebar is gone for the session. We catch it and ask
 * the parent to remount (which re-invokes the import).
 */
class SidebarErrorBoundary extends Component<
    { onError: () => void; children: ReactNode },
    ErrorState
> {
    state: ErrorState = { hasError: false }

    static getDerivedStateFromError(): ErrorState {
        return { hasError: true }
    }

    componentDidCatch() {
        this.props.onError()
    }

    render() {
        // While errored we render nothing; the parent's remount swaps in a fresh
        // subtree (new key) and resets this boundary with it.
        if (this.state.hasError) return null
        return this.props.children
    }
}

/**
 * Resilient wrapper for a lazily-loaded package sidebar.
 *
 * Two failure modes are handled, both observed under heavy CI contention where
 * the sidebar's Suspense boundary otherwise stays stuck on its skeleton forever:
 *
 *   1. The lazy `import()` REJECTS → SidebarErrorBoundary catches it and we
 *      remount to retry the import.
 *   2. The chunk loads (HTTP 200) but the boundary never commits the child — the
 *      Suspense sits in the fallback indefinitely. An error boundary can't see
 *      this (nothing threw), so a watchdog remounts the subtree if we're still
 *      showing the fallback after `stuckTimeoutMs`, re-triggering resolution.
 *
 * Each remount bumps `attempt`, which is the React key on the Suspense subtree —
 * a new key forces a fresh mount (and a fresh `import()` attempt). Retries are
 * capped so a genuinely-broken chunk doesn't loop forever.
 */
export function LazySidebarBoundary({
    fallback,
    children,
    stuckTimeoutMs = 15_000,
    maxRetries = 3,
}: LazySidebarBoundaryProps) {
    const [attempt, setAttempt] = useState(0)
    // Flipped true by the child once it actually mounts (see MountedSignal); the
    // watchdog only remounts while still false (i.e. still showing the skeleton).
    const mountedRef = useRef(false)

    const retry = useCallback(() => {
        setAttempt(a => nextAttempt(a, maxRetries))
    }, [maxRetries])

    const onMounted = useCallback(() => {
        mountedRef.current = true
    }, [])

    useEffect(() => {
        mountedRef.current = false
        if (attempt >= maxRetries) return
        // Watchdog: if we're still in the fallback after the timeout, remount to
        // re-trigger the lazy import. Re-armed each attempt.
        const id = setTimeout(() => {
            if (shouldRetryOnTimeout(mountedRef.current, attempt, maxRetries)) retry()
        }, stuckTimeoutMs)
        return () => clearTimeout(id)
    }, [attempt, maxRetries, stuckTimeoutMs, retry])

    return (
        <SidebarErrorBoundary key={attempt} onError={retry}>
            <Suspense fallback={fallback}>
                <MountedSignal onMounted={onMounted}>{children}</MountedSignal>
            </Suspense>
        </SidebarErrorBoundary>
    )
}

/**
 * Marks the lazy subtree as committed. Once Suspense unsuspends and this renders,
 * it flips the parent's mounted flag (in an effect, after commit) so the watchdog
 * stands down. Rendering only `children` keeps the tree otherwise unchanged.
 */
function MountedSignal({ onMounted, children }: { onMounted: () => void; children: ReactNode }) {
    useEffect(() => {
        onMounted()
    }, [onMounted])
    return <>{children}</>
}
