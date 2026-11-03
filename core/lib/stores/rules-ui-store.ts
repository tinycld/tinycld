import { create } from '@tinycld/core/lib/store'

/** Seeds a new rule's trigger, as "Add destination" on a web form does. */
export interface BuilderPreset {
    trigger: string
    triggerConfig: { cron?: string; form?: string }
    name?: string
}

type BuilderState =
    | { mode: 'closed' }
    | {
          mode: 'create'
          scope: 'personal' | 'org'
          presetPkg?: string
          nextOrder: number
          preset?: BuilderPreset
      }
    | { mode: 'edit'; ruleId: string }

interface RulesUiState {
    builder: BuilderState
    historyRuleId: string | null
    openCreate: (
        scope: 'personal' | 'org',
        nextOrder: number,
        presetPkg?: string,
        preset?: BuilderPreset
    ) => void
    openEdit: (ruleId: string) => void
    closeBuilder: () => void
    openHistory: (ruleId: string) => void
    closeHistory: () => void
}

// Not persisted: a restored open builder would reopen a dialog the user
// didn't ask for, and a restored draft would edit against a rule/catalog
// that may have since changed — the search-palette store's rationale.
export const useRulesUiStore = create<RulesUiState>()(set => ({
    builder: { mode: 'closed' },
    historyRuleId: null,
    openCreate: (scope, nextOrder, presetPkg, preset) =>
        set({ builder: { mode: 'create', scope, presetPkg, nextOrder, preset } }),
    openEdit: ruleId => set({ builder: { mode: 'edit', ruleId } }),
    closeBuilder: () => set({ builder: { mode: 'closed' } }),
    openHistory: ruleId => set({ historyRuleId: ruleId }),
    closeHistory: () => set({ historyRuleId: null }),
}))
