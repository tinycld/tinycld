import type { ChecklistItem } from '@tinycld/core/lib/forms/values'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { Check, Square, X } from 'lucide-react-native'
import { newRecordId } from 'pbtsdb/core'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'

export interface ChecklistFieldProps {
    value: ChecklistItem[] | null
    onChange: (items: ChecklistItem[]) => void
    disabled?: boolean
    testID?: string
    accessibilityLabel?: string
}

interface Store {
    items: ChecklistItem[]
    insert: (item: ChecklistItem) => void
    update: (item: ChecklistItem) => void
    remove: (id: string) => void
}

function useLocalStore(
    value: ChecklistItem[] | null,
    onChange: (items: ChecklistItem[]) => void
): Store {
    const items = value ?? []
    return {
        items,
        insert: item => onChange([...items, item]),
        update: item => onChange(items.map(i => (i.id === item.id ? item : i))),
        remove: id => onChange(items.filter(i => i.id !== id)),
    }
}

interface ViewProps {
    store: Store
    disabled?: boolean
    testID: string
    accessibilityLabel?: string
}

function Row({ item, store, disabled, testID }: ViewProps & { item: ChecklistItem }) {
    const fg = useThemeColor('foreground')
    const Icon = item.done ? Check : Square
    const textClass = item.done ? 'text-muted-foreground line-through' : 'text-foreground'
    return (
        <View className="flex-row items-center py-1">
            <Pressable
                onPress={() => store.update({ ...item, done: !item.done })}
                disabled={disabled}
                accessibilityRole="checkbox"
                accessibilityState={{ checked: item.done }}
                accessibilityLabel={item.text}
                testID={`${testID}-toggle-${item.id}`}
            >
                <Icon size={18} color={fg} />
            </Pressable>
            <Text className={`ml-2 flex-1 min-w-0 ${textClass}`}>{item.text}</Text>
            <Pressable
                onPress={() => store.remove(item.id)}
                disabled={disabled}
                accessibilityLabel={`Remove ${item.text}`}
                testID={`${testID}-remove-${item.id}`}
            >
                <X size={16} color={fg} />
            </Pressable>
        </View>
    )
}

function Rows({ store, disabled, testID }: ViewProps) {
    return (
        <>
            {store.items.map(item => (
                <Row key={item.id} item={item} store={store} disabled={disabled} testID={testID} />
            ))}
        </>
    )
}

function Composer({ store, disabled, testID }: ViewProps) {
    // An inline quick-add where an empty value cancels: the allowed useState exception.
    const [draft, setDraft] = useState('')
    const fg = useThemeColor('foreground')
    const muted = useThemeColor('muted-foreground')
    if (disabled) return null
    const submit = () => {
        const text = draft.trim()
        if (!text) return
        store.insert({ id: newRecordId(), text, done: false })
        setDraft('')
    }
    return (
        <PlainInput
            value={draft}
            onChangeText={setDraft}
            onSubmitEditing={submit}
            blurOnSubmit={false}
            placeholder="Add an item"
            placeholderTextColor={muted}
            style={{ color: fg }}
            className="border border-border rounded-md px-2 py-1 text-foreground"
            accessibilityLabel="Add a checklist item"
            testID={`${testID}-new`}
        />
    )
}

function ChecklistBody({ store, disabled, testID, accessibilityLabel }: ViewProps) {
    return (
        <View testID={testID} accessibilityLabel={accessibilityLabel}>
            <Rows store={store} disabled={disabled} testID={testID} />
            <Composer store={store} disabled={disabled} testID={testID} />
        </View>
    )
}

function LocalChecklist({
    value,
    onChange,
    disabled,
    testID,
    accessibilityLabel,
}: ChecklistFieldProps & { testID: string }) {
    const store = useLocalStore(value, onChange)
    return (
        <ChecklistBody
            store={store}
            disabled={disabled}
            testID={testID}
            accessibilityLabel={accessibilityLabel}
        />
    )
}

export function ChecklistField(props: ChecklistFieldProps) {
    return <LocalChecklist {...props} testID={props.testID ?? 'checklist'} />
}
