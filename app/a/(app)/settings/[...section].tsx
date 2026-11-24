import { PackagePanelScreen } from '@tinycld/core/components/settings/PackagePanelScreen'
import { packageSettings } from '@tinycld/core/lib/packages/derive-components'
import {
    declaresTerms,
    slugOfGroup,
    useTermsBlocked,
    useTermsOpen,
} from '@tinycld/core/lib/packages/use-package-terms'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useLocalSearchParams } from 'expo-router'
import { Text, View } from 'react-native'

// Org-administration panels (manifest `settings`). Per-user panels live under
// settings/account/[...section].tsx and are open to every role.
export default function PackageSettingsSection() {
    const { isAdmin } = useCurrentRole()
    const mutedColor = useThemeColor('muted-foreground')
    const bgColor = useThemeColor('background')
    // A package whose terms the owner has not accepted has no panels: its URL
    // reads "Settings not found".
    const groups = useTermsOpen(packageSettings, slugOfGroup)
    const { isReady: termsReady } = useTermsBlocked()
    const [pkgSlug] = useLocalSearchParams<{ section: string[] }>().section ?? []

    // Until the terms state is known every package with terms is closed: wait,
    // or an open package's panel would read "Settings not found" first. Only
    // for such a package, so no other panel unmounts (and loses its input) if
    // the state is lost again.
    if (declaresTerms(pkgSlug ?? '') && !termsReady) return null

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

    return <PackagePanelScreen groups={groups} />
}
