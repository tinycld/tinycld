import { Text, View } from 'react-native'

const AVATAR_COLORS = [
    '#3b82f6',
    '#22c55e',
    '#a855f7',
    '#f97316',
    '#ec4899',
    '#ef4444',
    '#eab308',
    '#06b6d4',
]

function hashString(value: string): number {
    let hash = 0
    for (let i = 0; i < value.length; i++) {
        hash = (hash << 5) - hash + value.charCodeAt(i)
        hash |= 0
    }
    return Math.abs(hash)
}

/**
 * Deterministic avatar background color for a given key.
 *
 * Exported so callers can pass a *stable* key (e.g. a record id) and so the
 * mapping is unit-testable. Editing a contact's name must not change its
 * avatar color — pass `colorKey={contact.id}` for that. When no stable key is
 * available the display name is a reasonable fallback.
 */
export function avatarColor(key: string): string {
    return AVATAR_COLORS[hashString(key) % AVATAR_COLORS.length]
}

interface NameAvatarProps {
    firstName: string
    lastName?: string
    size?: number
    /**
     * Stable identifier the background color is derived from. Defaults to the
     * full name, but prefer passing an immutable id so the color survives edits
     * to the name.
     */
    colorKey?: string
}

export function NameAvatar({ firstName, lastName, size = 40, colorKey }: NameAvatarProps) {
    const fullName = `${firstName} ${lastName ?? ''}`.trim()
    const backgroundColor = avatarColor(colorKey ?? fullName)
    const letter = (firstName[0] ?? '?').toUpperCase()
    const fontSize = size * 0.42

    return (
        <View
            className="items-center justify-center"
            style={{
                width: size,
                height: size,
                borderRadius: size / 2,
                backgroundColor,
            }}
        >
            <Text
                className="text-center"
                style={{
                    color: '#fff',
                    fontWeight: '600',
                    fontSize,
                    lineHeight: size,
                }}
            >
                {letter}
            </Text>
        </View>
    )
}
