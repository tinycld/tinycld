import type { TaxonomyFormValues } from '@tinycld/core/lib/classifier/taxonomy-schema'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { type Control, TextInput, Toggle } from '@tinycld/core/ui/form'
import { ArrowDown, ArrowUp, Trash2 } from 'lucide-react-native'
import { Pressable, View } from 'react-native'

interface TaxonomyRowProps {
    control: Control<TaxonomyFormValues>
    index: number
    isFirst: boolean
    isLast: boolean
    autoActionLabel: string
    onRemove: (index: number) => void
    onMove: (from: number, to: number) => void
    testID: string
}

export function TaxonomyRow({
    control,
    index,
    isFirst,
    isLast,
    autoActionLabel,
    onRemove,
    onMove,
    testID,
}: TaxonomyRowProps) {
    const mutedColor = useThemeColor('muted-foreground')
    const rowID = `${testID}-row-${index}`
    return (
        <View className="border border-border rounded-lg p-3 gap-1 bg-surface-secondary">
            <View className="flex-row items-start gap-2">
                <View className="flex-1 min-w-0">
                    <TextInput
                        control={control}
                        name={`rows.${index}.name`}
                        label="Label"
                        placeholder="Label name"
                        testID={`${rowID}-name`}
                    />
                </View>
                <Pressable
                    onPress={() => onMove(index, index - 1)}
                    disabled={isFirst}
                    accessibilityLabel="Move label up"
                    className={`p-2 mt-5 ${isFirst ? 'opacity-40' : ''}`}
                    testID={`${rowID}-up`}
                >
                    <ArrowUp size={16} color={mutedColor} />
                </Pressable>
                <Pressable
                    onPress={() => onMove(index, index + 1)}
                    disabled={isLast}
                    accessibilityLabel="Move label down"
                    className={`p-2 mt-5 ${isLast ? 'opacity-40' : ''}`}
                    testID={`${rowID}-down`}
                >
                    <ArrowDown size={16} color={mutedColor} />
                </Pressable>
                <Pressable
                    onPress={() => onRemove(index)}
                    accessibilityLabel="Remove label"
                    className="p-2 mt-5"
                    testID={`${rowID}-remove`}
                >
                    <Trash2 size={16} color={mutedColor} />
                </Pressable>
            </View>
            <TextInput
                control={control}
                name={`rows.${index}.assignWhen`}
                label="Use this label when"
                placeholder="Describe the messages that belong here"
                multiline
                numberOfLines={3}
                testID={`${rowID}-rule`}
            />
            <Toggle
                control={control}
                name={`rows.${index}.autoAction`}
                label={autoActionLabel}
                testID={`${rowID}-auto`}
            />
        </View>
    )
}
