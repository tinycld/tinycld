import { PB_SERVER_ADDR } from '@tinycld/core/lib/config'
import { pb as appPb } from '@tinycld/core/lib/pocketbase'
import PocketBase from 'pocketbase'
import { useCallback, useRef, useState } from 'react'

export function useSuperUserPB() {
    const pbRef = useRef<PocketBase | null>(null)
    if (!pbRef.current) {
        pbRef.current = new PocketBase(PB_SERVER_ADDR)
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
                // Mirror the superuser token onto the shared app pb client so the
                // pbtsdb stores the console writes through carry it. Without this
                // the stores stay anonymous and managed-field writes (e.g. setting
                // `verified` on a new org owner) fail the users manageRule with a
                // 400. PB superusers bypass that rule, so the mirrored token
                // authorizes the create.
                appPb.authStore.save(pb.authStore.token, pb.authStore.record)
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

    return {
        pb,
        login,
        isAuthenticated,
        error,
        isLoading,
    }
}
