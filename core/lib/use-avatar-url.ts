import type { AvatarImage } from '@tinycld/core/components/Avatar'
import { useFileToken } from '@tinycld/core/file-viewer/use-authed-file-url'
import { parseCrop } from '@tinycld/core/lib/avatar'
import { pb } from '@tinycld/core/lib/pocketbase'

/** One stored bitmap serves every circle, from an 18px watcher to a 96px preview. */
const AVATAR_THUMB = '256x256'

interface AvatarUser {
    id: string
    avatar?: string
    avatar_crop?: string
}

export interface ResolvedAvatar extends AvatarImage {
    /**
     * The un-thumbed original, same `?token=` auth as `fileUrl`. Re-editing
     * must start from this, not `fileUrl` — PocketBase's `WxH` thumb form
     * center-crops to a square, so feeding the thumbnail back into the
     * cropper re-crops an already-cropped image instead of reopening the
     * user's actual framing. Display code should keep using `fileUrl`.
     */
    sourceUrl: string
}

/**
 * Resolve a user's stored avatar into a displayable `AvatarImage` (plus the
 * un-thumbed `sourceUrl` for re-editing — see `ResolvedAvatar`).
 *
 * The `users` collection's file field sits behind its view rule, so the URL
 * needs the shared `?token=`. `useFileToken` caches one token per session and
 * dedupes across consumers — exactly the shape a board full of avatars
 * needs, and because the token is stable for the session, the browser's
 * URL-keyed HTTP cache actually hits instead of busting on every render.
 */
export function useAvatarUrl(user: AvatarUser | null | undefined): ResolvedAvatar | undefined {
    // Called unconditionally: hooks can't sit behind the early return below.
    const { data: token } = useFileToken()

    if (!user?.avatar) return undefined

    const url = pb.files.getURL({ collectionId: 'users', id: user.id }, user.avatar, {
        token: token ?? '',
    })

    return {
        fileUrl: `${url}${url.includes('?') ? '&' : '?'}thumb=${AVATAR_THUMB}`,
        sourceUrl: url,
        crop: parseCrop(user.avatar_crop),
    }
}
