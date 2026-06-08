import type { CheckDeps, StageDeps, UpdateManifest } from './types'

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

// relativePathFromUrl extracts the bundle-relative path the server encoded after
// the `/<platform>/` segment of a bundle/asset URL — e.g.
// `/api/app/bundle/build-200/ios/_expo/static/js/ios/index.hbc` → with platform
// `ios` → `_expo/static/js/ios/index.hbc`. The native module reconstructs the
// runtime layout from these paths, so we must preserve them under the staged dir.
function relativePathFromUrl(url: string, platform: string): string {
    const marker = `/${platform}/`
    const i = url.indexOf(marker)
    if (i === -1) throw new Error(`malformed bundle/asset url (no /${platform}/ segment): ${url}`)
    return url.slice(i + marker.length)
}

// downloadAndStage fetches the bundle + every asset into
// `<tmpDir>native/<platform>/<relative-path>` — the exact layout the native
// module's locateHbc walks — verifies each SHA-256 against the manifest, and
// hands tmpDir to the native module to stage as pending. Any hash mismatch
// aborts BEFORE staging so a corrupt/MITM'd bundle never loads. The download
// layout MUST match the native search path (`native/<platform>/`); otherwise the
// staged bundle is never found and the update silently rolls back to embedded.
export async function downloadAndStage(manifest: UpdateManifest, deps: StageDeps): Promise<void> {
    const { serverUrl, platform, downloadFn, hashFn, stageBundleFn, tmpDir } = deps
    const nativeRoot = `${tmpDir}native/${platform}/`

    const bundleRel = relativePathFromUrl(manifest.bundleUrl, platform)
    const got = await downloadFn(`${serverUrl}${manifest.bundleUrl}`, `${nativeRoot}${bundleRel}`)
    const bundleHash = await hashFn(got.uri)
    if (bundleHash !== manifest.bundleHash) {
        throw new Error(`bundle hash mismatch: got ${bundleHash}, want ${manifest.bundleHash}`)
    }

    for (const asset of manifest.assets) {
        const assetRel = relativePathFromUrl(asset.url, platform)
        const a = await downloadFn(`${serverUrl}${asset.url}`, `${nativeRoot}${assetRel}`)
        const h = await hashFn(a.uri)
        if (h !== asset.hash) {
            throw new Error(`asset hash mismatch for ${asset.key}: got ${h}, want ${asset.hash}`)
        }
    }

    await stageBundleFn(tmpDir, manifest.id)
}
