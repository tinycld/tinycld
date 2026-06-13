interface UpdateManifest {
    id: string
    runtimeVersion: string
    bundleHash: string
    bundleUrl: string
    assets: unknown[]
}

interface PrecheckArgs {
    serverUrl: string // e.g. http://localhost:7200
    runtimeVersion: string // app version, e.g. 1.13.7
    embeddedId: string // embedded-<version>
}

// Ask the server, as the app would, whether a newer iOS bundle is available for
// a client currently on the embedded bundle. Returns the new bundle id (N) when
// the server offers an update (200), or throws a diagnostic error when it
// answers 204 — which means no newer bundle was staged, the single most likely
// reason the whole run would otherwise hang waiting for a reload that can't come.
export async function precheckNewerBundle(args: PrecheckArgs): Promise<string> {
    const { serverUrl, runtimeVersion, embeddedId } = args
    const url =
        `${serverUrl}/api/app/update?platform=ios` +
        `&runtimeVersion=${encodeURIComponent(runtimeVersion)}` +
        `&currentId=${encodeURIComponent(embeddedId)}` +
        `&currentHash=`

    const res = await fetch(url)
    if (res.status === 204) {
        throw new Error(
            `Precheck failed: server has no newer ios bundle for runtime=${runtimeVersion} ` +
                `(GET ${url} → 204). The bundle-staging step did not produce one — ` +
                `verify the server ran a build/install that exported an ios bundle.`
        )
    }
    if (!res.ok) {
        throw new Error(`Precheck failed: GET ${url} → ${res.status}`)
    }
    const manifest = (await res.json()) as UpdateManifest
    return manifest.id
}
