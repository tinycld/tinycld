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

const CELL_CLASS = {
    filled: 'h-14 w-11 items-center justify-center rounded-lg border-2 border-primary bg-accent',
    empty: 'h-14 w-11 items-center justify-center rounded-lg border-2 border-border bg-background',
} as const

function Cell({ char }: { char: string }) {
    return (
        <View className={char ? CELL_CLASS.filled : CELL_CLASS.empty}>
            <Text className="font-mono text-2xl font-bold text-foreground">{char}</Text>
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
    const cells = cellsOf(value).map(c => <Cell key={c.key} char={c.char} />)
    const first = cells.slice(0, 4)
    const second = cells.slice(4)
    return (
        <Pressable
            onPress={() => ref.current?.focus()}
            className="relative flex-row items-center gap-2 self-start"
        >
            {first}
            <View className="mx-1 h-0.5 w-3 rounded bg-muted-foreground" />
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
