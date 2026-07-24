import { AuthGate } from '@tinycld/core/components/workspace/AuthGate'
import { SkeletonLayout } from '@tinycld/core/components/workspace/SkeletonLayout'
import { useAuth } from '@tinycld/core/lib/auth'
import { trace } from '@tinycld/core/lib/debug-trace'
import { navigateToOrg } from '@tinycld/core/lib/org-url'
import { getResolvedAddress } from '@tinycld/core/lib/server-address'
import { router } from 'expo-router'
import { useEffect } from 'react'

export default function Index() {
    const auth = useAuth({ throwIfAnon: false })
    const hasServer = !!getResolvedAddress()

    trace('Index render', {
        isLoggedIn: auth.isLoggedIn,
        isInitializing: auth.isInitializing,
        hasServer,
    })

    useEffect(() => {
        trace('Index effect', {
            isLoggedIn: auth.isLoggedIn,
            hasServer,
            isInitializing: auth.isInitializing,
        })
        if (auth.isLoggedIn) {
            trace('Index navigateToOrg')
            navigateToOrg()
        } else if (!auth.isInitializing && !hasServer) {
            trace('Index replace /connect')
            router.replace('/connect')
        }
    }, [auth.isLoggedIn, hasServer, auth.isInitializing])

    if (auth.isInitializing || auth.isLoggedIn || !hasServer) {
        return <SkeletonLayout />
    }

    return (
        <>
            <SkeletonLayout />
            <AuthGate />
        </>
    )
}
