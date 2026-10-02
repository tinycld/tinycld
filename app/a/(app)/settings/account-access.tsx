import { DisableAccountSection } from '@tinycld/core/components/settings/account/DisableAccountSection'
import { DeleteAccountSection } from '@tinycld/core/components/settings/DeleteAccountSection'
import { SettingsScreen } from '@tinycld/core/components/settings/SettingsScreen'

export default function AccountAccessSettings() {
    return (
        <SettingsScreen title="Account access">
            <DisableAccountSection />
            <DeleteAccountSection />
        </SettingsScreen>
    )
}
