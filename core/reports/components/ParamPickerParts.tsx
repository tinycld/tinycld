import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { Check } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import type { ParamValue } from '../lib/params'
import type { ParamDef } from '../lib/spec'
import { REPORT_TAP_TARGET } from './ReportHeader'

// The parts every param picker body shares (ParamPickers, SourceChoicePicker).

export const LIST_STYLE = { maxHeight: 360 }
export const TARGET_STYLE = { minHeight: REPORT_TAP_TARGET }

export interface BodyProps {
    def: ParamDef
    value: ParamValue | null
    onApply(value: ParamValue): void
}

export function ApplyButton({ isDisabled, onPress }: { isDisabled: boolean; onPress: () => void }) {
    return (
        <View className="border-t border-border pt-2">
            <Button
                testID="report-filter-apply"
                style={TARGET_STYLE}
                isDisabled={isDisabled}
                onPress={onPress}
            >
                <ButtonText>Apply</ButtonText>
            </Button>
        </View>
    )
}

function CheckMark({ isVisible }: { isVisible: boolean }) {
    const color = useThemeColor('primary-foreground')
    if (!isVisible) return null
    return <Check size={14} color={color} />
}

interface OptionRowProps {
    value: string
    label: string
    isChecked: boolean
    isRadio: boolean
    /** Checked by something else (its whole group is picked), so it is not toggled alone. */
    isDisabled?: boolean
    onPress: () => void
}

export function OptionRow({
    value,
    label,
    isChecked,
    isRadio,
    isDisabled = false,
    onPress,
}: OptionRowProps) {
    const tone = isChecked ? 'border-primary bg-primary' : 'border-border bg-transparent'
    const shape = isRadio ? 'rounded-full' : 'rounded'
    const role = isRadio ? 'radio' : 'checkbox'
    const fade = isDisabled ? 'opacity-60' : ''
    return (
        <Pressable
            testID={`report-filter-option-${value}`}
            accessibilityRole={role}
            accessibilityState={{ checked: isChecked, disabled: isDisabled }}
            aria-checked={isChecked}
            aria-disabled={isDisabled}
            disabled={isDisabled}
            onPress={onPress}
            className={`flex-row items-center gap-3 rounded-md px-2 ${fade}`}
            style={TARGET_STYLE}
        >
            <View className={`h-5 w-5 items-center justify-center border ${shape} ${tone}`}>
                <CheckMark isVisible={isChecked} />
            </View>
            <Text numberOfLines={1} className="min-w-0 flex-1 text-sm text-foreground">
                {label}
            </Text>
        </Pressable>
    )
}
