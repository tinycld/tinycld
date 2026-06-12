import { ChangeServerLink } from '@tinycld/core/components/ChangeServerLink'
import { PB_SERVER_ADDR } from '@tinycld/core/lib/config'
import { pb as appPb } from '@tinycld/core/lib/pocketbase'
import { useIsSuperAdmin } from '@tinycld/core/lib/use-is-super-admin'
import { useSuperUserPB } from '@tinycld/core/lib/use-superuser-pb'
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
    const isSuperAdmin = useIsSuperAdmin()
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

    // A super-admin app user reaches the console with their normal session — the
    // server's admin endpoints accept their token (see requireAdmin), so we skip
    // the separate _superusers login entirely and drive the dashboard with the
    // app's authenticated pb client.
    if (isSuperAdmin) {
        return (
            <GestureHandlerRootView className="flex-1">
                <SetupDashboard pb={appPb} />
            </GestureHandlerRootView>
        )
    }

    // Fallback for anyone who isn't a super-admin app user (e.g. a raw PB
    // superuser doing recovery, or a fresh deploy before the first grant):
    // authenticate against _superusers directly.
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
