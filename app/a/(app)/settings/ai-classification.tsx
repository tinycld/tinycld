import { ClassifierPanel } from '@tinycld/core/components/settings/system/ClassifierPanel'
import { SystemSettingsScreen } from '@tinycld/core/components/settings/system/SystemSettingsScreen'

export default function AiClassificationSettings() {
    return (
        <SystemSettingsScreen
            title="AI Classification"
            testID="settings-section-ai-classification"
            keyPrefix="classifier."
        >
            <ClassifierPanel />
        </SystemSettingsScreen>
    )
}
