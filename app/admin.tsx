import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { SetupPage } from '@tinycld/core/components/setup/SetupPage'
import { useAuth } from '@tinycld/core/lib/auth'
import { useIsSuperAdmin } from '@tinycld/core/lib/use-is-super-admin'
import { Redirect, useLocalSearchParams } from 'expo-router'

// Top-level /admin is the superuser bootstrap & recovery surface: first-run
// setup wizard (with ?token=) and the _superusers login. A super-admin APP user
// who lands here is sent to the in-shell admin area (/a/<org>/admin), which is
// their real console — the rail icon already points there. Without a token and
// without an app session, SetupPage falls through to the superuser login.
export default function Admin() {
    const { token } = useLocalSearchParams<{ token?: string }>()
    const auth = useAuth({ throwIfAnon: false })
    const isSuperAdmin = useIsSuperAdmin()

    // Only redirect once auth has settled and there's no first-run token to honor.
    if (!token && !auth.isInitializing && isSuperAdmin && auth.user?.primaryOrgSlug) {
        return (
            <Redirect
                href={{
                    pathname: '/a/[orgSlug]/admin',
                    params: { orgSlug: auth.user.primaryOrgSlug },
                }}
            />
        )
    }

    return (
        <>
            <DocumentTitle title="Admin" includeOrg={false} />
            <SetupPage token={token} />
        </>
    )
}
