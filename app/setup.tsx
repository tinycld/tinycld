import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { SetupPage } from '@tinycld/core/components/setup/SetupPage'
import { useAuth } from '@tinycld/core/lib/auth'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { Redirect, useLocalSearchParams } from 'expo-router'

// /setup is the pre-auth bootstrap & recovery door: first-run setup wizard (with
// ?token=) and the _superusers recovery login. It's a standalone route (outside
// app/(app)/, which requires an app session) so it works before any app user
// exists. Administration for logged-in owner/admin app users lives in the
// in-shell /settings area — an admin who lands here is sent there. Without a
// token and without an app session, SetupPage falls through to the superuser
// login.
export default function Setup() {
    const { token } = useLocalSearchParams<{ token?: string }>()
    const auth = useAuth({ throwIfAnon: false })
    const { isAdmin } = useCurrentRole()

    // Only redirect once auth has settled and there's no first-run token to honor.
    if (!token && !auth.isInitializing && isAdmin && auth.isLoggedIn) {
        return <Redirect href="/settings" />
    }

    return (
        <>
            <DocumentTitle title="Setup" includeOrg={false} />
            <SetupPage token={token} />
        </>
    )
}
