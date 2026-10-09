import { create } from '@tinycld/core/lib/store'

/** What the wizard footer's Continue does on the screen now showing. */
export interface SetupContinueAction {
    label: string
    onPress: () => void
    isDisabled: boolean
    /** Pressing it sends a change to the server, so it waits out an outage. */
    requiresServer: boolean
}

interface SetupContinueState {
    action: SetupContinueAction | null
    publish: (action: SetupContinueAction | null) => void
}

// The wizard shell draws Continue in its footer, but only the step knows what
// pressing it does (submit a form, advance, create the owner). The step
// publishes that here; the shell renders it. One wizard screen shows at a
// time, so one slot is enough.
export const useSetupContinueStore = create<SetupContinueState>()(set => ({
    action: null,
    publish: action => set({ action }),
}))
