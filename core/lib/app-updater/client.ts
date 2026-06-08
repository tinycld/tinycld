import type { CheckDeps, UpdateManifest } from './types'

// checkForUpdate asks the connected server for a newer bundle. Returns the
// manifest when one is available, or null when the server replies 204 (up to
// date, runtime mismatch, or no native bundle). Network/parse errors propagate
// to the caller, which captures them.
export async function checkForUpdate(deps: CheckDeps): Promise<UpdateManifest | null> {
    const { serverUrl, platform, runtimeVersion, currentId, fetchFn } = deps
    const url =
        `${serverUrl}/api/app/update?platform=${platform}` +
        `&runtimeVersion=${encodeURIComponent(runtimeVersion)}` +
        `&currentId=${encodeURIComponent(currentId)}`
    const res = await fetchFn(url)
    if (res.status === 204) return null
    if (!res.ok) throw new Error(`update check failed: ${res.status}`)
    return (await res.json()) as UpdateManifest
}
