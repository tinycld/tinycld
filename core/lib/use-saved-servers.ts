import { router } from 'expo-router'
import { useCallback, useEffect, useState } from 'react'
import { captureException } from './errors'
import { navigateToOrgUrl } from './org-url'
import { isReloadAvailable, ReloadUnavailableError } from './reload-js-context'
import { forgetServer } from './remove-server'
import { getResolvedAddress } from './server-address'
import {
    canSwitchInPlace,
    isSameServer,
    isSavedServersSupported,
    readServers,
    type SavedServer,
} from './servers'
import { switchToServer } from './switch-server'

interface SavedServersState {
    servers: SavedServer[]
    activeOrigin: string | null
    // The origin of a row with an operation in flight, so its row shows a
    // spinner and every other control is disabled — a switch restarts the JS
    // context, so a second concurrent action would be acting on a doomed graph.
    busyOrigin: string | null
    error: string | null
    // False when this build cannot restart the JS context (dev, web, a binary
    // without reload support), so a surface can warn BEFORE the user taps rather
    // than only afterwards. A switch is still worth attempting: it persists the
    // active pointer, so a manual restart completes it.
    canReload: boolean
    add: () => void
    switchTo: (origin: string) => void
    remove: (origin: string) => void
}

// seedServers gives the first render the one server we can know synchronously.
//
// `activeOrigin` is a sync module read while the list arrives from AsyncStorage a
// frame or two later, so a length-gated surface would pop in on every open. The
// seed is not a lie: trimToCap deliberately reserves the active server's slot, so
// it is always in the stored list — this just under-counts until the read lands,
// and the section grows rather than appearing from nothing.
function seedServers(activeOrigin: string | null): SavedServer[] {
    if (!activeOrigin) return []
    return [{ origin: activeOrigin, label: labelForOrigin(activeOrigin), addedAt: 0 }]
}

// withActive guarantees the active server appears in the list exactly once,
// prepending it when the stored list does not already know about it.
function withActive(saved: SavedServer[], activeOrigin: string | null): SavedServer[] {
    if (!activeOrigin) return saved
    if (saved.some(s => isSameServer(s.origin, activeOrigin))) return saved
    return [...seedServers(activeOrigin), ...saved]
}

function labelForOrigin(origin: string): string {
    try {
        return new URL(origin).host
    } catch {
        return origin
    }
}

// useSavedServers owns the saved-server list and the add/switch/remove actions,
// keeping consumers' JSX to layout only.
//
// The list is read once on mount and after a removal rather than subscribed to:
// the only other way it changes is a switch, which restarts the JS context and
// re-runs this from scratch.
//
export function useSavedServers(): SavedServersState {
    const supported = isSavedServersSupported()
    const activeOrigin = getResolvedAddress()
    const [servers, setServers] = useState<SavedServer[]>(() =>
        supported ? seedServers(activeOrigin) : []
    )
    const [busyOrigin, setBusyOrigin] = useState<string | null>(null)
    const [error, setError] = useState<string | null>(null)

    useEffect(() => {
        if (!supported) return
        let cancelled = false
        readServers().then(saved => {
            if (cancelled) return
            // Never let the stored list drop the server we are ON. Web reaches
            // this with an empty list — nothing there ever calls setActiveServer,
            // since the address is window.location.origin — and replacing the
            // seed with [] would blank the switcher on the one origin it can
            // always describe. Native normally finds itself already present
            // (setActiveServer writes both halves), so this is a no-op there.
            setServers(withActive(saved, activeOrigin))
        })
        return () => {
            cancelled = true
        }
    }, [supported, activeOrigin])

    const add = useCallback(() => {
        // ?mode=add tells the connect screen to switch rather than replace, so
        // the server the user is currently signed into keeps its session.
        router.push('/connect?mode=add')
    }, [])

    const switchTo = useCallback(async (origin: string) => {
        setError(null)
        setBusyOrigin(origin)

        // Web cannot switch in place: localStorage is origin-partitioned, so the
        // session for another origin does not exist in this document and no
        // amount of repointing `pb` would produce one. The only real move is to
        // navigate there and let that origin decide who you are. This mirrors
        // what the cookie org switcher already does (navigateToOrgUrl).
        if (!canSwitchInPlace()) {
            navigateToOrgUrl(origin)
            return
        }

        try {
            // On success this never returns — the JS context restarts.
            await switchToServer(origin)
        } catch (err) {
            setBusyOrigin(null)
            if (err instanceof ReloadUnavailableError) {
                // The switch was refused, not half-applied: the saved active
                // pointer is written, so a manual restart completes it.
                setError('Restart the app to finish switching servers.')
                return
            }
            captureException('use-saved-servers.switch', err)
            setError('Could not switch servers. Please try again.')
        }
    }, [])

    const remove = useCallback(async (origin: string) => {
        setError(null)
        setBusyOrigin(origin)
        try {
            const outcome = await forgetServer(origin)
            if (outcome.status === 'disconnected') {
                router.replace('/connect')
                return
            }
            // 'switched' restarts the JS context, so only 'removed' returns here
            // with UI still to update.
            setServers(await readServers())
            setBusyOrigin(null)
        } catch (err) {
            setBusyOrigin(null)
            if (err instanceof ReloadUnavailableError) {
                setError('Removed. Restart the app to finish switching servers.')
                setServers(await readServers())
                return
            }
            captureException('use-saved-servers.remove', err)
            setError('Could not remove that server. Please try again.')
        }
    }, [])

    // Every hook above runs unconditionally (rules of hooks); only the RESULT is
    // gated. The effect self-skips, so an unsupported platform does no storage
    // work and gets callbacks that cannot navigate anywhere it shouldn't.
    if (!supported) return INERT

    return {
        servers,
        activeOrigin,
        busyOrigin,
        error,
        canReload: isReloadAvailable(),
        add,
        switchTo,
        remove,
    }
}

const noop = () => {}

const INERT: SavedServersState = {
    servers: [],
    activeOrigin: null,
    busyOrigin: null,
    error: null,
    canReload: false,
    add: noop,
    switchTo: noop,
    remove: noop,
}
