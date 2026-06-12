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
    // Hex SHA-256 of the bundle the app is currently running. Sent so the server
    // can recognize an already-current bundle even when ids differ across the
    // embedded→OTA boundary (a fresh install's id is `embedded-<version>`, never
    // equal to a server `build-<ts>-<platform>` id). Empty string when the native
    // module can't supply it — the server then falls back to the id comparison.
    currentHash: string
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
    // Hands the staged dir, the bundle id, and its hex SHA-256 to the native
    // module. The hash is persisted so getCurrentBundleHash can report it once
    // this bundle becomes the active one (the server's up-to-date check).
    stageBundleFn: (localDir: string, id: string, hash: string) => Promise<void>
    tmpDir: string
}
