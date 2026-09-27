import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { SetupStepScreen } from '@tinycld/core/components/setup/wizard/SetupStepScreen'
import { AuthGate } from '@tinycld/core/components/workspace/AuthGate'
import { useAuth } from '@tinycld/core/lib/auth'
import { appHref } from '@tinycld/core/lib/org-routes'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { Redirect, useLocalSearchParams } from 'expo-router'

// The signed-in wizard. Outside app/(app)/ so the workspace chrome (and its
// own redirect into the wizard) does not wrap it; it asks for a session itself.
export default function SetupStep() {
    const { step } = useLocalSearchParams<{ step: string }>()
    const auth = useAuth({ throwIfAnon: false })
    const { isAdmin, isReady: roleReady } = useCurrentRole()
    if (auth.isInitializing) return null
    if (!auth.isLoggedIn) return <AuthGate />
    if (!roleReady) return null
    if (!isAdmin) return <Redirect href={appHref('')} />
    return (
        <>
            <DocumentTitle title="Set up" includeOrg={false} />
            <SetupStepScreen param={step} />
        </>
    )
}
