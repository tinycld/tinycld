import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import type { LucideIcon } from 'lucide-react-native'
import { Pressable } from 'react-native'
import { useSafeAreaInsets } from 'react-native-safe-area-context'

interface FABProps {
    icon: LucideIcon
    onPress: () => void
    accessibilityLabel: string
    isVisible: boolean
    size?: number
    iconSize?: number
}

// Approximate height of the bottom tab bar's own content (pt-2 + icon + label
// + pb-1), excluding the safe-area inset which we add separately. The FAB
// clears this plus a small gap so it floats just above the toolbar.
const BOTTOM_BAR_CONTENT_HEIGHT = 56
const FAB_GAP_ABOVE_BAR = 16

// All FABs anchor to the bottom-RIGHT corner for consistency across packages
// (compose in mail, "+" in calendar, etc.) and sit just above the bottom tab
// bar. The bottom offset tracks the safe-area inset so the gap above the bar
// is uniform on notched and non-notched devices alike.
export function FAB({
    icon: Icon,
    onPress,
    accessibilityLabel,
    isVisible,
    size = 56,
    iconSize = 22,
}: FABProps) {
    const primaryFg = useThemeColor('primary-foreground')
    const insets = useSafeAreaInsets()

    if (!isVisible) return null

    return (
        <Pressable
            className="absolute items-center justify-center bg-primary"
            style={{
                bottom: insets.bottom + BOTTOM_BAR_CONTENT_HEIGHT + FAB_GAP_ABOVE_BAR,
                right: 16,
                width: size,
                height: size,
                borderRadius: size / 2,
                elevation: 4,
                shadowColor: '#000',
                shadowOffset: { width: 0, height: 2 },
                shadowOpacity: 0.25,
                shadowRadius: 4,
                zIndex: 50,
            }}
            onPress={onPress}
            accessibilityLabel={accessibilityLabel}
        >
            <Icon size={iconSize} color={primaryFg} />
        </Pressable>
    )
}
