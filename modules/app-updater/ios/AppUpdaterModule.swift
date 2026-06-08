import ExpoModulesCore
import Foundation

public class AppUpdaterModule: Module {
    public func definition() -> ModuleDefinition {
        Name("AppUpdaterModule")

        Function("getEmbeddedId") { embeddedId() }
        Function("getRuntimeVersion") { embeddedRuntimeVersion() }
        Function("getCurrentBundleId") { Store.shared.currentId() ?? embeddedId() }

        AsyncFunction("stageBundle") { (localDir: String, id: String) in
            try Store.shared.stagePending(dir: localDir, id: id)
        }

        Function("markBundleHealthy") { Store.shared.clearBootMarker() }

        AsyncFunction("reload") {
            DispatchQueue.main.async { Store.shared.requestReload() }
        }
    }

    private func embeddedId() -> String {
        Bundle.main.object(forInfoDictionaryKey: "TinyCldBundleId") as? String ?? "embedded"
    }
    private func embeddedRuntimeVersion() -> String {
        Bundle.main.object(forInfoDictionaryKey: "TinyCldRuntimeVersion") as? String ?? ""
    }
}

final class Store {
    static let shared = Store()
    private let fm = FileManager.default

    private var root: URL {
        let base = fm.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("app-updater", isDirectory: true)
        try? fm.createDirectory(at: base, withIntermediateDirectories: true)
        return base
    }
    private var currentURL: URL { root.appendingPathComponent("current.json") }
    private var pendingURL: URL { root.appendingPathComponent("pending.json") }
    private var previousURL: URL { root.appendingPathComponent("previous.json") }
    private var bootURL: URL { root.appendingPathComponent("boot.json") }

    func currentId() -> String? { readJSON(currentURL)?["id"] as? String }
    private func currentDir() -> String? { readJSON(currentURL)?["dir"] as? String }

    func stagePending(dir: String, id: String) throws {
        writeJSON(pendingURL, ["id": id, "dir": dir])
    }

    func clearBootMarker() { try? fm.removeItem(at: bootURL) }

    /// Called from the AppDelegate's `bundleURL()` BEFORE the React bridge loads.
    /// Promotes a pending bundle, applies crash-rollback, and returns the `.hbc`
    /// to load — or `nil` to fall back to the embedded `main.jsbundle`.
    func resolveBundleURL() -> URL? {
        promotePendingIfAny()
        guard let dir = currentDir(), let id = currentId() else { return nil }
        guard let bundlePath = locateHbc(in: dir) else {
            rollbackToPrevious()
            return resolveAfterRollback()
        }
        var boot = readJSON(bootURL) ?? ["id": id, "launchCount": 0]
        if (boot["id"] as? String) != id { boot = ["id": id, "launchCount": 0] }
        let count = (boot["launchCount"] as? Int ?? 0) + 1
        if count >= 2 {
            rollbackToPrevious()
            return resolveAfterRollback()
        }
        boot["launchCount"] = count
        writeJSON(bootURL, boot)
        return URL(fileURLWithPath: bundlePath)
    }

    private func promotePendingIfAny() {
        guard let p = readJSON(pendingURL), p["id"] is String, p["dir"] is String else { return }
        if let cur = readJSON(currentURL) { writeJSON(previousURL, cur) }
        writeJSON(currentURL, p)
        try? fm.removeItem(at: pendingURL)
        try? fm.removeItem(at: bootURL)
    }

    private func rollbackToPrevious() {
        if let prev = readJSON(previousURL) { writeJSON(currentURL, prev) }
        else { try? fm.removeItem(at: currentURL) }
        try? fm.removeItem(at: previousURL)
        try? fm.removeItem(at: bootURL)
    }
    private func resolveAfterRollback() -> URL? {
        guard let dir = currentDir() else { return nil }
        return locateHbc(in: dir).map { URL(fileURLWithPath: $0) }
    }

    func requestReload() {
        AppUpdaterReload.trigger(withReason: "app-updater OTA reload")
    }

    private func locateHbc(in dir: String) -> String? {
        let base = URL(fileURLWithPath: dir).appendingPathComponent("native/ios", isDirectory: true)
        guard let en = fm.enumerator(at: base, includingPropertiesForKeys: nil) else { return nil }
        for case let f as URL in en where f.pathExtension == "hbc" { return f.path }
        return nil
    }
    private func readJSON(_ u: URL) -> [String: Any]? {
        guard let d = try? Data(contentsOf: u) else { return nil }
        return (try? JSONSerialization.jsonObject(with: d)) as? [String: Any]
    }
    private func writeJSON(_ u: URL, _ v: [String: Any]) {
        if let d = try? JSONSerialization.data(withJSONObject: v) { try? d.write(to: u, options: .atomic) }
    }
}
