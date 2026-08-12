import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Image } from 'expo-image'
import { View } from 'react-native'
import { getFileIconForMime } from './file-icons'
import type { ThumbnailProps } from './types'
import { useAuthedThumbnailURL } from './use-authed-file-url'

export function Thumbnail({ source, size = 120 }: ThumbnailProps) {
    const mutedColor = useThemeColor('muted-foreground')
    const { icon: FileIcon, color: iconColor } = getFileIconForMime(source.mimeType, mutedColor)

    const { url } = useAuthedThumbnailURL(source, `${size}x${size}`)

    if (!url) {
        return (
            // Square, like the image branch — NOT `w-full`. A thumbnail that
            // fills its parent works in drive's fixed-width grid cell and
            // silently eats the whole row anywhere else: in a flex row it
            // takes every pixel and collapses the filename beside it to zero
            // width. `size` is a request for a box, and both branches owe the
            // caller the same one.
            <View
                className="items-center justify-center shrink-0"
                style={{ width: size, height: size }}
            >
                <FileIcon size={size * 0.33} color={iconColor} />
            </View>
        )
    }

    return (
        <Image
            source={{ uri: url }}
            style={{ width: size, height: size, borderRadius: 4 }}
            contentFit="cover"
            // Cache thumbnails on disk + memory so scrolling/paging the list
            // doesn't refetch them. recyclingKey is the stable record id so the
            // cached bitmap is reused across the rotating ?token= in the URL
            // (which would otherwise bust a URL-keyed cache every ~90s).
            cachePolicy="memory-disk"
            recyclingKey={source.recordId}
            transition={100}
        />
    )
}
