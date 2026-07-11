import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Star } from 'lucide-react-native'

interface StarIconProps {
    isStarred: boolean
    size?: number
}

export function StarIcon({ isStarred, size = 16 }: StarIconProps) {
    const mutedColor = useThemeColor('muted-foreground')
    // `warning` is the theme's amber/gold token — it renders as a legible gold in
    // both light and dark, unlike a fixed hex that only suited light mode.
    const starredColor = useThemeColor('warning')

    return (
        <Star
            size={size}
            color={isStarred ? starredColor : mutedColor}
            fill={isStarred ? starredColor : 'transparent'}
        />
    )
}
