import { useQuery } from '@tanstack/react-query'
import { PB_SERVER_ADDR } from './config'

/** One bundled package, pinned at a tagged release. Mirrors the PinnedMember
 *  shape written by the release pipeline (utils/lib/pin-release.ts) and served
 *  by the Go /api/release handler. */
export interface ReleaseMember {
    name: string
    repo: string
    tag: string
    sha: string
}

/** The pinned-release manifest baked into the current image. `members` is
 *  empty on non-release builds (the server returns an empty shape rather than
 *  erroring), which the UI treats as "no version inventory available". */
export interface ReleaseManifest {
    appTag?: string
    appSha?: string
    releasedAt?: string
    members: ReleaseMember[]
}

// Exported for unit testing; prefer useReleaseManifest in components.
export async function fetchReleaseManifest(): Promise<ReleaseManifest> {
    const res = await fetch(`${PB_SERVER_ADDR}/api/release`, { cache: 'no-store' })
    if (!res.ok) return { members: [] }
    const body = (await res.json()) as Partial<ReleaseManifest>
    return { ...body, members: body.members ?? [] }
}

// The manifest is fixed for the lifetime of a running image — it only changes
// across deploys. A long staleTime avoids needless refetches, and retry:false
// keeps a transient blip from hammering the endpoint; the About panel simply
// renders nothing extra until data arrives.
export function useReleaseManifest() {
    return useQuery<ReleaseManifest>({
        queryKey: ['release-manifest'],
        queryFn: fetchReleaseManifest,
        staleTime: Number.POSITIVE_INFINITY,
        retry: false,
    })
}
