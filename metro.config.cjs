const path = require('node:path')
const { getDefaultConfig } = require('expo/metro-config')
const { withUniwindConfig } = require('uniwind/metro')

const config = getDefaultConfig(__dirname)

// The workspace root is one level up (app/.. ). Watch it so Metro bundles
// member source (core/, the feature siblings) reached through the
// node_modules/@tinycld/* symlinks AND resolves their deps from the
// workspace-root node_modules (members have no node_modules of their own).
const workspaceRoot = path.resolve(__dirname, '..')
config.watchFolders = [workspaceRoot]

// `@tinycld/app-generated/*` — package-generator output written to
// lib/generated/. This is a build-time contract (not a symlink artifact):
// @tinycld/core imports these virtual modules by name and the app supplies
// the concrete files. Metro doesn't read tsconfig paths, so alias here.
const APP_GENERATED_DIR = path.join(__dirname, 'lib', 'generated')
// The `app-updater` local Expo native module (modules/app-updater/) is NOT a
// node_modules package: Expo autolinking wires its NATIVE code into the iOS/
// Android build by absolute path, but it never creates a node_modules/app-updater
// symlink, so Metro's JS resolver can't satisfy the bare `app-updater` specifier
// (reached via use-app-updates.ts and mark-bundle-healthy.ts) on any platform.
// Map the specifier to the module's own entry: the web stub on web (no native
// module exists there), and index.ts on native (which requireNativeModule's the
// autolinked AppUpdaterModule). Without the native branch, `expo run:ios` 500s
// the bundle with "Unable to resolve app-updater".
const APP_UPDATER_DIR = path.join(__dirname, 'modules', 'app-updater')
const APP_UPDATER_WEB_STUB = path.join(APP_UPDATER_DIR, 'index.web.ts')
const APP_UPDATER_NATIVE_ENTRY = path.join(APP_UPDATER_DIR, 'index.ts')
const originalResolveRequest = config.resolver.resolveRequest
config.resolver.resolveRequest = (context, moduleName, platform) => {
    if (moduleName.startsWith('@tinycld/app-generated/')) {
        const subpath = moduleName.slice('@tinycld/app-generated/'.length)
        return context.resolveRequest(context, path.join(APP_GENERATED_DIR, subpath), platform)
    }
    if (moduleName === 'app-updater') {
        const target = platform === 'web' ? APP_UPDATER_WEB_STUB : APP_UPDATER_NATIVE_ENTRY
        return context.resolveRequest(context, target, platform)
    }
    return (originalResolveRequest ?? context.resolveRequest)(context, moduleName, platform)
}

module.exports = withUniwindConfig(config, {
    cssEntryFile: './global.css',
    dtsFile: './uniwind-types.d.ts',
    extraThemes: ['dark'],
})
