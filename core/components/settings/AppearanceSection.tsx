import { SectionCard } from '@tinycld/core/components/settings/SectionCard'
import { COLOR_THEMES, type ColorThemeSlug } from '@tinycld/core/lib/color-themes'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useColorTheme } from '@tinycld/core/lib/use-color-theme'
import { type ThemePreference, useThemePreference } from '@tinycld/core/lib/use-theme-preference'
import { Check } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'

const THEME_OPTIONS: { value: ThemePreference; label: string; description: string }[] = [
    { value: 'system', label: 'System', description: 'Follow your device settings' },
    { value: 'light', label: 'Light', description: 'Always use light theme' },
    { value: 'dark', label: 'Dark', description: 'Always use dark theme' },
]

export function AppearanceSection() {
    const primaryColor = useThemeColor('primary')
    const { preference, setPreference, resolved } = useThemePreference()
    const { colorTheme, setColorTheme } = useColorTheme()

    return (
        <View className="gap-3">
            <Text className="text-foreground text-xl font-bold">Theme</Text>
            <SectionCard>
                <View className="gap-4">
                    <View className="gap-1">
                        {THEME_OPTIONS.map(option => (
                            <Pressable
                                key={option.value}
                                onPress={() => setPreference(option.value)}
                                className="flex-row items-center py-2.5 px-1 rounded-lg"
                            >
                                <View className="flex-1">
                                    <Text className="text-foreground text-base font-semibold">
                                        {option.label}
                                    </Text>
                                    <Text className="text-muted-foreground text-[13px]">
                                        {option.description}
                                    </Text>
                                </View>
                                {preference === option.value && (
                                    <Check size={20} color={primaryColor} />
                                )}
                            </Pressable>
                        ))}
                    </View>

                    <View className="h-px bg-muted-foreground/30" />

                    <View className="gap-2">
                        <Text className="text-foreground text-sm font-semibold">Accent Color</Text>
                        <ColorThemePicker
                            selected={colorTheme}
                            onSelect={setColorTheme}
                            isDark={resolved === 'dark'}
                        />
                    </View>
                </View>
            </SectionCard>
        </View>
    )
}

function ColorThemePicker({
    selected,
    onSelect,
    isDark,
}: {
    selected: ColorThemeSlug
    onSelect: (slug: ColorThemeSlug) => void
    isDark: boolean
}) {
    const borderColor = useThemeColor('border')
    const onSwatchColor = useThemeColor('primary-foreground')

    return (
        <View className="flex-row gap-4 flex-wrap">
            {COLOR_THEMES.map(theme => {
                const isActive = selected === theme.slug
                const swatchColor = isDark ? theme.swatchDark : theme.swatch
                return (
                    <Pressable
                        key={theme.slug}
                        onPress={() => onSelect(theme.slug)}
                        className="items-center gap-1.5"
                    >
                        <View
                            className="items-center justify-center"
                            style={{
                                width: 40,
                                height: 40,
                                borderRadius: 20,
                                backgroundColor: swatchColor,
                                borderWidth: isActive ? 3 : 1,
                                borderColor: isActive ? swatchColor : borderColor,
                            }}
                        >
                            {isActive && <Check size={18} color={onSwatchColor} />}
                        </View>
                        <Text
                            className={`text-[11px] ${isActive ? 'font-semibold text-foreground' : 'font-normal text-muted-foreground'}`}
                        >
                            {theme.label}
                        </Text>
                    </Pressable>
                )
            })}
        </View>
    )
}
