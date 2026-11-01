import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { type Control, SelectInput } from '@tinycld/core/ui/form'
import { type LucideIcon, Trash2 } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import type { BuilderDraft, FormulaDraft } from '../../lib/builder/draft'
import { REPORT_TAP_TARGET } from '../ReportHeader'

// Small parts every builder section shares.

interface IconButtonProps {
    label: string
    testID: string
    isVisible: boolean
    icon: LucideIcon
    onPress: () => void
}

/** A square icon-only action (move, remove) with its label for screen readers. */
export function IconButton({ label, testID, isVisible, icon: Icon, onPress }: IconButtonProps) {
    const color = useThemeColor('muted-foreground')
    if (!isVisible) return null
    return (
        <Pressable
            testID={testID}
            accessibilityRole="button"
            accessibilityLabel={label}
            onPress={onPress}
            className="items-center justify-center"
            style={{ width: REPORT_TAP_TARGET, height: REPORT_TAP_TARGET }}
        >
            <Icon size={16} color={color} />
        </Pressable>
    )
}

/** A new formula column or computed row starts at 0, which the engine reads as a number. */
export const NEW_FORMULA: FormulaDraft = { kind: 'editable', expr: 0 }

interface FixedFormulaProps {
    testID: string
    onRemove: () => void
}

/**
 * A stored formula with parts the formula editor cannot show (R15): read-only and
 * saved unchanged, or removed to start a new formula.
 */
export function FixedFormula({ testID, onRemove }: FixedFormulaProps) {
    return (
        <View
            testID={testID}
            className="flex-row items-center gap-2 rounded-md border border-border p-2"
        >
            <Text className="flex-1 text-xs text-muted-foreground">
                This formula uses parts the builder cannot edit. It is kept as it is.
            </Text>
            <IconButton
                label="Remove formula"
                testID={`${testID}-remove`}
                isVisible
                icon={Trash2}
                onPress={onRemove}
            />
        </View>
    )
}

/** How a date groups: by each value, or into one of the engine's buckets. */
export const BUCKET_OPTIONS = [
    { value: '', label: 'Each value' },
    { value: 'day', label: 'Day' },
    { value: 'month', label: 'Month' },
    { value: 'quarter', label: 'Quarter' },
    { value: 'year', label: 'Year' },
    { value: 'fiscalYear', label: 'Fiscal year' },
]

interface BucketSelectProps {
    isVisible: boolean
    name: 'pivot.bucket' | `groups.${number}.bucket`
    control: Control<BuilderDraft>
    testID: string
}

/** A date's bucket; only a date field has one. */
export function BucketSelect({ isVisible, name, control, testID }: BucketSelectProps) {
    if (!isVisible) return null
    return (
        <SelectInput
            name={name}
            control={control}
            label="By"
            options={BUCKET_OPTIONS}
            testID={testID}
        />
    )
}
