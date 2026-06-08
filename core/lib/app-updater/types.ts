export interface UpdateAsset {
    key: string
    hash: string
    contentType: string
    url: string
}

export interface UpdateManifest {
    id: string
    runtimeVersion: string
    bundleUrl: string
    bundleHash: string
    assets: UpdateAsset[]
}

export interface CheckDeps {
    serverUrl: string
    platform: 'ios' | 'android'
    runtimeVersion: string
    currentId: string
    fetchFn: typeof fetch
}
