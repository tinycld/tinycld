import { parseNativeEmoji } from '@tinycld/core/lib/emoji/parse'
import { applyTone, NEUTRAL_TONE, SKIN_TONES, type ToneChoice } from '@tinycld/core/lib/emoji/tones'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { Search, X } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'

/** The hand the tone swatch previews. Tonable, and reads as a person. */
const TONE_PREVIEW = '1f44b'

/** Default first, then light to dark — the order every other picker uses. */
const TONE_CHOICES: readonly ToneChoice[] = [NEUTRAL_TONE, ...SKIN_TONES]

export interface EmojiSearchProps {
    query: string
    onQueryChange: (query: string) => void
    tone: ToneChoice
    onToneChange: (tone: ToneChoice) => void
    autoFocus?: boolean
}

/**
 * Search field plus the skin-tone swatch. Filtering is local, so there is no
 * debounce — useDebouncedValue exists for network round-trips.
 */
export function EmojiSearch({
    query,
    onQueryChange,
    tone,
    onToneChange,
    autoFocus = false,
}: EmojiSearchProps) {
    const mutedColor = useThemeColor('muted')
    const placeholderColor = useThemeColor('field-placeholder')

    return (
        <View className="flex-row items-center gap-1.5 px-2 py-1.5">
            <View className="flex-1 flex-row items-center gap-1.5 rounded-md border border-border px-2 py-1">
                <Search size={13} color={mutedColor} />
                <PlainInput
                    value={query}
                    onChangeText={onQueryChange}
                    placeholder="Search emoji"
                    placeholderTextColor={placeholderColor}
                    autoFocus={autoFocus}
                    autoCorrect={false}
                    autoCapitalize="none"
                    testID="emoji-search"
                    className="flex-1 text-[13px] text-foreground"
                />
                <ClearButton isVisible={query !== ''} onPress={() => onQueryChange('')} />
            </View>
            <TonePicker tone={tone} onToneChange={onToneChange} />
        </View>
    )
}

function ClearButton({ isVisible, onPress }: { isVisible: boolean; onPress: () => void }) {
    const mutedColor = useThemeColor('muted')
    if (!isVisible) return null
    return (
        <Pressable
            onPress={onPress}
            accessibilityRole="button"
            accessibilityLabel="Clear search"
            testID="emoji-search-clear"
            hitSlop={6}
        >
            <X size={13} color={mutedColor} />
        </Pressable>
    )
}

/**
 * Collapsed to one swatch until pressed, then the five tones inline. A
 * persistent row of six would take a third of the header for a control most
 * people set once.
 */
function TonePicker({
    tone,
    onToneChange,
}: {
    tone: ToneChoice
    onToneChange: (tone: ToneChoice) => void
}) {
    const [isOpen, setIsOpen] = useState(false)

    const choose = (next: ToneChoice) => {
        onToneChange(next)
        setIsOpen(false)
    }

    if (!isOpen) {
        return (
            <ToneSwatch
                tone={tone}
                label="Change skin tone"
                testID="emoji-tone-toggle"
                onPress={() => setIsOpen(true)}
            />
        )
    }

    return (
        <View className="flex-row items-center rounded-md border border-border">
            {TONE_CHOICES.map(choice => (
                <ToneSwatch
                    key={choice}
                    tone={choice}
                    label={`Skin tone ${choice === NEUTRAL_TONE ? 'default' : choice}`}
                    testID={`emoji-tone-${choice}`}
                    isActive={choice === tone}
                    onPress={() => choose(choice)}
                />
            ))}
        </View>
    )
}

function ToneSwatch({
    tone,
    label,
    testID,
    isActive = false,
    onPress,
}: {
    tone: ToneChoice
    label: string
    testID: string
    isActive?: boolean
    onPress: () => void
}) {
    return (
        <Pressable
            onPress={onPress}
            accessibilityRole="button"
            accessibilityLabel={label}
            accessibilityState={{ selected: isActive }}
            testID={testID}
            className={`h-6 w-6 items-center justify-center rounded web:outline-none web:focus-visible:ring-2 web:focus-visible:ring-ring ${isActive ? 'bg-accent' : ''}`}
        >
            <Text className="text-[15px]">{parseNativeEmoji(applyTone(TONE_PREVIEW, tone))}</Text>
        </Pressable>
    )
}
