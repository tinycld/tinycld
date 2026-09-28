import { Pressable, Text, View } from 'react-native'

export interface ProviderTab<V extends string> {
    value: V
    label: string
}

const TAB_CLASS = {
    selected: '-mb-px border-b-2 border-primary px-1 pb-2.5',
    idle: '-mb-px border-b-2 border-transparent px-1 pb-2.5',
} as const

const LABEL_CLASS = {
    selected: 'text-sm font-semibold text-foreground',
    idle: 'text-sm font-medium text-muted-foreground',
} as const

/**
 * A tab bar whose panel is the fields below it, so it reads as one control
 * over one section rather than a row of unrelated buttons.
 */
export function ProviderTabs<V extends string>({
    tabs,
    value,
    onChange,
    testID,
}: {
    tabs: readonly ProviderTab<V>[]
    value: V
    onChange: (value: V) => void
    testID?: string
}) {
    const items = tabs.map(tab => {
        const state = tab.value === value ? 'selected' : 'idle'
        return (
            <Pressable
                key={tab.value}
                testID={testID ? `${testID}-${tab.value}` : undefined}
                accessibilityRole="tab"
                accessibilityState={{ selected: state === 'selected' }}
                onPress={() => onChange(tab.value)}
                className={TAB_CLASS[state]}
            >
                <Text className={LABEL_CLASS[state]}>{tab.label}</Text>
            </Pressable>
        )
    })
    return (
        <View accessibilityRole="tablist" className="mb-4 flex-row gap-6 border-b border-border">
            {items}
        </View>
    )
}
