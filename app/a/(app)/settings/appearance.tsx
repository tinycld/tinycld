import { AppearanceSection } from '@tinycld/core/components/settings/AppearanceSection'
import { NavigationSection } from '@tinycld/core/components/settings/NavigationSection'
import { SettingsScreen } from '@tinycld/core/components/settings/SettingsScreen'
import { GestureHandlerRootView } from 'react-native-gesture-handler'

// NavigationSection's drag-to-reorder list needs a gesture root.
export default function AppearanceSettings() {
    return (
        <GestureHandlerRootView className="flex-1">
            <SettingsScreen title="Appearance">
                <AppearanceSection />
                <NavigationSection />
            </SettingsScreen>
        </GestureHandlerRootView>
    )
}
