import { Avatar, type AvatarProps } from '@tinycld/core/components/Avatar'
import { Text, View } from 'react-native'

export type AvatarStackItem = { key: string } & Omit<AvatarProps, 'size' | 'ring' | 'testID'>

interface AvatarStackProps {
    items: readonly AvatarStackItem[]
    max?: number
    size?: number
    ring?: AvatarProps['ring']
    testID?: string
}

/**
 * An overlapping row of avatars with a "+N" badge for the remainder. Shared by
 * every stacked-identity surface (document presence, card watchers, assignees)
 * so the overlap and overflow behave identically across them.
 */
export function AvatarStack({
    items,
    max = 4,
    size = 24,
    ring = 'none',
    testID,
}: AvatarStackProps) {
    if (items.length === 0) return null

    const visible = items.slice(0, max)
    const overflow = items.length - visible.length
    const offset = -size / 3

    return (
        <View testID={testID} className="flex-row items-center">
            {visible.map((item, index) => {
                const { key, ...avatarProps } = item
                return (
                    <View
                        key={key}
                        testID={testID ? `${testID}-item-${index}` : undefined}
                        style={{ marginLeft: index === 0 ? 0 : offset }}
                    >
                        <Avatar {...avatarProps} size={size} ring={ring} />
                    </View>
                )
            })}
            <OverflowBadge
                count={overflow}
                size={size}
                offset={offset}
                testID={testID ? `${testID}-overflow` : undefined}
            />
        </View>
    )
}

function OverflowBadge({
    count,
    size,
    offset,
    testID,
}: {
    count: number
    size: number
    offset: number
    testID: string | undefined
}) {
    if (count <= 0) return null

    return (
        <View
            testID={testID}
            className="items-center justify-center bg-surface-secondary border border-background"
            style={{
                width: size,
                height: size,
                borderRadius: size / 2,
                marginLeft: offset,
            }}
        >
            <Text className="text-foreground font-semibold" style={{ fontSize: size * 0.4 }}>
                +{count}
            </Text>
        </View>
    )
}
