import { Tooltip } from '@tinycld/core/components/Tooltip'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import type { LucideIcon } from 'lucide-react-native'
import { Pressable } from 'react-native'

/**
 * A row's trailing icon action, labelled on hover.
 *
 * The tooltip is core's shared Tooltip — this file used to carry its own copy
 * of the CSS, one of three that had drifted apart (text's ToolbarTooltip had a
 * hover delay this lacked; this had the above/below flip that one lacked).
 */
export function HoverAction({
    icon: Icon,
    label,
    onPress,
    iconColor,
    iconFill,
    tooltipPosition = 'above',
}: {
    icon: LucideIcon
    label: string
    onPress?: () => void
    iconColor?: string
    iconFill?: string
    tooltipPosition?: 'above' | 'below'
}) {
    const mutedColor = useThemeColor('muted-foreground')

    return (
        <Tooltip label={label} position={tooltipPosition}>
            <Pressable
                className="p-1.5 rounded-full"
                onPress={e => {
                    e.stopPropagation()
                    e.preventDefault()
                    onPress?.()
                }}
                accessibilityLabel={label}
            >
                <Icon size={16} color={iconColor ?? mutedColor} fill={iconFill ?? 'none'} />
            </Pressable>
        </Tooltip>
    )
}
