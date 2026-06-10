const {
    withAppDelegate,
    withInfoPlist,
    withMainApplication,
    withStringsXml,
    createRunOncePlugin,
    AndroidConfig,
} = require('@expo/config-plugins')

const pkg = { name: 'with-app-updater', version: '1.0.0' }

/**
 * Wires the self-hosted OTA updater (`app-updater` local module) into the iOS
 * and Android native projects at `expo prebuild` time. Hand-editing `ios/` and
 * `android/` is futile — they are gitignored prebuild OUTPUT, regenerated on
 * every prebuild — so this seam MUST live in a config plugin.
 *
 * Three native modifications:
 *   iOS:     AppDelegate.bundleURL() consults AppUpdaterBundle.stagedBundleURL();
 *            Info.plist stamps the embedded identity.
 *   Android: the RN host's getJSBundleFile() consults
 *            AppUpdaterBundle.stagedBundlePath(); strings.xml stamps identity.
 *
 * Every mod throws if its injection marker is absent — a silent no-op would ship
 * an app that never loads OTA bundles with no signal.
 */

// --- iOS: AppDelegate.bundleURL() -------------------------------------------

// Marker: the release branch of the SDK 55 AppDelegate.swift template is
// `return Bundle.main.url(forResource: "main", withExtension: "jsbundle")`.
// We match the whole `return` statement (the embedded-load expression is the
// argument of a `return`, so we must own the `return` to emit valid Swift) but
// tolerate variable whitespace between `return` and the expression.
const IOS_BUNDLE_RE = /return\s+Bundle\.main\.url\(forResource: "main", withExtension: "jsbundle"\)/

const IOS_INJECTED_RETURN =
    'if let staged = AppUpdaterBundle.stagedBundleURL() { return staged }\n' +
    '    return Bundle.main.url(forResource: "main", withExtension: "jsbundle")'

// The `app-updater` module compiles as a static framework (AppUpdater.podspec
// sets `static_framework = true` + DEFINES_MODULE), so its
// `@objc public class AppUpdaterBundle` lives in a separate Swift module named
// `AppUpdater`. AppDelegate.swift must import it; without the import the injected
// call fails with "cannot find 'AppUpdaterBundle' in scope".
//
// The SDK 55 template uses Swift 6 access-level-qualified imports (the first
// line is `internal import Expo`). Swift rejects a bare `import AppUpdater` mixed
// with access-qualified ones as "ambiguous implicit access level", so we MUST
// emit the import with a matching access level. We anchor on the file's first
// import line (qualified or not) and reuse whatever access modifier it carries
// (default `internal`), inserting our import right after it.
const IOS_IMPORT_MODULE = 'AppUpdater'
// Captures an optional access modifier (group 1) on the first import statement.
const IOS_FIRST_IMPORT_RE =
    /^(public |internal |fileprivate |private |package )?import .*$/m

function addImportAppUpdater(src) {
    if (new RegExp(`\\bimport ${IOS_IMPORT_MODULE}\\b`).test(src)) {
        return src // already imported (idempotent re-run)
    }
    const match = IOS_FIRST_IMPORT_RE.exec(src)
    if (!match) {
        throw new Error(
            '[with-app-updater] iOS: could not find any `import` line in ' +
                'AppDelegate.swift to anchor `import AppUpdater`. The SDK 55 ' +
                'template may have changed — update plugins/with-app-updater.cjs.'
        )
    }
    // Mirror the access level of the file's first import so Swift 6 doesn't flag
    // a mixed bare/qualified import as ambiguous. `internal` is the implicit
    // default when the first import is unqualified.
    const accessLevel = (match[1] ?? 'internal ').trim()
    const importLine = `${accessLevel} import ${IOS_IMPORT_MODULE}`
    const insertAt = match.index + match[0].length
    return `${src.slice(0, insertAt)}\n${importLine}${src.slice(insertAt)}`
}

