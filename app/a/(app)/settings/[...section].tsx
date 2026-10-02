import { PackagePanelScreen } from '@tinycld/core/components/settings/PackagePanelScreen'
import { packageSettings } from '@tinycld/core/lib/packages/derive-components'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { Text, View } from 'react-native'

// Org-administration panels (manifest `settings`). Per-user panels live under
// settings/account/[...section].tsx and are open to every role.
export default function PackageSettingsSection() {
    const { isAdmin } = useCurrentRole()
    const mutedColor = useThemeColor('muted-foreground')
    const bgColor = useThemeColor('background')

    if (!isAdmin) {
        return (
            <View
                className="flex-1 p-5 items-center justify-center"
                style={{ backgroundColor: bgColor }}
            >
                <Text style={{ fontSize: 16, color: mutedColor }}>
                    Only admins can access package settings.
                </Text>
            </View>
        )
    }

    return <PackagePanelScreen groups={packageSettings} />
}
