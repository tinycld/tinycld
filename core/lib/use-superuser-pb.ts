import { PB_SERVER_ADDR } from '@tinycld/core/lib/config'
import { pb as appPb } from '@tinycld/core/lib/pocketbase'
import PocketBase, { BaseAuthStore } from 'pocketbase'
import { useCallback, useEffect, useRef, useState } from 'react'

// The raw-superuser fallback (SetupPage's SuperuserLoginForm) drives the admin
// console through the pbtsdb stores, which are bound to the shared app client —
// so those reads/writes only pass PocketBase's collection rules while that
// client carries the rule-bypassing superuser token. Mirroring the token onto
// the shared client's persistent auth store (the previous approach) left the
// WHOLE app running with collection-rule bypass — surviving reloads — until an
// explicit logout. Instead we temporarily swap the shared client's auth store
// for an in-memory one holding the superuser token, and swap the original store
// back when the admin surface unmounts. The superuser token never reaches
// persisted storage, and leaving the admin surface restores the prior session.
let elevation: { prior: BaseAuthStore; superuserToken: string } | null = null

function elevateSharedClient(superuserPb: PocketBase) {
    const token = superuserPb.authStore.token
    const record = superuserPb.authStore.record
    if (elevation) {
        // Re-login while already elevated (e.g. an expired token): refresh the
        // in-memory store rather than nesting another swap.
        elevation.superuserToken = token
        appPb.authStore.save(token, record)
        return
    }
    const memory = new BaseAuthStore()
    memory.save(token, record)
    elevation = { prior: appPb.authStore, superuserToken: token }
    appPb.authStore = memory
}

function restoreSharedClient() {
    if (!elevation) return
    const memory = appPb.authStore
    const { prior, superuserToken } = elevation
    elevation = null
    appPb.authStore = prior
    if (memory.token && memory.token !== superuserToken) {
        // Impersonation ("Visit" on an org row) saved the org owner's token
        // onto the shared client while elevated — carry that ordinary user
        // auth over to the real (persistent) store instead of discarding it.
        prior.save(memory.token, memory.record)
    } else if (!memory.token) {
        // Something cleared the session while elevated (logout/disconnect) —
        // propagate the clear so the prior auth doesn't resurrect.
        prior.clear()
    }
}

export function useSuperUserPB() {
    const pbRef = useRef<PocketBase | null>(null)
    if (!pbRef.current) {
        // Explicit in-memory store: the SDK's default LocalAuthStore persists
        // to window.localStorage on web, which would leak the superuser token
        // across sessions.
        pbRef.current = new PocketBase(PB_SERVER_ADDR, new BaseAuthStore())
    }
    const pb = pbRef.current

    const [isAuthenticated, setIsAuthenticated] = useState(false)
    const [error, setError] = useState<string | null>(null)
    const [isLoading, setIsLoading] = useState(false)

    const login = useCallback(
        async (email: string, password: string) => {
            setError(null)
            setIsLoading(true)
            try {
                await pb.collection('_superusers').authWithPassword(email, password)
                elevateSharedClient(pb)
                setIsAuthenticated(true)
            } catch (err) {
                const message = err instanceof Error ? err.message : 'Authentication failed'
                setError(message)
            } finally {
                setIsLoading(false)
            }
        },
        [pb]
    )

    // Tear the elevation down when the admin surface unmounts so nothing keeps
    // running with superuser auth on the shared client after leaving it.
    useEffect(() => restoreSharedClient, [])

    return {
        pb,
        login,
        isAuthenticated,
        error,
        isLoading,
    }
}
