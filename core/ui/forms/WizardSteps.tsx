import type { FormPage } from '@tinycld/core/lib/forms/definition'
import { Text, View } from 'react-native'

export interface WizardStepsProps {
    pages: FormPage[]
    index: number
}

function toneFor(isCurrent: boolean, isDone: boolean) {
    if (isCurrent) return 'text-foreground font-semibold'
    return isDone ? 'text-foreground' : 'text-muted-foreground'
}

function Step({
    page,
    isCurrent,
    isDone,
}: {
    page: FormPage
    isCurrent: boolean
    isDone: boolean
}) {
    return (
        <Text
            className={`shrink min-w-0 text-xs mr-3 ${toneFor(isCurrent, isDone)}`}
            accessibilityState={{ selected: isCurrent }}
        >
            {isCurrent ? '● ' : '○ '}
            {page.title}
        </Text>
    )
}

export function WizardSteps({ pages, index }: WizardStepsProps) {
    if (pages.length < 2) return null
    return (
        <View
            className="flex-row flex-wrap mb-3"
            accessibilityRole="tablist"
            testID="form-wizard-steps"
        >
            {pages.map((p, i) => (
                <Step key={p.key} page={p} isCurrent={i === index} isDone={i < index} />
            ))}
        </View>
    )
}
