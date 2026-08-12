import { create } from '@tinycld/core/lib/store'

type BuilderState =
    | { mode: 'closed' }
    | { mode: 'create'; scope: 'personal' | 'org'; presetPkg?: string; nextOrder: number }
    | { mode: 'edit'; ruleId: string }

interface RulesUiState {
    builder: BuilderState
    historyRuleId: string | null
    openCreate: (scope: 'personal' | 'org', nextOrder: number, presetPkg?: string) => void
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
    openCreate: (scope, nextOrder, presetPkg) =>
        set({ builder: { mode: 'create', scope, presetPkg, nextOrder } }),
    openEdit: ruleId => set({ builder: { mode: 'edit', ruleId } }),
    closeBuilder: () => set({ builder: { mode: 'closed' } }),
    openHistory: ruleId => set({ historyRuleId: ruleId }),
    closeHistory: () => set({ historyRuleId: null }),
}))
