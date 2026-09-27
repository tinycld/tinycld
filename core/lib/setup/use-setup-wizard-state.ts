import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import type { WizardState } from '@tinycld/core/lib/setup/types'
import { parseWizardState } from '@tinycld/core/lib/setup/wizard-logic'

export const SETUP_WIZARD_KEY = 'setup.wizard'

/**
 * The deployment's wizard progress. system_settings is owner/admin-only, so
 * for anyone else `state` is null — which is also what "no wizard" means.
 *
 * The app layout calls this on every load for owners and admins, so it asks
 * for the one wizard row only: system_settings is on-demand, and the rest of
 * the collection holds secrets this screen has no use for.
 */
export function useSetupWizardState() {
    const [systemSettings] = useStore('system_settings')
    const { data: rows = [], isReady } = useLiveQuery(query =>
        query.from({ s: systemSettings }).where(({ s }) => eq(s.key, SETUP_WIZARD_KEY))
    )
    const row = rows[0]
    const state = parseWizardState(row?.value)

    // Only the two server paths that create an owner write the first row, so
    // there is always a row to update when there is a state to patch.
    const write = useMutation({
        mutationFn: mutation(function* (input: { id: string; value: string }) {
            yield systemSettings.update(input.id, draft => {
                draft.value = input.value
            })
        }),
    })

    const update = async (patch: (s: WizardState) => WizardState) => {
        if (!state || !row) return
        await write.mutateAsync({ id: row.id, value: JSON.stringify(patch(state)) })
    }

    return { state, isReady, update }
}
