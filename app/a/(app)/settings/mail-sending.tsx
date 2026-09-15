import { MailSendingPanel } from '@tinycld/core/components/settings/system/MailSendingPanel'
import { SystemSettingsScreen } from '@tinycld/core/components/settings/system/SystemSettingsScreen'

export default function MailSendingSettings() {
    return (
        <SystemSettingsScreen title="Mail Sending" testID="settings-section-mail-sending">
            <MailSendingPanel />
        </SystemSettingsScreen>
    )
}
