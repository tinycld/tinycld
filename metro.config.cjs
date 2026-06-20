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

// Per-icon deep imports for lucide-react-native. Importing named icons from the
// package root (`import { Folder } from 'lucide-react-native'`) pulls the whole
// ~1700-icon barrel into the bundle — Metro doesn't tree-shake, so a handful of
// icons costs ~3.3 MB. Each icon also exists as its own module
// (dist/esm/icons/<kebab>.mjs, default export), but lucide's `exports` map only
// publishes `.` and `./icons`, so a bare deep specifier throws
// ERR_PACKAGE_PATH_NOT_EXPORTED. We map `lucide-react-native/icons/<kebab>` to
// the real per-icon file ourselves, bypassing the exports gate. The
// babel-plugin-lucide-deep-imports rewrite (and the generated package-icons map)
// emit this specifier so only referenced icons get bundled.
//
// lucide's exports map is strict — even `./package.json` and `./dist/*` throw —
// so resolve the allowed main entry and walk up to the package root, then point
// at the ESM per-icon dir (the same build the `module`/`browser` entry uses).
const LUCIDE_PKG_ROOT = (() => {
    let dir = path.dirname(require.resolve('lucide-react-native'))
    while (path.basename(dir) !== 'lucide-react-native' && dir !== path.dirname(dir)) {
        dir = path.dirname(dir)
    }
    return dir
})()
const LUCIDE_ICONS_DIR = path.join(LUCIDE_PKG_ROOT, 'dist', 'esm', 'icons')
const LUCIDE_ICON_PREFIX = 'lucide-react-native/icons/'
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
    if (moduleName.startsWith(LUCIDE_ICON_PREFIX)) {
        const icon = moduleName.slice(LUCIDE_ICON_PREFIX.length)
        return context.resolveRequest(
            context,
            path.join(LUCIDE_ICONS_DIR, `${icon}.mjs`),
            platform
        )
    }
    return (originalResolveRequest ?? context.resolveRequest)(context, moduleName, platform)
}

module.exports = withUniwindConfig(config, {
    cssEntryFile: './global.css',
    dtsFile: './uniwind-types.d.ts',
    extraThemes: ['dark'],
})
