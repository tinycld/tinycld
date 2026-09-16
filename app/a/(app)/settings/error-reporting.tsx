import { SentryPanel } from '@tinycld/core/components/settings/system/SentryPanel'
import { SystemSettingsScreen } from '@tinycld/core/components/settings/system/SystemSettingsScreen'

export default function ErrorReportingSettings() {
    return (
        <SystemSettingsScreen
            title="Error Reporting"
            testID="settings-section-error-reporting"
            keyPrefix="sentry."
        >
            <SentryPanel />
        </SystemSettingsScreen>
    )
}
