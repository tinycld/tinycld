import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import type { LucideIcon } from 'lucide-react-native'
import { Pressable } from 'react-native'
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
