import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Calendar, X } from 'lucide-react-native'
import type { Ref } from 'react'
import { Pressable, Text, View, type ViewInstance } from 'react-native'
import { REPORT_TAP_TARGET } from './ReportHeader'

const TARGET_STYLE = { minHeight: REPORT_TAP_TARGET, marginVertical: -1 }
const REMOVE_STYLE = { ...TARGET_STYLE, minWidth: REPORT_TAP_TARGET }
const CHIP_STYLE = { minHeight: REPORT_TAP_TARGET }

interface RemoveControlProps {
    isVisible: boolean
    label: string
    testID: string
    onRemove: () => void
}

function RemoveControl({ isVisible, label, testID, onRemove }: RemoveControlProps) {
    const muted = useThemeColor('muted-foreground')
    if (!isVisible) return null
    return (
        <Pressable
            testID={testID}
            accessibilityRole="button"
            accessibilityLabel={label}
            onPress={onRemove}
            className="items-center justify-center"
            style={REMOVE_STYLE}
        >
            <X size={14} color={muted} />
        </Pressable>
    )
}

function ChipIcon({ isVisible }: { isVisible: boolean }) {
    const muted = useThemeColor('muted-foreground')
    if (!isVisible) return null
    return <Calendar size={14} color={muted} />
}

interface ParamChipProps {
    paramId: string
    text: string
    label: string
    isDate: boolean
    isRemovable: boolean
    onPress?: () => void
    onRemove: () => void
    /** Set by the picker's Popover, which clones its trigger with an anchor ref. */
    ref?: Ref<ViewInstance>
}

// A chip shows its value at rest (the shell's spark 3). Its label opens the picker;
// a removable chip has its own 44pt remove target.
export function ParamChip({
    paramId,
    text,
    label,
    isDate,
    isRemovable,
    onPress,
    onRemove,
    ref,
}: ParamChipProps) {
    const padding = isRemovable ? '' : 'pr-3'
    return (
        <View
            ref={ref}
            className={`flex-row items-center gap-1.5 rounded-full border border-border bg-secondary pl-3 ${padding}`}
            style={CHIP_STYLE}
        >
            <ChipIcon isVisible={isDate} />
            <Pressable
                testID={`report-param-${paramId}`}
                accessibilityRole="button"
                accessibilityLabel={`${label}: ${text}`}
                onPress={onPress}
                className="justify-center"
                style={TARGET_STYLE}
            >
                <Text numberOfLines={1} className="text-sm text-secondary-foreground">
                    {text}
                </Text>
            </Pressable>
            <RemoveControl
                isVisible={isRemovable}
                label={`Remove ${label} filter`}
                testID={`report-filter-remove-${paramId}`}
                onRemove={onRemove}
            />
        </View>
    )
}
