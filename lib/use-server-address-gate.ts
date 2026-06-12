import {
    getResolvedAddress,
    readCached,
    resolveEnvAddress,
    setResolvedAddress,
    subscribeResolvedAddress,
} from '@tinycld/core/lib/server-address'
import { router } from 'expo-router'
import { type ComponentType, type ReactNode, useEffect, useState } from 'react'

type ProvidersComponent = ComponentType<{ children: ReactNode }>

export type GateState =
    | { status: 'resolving' }
    | { status: 'resolved'; Providers: ProvidersComponent }
    | { status: 'unresolved' }
    | { status: 'failed'; error: string }

// useServerAddressGate resolves the PocketBase server address the app shell needs
// before it can mount the real provider tree, and reports progress as a GateState
// the layout renders. It (1) seeds from env/cache, (2) dynamically imports the
// heavy Providers module once an address is known, and (3) redirects to /connect
// when no address can be resolved. Kept out of _layout.tsx so the layout is just
// a thin state→screen switch.
export function useServerAddressGate(pathname: string): GateState {
    const [state, setState] = useState<GateState>(() => {
        const env = resolveEnvAddress()
        if (env) setResolvedAddress(env)
        return { status: 'resolving' }
    })

    useEffect(() => {
        let cancelled = false

        async function resolve() {
            try {
                if (!getResolvedAddress()) {
                    const cached = await readCached()
                    if (cached) setResolvedAddress(cached)
                }

                if (cancelled) return

                if (getResolvedAddress()) {
                    // Flip out of 'unresolved' synchronously so a navigation
                    // racing the dynamic import below (e.g. /connect doing
                    // setResolvedAddress + router.replace('/')) doesn't trip
                    // the unresolved→/connect redirect effect.
                    setState(prev =>
                        prev.status === 'unresolved' ? { status: 'resolving' } : prev
                    )
                    const mod = await import('@tinycld/core/components/Providers')
                    if (cancelled) return
                    setState({ status: 'resolved', Providers: mod.Providers })
                } else {
                    setState({ status: 'unresolved' })
                }
            } catch (err) {
                // Without this catch a failure inside the dynamic Providers
                // import (e.g. a transitive native module that fails to
                // initialize after a binary/JS mismatch) leaves the gate
                // stuck at "resolving" → permanent blank white screen with
                // nothing in the logs. Surface it so the next layer can show
                // diagnostic UI.
                if (cancelled) return
                const message = err instanceof Error ? err.message : String(err)
                // biome-ignore lint/suspicious/noConsole: pre-Sentry boot path
                console.error('[layout-gate] failed to resolve providers:', err)
                setState({ status: 'failed', error: message })
            }
        }

        resolve()
        const unsubscribe = subscribeResolvedAddress(() => {
            if (cancelled) return
            resolve()
        })
        return () => {
            cancelled = true
            unsubscribe()
        }
    }, [])

    useEffect(() => {
        if (state.status !== 'unresolved') return
        if (pathname === '/connect') return
        const backTo = encodeURIComponent(pathname || '/')
        router.replace(`/connect?backTo=${backTo}`)
    }, [state.status, pathname])

    return state
}
