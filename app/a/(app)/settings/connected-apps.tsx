import { ConnectedAppsSection } from '@tinycld/core/components/settings/ConnectedAppsSection'
import { SettingsScreen } from '@tinycld/core/components/settings/SettingsScreen'

export default function ConnectedAppsSettings() {
    return (
        <SettingsScreen title="Connected apps">
            <ConnectedAppsSection />
        </SettingsScreen>
    )
}
