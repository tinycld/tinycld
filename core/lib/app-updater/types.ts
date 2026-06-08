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

export interface StageDeps {
    serverUrl: string
    // The platform the bundle is for. Downloads are laid out under
    // `<tmpDir>native/<platform>/...` because the native module locates the .hbc
    // by searching that subtree — staging a flat layout would never be found and
    // every update would silently roll back to the embedded bundle.
    platform: 'ios' | 'android'
    downloadFn: (url: string, destUri: string) => Promise<{ uri: string }>
    hashFn: (fileUri: string) => Promise<string> // lowercase hex sha256
    stageBundleFn: (localDir: string, id: string) => Promise<void>
    tmpDir: string
}
