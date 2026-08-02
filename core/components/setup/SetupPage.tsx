import { ChangeServerLink } from '@tinycld/core/components/ChangeServerLink'
import { PB_SERVER_ADDR } from '@tinycld/core/lib/config'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useSuperUserPB } from '@tinycld/core/lib/use-superuser-pb'
import { Redirect } from 'expo-router'
import { useEffect, useState } from 'react'
import { ScrollView, Text, View } from 'react-native'
import { GestureHandlerRootView } from 'react-native-gesture-handler'
import { SetupDashboard } from './SetupDashboard'
import { SetupWizard } from './SetupWizard'
import { SuperuserLoginForm } from './SuperuserLoginForm'

interface SetupPageProps {
    token?: string
}

export function SetupPage({ token }: SetupPageProps) {
    const { pb, login, isAuthenticated, error, isLoading } = useSuperUserPB()
    const { isAdmin } = useCurrentRole()
    const [needsSetup, setNeedsSetup] = useState<boolean | null>(null)

    useEffect(() => {
        fetch(`${PB_SERVER_ADDR}/api/setup/check`)
            .then(res => res.json())
            .then(data => setNeedsSetup(data.needsSetup === true))
            .catch(() => setNeedsSetup(false))
    }, [])

    if (needsSetup === null) return null

    if (needsSetup && token) {
        return (
            <GestureHandlerRootView className="flex-1">
                <ScrollView>
                    <SetupWizard token={token} />
                </ScrollView>
            </GestureHandlerRootView>
        )
    }

    if (needsSetup) {
        return (
            <View className="flex-1 items-center justify-center p-5">
                <View className="gap-3 items-center" style={{ maxWidth: 380 }}>
                    <Text className="text-foreground" style={{ fontSize: 20, fontWeight: 'bold' }}>
                        Setup Required
                    </Text>
                    <Text
                        className="text-center text-muted-foreground"
                        style={{ fontSize: 14, lineHeight: 20 }}
                    >
                        No superuser account exists yet. Visit the setup URL printed in the server
                        console to complete initial setup.
                    </Text>
                </View>
            </View>
        )
    }

    // An owner/admin reaches the console with their normal session — send them
    // to Settings, the single in-app administration surface. The /setup route is
    // now only the pre-auth bootstrap door (first-run wizard + raw-superuser
    // recovery) for cases where no app session exists.
    if (isAdmin) {
        return <Redirect href="/settings" />
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
