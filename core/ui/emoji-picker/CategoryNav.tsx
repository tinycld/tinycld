import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Pressable, View } from 'react-native'
import { CATEGORY_ICONS, CATEGORY_LABELS } from './categories'

export interface CategoryNavProps {
    categories: readonly string[]
    active: string | null
    onSelect: (category: string) => void
}

/** The icon row that jumps the grid to a category. */
export function CategoryNav({ categories, active, onSelect }: CategoryNavProps) {
    return (
        <View className="flex-row items-center justify-between border-b border-border px-2 pb-1.5">
            {categories.map(category => (
                <CategoryButton
                    key={category}
                    category={category}
                    isActive={category === active}
                    onPress={() => onSelect(category)}
                />
            ))}
        </View>
    )
}

function CategoryButton({
    category,
    isActive,
    onPress,
}: {
    category: string
    isActive: boolean
    onPress: () => void
}) {
    const activeColor = useThemeColor('foreground')
    const mutedColor = useThemeColor('muted')
    const Icon = CATEGORY_ICONS[category]
    if (!Icon) return null

    return (
        <Pressable
            onPress={onPress}
            accessibilityRole="tab"
            accessibilityState={{ selected: isActive }}
            accessibilityLabel={CATEGORY_LABELS[category] ?? category}
            testID={`emoji-category-${category}`}
            className={`h-7 w-7 items-center justify-center rounded web:outline-none web:focus-visible:ring-2 web:focus-visible:ring-ring ${isActive ? 'bg-accent' : ''}`}
        >
            <Icon size={15} color={isActive ? activeColor : mutedColor} strokeWidth={2} />
        </Pressable>
    )
}
