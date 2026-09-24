import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import {
    GroupDrawer,
    type GroupDrawerMode,
} from '@tinycld/core/components/settings/groups/GroupDrawer'
import { GroupsList } from '@tinycld/core/components/settings/groups/GroupsList'
import { useGroupsAdmin } from '@tinycld/core/lib/groups/use-groups-admin'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useNavigateBack } from '@tinycld/core/lib/use-navigate-back'
import { ArrowLeft, Plus, UsersRound } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'

export default function GroupsSettings() {
    const orgHref = useOrgHref()
    const navigateBack = useNavigateBack(() => orgHref('settings'))
    const { isAdmin } = useCurrentRole()
    const { groups, isReady } = useGroupsAdmin()
    const [drawerMode, setDrawerMode] = useState<GroupDrawerMode>({ kind: 'closed' })

    const fgColor = useThemeColor('foreground')
    const mutedColor = useThemeColor('muted-foreground')
    const primaryFgColor = useThemeColor('primary-foreground')

    if (!isAdmin) return <AdminRequired color={mutedColor} />

    return (
        <>
            <DocumentTitle pkg="Settings" title="Groups" />
            <ScrollView contentContainerStyle={{ flexGrow: 1 }} className="bg-background">
                <View className="flex-1 gap-6 p-5" style={{ maxWidth: 820 }}>
                    <View className="flex-row items-center gap-3">
                        <Pressable
                            onPress={navigateBack}
                            hitSlop={12}
                            className="rounded-full"
                            style={{ padding: 6 }}
                        >
                            <ArrowLeft size={22} color={fgColor} />
                        </Pressable>
                        <View className="flex-1 gap-0.5">
                            <Text
                                className="text-muted-foreground"
                                style={{ fontSize: 11, letterSpacing: 0.6 }}
                            >
                                Settings
                            </Text>
                            <Text
                                className="text-foreground"
                                style={{ fontSize: 24, fontWeight: '800' }}
                            >
                                Groups
                            </Text>
                        </View>
                        <Pressable
                            testID="groups-new-button"
                            onPress={() => setDrawerMode({ kind: 'create' })}
                            className="flex-row items-center gap-1.5 rounded-lg bg-primary"
                            style={{ paddingVertical: 9, paddingHorizontal: 14 }}
                        >
                            <Plus size={14} color={primaryFgColor} />
                            <Text
                                className="text-primary-foreground"
                                style={{ fontSize: 13, fontWeight: '700' }}
                            >
                                New group
                            </Text>
                        </Pressable>
                    </View>
                    <GroupsList
                        groups={groups}
                        isReady={isReady}
                        onOpen={groupId => setDrawerMode({ kind: 'view', groupId })}
                    />
                </View>
            </ScrollView>
            <GroupDrawer
                mode={drawerMode}
                onClose={() => setDrawerMode({ kind: 'closed' })}
                onCreated={groupId => setDrawerMode({ kind: 'view', groupId })}
            />
        </>
    )
}

function AdminRequired({ color }: { color: string }) {
    return (
        <View className="flex-1 items-center justify-center p-5 bg-background">
            <DocumentTitle pkg="Settings" title="Groups" />
            <View
                className="items-center gap-3 rounded-xl bg-surface-secondary border border-border"
                style={{ paddingVertical: 32, paddingHorizontal: 24 }}
            >
                <UsersRound size={28} color={color} />
                <Text className="text-foreground" style={{ fontSize: 15, fontWeight: '600' }}>
                    Admin access required
                </Text>
                <Text
                    className="text-muted-foreground"
                    style={{ fontSize: 13, textAlign: 'center' }}
                >
                    Only admins and owners can manage groups.
                </Text>
            </View>
        </View>
    )
}
