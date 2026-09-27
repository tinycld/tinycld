import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { SetupPage } from '@tinycld/core/components/setup/SetupPage'

// The raw-superuser recovery console. Pre-auth (outside app/(app)/) so it works
// when no app session exists; an owner/admin who lands here is sent to Settings.
export default function SetupRecovery() {
    return (
        <>
            <DocumentTitle title="Recovery" includeOrg={false} />
            <SetupPage />
        </>
    )
}
