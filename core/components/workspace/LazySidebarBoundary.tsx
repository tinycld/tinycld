import { captureException } from '@tinycld/core/lib/errors'
import {
    Component,
    type ErrorInfo,
    type ReactNode,
    Suspense,
    useCallback,
    useEffect,
    useRef,
    useState,
} from 'react'
import { nextAttempt, shouldRetryOnTimeout } from './lazy-sidebar-retry'

// Copious logging around sidebar-mount failures: when this boundary has to
// recover, the next person debugging CI (or a user report) needs to see exactly
// what happened — which slug, which failure mode, which attempt — not a silent
// retry. `pkgSlug` is threaded in so every line names the package.
function logSidebar(slug: string | undefined, msg: string, extra?: Record<string, unknown>) {
    // __DEV__-guarded per house rule (no unguarded console). CI runs the dev
    // bundle (Development-level warnings: ON), so these lines DO surface in CI
    // failure logs — which is exactly where a wedged sidebar gets diagnosed.
    // Production reporting goes through captureException at the call sites.
    if (!__DEV__) return
    const tag = `[sidebar-boundary${slug ? `:${slug}` : ''}]`
    console.warn(tag, msg, extra ?? '')
}

interface LazySidebarBoundaryProps {
    /** Skeleton shown while the lazy chunk loads (Suspense fallback). */
    fallback: ReactNode
    /** The lazy sidebar subtree. */
    children: ReactNode
    /** Package slug, for log/Sentry context (which sidebar failed). */
    slug?: string
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
    { onError: (error: Error, info: ErrorInfo) => void; children: ReactNode },
    ErrorState
> {
    state: ErrorState = { hasError: false }

    static getDerivedStateFromError(): ErrorState {
        return { hasError: true }
    }

    componentDidCatch(error: Error, info: ErrorInfo) {
        this.props.onError(error, info)
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
    slug,
    stuckTimeoutMs = 15_000,
    maxRetries = 3,
}: LazySidebarBoundaryProps) {
    const [attempt, setAttempt] = useState(0)
    // Flipped true by the child once it actually mounts (see MountedSignal); the
    // watchdog only remounts while still false (i.e. still showing the skeleton).
    const mountedRef = useRef(false)

    const retry = useCallback(
        (reason: string) => {
            setAttempt(a => {
                const next = nextAttempt(a, maxRetries)
                if (next === a) {
                    // Cap hit: stop retrying. This is the user-visible broken
                    // state (sidebar stays a skeleton) — shout about it.
                    logSidebar(slug, `giving up after ${a} attempt(s) — ${reason}`, {
                        attempt: a,
                        maxRetries,
                    })
                    captureException('workspace.sidebar.mount-failed', new Error(reason), {
                        slug,
                        attempts: a,
                        maxRetries,
                    })
                } else {
                    logSidebar(slug, `retrying (attempt ${next}/${maxRetries}) — ${reason}`, {
                        from: a,
                        to: next,
                    })
                }
                return next
            })
        },
        [maxRetries, slug]
    )

    const onMounted = useCallback(() => {
        mountedRef.current = true
        if (attempt > 0) {
            logSidebar(slug, `recovered: sidebar mounted on attempt ${attempt}`, { attempt })
        }
    }, [attempt, slug])

    // Surface the actual error a rejecting lazy import threw, with React's
    // component stack — this is the single most useful artifact when a sidebar
    // chunk fails to load/evaluate.
    const onError = useCallback(
        (error: Error, info: ErrorInfo) => {
            logSidebar(slug, 'lazy sidebar threw while mounting', {
                error: String(error?.stack || error),
                componentStack: info?.componentStack,
                attempt,
            })
            captureException('workspace.sidebar.lazy-threw', error, {
                slug,
                attempt,
                componentStack: info?.componentStack,
            })
            retry(`import threw: ${error?.message ?? error}`)
        },
        [slug, attempt, retry]
    )

    useEffect(() => {
        mountedRef.current = false
        if (attempt >= maxRetries) return
        // Watchdog: if we're still in the fallback after the timeout, remount to
        // re-trigger the lazy import. Re-armed each attempt.
        const id = setTimeout(() => {
            if (shouldRetryOnTimeout(mountedRef.current, attempt, maxRetries)) {
                logSidebar(
                    slug,
                    `still on skeleton after ${stuckTimeoutMs}ms — chunk loaded but never committed; remounting`,
                    { attempt }
                )
                retry(`stuck on skeleton for ${stuckTimeoutMs}ms`)
            }
        }, stuckTimeoutMs)
        return () => clearTimeout(id)
    }, [attempt, maxRetries, stuckTimeoutMs, retry, slug])

    return (
        <SidebarErrorBoundary key={attempt} onError={onError}>
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
