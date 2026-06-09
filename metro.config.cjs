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
// The `app-updater` local Expo native module (modules/app-updater/) resolves by
// name only AFTER `expo prebuild` autolinks it into node_modules on iOS/Android.
// The web bundle has no prebuild, so the bare `app-updater` import (reached via
// use-app-updates.ts and mark-bundle-healthy.ts) is otherwise unresolvable and
// 500s the web bundle — even though those callers no-op on web. Point it at the
// module's web stub so the web bundle resolves; native keeps using autolinking.
const APP_UPDATER_WEB_STUB = path.join(__dirname, 'modules', 'app-updater', 'index.web.ts')
const originalResolveRequest = config.resolver.resolveRequest
config.resolver.resolveRequest = (context, moduleName, platform) => {
    if (moduleName.startsWith('@tinycld/app-generated/')) {
        const subpath = moduleName.slice('@tinycld/app-generated/'.length)
        return context.resolveRequest(context, path.join(APP_GENERATED_DIR, subpath), platform)
    }
    if (moduleName === 'app-updater' && platform === 'web') {
        return context.resolveRequest(context, APP_UPDATER_WEB_STUB, platform)
    }
    return (originalResolveRequest ?? context.resolveRequest)(context, moduleName, platform)
}

module.exports = withUniwindConfig(config, {
    cssEntryFile: './global.css',
    dtsFile: './uniwind-types.d.ts',
    extraThemes: ['dark'],
})
