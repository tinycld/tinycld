import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { BackupsSection } from '@tinycld/core/components/settings/backups/BackupsSection'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useNavigateBack } from '@tinycld/core/lib/use-navigate-back'
import { ArrowLeft } from 'lucide-react-native'
import { Pressable, ScrollView, Text, View } from 'react-native'

// Admin-gated as a whole: an admin can take a backup. The restore form inside
// the section is owner-only, because a restore replaces the organization.
// Gated on isReady too, so a cold-load admin isn't bounced by the transient
// null role.
export default function BackupSettings() {
    const orgHref = useOrgHref()
    const navigateBack = useNavigateBack(() => orgHref('settings'))
    const { isAdmin, isReady } = useCurrentRole()
    const fgColor = useThemeColor('foreground')

    if (!isReady) return null

    if (!isAdmin) {
        return (
            <View className="flex-1 p-5 items-center justify-center bg-background">
                <DocumentTitle pkg="Settings" title="Backups" />
                <Text className="text-muted-foreground text-base">
                    Only admins can manage backups.
                </Text>
            </View>
        )
    }

    return (
        <ScrollView className="flex-1 bg-background" contentContainerStyle={{ flexGrow: 1 }}>
            <DocumentTitle pkg="Settings" title="Backups" />
            <View className="p-5 w-full gap-4" style={{ maxWidth: 1040 }}>
                <View className="flex-row gap-3 items-center">
                    <Pressable onPress={navigateBack}>
                        <ArrowLeft size={24} color={fgColor} />
                    </Pressable>
                    <Text className="text-foreground text-[22px] font-bold">Backups</Text>
                </View>
                <BackupsSection />
            </View>
        </ScrollView>
    )
}
