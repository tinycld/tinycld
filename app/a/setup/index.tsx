import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { PreAuthSetup } from '@tinycld/core/components/setup/wizard/PreAuthSetup'
import { appHref } from '@tinycld/core/lib/org-routes'
import { useNeedsSetup } from '@tinycld/core/lib/setup/use-needs-setup'
import { Redirect, useLocalSearchParams } from 'expo-router'

// First-run entry. Pre-auth by design (outside app/(app)/): nobody can sign in
// until the owner this screen creates exists. Once the server is claimed, the
// signed-in wizard lives at /a/setup/<step> and recovery at /a/setup/recovery.
export default function SetupIndex() {
    const { code } = useLocalSearchParams<{ code?: string }>()
    const needsSetup = useNeedsSetup()
    if (needsSetup === undefined) return null
    if (!needsSetup) return <Redirect href={appHref('setup/next')} />
    return (
        <>
            <DocumentTitle title="Set up" includeOrg={false} />
            <PreAuthSetup initialCode={code} />
        </>
    )
}
