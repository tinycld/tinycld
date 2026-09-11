import {
    avatarColor,
    type CropRect,
    cropToTransform,
    DEFAULT_CROP,
    resolveInitials,
    softAvatarColors,
} from '@tinycld/core/lib/avatar'
import { Image } from 'expo-image'
import { Text, View } from 'react-native'

export interface AvatarImage {
    fileUrl: string
    crop?: CropRect
}

export interface AvatarProps {
    name: string
    email?: string
    size?: number
    /** Stable id the color derives from; falls back to email, then name. */
    colorKey?: string
    avatar?: AvatarImage
    emoji?: string
    /** Explicit background override — presence passes its awareness color. */
    color?: string
    palette?: 'solid' | 'soft'
    shape?: 'circle' | 'squircle'
    ring?: 'none' | 'background' | 'card'
    dimmed?: boolean
    testID?: string
}

const RING_CLASS = {
    none: '',
    background: 'border border-background',
    card: 'border-2 border-card',
} as const

/**
 * The one circle that represents a person or an organization.
 *
 * Renders image → emoji → initials, so a user who has uploaded a photo sees it
 * everywhere and one who hasn't still gets a stable, legible circle. Presence
 * callers pass `color` to override the identity-derived palette: a transient
 * viewer must not read as an assignee.
 */
export function Avatar({
    name,
    email,
    size = 40,
    colorKey,
    avatar,
    emoji,
    color,
    palette = 'solid',
    shape = 'circle',
    ring = 'none',
    dimmed = false,
    testID,
}: AvatarProps) {
    const key = colorKey ?? (email || name)
    const [softBg, softFg] = softAvatarColors(key)
    const backgroundColor = color ?? (palette === 'soft' ? softBg : avatarColor(key))
    const foregroundColor = palette === 'soft' && !color ? softFg : '#fff'
    const borderRadius = shape === 'circle' ? size / 2 : size * 0.32

    return (
        <View
            testID={testID}
            accessibilityRole="image"
            accessibilityLabel={name || email || 'Avatar'}
            className={`items-center justify-center overflow-hidden ${RING_CLASS[ring]}`}
            style={{
                width: size,
                height: size,
                borderRadius,
                backgroundColor,
                opacity: dimmed ? 0.55 : 1,
            }}
        >
            <AvatarContent
                avatar={avatar}
                emoji={emoji}
                name={name}
                email={email}
                size={size}
                foregroundColor={foregroundColor}
                testID={testID}
            />
        </View>
    )
}

function AvatarContent({
    avatar,
    emoji,
    name,
    email,
    size,
    foregroundColor,
    testID,
}: {
    avatar: AvatarImage | undefined
    emoji: string | undefined
    name: string
    email: string | undefined
    size: number
    foregroundColor: string
    testID: string | undefined
}) {
    if (avatar?.fileUrl) {
        const { width, height, translateX, translateY } = cropToTransform(
            avatar.crop ?? DEFAULT_CROP,
            size
        )
        return (
            <Image
                testID={testID ? `${testID}-image` : undefined}
                source={{ uri: avatar.fileUrl }}
                contentFit="cover"
                style={{
                    width,
                    height,
                    transform: [{ translateX }, { translateY }],
                }}
            />
        )
    }

    if (emoji) {
        return <Text style={{ fontSize: size * 0.52, lineHeight: size }}>{emoji}</Text>
    }

    return (
        <Text
            style={{
                color: foregroundColor,
                fontWeight: '700',
                fontSize: size * 0.36,
                letterSpacing: 0.2,
            }}
        >
            {resolveInitials(name, email)}
        </Text>
    )
}
