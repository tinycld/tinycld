import { router } from 'expo-router'
import { useCallback, useEffect, useState } from 'react'
import { captureException } from './errors'
import { ReloadUnavailableError } from './reload-js-context'
import { forgetServer } from './remove-server'
import { getResolvedAddress } from './server-address'
import { readServers, type SavedServer } from './servers'
import { switchToServer } from './switch-server'

interface SavedServersState {
    servers: SavedServer[]
    activeOrigin: string | null
    // The origin of a row with an operation in flight, so its row shows a
    // spinner and every other control is disabled — a switch restarts the JS
    // context, so a second concurrent action would be acting on a doomed graph.
    busyOrigin: string | null
    error: string | null
    add: () => void
    switchTo: (origin: string) => void
    remove: (origin: string) => void
}

// useSavedServers owns the saved-server list and the add/switch/remove actions,
// keeping the Settings section's JSX to layout only.
//
// The list is read once on mount and after a removal rather than subscribed to:
// the only other way it changes is a switch, which restarts the JS context and
// re-runs this from scratch.
export function useSavedServers(): SavedServersState {
    const [servers, setServers] = useState<SavedServer[]>([])
    const [busyOrigin, setBusyOrigin] = useState<string | null>(null)
    const [error, setError] = useState<string | null>(null)
    const activeOrigin = getResolvedAddress()

    useEffect(() => {
        let cancelled = false
        readServers().then(saved => {
            if (!cancelled) setServers(saved)
        })
        return () => {
            cancelled = true
        }
    }, [])

    const add = useCallback(() => {
        // ?mode=add tells the connect screen to switch rather than replace, so
        // the server the user is currently signed into keeps its session.
        router.push('/connect?mode=add')
    }, [])

    const switchTo = useCallback(async (origin: string) => {
        setError(null)
        setBusyOrigin(origin)
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

    return { servers, activeOrigin, busyOrigin, error, add, switchTo, remove }
}
