import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { PackageManager } from '@tinycld/core/components/setup/PackageManager'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { pb } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useNavigateBack } from '@tinycld/core/lib/use-navigate-back'
import { ArrowLeft } from 'lucide-react-native'
import { Pressable, ScrollView, Text, View } from 'react-native'

// Owner-only. Package enablement and installation are the same decision here:
// a single-org deployment IS the org, so "hide this from our members" and
// "don't run this on this deployment" collapse into one switch — pkg_registry
// .status, which the manager's row toggle writes. Installing or removing a
// package rebuilds the artifact the whole deployment runs, which is why this
// is owner-gated rather than admin-gated. requireOwner
// (server/coreserver/pkg_install.go) is the real enforcement; the check here
// just avoids offering a guaranteed 403.
export default function OrgPackageSettings() {
    const { isOwner, isReady } = useCurrentRole()
    const orgHref = useOrgHref()
    const navigateBack = useNavigateBack(() => orgHref('settings'))
    const fgColor = useThemeColor('foreground')

    // Gated on isReady as well as isOwner: `role` reads null while the live
    // query loads, so a bare isOwner check flashes the refusal for a
    // legitimate owner on a cold load.
    if (!isReady) return null

    if (!isOwner) {
        return (
            <View className="flex-1 p-5 items-center justify-center bg-background">
                <DocumentTitle pkg="Settings" title="Packages" />
                <Text className="text-muted-foreground text-base">
                    Only the owner can manage packages.
                </Text>
            </View>
        )
    }

    return (
        <ScrollView className="flex-1 bg-background" contentContainerStyle={{ flexGrow: 1 }}>
            <DocumentTitle pkg="Settings" title="Packages" />
            <View className="p-5 w-full gap-4">
                <Pressable onPress={navigateBack} className="self-start">
                    <ArrowLeft size={24} color={fgColor} />
                </Pressable>
                {/* PackageManager renders its own "Packages" PageHeader, so this
                    screen deliberately adds no title of its own. */}
                <View
                    className="w-full gap-4"
                    style={{ maxWidth: 1040 }}
                    testID="settings-install-manager"
                >
                    <PackageManager pb={pb} isVisible />
                </View>
            </View>
        </ScrollView>
    )
}
