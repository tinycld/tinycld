import { useRef } from 'react'
import { Pressable, Text, TextInput, View } from 'react-native'

const CELLS = 8

// Cell positions are fixed, so the position is a stable key.
function cellsOf(value: string): { key: string; char: string }[] {
    const chars = value
        .replace(/[^A-Z0-9]/gi, '')
        .toUpperCase()
        .slice(0, CELLS)
        .split('')
    return Array.from({ length: CELLS }, (_, i) => ({ key: `cell-${i}`, char: chars[i] ?? '' }))
}

function Cell({ char, isFilled }: { char: string; isFilled: boolean }) {
    return (
        <View
            className={`h-11 w-9 items-center justify-center rounded-lg border-[1.5px] ${isFilled ? 'border-primary bg-primary/10' : 'border-border'}`}
        >
            <Text className="font-mono text-xl font-bold text-foreground">{char}</Text>
        </View>
    )
}

/**
 * One TextInput so paste and autofill work on both platforms; the eight cells
 * are drawn behind it and the input itself is invisible.
 */
export function CodeInput({
    value,
    onChangeText,
}: {
    value: string
    onChangeText: (v: string) => void
}) {
    const ref = useRef<TextInput>(null)
    const cells = cellsOf(value).map(c => (
        <Cell key={c.key} char={c.char} isFilled={c.char !== ''} />
    ))
    const first = cells.slice(0, 4)
    const second = cells.slice(4)
    return (
        <Pressable
            onPress={() => ref.current?.focus()}
            className="relative flex-row items-center gap-1.5 self-start"
        >
            {first}
            <Text className="text-lg text-muted-foreground">–</Text>
            {second}
            <TextInput
                ref={ref}
                testID="setup-code"
                accessibilityLabel="Setup code"
                value={value}
                onChangeText={onChangeText}
                autoCapitalize="characters"
                autoCorrect={false}
                autoComplete="one-time-code"
                textContentType="oneTimeCode"
                maxLength={9}
                className="absolute inset-0 opacity-0"
            />
        </Pressable>
    )
}
