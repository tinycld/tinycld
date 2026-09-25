import { useSystemSettings } from '../../components/setup/system-settings-store'
import type { WizardState } from './types'
import { parseWizardState } from './wizard-logic'

export const SETUP_WIZARD_KEY = 'setup.wizard'

/**
 * The deployment's wizard progress. system_settings is owner/admin-only, so
 * for anyone else `state` is null — which is also what "no wizard" means.
 */
export function useSetupWizardState() {
    const { byKey, upsert, isReady } = useSystemSettings()
    const state = parseWizardState(byKey.get(SETUP_WIZARD_KEY)?.value)

    const update = async (patch: (s: WizardState) => WizardState) => {
        if (!state) return
        await upsert.mutateAsync({
            key: SETUP_WIZARD_KEY,
            value: JSON.stringify(patch(state)),
            isSecret: false,
        })
    }

    return { state, isReady, update }
}
