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

// downloadAndStage fetches the bundle + every asset into tmpDir (preserving the
// server's relative paths), verifies each SHA-256 against the manifest, and
// hands the dir to the native module to stage as pending. Any hash mismatch
// aborts BEFORE staging so a corrupt/MITM'd bundle never loads.
export async function downloadAndStage(manifest: UpdateManifest, deps: StageDeps): Promise<void> {
    const { serverUrl, downloadFn, hashFn, stageBundleFn, tmpDir } = deps

    const bundleDest = `${tmpDir}bundle.hbc`
    const got = await downloadFn(`${serverUrl}${manifest.bundleUrl}`, bundleDest)
    const bundleHash = await hashFn(got.uri)
    if (bundleHash !== manifest.bundleHash) {
        throw new Error(`bundle hash mismatch: got ${bundleHash}, want ${manifest.bundleHash}`)
    }

    for (const asset of manifest.assets) {
        const dest = `${tmpDir}${asset.key}`
        const a = await downloadFn(`${serverUrl}${asset.url}`, dest)
        const h = await hashFn(a.uri)
        if (h !== asset.hash) {
            throw new Error(`asset hash mismatch for ${asset.key}: got ${h}, want ${asset.hash}`)
        }
    }

    await stageBundleFn(tmpDir, manifest.id)
}
