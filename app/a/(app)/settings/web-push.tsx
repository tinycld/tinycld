import { SystemSettingsScreen } from '@tinycld/core/components/settings/system/SystemSettingsScreen'
import { VapidPanel } from '@tinycld/core/components/settings/system/VapidPanel'

export default function WebPushSettings() {
    return (
        <SystemSettingsScreen title="Web Push" testID="settings-section-web-push">
            <VapidPanel />
        </SystemSettingsScreen>
    )
}
