import type { WizardSummary } from '@tinycld/core/lib/setup/wizard-logic'
import { View } from 'react-native'

const PHASE_CLASS = { done: 'bg-primary', skipped: 'bg-muted/40', todo: 'bg-border' } as const

export function ProgressSegments({
    summary,
    currentStepId,
}: {
    summary: WizardSummary
    currentStepId: string | null
}) {
    const segments = summary.steps.map(s => (
        <View
            key={s.id}
            accessibilityLabel={`${s.label}: ${s.phase}`}
            className={`h-1 flex-1 rounded-sm ${s.id === currentStepId ? 'bg-primary/60' : PHASE_CLASS[s.phase]}`}
        />
    ))
    return <View className="flex-1 flex-row items-center gap-1">{segments}</View>
}
