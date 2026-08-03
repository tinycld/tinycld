import {
    DEMO_SERVER,
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
        // A universal-link demo open on a fresh install has no env/cached address,
        // so the gate would resolve to 'unresolved' and blank-screen /p/demo before
        // it can pin the server itself. Seed the hosted demo server synchronously
        // here so the gate resolves normally and the demo screen mounts inside the
        // provider tree. __DEV__ is exempt (keeps whatever dev server is resolved,
        // matching app/p/demo.tsx) so local testing hits localhost.
        else if (!__DEV__ && pathname === '/p/demo') setResolvedAddress(DEMO_SERVER)
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
        // Routes that resolve the server address themselves must be exempt from the
        // →/connect redirect, or they get bounced before they can set it. /connect
        // is the server picker; /pick-org is the org picker a multi-org apex sends
        // users to (it resolves an org's address the same way /connect resolves a
        // server's, so bouncing it would strand the user in a loop between the
        // two); /p/demo pins the public demo server (see app/p/demo.tsx), so a
        // universal-link demo open on a fresh install (no cached address) must
        // reach it instead of being sent to /connect.
        if (pathname === '/connect' || pathname === '/pick-org' || pathname === '/p/demo') return
        const backTo = encodeURIComponent(pathname || '/')
        router.replace(`/connect?backTo=${backTo}`)
    }, [state.status, pathname])

    return state
}
