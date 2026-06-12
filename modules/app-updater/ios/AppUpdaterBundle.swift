import Foundation

/// Public bridge over the internal `Store` so the (config-plugin-injected)
/// `AppDelegate.bundleURL()` can resolve a staged OTA bundle without importing
/// the module's internal `Store`. Compiled into the AppUpdater pod alongside
/// `AppUpdaterModule.swift` (the podspec globs `*.{swift,h,m}`), so the
/// AppDelegate can reference this public class by name.
///
/// Thin wrapper — no behavior of its own; delegates to `Store.shared`.
@objc public class AppUpdaterBundle: NSObject {
    /// Returns the staged bundle URL to load, or `nil` to fall back to the
    /// embedded `main.jsbundle`. Safe to call once, early, from `bundleURL()`.
    @objc public static func stagedBundleURL() -> URL? {
        Store.shared.resolveBundleURL()
    }
}