function withIosBundleSeam(config) {
    return withAppDelegate(config, cfg => {
        let src = cfg.modResults.contents
        if (src.includes('AppUpdaterBundle.stagedBundleURL()')) {
            // Call already wired; still ensure the import is present (covers an
            // ios/ tree patched by an older plugin version that lacked it).
            cfg.modResults.contents = addImportAppUpdater(src)
            return cfg
        }
        if (!IOS_BUNDLE_RE.test(src)) {
            throw new Error(
                '[with-app-updater] iOS: could not find the embedded-jsbundle return ' +
                    '`return Bundle.main.url(forResource: "main", withExtension: "jsbundle")` ' +
                    'in AppDelegate.swift. The SDK 55 template may have changed — update ' +
                    'plugins/with-app-updater.js before shipping.'
            )
        }
        // Replace the FIRST match (the release `#else` branch). The DEBUG branch
        // uses RCTBundleURLProvider, so the marker appears exactly once.
        src = src.replace(IOS_BUNDLE_RE, IOS_INJECTED_RETURN)
        cfg.modResults.contents = addImportAppUpdater(src)
        return cfg
    })
}

// --- iOS: Info.plist embedded identity --------------------------------------

function withIosIdentity(config) {
    const version = config.version || ''
    return withInfoPlist(config, cfg => {
        cfg.modResults.TinyCldBundleId = `embedded-${version}`
        cfg.modResults.TinyCldRuntimeVersion = version
        return cfg
    })
}

// --- Android: RN host getJSBundleFile() -------------------------------------

// Anchor on the host object's OPENING BRACE rather than a sibling override.
// The SDK 55 / RN 0.83 MainApplication.kt template declares the host as
// `object : DefaultReactNativeHost(this) {` — a stable, single-line structural
// point. Earlier this anchored on `getJSMainModuleName(): String =`, but the
// template renders that as an expression body whose value can sit on the NEXT
// line, so inserting after the `=` line split the expression and produced
// uncompilable Kotlin. Injecting right after the object's `{` always lands as
// the first member, valid regardless of how the other overrides are formatted.
// We use a regex so whitespace between the ctor call and `{` doesn't matter.
const ANDROID_HOST_OPEN_RE = /object\s*:\s*DefaultReactNativeHost\([^)]*\)\s*\{/

// Block-body (not expression-body) override so there's no next-line ambiguity.
const ANDROID_BUNDLE_OVERRIDE =
    '\n          override fun getJSBundleFile(): String? {\n' +
    '            return org.tinycld.appupdater.AppUpdaterBundle.stagedBundlePath(applicationContext)\n' +
    '              ?: super.getJSBundleFile()\n' +
    '          }\n'

function withAndroidBundleSeam(config) {
    return withMainApplication(config, cfg => {
        const src = cfg.modResults.contents
        if (src.includes('AppUpdaterBundle.stagedBundlePath')) {
            return cfg // already wired (idempotent re-run)
        }
        const match = ANDROID_HOST_OPEN_RE.exec(src)
        if (!match) {
            throw new Error(
                '[with-app-updater] Android: could not find the RN host opener ' +
                    '`object : DefaultReactNativeHost(...) {` in MainApplication.kt. ' +
                    'The SDK 55 template may have changed — update plugins/with-app-updater.js.'
            )
        }
        // Insert immediately after the matched `{` as the first object member.
        const insertAt = match.index + match[0].length
        cfg.modResults.contents =
            src.slice(0, insertAt) + ANDROID_BUNDLE_OVERRIDE + src.slice(insertAt)
        return cfg
    })
}

// --- Android: strings.xml embedded identity ---------------------------------

function withAndroidIdentity(config) {
    const version = config.version || ''
    return withStringsXml(config, cfg => {
        cfg.modResults = AndroidConfig.Strings.setStringItem(
            [
                AndroidConfig.Resources.buildResourceItem({
                    name: 'tinycld_bundle_id',
                    value: `embedded-${version}`,
                    translatable: false,
                }),
                AndroidConfig.Resources.buildResourceItem({
                    name: 'tinycld_runtime_version',
                    value: version,
                    translatable: false,
                }),
            ],
            cfg.modResults
        )
        return cfg
    })
}

function withAppUpdater(config) {
    config = withIosBundleSeam(config)
    config = withIosIdentity(config)
    config = withAndroidBundleSeam(config)
    config = withAndroidIdentity(config)
    return config
}

module.exports = createRunOncePlugin(withAppUpdater, pkg.name, pkg.version)
