import type { WizardSummary } from '@tinycld/core/lib/setup/wizard-logic'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Check, Minus } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'

type Phase = 'done' | 'skipped' | 'todo' | 'current'

const MARK_CLASS: Record<Phase, string> = {
    done: 'size-[18px] items-center justify-center rounded-full bg-primary',
    current: 'size-[18px] items-center justify-center rounded-full border-2 border-primary',
    skipped: 'size-[18px] items-center justify-center rounded-full border border-border',
    todo: 'size-[18px] items-center justify-center rounded-full border border-border',
}

const LABEL_CLASS: Record<Phase, string> = {
    done: 'text-[13px] text-foreground',
    current: 'text-[13px] font-semibold text-foreground',
    skipped: 'text-[13px] text-muted-foreground',
    todo: 'text-[13px] text-muted-foreground',
}

function Mark({ phase }: { phase: Phase }) {
    const onPrimary = useThemeColor('primary-foreground')
    const muted = useThemeColor('muted-foreground')
    if (phase === 'done') {
        return (
            <View className={MARK_CLASS.done}>
                <Check size={11} color={onPrimary} strokeWidth={3} />
            </View>
        )
    }
    if (phase === 'skipped') {
        return (
            <View className={MARK_CLASS.skipped}>
                <Minus size={10} color={muted} strokeWidth={3} />
            </View>
        )
    }
    if (phase === 'current') {
        return (
            <View className={MARK_CLASS.current}>
                <View className="size-2 rounded-full bg-primary" />
            </View>
        )
    }
    return <View className={MARK_CLASS.todo} />
}

function StepItem({
    id,
    label,
    phase,
    onOpen,
}: {
    id: string
    label: string
    phase: Phase
    onOpen: ((id: string) => void) | undefined
}) {
    const body = (
        <>
            <Mark phase={phase} />
            <Text className={LABEL_CLASS[phase]}>{label}</Text>
        </>
    )
    // The current step is where the person already is; every other step is a
    // link to it, so the list doubles as the way back and forward.
    if (!onOpen || phase === 'current') {
        return (
            <View
                accessibilityLabel={`${label}: ${phase}`}
                accessibilityState={{ selected: phase === 'current' }}
                className="flex-row items-center gap-2"
            >
                {body}
            </View>
        )
    }
    return (
        <Pressable
            accessibilityRole="link"
            accessibilityLabel={`${label}: ${phase}`}
            testID={`setup-step-link-${id}`}
            onPress={() => onOpen(id)}
            className="flex-row items-center gap-2"
        >
            {body}
        </Pressable>
    )
}

/**
 * Every step by name, in order, with its state. The order is real: steps run
 * in this sequence, and the marks tell the person where they are and what
 * they passed over. With `onOpen`, each other step opens on press.
 */
export function StepList({
    summary,
    currentStepId,
    onOpen,
}: {
    summary: WizardSummary
    currentStepId: string | null
    onOpen?: (id: string) => void
}) {
    const items = summary.steps.map(s => (
        <StepItem
            key={s.id}
            id={s.id}
            label={s.label}
            phase={s.id === currentStepId ? 'current' : s.phase}
            onOpen={onOpen}
        />
    ))
    return <View className="flex-row flex-wrap gap-x-5 gap-y-2.5">{items}</View>
}
