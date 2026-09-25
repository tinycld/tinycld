import { ChangeServerLink } from '@tinycld/core/components/ChangeServerLink'
import { appHref } from '@tinycld/core/lib/org-routes'
import { useNeedsSetup } from '@tinycld/core/lib/setup/use-needs-setup'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useSuperUserPB } from '@tinycld/core/lib/use-superuser-pb'
import { Redirect } from 'expo-router'
import { View } from 'react-native'
import { GestureHandlerRootView } from 'react-native-gesture-handler'
import { SetupDashboard } from './SetupDashboard'
import { SuperuserLoginForm } from './SuperuserLoginForm'

export function SetupPage() {
    const { pb, login, isAuthenticated, error, isLoading } = useSuperUserPB()
    const { isAdmin } = useCurrentRole()
    const needsSetup = useNeedsSetup()

    if (needsSetup === undefined) return null
    // Recovery needs an owner to recover; before one exists the claim flow is the way in.
    if (needsSetup) return <Redirect href={appHref('setup')} />

    // An owner/admin reaches the console with their normal session — send them
    // to Settings, the single in-app administration surface. /setup/recovery is
    // only the raw-superuser door for cases where no app session exists.
    if (isAdmin) {
        return <Redirect href={appHref('settings')} />
    }

    // Fallback for anyone without an owner/admin app session (e.g. a raw PB
    // superuser doing recovery): authenticate against _superusers directly,
    // then drive the recovery console.
    if (!isAuthenticated) {
        return (
            <View className="flex-1 items-center justify-center gap-4">
                <SuperuserLoginForm login={login} error={error} isLoading={isLoading} />
                <ChangeServerLink />
            </View>
        )
    }

    return (
        <GestureHandlerRootView className="flex-1">
            <SetupDashboard pb={pb} />
        </GestureHandlerRootView>
    )
}
