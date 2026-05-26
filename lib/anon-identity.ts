import AsyncStorage from '@react-native-async-storage/async-storage'
import { useQuery } from '@tanstack/react-query'
import { PB_SERVER_ADDR } from './config'

// The anon_id is the visitor's long-lived, stable identity for public
// share links. It is persisted across visits (AsyncStorage = localStorage
// on web, native store on device) so the same person keeps the same
// "Anon <Animal>" name across links and reloads. It is NOT a credential —
// the server re-derives the display name from it and signs a short-lived
// session token that actually authorizes anon actions.
const ANON_ID_KEY = 'tinycld:anon-id'

export async function readAnonId(): Promise<string | null> {
    return AsyncStorage.getItem(ANON_ID_KEY)
}

export async function writeAnonId(anonId: string): Promise<void> {
    await AsyncStorage.setItem(ANON_ID_KEY, anonId)
}

export interface ShareSession {
    sessionToken: string
    anonId: string
    displayName: string
    role: 'viewer' | 'commentor' | 'editor'
    itemId: string
    name: string
    mimeType: string
    orgName: string
    orgSlug: string
}

interface SessionResponse {
    session_token: string
    anon_id: string
    display_name: string
    role: ShareSession['role']
    item_id: string
    name: string
    mime_type: string
    org_name: string
    org_slug: string
}

// mintShareSession calls the public session endpoint, sending the cached
// anon_id (if any) so the visitor resumes their identity. The server
// mints a fresh anon_id when none is supplied; we persist whatever comes
// back so the identity sticks.
async function mintShareSession(token: string): Promise<ShareSession> {
    const cachedAnonId = await readAnonId()
    const resp = await fetch(`${PB_SERVER_ADDR}/api/drive/share-link/${token}/session`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(cachedAnonId ? { anon_id: cachedAnonId } : {}),
    })
    if (!resp.ok) {
        throw new Error(`share session failed: HTTP ${resp.status}`)
    }
    const data: SessionResponse = await resp.json()
    if (data.anon_id && data.anon_id !== cachedAnonId) {
        await writeAnonId(data.anon_id)
    }
    return {
        sessionToken: data.session_token,
        anonId: data.anon_id,
        displayName: data.display_name,
        role: data.role,
        itemId: data.item_id,
        name: data.name,
        mimeType: data.mime_type,
        orgName: data.org_name,
        orgSlug: data.org_slug,
    }
}

// useShareSession mints/resumes an anonymous share session for a public
// link token. `enabled` lets callers pause until they know the visitor is
// anonymous (logged-in org members take a different path entirely).
//
// retryOnMount=false is critical when the share endpoint returns 410
// (revoked or expired). ShareTokenPage flips between FullScreenSpinner
// and <ShareView/> based on `visitor.role` (which depends on
// sessionLoading). ShareView has its own useShareSession observer that
// remounts on each flip. React Query's default `retryOnMount: true`
// means: if the query is in `error` state and a new observer mounts,
// re-fire the queryFn (because errored queries have no data, hence
// they're treated as "needs initial load"). That single behavior turns
// the mount/unmount cycle into a tight refetch loop — fetch errors →
// flip → remount → refetch → error → flip → ... — saturating the
// public 60/min rate limiter and preventing the expired-link UI from
// rendering. `retry: false` alone is NOT enough (it suppresses retries
// of the SAME fetch attempt, not refetches from new mounts).
// `refetchOnMount: false` is also distinct (it covers the
// already-have-data case). The flag we want here is `retryOnMount:
// false`, so a failed mint stays failed until the page reloads.
export function useShareSession(token: string, enabled = true) {
    return useQuery<ShareSession>({
        queryKey: ['share-session', token],
        queryFn: () => mintShareSession(token),
        enabled: enabled && !!token,
        // The session token is short-lived server-side; refetch on a
        // cadence well under SessionTTL (12h) so long-lived tabs don't
        // hit an expired token mid-action.
        staleTime: 6 * 60 * 60 * 1000,
        retry: false,
        retryOnMount: false,
    })
}
