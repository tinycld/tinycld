import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { type Control, SelectInput } from '@tinycld/core/ui/form'
import { type PickItem, PickMenu } from '@tinycld/core/ui/pick-menu'
import { type LucideIcon, Trash2 } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { firstTextField } from '../../lib/builder/columns'
import type { BuilderDraft, FormulaDraft } from '../../lib/builder/draft'
import type { CatalogSource } from '../../lib/types'
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

/** A refusal under the part it is about; nothing when there is none. */
export function FieldError({ message, testID }: { message: string | undefined; testID: string }) {
    if (!message) return null
    return (
        <Text testID={testID} role="alert" className="text-xs text-danger">
            {message}
        </Text>
    )
}

/** A new formula column or computed row starts at 0, which the engine reads as a number. */
export const NEW_FORMULA: FormulaDraft = { kind: 'editable', node: { value: 0 } }

interface FixedFormulaProps {
    testID: string
    onRemove: () => void
}

/**
 * A stored formula with parts the formula editor cannot show (R17): read-only and
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
    onValueChange?: () => void
}

/** A date's bucket; only a date field has one. */
export function BucketSelect({
    isVisible,
    name,
    control,
    testID,
    onValueChange,
}: BucketSelectProps) {
    if (!isVisible) return null
    return (
        <SelectInput
            name={name}
            control={control}
            label="By"
            options={BUCKET_OPTIONS}
            testID={testID}
            onValueChange={onValueChange}
        />
    )
}

interface RecordLabelFieldProps {
    /** The record the values point at; nothing is shown without one. */
    target: CatalogSource | undefined
    labelField: string
    purpose: string
    testID: string
    onChange: (labelField: string) => void
}

/**
 * "Show as": the field a record is named by where its values show, a link column's
 * cells or a pivot's columns (R31, F22). '' is the record's first text field.
 */
export function RecordLabelField({
    target,
    labelField,
    purpose,
    testID,
    onChange,
}: RecordLabelFieldProps) {
    if (!target) return null
    const first = firstTextField(target)
    const items: PickItem[] = [
        {
            id: ':first',
            label: first ? `${first.label} (first text field)` : target.label,
            isSelected: labelField === '',
            onSelect: () => onChange(''),
        },
        ...target.fields
            .filter(f => f.key !== 'id')
            .map(f => ({
                id: f.key,
                label: f.label,
                isSelected: f.key === labelField,
                onSelect: () => onChange(f.key),
            })),
    ]
    const shown = items.find(i => i.isSelected)?.label ?? `${labelField} (missing)`
    return (
        <View className="flex-row items-center gap-2">
            <Text className="text-xs text-muted-foreground">Show as</Text>
            <PickMenu label={shown} purpose={purpose} items={items} testID={testID} />
        </View>
    )
}
